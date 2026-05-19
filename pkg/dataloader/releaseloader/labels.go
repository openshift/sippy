package releaseloader

import (
	"context"
	"fmt"
	"os"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
)

// GatherBulkLabelsFromBQ queries BigQuery for labels for multiple job runs in a single query.
// The releaseTime is used to constrain the scan to a single date partition.
// Returns a map of buildID → labels.
func GatherBulkLabelsFromBQ(ctx context.Context, bqClient *bqcachedclient.Client, buildIDs []string, releaseTime time.Time) (map[string]pq.StringArray, error) {
	if bqClient == nil || len(buildIDs) == 0 {
		return nil, nil
	}

	dataset := os.Getenv(prowloader.LabelsDatasetEnv)
	if dataset == "" {
		dataset = bqClient.Dataset
	}
	table := fmt.Sprintf("`%s.%s`", dataset, prowloader.LabelsTableName)
	q := bqClient.Query(ctx, bqlabel.ProwLoaderJobLabels, `
		SELECT prowjob_build_id, ARRAY_AGG(DISTINCT label ORDER BY label ASC) AS labels
		FROM `+table+`
		WHERE prowjob_build_id IN UNNEST(@BuildIDs)
		  AND DATE(prowjob_start) >= DATE(@ReleaseTime)
		GROUP BY prowjob_build_id
	`)
	q.Parameters = []bigquery.QueryParameter{
		{
			Name:  "BuildIDs",
			Value: buildIDs,
		},
		{
			Name:  "ReleaseTime",
			Value: releaseTime,
		},
	}

	type row struct {
		BuildID string   `bigquery:"prowjob_build_id"`
		Labels  []string `bigquery:"labels"`
	}

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Warning("error querying bulk labels from bigquery")
		return nil, err
	}

	result := make(map[string]pq.StringArray, len(buildIDs))
	for {
		var r row
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Warning("error parsing bulk labels from bigquery")
			return result, err
		}
		result[r.BuildID] = r.Labels
	}

	return result, nil
}
