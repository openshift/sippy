package bigquery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"

	v1 "github.com/openshift-eng/ci-test-mapping/pkg/api/types/v1"
)

const mappingTableName = "component_mapping"

type MappingTableManager struct {
	ctx    context.Context
	client *Client
}

func NewMappingTableManager(ctx context.Context, client *Client) *MappingTableManager {
	return &MappingTableManager{
		ctx:    ctx,
		client: client,
	}
}

func (tm *MappingTableManager) Migrate() error {
	dataset := tm.client.bigquery.Dataset(tm.client.datasetName)
	table := dataset.Table(mappingTableName)

	md, err := table.Metadata(tm.ctx)
	// Create table if it doesn't exist
	if gbErr, ok := err.(*googleapi.Error); err != nil && ok && gbErr.Code == 404 {
		log.Infof("table doesn't existing, creating table %q", mappingTableName)
		if err := table.Create(tm.ctx, &bigquery.TableMetadata{
			Schema: v1.MappingTableSchema,
		}); err != nil {
			return err
		}
		log.Infof("table created %q", mappingTableName)
	} else if err != nil {
		return err
	} else {
		if !schemasEqual(md.Schema, v1.MappingTableSchema) {
			if _, err := table.Update(tm.ctx, bigquery.TableMetadataToUpdate{Schema: v1.MappingTableSchema}, md.ETag); err != nil {
				log.WithError(err).Errorf("failed to update table schema for %q", mappingTableName)
				return err
			}
			log.Infof("table schema updated %q", mappingTableName)
		} else {
			log.Infof("table schema is up-to-date %q", mappingTableName)
		}
	}

	return nil
}

func (tm *MappingTableManager) ListMappings() ([]v1.TestOwnership, error) {
	now := time.Now()
	log.Infof("fetching mappings from bigquery")
	table := tm.client.bigquery.Dataset(tm.client.datasetName).Table(mappingTableName + "_latest") // use the view

	sql := fmt.Sprintf(`
		SELECT 
		    *
		FROM
			%s.%s.%s`,
		table.ProjectID, tm.client.datasetName, table.TableID)
	log.Debugf("query is %q", sql)

	q := tm.client.bigquery.Query(sql)
	it, err := q.Read(tm.ctx)
	if err != nil {
		return nil, err
	}

	var results []v1.TestOwnership
	for {
		var testOwnership v1.TestOwnership
		err := it.Next(&testOwnership)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		results = append(results, testOwnership)
	}
	log.Infof("fetched %d mapping from bigquery in %v", len(results), time.Since(now))

	return results, nil
}

func (tm *MappingTableManager) PushMappings(mappings []v1.TestOwnership) error {
	var batchSize = 500

	table := tm.client.bigquery.Dataset(tm.client.datasetName).Table(mappingTableName)
	inserter := table.Inserter()
	for i := 0; i < len(mappings); i += batchSize {
		end := i + batchSize
		if end > len(mappings) {
			end = len(mappings)
		}

		if err := inserter.Put(tm.ctx, mappings[i:end]); err != nil {
			return err
		}
		log.Infof("added %d rows to mapping bigquery table", end-i)
	}

	return nil
}

func (tm *MappingTableManager) PruneMappings() error {
	now := time.Now()
	log.Infof("pruning mappings from bigquery")
	table := tm.client.bigquery.Dataset(tm.client.datasetName).Table(mappingTableName)

	tableLocator := fmt.Sprintf("%s.%s.%s", table.ProjectID, tm.client.datasetName, table.TableID)

	sql := fmt.Sprintf(`DELETE FROM %s WHERE created_at < (SELECT MAX(created_at) FROM %s)`, tableLocator, tableLocator)
	log.Infof("query is %q", sql)

	q := tm.client.bigquery.Query(sql)
	_, err := q.Read(tm.ctx)
	log.Infof("pruned mapping table in %+v", time.Since(now))
	if err != nil && strings.Contains(err.Error(), "streaming") {
		log.Warningf("got error while trying to prune the table; please wait 90 minutes and try again. You cannot prune after modifying the table.")
	}
	return err
}

func (tm *MappingTableManager) Table() *bigquery.Table {
	dataset := tm.client.bigquery.Dataset(tm.client.datasetName)
	return dataset.Table(mappingTableName)
}

func schemasEqual(a, b bigquery.Schema) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Type != b[i].Type ||
			a[i].Repeated != b[i].Repeated ||
			a[i].Required != b[i].Required {
			return false
		}
	}

	return true
}
