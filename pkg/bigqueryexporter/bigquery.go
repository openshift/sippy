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
	query := client.Query(`
		SELECT
			ROW_NUMBER() OVER() id, *
		FROM ` + "`ci_data.ReleaseTags`")
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

	return dbClient.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rows).Error
}

func (client *Client) ExportPullRequests(ctx context.Context, dbClient *db.DB) error {
	rows := make([]models.PullRequest, 0)
	query := client.Query(`
		SELECT
			ROW_NUMBER() OVER() id, *
		FROM ` + "`ci_data.ReleasePullRequests`")
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
	return dbClient.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rows).Error
}

func (client *Client) ExportRepositories(ctx context.Context, dbClient *db.DB) error {
	rows := make([]models.Repository, 0)
	query := client.Query(`
		SELECT
			ROW_NUMBER() OVER() id, *
		FROM ` + "`ci_data.ReleaseRepositories`")
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

	return dbClient.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rows).Error
}

func (client *Client) ExportJobRuns(ctx context.Context, dbClient *db.DB) error {
	rows := make([]models.JobRun, 0)
	query := client.Query(`
		SELECT
			ROW_NUMBER() OVER() id, *
		FROM ` + "`ci_data.ReleaseJobRuns`")
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

	return dbClient.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rows).Error
}
