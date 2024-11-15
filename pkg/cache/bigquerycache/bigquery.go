package bigquerycache

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	uuid2 "github.com/google/uuid"
	"github.com/openshift/sippy/pkg/apis/cache"
	sippybq "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/cache/compressed"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

// https://cloud.google.com/bigquery/quotas#streaming_inserts
// Maximum row size 	10 MB 	Exceeding this value causes invalid errors.
// HTTP request size limit 	10 MB Exceeding this value causes invalid errors.
const (
	cachedTable     = "cached_data"
	partitionColumn = "modified_time"
	chunkSize       = 7000000 // ~7MB to stay under the max row limit
)

type Cache struct {
	client     *sippybq.Client
	readOnly   bool
	expiration time.Duration
}

func NewBigQueryCache(client *sippybq.Client, expiration time.Duration, readOnly bool) (cache.Cache, error) {
	c := &Cache{
		client:     client,
		readOnly:   readOnly,
		expiration: expiration,
	}

	return &compressed.Cache{Cache: c}, nil
}

type CacheRecord struct {
	Key        string    `bigquery:"key"`
	UUID       string    `bigquery:"uuid"`
	Modified   time.Time `bigquery:"modified_time"`
	Expiration time.Time `bigquery:"expiration"`
	Data       []byte    `bigquery:"data"`
	ChunkIndex int       `bigquery:"chunk_index"`
}

func (c Cache) Get(ctx context.Context, key string) ([]byte, error) {

	// if we have it in our warm cache use it
	if c.client.Cache != nil {
		data, err := c.client.Cache.Get(ctx, key)
		if err != nil {
			logrus.Debugf("Failure retrieving %s from warm cache %v", key, err)
		} else if data != nil {
			return data, nil
		}
	}

	before := time.Now()
	defer func(key string, before time.Time) {
		logrus.Infof("BigQuery Cache Get completed in %s for %s", time.Since(before), key)
	}(key, before)

	// get the most recent modified time for this key along with uuid and checksum
	// limit the columns so we don't query too much data
	cacheQuery := fmt.Sprintf("SELECT modified_time, expiration, uuid FROM `%s.%s` WHERE `%s` > TIMESTAMP(\"%s\") AND `%s` > TIMESTAMP(\"%s\") AND %s = '%s' order by %s desc limit 1", c.client.Dataset, cachedTable, partitionColumn, time.Now().Add(-1*c.expiration).Format(time.RFC3339), "expiration", time.Now().Format(time.RFC3339), "key", key, partitionColumn)

	query := c.client.BQ.Query(cacheQuery)

	metadataRecord := CacheRecord{}

	it, err := query.Read(ctx)
	if err != nil {
		return nil, err
	}

	err = it.Next(&metadataRecord)
	if err != nil {
		return nil, err
	}

	// that gives us the modified time
	// we have to add a +/- 5 second grace as exact match doesn't work
	// now get the data
	cacheQuery = fmt.Sprintf("SELECT * FROM `%s.%s` WHERE `%s` > TIMESTAMP(\"%s\") AND `%s` < TIMESTAMP(\"%s\")  AND %s = '%s' AND %s = '%s' order by %s asc", c.client.Dataset, cachedTable, partitionColumn, metadataRecord.Modified.Add(-5*time.Second).Format(time.RFC3339), partitionColumn, metadataRecord.Modified.Add(5*time.Second).Format(time.RFC3339), "key", key, "uuid", metadataRecord.UUID, "chunk_index")

	query = c.client.BQ.Query(cacheQuery)

	record := CacheRecord{}
	var records [][]byte

	it, err = query.Read(ctx)
	if err != nil {
		return nil, err
	}

	for {
		err = it.Next(&record)
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}
		records = append(records, record.Data)
	}

	// we should have the records, now assemble them into a single array
	data := unchunk(records)

	// if we have a warm cache, and we had a cache miss we should update it now
	// we don't have the exact duration so we diff the expiration value and now
	if data != nil && c.client.Cache != nil {
		err = c.client.Cache.Set(ctx, key, data, time.Until(metadataRecord.Expiration))
		if err != nil {
			logrus.WithError(err).Warn("Error updating warm cache during get")
		}
	}

	return data, nil
}

func (c Cache) Set(ctx context.Context, key string, content []byte, duration time.Duration) error {

	// save to the warm cache too if enabled
	// but follow through and persist as well
	if c.client.Cache != nil {
		err := c.client.Cache.Set(ctx, key, content, duration)
		if err != nil {
			logrus.WithError(err).Errorf("Failure setting %s for warm cache", key)
		}
	}

	// read only applies to bigquery only (I think)
	if c.readOnly {
		// valuable to see how often this gets called
		// hope is that it wouldn't get called at all
		// if our process for updating the cache is running properly
		logrus.Debugf("Set called in readonly mode for: %s", key)
		return nil
	}
	before := time.Now()
	defer func(key string, before time.Time) {
		logrus.Infof("BigQuery Cache Set completed in %s for %s", time.Since(before), key)
	}(key, before)

	t := c.client.BQ.Dataset(c.client.Dataset).Table(cachedTable)

	if t == nil {
		return fmt.Errorf("failed to retrieve table '%s' from dataset '%s'", cachedTable, c.client.Dataset)
	}

	i := t.Inserter()

	if i == nil {
		return fmt.Errorf("failed to retrieve insterter for table '%s' from dataset '%s'", cachedTable, c.client.Dataset)
	}

	// chunk it
	chunks := chunk(content, chunkSize)
	modifiedTime := time.Now()
	expiration := modifiedTime.Add(duration)
	uuid := uuid2.New().String()

	// then we upload chunks 1 by 1
	for index, chunk := range chunks {
		record := CacheRecord{Key: key, Modified: modifiedTime, Data: chunk, Expiration: expiration, ChunkIndex: index, UUID: uuid}
		err := i.Put(ctx, bigquery.ValueSaver(&record))
		if err != nil {
			return err
		}
	}
	return nil
}

// Save implements the ValueSaver interface.
// Can just use the struct as well
func (c *CacheRecord) Save() (row map[string]bigquery.Value, insertID string, err error) {

	row = make(map[string]bigquery.Value, 6)

	row["key"] = c.Key
	row["modified_time"] = c.Modified
	row["data"] = c.Data
	row["expiration"] = c.Expiration
	row["chunk_index"] = c.ChunkIndex
	row["uuid"] = c.UUID

	return row, fmt.Sprintf("%s-%d", c.UUID, c.ChunkIndex), nil
}

func chunk(value []byte, maxChunk int) [][]byte {
	var ret [][]byte
	max := len(value)

	for i := 0; i < max; i += maxChunk {
		end := i + maxChunk

		if end > max {
			end = max
		}

		ret = append(ret, value[i:end])
	}

	return ret
}

func unchunk(chunked [][]byte) []byte {
	var ret []byte

	for _, chunk := range chunked {
		ret = append(ret, chunk...)
	}

	return ret
}
