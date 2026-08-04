package api

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	bq "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
)

func GetBackendDisruptionByRun(ctx context.Context, bigQueryClient *bq.Client, jobRunNames []string, backendName string) (apitype.BackendDisruptionRunsResult, error) {
	filterStr := ""
	if backendName != "" {
		filterStr = `
  AND BackendName LIKE CONCAT('%', @BackendName, '%')`
	}

	queryStr := `SELECT
    BackendName,
    DisruptionSeconds,
    JobName,
    JobRunName,
    CAST(JobRunStartTime AS STRING) AS JobRunStartTime,
    CAST(JobRunEndTime AS STRING) AS JobRunEndTime,
    Cluster,
    ReleaseTag,
    MasterNodesUpdated,
    JobRunStatus
FROM ` + "`openshift-ci-data-analysis.ci_data.BackendDisruption`" + `
WHERE JobRunName IN UNNEST(@JobRunNames)` + filterStr + `
ORDER BY JobRunName, DisruptionSeconds DESC`

	q := bigQueryClient.Query(ctx, bqlabel.BackendDisruptionByRun, queryStr)
	q.Parameters = []bigquery.QueryParameter{
		{
			Name:  "JobRunNames",
			Value: jobRunNames,
		},
	}

	if backendName != "" {
		q.Parameters = append(q.Parameters, bigquery.QueryParameter{
			Name:  "BackendName",
			Value: backendName,
		})
	}

	bq.LogQueryWithParamsReplaced(log.WithField("type", "BackendDisruptionByRun"), q)

	it, err := bq.LoggedRead(ctx, q)
	if err != nil {
		log.WithError(err).Error("error querying backend disruption from bigquery")
		return apitype.BackendDisruptionRunsResult{}, fmt.Errorf("error querying backend disruption from bigquery: %w", err)
	}

	type bqRow struct {
		BackendName        string               `bigquery:"BackendName"`
		DisruptionSeconds  int                   `bigquery:"DisruptionSeconds"`
		JobName            bigquery.NullString   `bigquery:"JobName"`
		JobRunName         string                `bigquery:"JobRunName"`
		JobRunStartTime    bigquery.NullString   `bigquery:"JobRunStartTime"`
		JobRunEndTime      bigquery.NullString   `bigquery:"JobRunEndTime"`
		Cluster            bigquery.NullString   `bigquery:"Cluster"`
		ReleaseTag         bigquery.NullString   `bigquery:"ReleaseTag"`
		MasterNodesUpdated bigquery.NullString   `bigquery:"MasterNodesUpdated"`
		JobRunStatus       bigquery.NullString   `bigquery:"JobRunStatus"`
	}

	var rows []apitype.BackendDisruptionRunRow
	for {
		var row bqRow
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error reading backend disruption row from bigquery")
			continue
		}

		apiRow := apitype.BackendDisruptionRunRow{
			BackendName:       row.BackendName,
			DisruptionSeconds: row.DisruptionSeconds,
			JobRunName:        row.JobRunName,
		}
		if row.JobName.Valid {
			apiRow.JobName = row.JobName.StringVal
		}
		if row.JobRunStartTime.Valid {
			apiRow.JobRunStartTime = row.JobRunStartTime.StringVal
		}
		if row.JobRunEndTime.Valid {
			apiRow.JobRunEndTime = row.JobRunEndTime.StringVal
		}
		if row.Cluster.Valid {
			apiRow.Cluster = row.Cluster.StringVal
		}
		if row.ReleaseTag.Valid {
			apiRow.ReleaseTag = row.ReleaseTag.StringVal
		}
		if row.MasterNodesUpdated.Valid {
			apiRow.MasterNodesUpdated = row.MasterNodesUpdated.StringVal
		}
		if row.JobRunStatus.Valid {
			apiRow.JobRunStatus = row.JobRunStatus.StringVal
		}
		rows = append(rows, apiRow)
	}

	return apitype.BackendDisruptionRunsResult{Rows: rows}, nil
}
