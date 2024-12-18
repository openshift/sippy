package bigquerycache

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

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
	cachedTable                = "cached_data"
	partitionColumn            = "modified_time"
	chunkSize                  = 7000000 // ~7MB to stay under the max row limit
	persistentCacheWarmMiss    = "sippy_persistent_warm_cache_miss"
	persistentCacheBackendMiss = "sippy_persistent_backend_cache_miss"
	persistentCacheGet         = "sippy_persistent_cache_get"
	persistentCacheBackendGet  = "sippy_persistent_backend_cache_get"
	persistentCacheReadOnlySet = "sippy_persistent_cache_read_only_set"
	persistentCacheBackendSet  = "sippy_persistent_cache_backend_set"
)

var (
	persistentCacheWarmMissMetric = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: persistentCacheWarmMiss,
		Help: "Number of persistent cache gets that were not in the warm cache.",
	}, []string{})
	persistentCacheBackendMissMetric = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: persistentCacheBackendMiss,
		Help: "Number of persistent cache gets that were not in the backend cache.",
	}, []string{})
	persistentCacheGetMetric = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: persistentCacheGet,
		Help: "Number of persistent cache get requests.",
	}, []string{})
	persistentCacheBackendGetMetric = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: persistentCacheBackendGet,
		Help: "Number of persistent cache get that query the backend cache.",
	}, []string{})
	persistentCacheReadOnlySetMetric = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: persistentCacheReadOnlySet,
		Help: "Number of persistent cache set calls for a read only client.",
	}, []string{})
	persistentCacheBackendSetMetric = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: persistentCacheBackendSet,
		Help: "Number of persistent cache backend set calls.",
	}, []string{})
)

// Cache implementation that supports storing data to a bigquery table
// Introduced to handle large data structures that are costly in time and $$
// as well as considered largely static, allowing for longer storage between queries
//
// Additionally the concept of the cache being read only supports separate processes
// writing cached data and reading.  In a case where one process will write the cache data
// it should have a shorter maxExpiration duration in the Cache initialization via input parameter.
// This duration is separate from the Duration passed in the Set call
// and can be used to cause new entries to be written to the backend table before they are considered expired.
// If a cache item has a 7 day duration passed in on the Set call
// and the read only cache process is configured with a maximum 10 day duration for all cache items
// the process that writes to the cache could be configured with a 1 (or more) day maximum duration that would cause
// it to ignore any entries older than 24 hours and perform a new query and update the cache once a day.
// Providing regular cache updates to the read only process and allowing for outages as well.
// To ensure that the persisted cache is being updated properly in this scenario forcePersistentLookup = true
// should be set to skip any warm cache lookup, which doesn't check the maxExpiration value, and validate the
// contents in the persistent backend or update the cache values, including any warm cache.
//
// minExpiration is the minimum duration that will be written to the bigquery cache table
// items that are cached with a Set duration shorter than minExpiration will skip the biqquery
// caching and rely on the main caching layer.  The minExpiration is also used to determine if a Get
// that misses the main caching layer should check the bigquery cache table.  If the duration is below the min
// it will not check for an entry but instead return a miss.
type Cache struct {
	client                *sippybq.Client
	readOnly              bool
	maxExpiration         time.Duration
	minExpiration         time.Duration
	forcePersistentLookup bool
}

func NewBigQueryCache(client *sippybq.Client, maxExpiration, minExpiration time.Duration, readOnly, forcePersistentLookup bool) (cache.Cache, error) {
	c := &Cache{
		client:                client,
		readOnly:              readOnly,
		maxExpiration:         maxExpiration,
		minExpiration:         minExpiration,
		forcePersistentLookup: forcePersistentLookup,
	}
	return compressed.NewCompressedCache(c)
}

type CacheRecord struct {
	Key        string    `bigquery:"key"`
	UUID       string    `bigquery:"uuid"`
	Modified   time.Time `bigquery:"modified_time"`
	Expiration time.Time `bigquery:"expiration"`
	Data       []byte    `bigquery:"data"`
	ChunkIndex int       `bigquery:"chunk_index"`
}

func (c Cache) Get(ctx context.Context, key string, duration time.Duration) ([]byte, error) {

	persistentCacheGetMetric.WithLabelValues().Inc()

	// if we have it in our warm cache use it
	if c.client.Cache != nil && (duration < c.minExpiration || !c.forcePersistentLookup) {
		data, err := c.client.Cache.Get(ctx, key, duration)
		if err != nil {
			logrus.Debugf("Failure retrieving %s from warm cache %v", key, err)
		} else if data != nil {
			return data, nil
		}

		// we have a cache but didn't return so inc the miss
		persistentCacheWarmMissMetric.WithLabelValues().Inc()
	}

	// don't look in big query unless it meets our minExpiration threshold
	if duration < c.minExpiration {
		return nil, nil
	}

	persistentCacheBackendGetMetric.WithLabelValues().Inc()

	before := time.Now()
	defer func(key string, before time.Time) {
		logrus.Infof("BigQuery Cache Get completed in %s for %s", time.Since(before), key)
	}(key, before)

	metadataRecord, err := c.findCacheEntry(ctx, key)
	if err != nil {
		return nil, err
	}
	data, err := c.getFullCacheRecords(ctx, key, metadataRecord)
	if err != nil {
		return nil, err
	}

	// if we have a warm cache, and we had a cache miss we should update it now
	// we don't have the exact duration so we diff the expiration value and now
	if data != nil {
		if c.client.Cache != nil {
			err = c.client.Cache.Set(ctx, key, data, time.Until(metadataRecord.Expiration))
			if err != nil {
				logrus.WithError(err).Warn("Error updating warm cache during get")
			}
		}
	} else {
		persistentCacheBackendMissMetric.WithLabelValues().Inc()
	}

	return data, nil
}

func (c Cache) findCacheEntry(ctx context.Context, key string) (CacheRecord, error) {
	// get the most recent modified time for this key along with uuid and checksum
	// limit the columns so we don't query too much data
	query := c.client.BQ.Query(fmt.Sprintf(
		"SELECT modified_time, expiration, uuid FROM `%s.%s` "+
			`WHERE %s > TIMESTAMP(@expByNowTime)
		  AND expiration > TIMESTAMP(@expTime)
		  AND key = @keyParam
		ORDER BY %s DESC LIMIT 1`,
		c.client.Dataset, cachedTable,
		partitionColumn, partitionColumn))
	query.Parameters = []bigquery.QueryParameter{
		{ // limit partitions to those that could contain un-expired entries
			Name:  "expByNowTime",
			Value: time.Now().Add(-1 * c.maxExpiration).Format(time.RFC3339),
		},
		{ // entry itself is not already expired
			Name:  "expTime",
			Value: time.Now().Format(time.RFC3339),
		},
		{
			Name:  "keyParam",
			Value: key,
		},
	}

	metadataRecord := CacheRecord{}

	it, err := sippybq.LoggedRead(ctx, query)
	if err != nil {
		return metadataRecord, err
	}

	err = it.Next(&metadataRecord)
	return metadataRecord, err
}

func (c Cache) getFullCacheRecords(ctx context.Context, key string, metadataRecord CacheRecord) ([]byte, error) {
	// metadataRecord gave us the modified time; now get the data
	// we have to add a +/- 5 second grace as exact match doesn't work
	query := c.client.BQ.Query(fmt.Sprintf(
		"SELECT * FROM `%s.%s` "+
			`WHERE %s BETWEEN TIMESTAMP(@tsLower) AND TIMESTAMP(@tsUpper)
		       AND key = @keyParam
		       AND uuid = @uuidParam
		     ORDER BY chunk_index ASC`,
		c.client.Dataset, cachedTable, partitionColumn,
	))
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "tsLower",
			Value: metadataRecord.Modified.Add(-5 * time.Second).Format(time.RFC3339),
		},
		{
			Name:  "tsUpper",
			Value: metadataRecord.Modified.Add(5 * time.Second).Format(time.RFC3339),
		},
		{
			Name:  "keyParam",
			Value: key,
		},
		{
			Name:  "uuidParam",
			Value: metadataRecord.UUID,
		},
	}

	it, err := query.Read(ctx)
	if err != nil {
		return nil, err
	}

	record := CacheRecord{}
	var records [][]byte
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

	// don't save to big query unless it meets our minExpiration threshold
	if duration < c.minExpiration {
		return nil
	}

	// read only applies to bigquery only (I think)
	if c.readOnly {
		// valuable to see how often this gets called
		// hope is that it wouldn't get called at all
		// if our process for updating the cache is running properly
		persistentCacheReadOnlySetMetric.WithLabelValues().Inc()
		logrus.Warnf("Set called in readonly mode for: %s", key)
		return nil
	}

	persistentCacheBackendSetMetric.WithLabelValues().Inc()
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
