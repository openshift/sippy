package bigqueryexporter

import (
	"context"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"google.golang.org/api/iterator"
	"gorm.io/gorm/clause"
)

var (
	projectID = "openshift-ci-data-analysis"
)

type Client struct {
	*bigquery.Client
}

func New(ctx context.Context) (*Client, error) {
	client := &Client{}

	bqClient, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	client.Client = bqClient

	return client, nil
}

func (client *Client) ExportData(ctx context.Context, dbClient *db.DB) error {
	if err := client.ExportReleaseTags(ctx, dbClient); err != nil {
		return err
	}

	if err := client.ExportPullRequests(ctx, dbClient); err != nil {
		return err
	}

	if err := client.ExportRepositories(ctx, dbClient); err != nil {
		return err
	}

	if err := client.ExportJobRuns(ctx, dbClient); err != nil { //nolint:if-return
		return err
	}

	return nil
}

func (client *Client) ExportReleaseTags(ctx context.Context, dbClient *db.DB) error {
	rows := make([]models.ReleaseTag, 0)

	// Note: BigQuery does not support autoincrementing primary keys, so we use
	// ROW_NUMBER() OVER(...) -- this produces stable ID's if we do not
	// remove older records. Currently, we use BQ as append-only and for the
	// foreseeable future will probably maintain release information
	// indefinitely (it's not a lot of data).
	query := client.Query(`SELECT * FROM (SELECT ROW_NUMBER() OVER() id, * FROM ` + "`ci_data.ReleaseTags` ORDER BY releaseTag ASC) WHERE id > @lastID")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "lastID",
			Value: dbClient.LastID("release_tags"),
		},
	}
	it, err := query.Read(ctx)
	if err != nil {
		return err
	}

	for {
		row := models.ReleaseTag{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}

	return dbClient.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, dbClient.BatchSize).Error
}

func (client *Client) ExportPullRequests(ctx context.Context, dbClient *db.DB) error {
	rows := make([]models.PullRequest, 0)

	// Note: BigQuery does not support autoincrementing primary keys, so we use
	// ROW_NUMBER() OVER(...) -- this produces stable ID's if we do not
	// remove older records. Currently, we use BQ as append-only and for the
	// foreseeable future will probably maintain release information
	// indefinitely (it's not a lot of data).
	query := client.Query(`SELECT * FROM (SELECT ROW_NUMBER() OVER() id, * FROM ` + "`ci_data.ReleasePullRequests` ORDER BY releaseTag ASC) WHERE id > @lastID")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "lastID",
			Value: dbClient.LastID("pull_requests"),
		},
	}
	it, err := query.Read(ctx)
	if err != nil {
		return err
	}

	for {
		row := models.PullRequest{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return dbClient.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, dbClient.BatchSize).Error
}

func (client *Client) ExportRepositories(ctx context.Context, dbClient *db.DB) error {
	rows := make([]models.Repository, 0)

	// Note: BigQuery does not support autoincrementing primary keys, so we use
	// ROW_NUMBER() OVER(...) -- this produces stable ID's if we do not
	// remove older records. Currently, we use BQ as append-only and for the
	// foreseeable future will probably maintain release information
	// indefinitely (it's not a lot of data).
	query := client.Query(`SELECT * FROM (SELECT ROW_NUMBER() OVER() id, * FROM ` + "`ci_data.ReleaseRepositories` ORDER BY releaseTag ASC) WHERE id > @lastID")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "lastID",
			Value: dbClient.LastID("repositories"),
		},
	}
	it, err := query.Read(ctx)
	if err != nil {
		return err
	}

	for {
		row := models.Repository{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}

	return dbClient.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, dbClient.BatchSize).Error
}

func (client *Client) ExportJobRuns(ctx context.Context, dbClient *db.DB) error {
	rows := make([]models.JobRun, 0)

	// Note: BigQuery does not support autoincrementing primary keys, so we use
	// ROW_NUMBER() OVER(...) -- this produces stable ID's if we do not
	// remove older records. Currently, we use BQ as append-only and for the
	// foreseeable future will probably maintain release information
	// indefinitely (it's not a lot of data).
	query := client.Query(`SELECT * FROM (SELECT ROW_NUMBER() OVER() id, * FROM ` + "`ci_data.ReleaseJobRuns` ORDER BY releaseTag ASC) WHERE id > @lastID")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "lastID",
			Value: dbClient.LastID("job_runs"),
		},
	}
	it, err := query.Read(ctx)
	if err != nil {
		return err
	}

	for {
		row := models.JobRun{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}

	return dbClient.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, dbClient.BatchSize).Error
}
