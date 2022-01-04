package bigqueryexporter

import (
	"context"
	"time"

	"k8s.io/klog"

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

	postgres   *db.DB
	mostRecent time.Time
}

func New(ctx context.Context, postgres *db.DB) (*Client, error) {
	client := &Client{
		postgres: postgres,
	}

	// Fetch most recent release tag stored locally
	tag := models.ReleaseTag{}
	postgres.DB.
		Table("release_tags").
		Select(`release_tags."releaseTime"`).
		Order(`release_tags."releaseTime" DESC`).
		Last(&tag)
	client.mostRecent = tag.ReleaseTime
	klog.V(1).Infof("Most recent release tag was from %v", tag.ReleaseTime)

	bqClient, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	client.Client = bqClient
	return client, nil
}

func (client *Client) ExportData(ctx context.Context) error {
	if err := client.ExportReleaseTags(ctx); err != nil {
		return err
	}

	if err := client.ExportPullRequests(ctx); err != nil {
		return err
	}

	if err := client.ExportRepositories(ctx); err != nil {
		return err
	}

	if err := client.ExportJobRuns(ctx); err != nil { //nolint:if-return
		return err
	}

	return nil
}

func (client *Client) ExportReleaseTags(ctx context.Context) error {
	rows := make([]models.ReleaseTag, 0)

	// Note: BigQuery does not support autoincrementing primary keys, so we see
	// what the newest entry in our postgres db, and look for any release tags
	// newer than that.
	query := client.Query(`SELECT * FROM ` + "`ci_data.ReleaseTags`" + `WHERE releaseTime > @releaseTime`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "releaseTime",
			Value: client.mostRecent,
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

	return client.postgres.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, client.postgres.BatchSize).Error
}

func (client *Client) ExportPullRequests(ctx context.Context) error {
	rows := make([]models.PullRequest, 0)

	query := client.Query("SELECT * FROM `ci_data.ReleasePullRequests` AS pullRequests INNER JOIN `ci_data.ReleaseTags` AS releaseTags ON pullRequests.releaseTag = releaseTags.releaseTag WHERE releaseTags.releaseTime > @releaseTime")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "releaseTime",
			Value: client.mostRecent,
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
	return client.postgres.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, client.postgres.BatchSize).Error
}

func (client *Client) ExportRepositories(ctx context.Context) error {
	rows := make([]models.Repository, 0)

	query := client.Query("SELECT * FROM `ci_data.ReleaseRepositories` AS repositories INNER JOIN `ci_data.ReleaseTags` AS releaseTags ON repositories.releaseTag = releaseTags.releaseTag WHERE releaseTags.releaseTime > @releaseTime")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "releaseTime",
			Value: client.mostRecent,
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

	return client.postgres.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, client.postgres.BatchSize).Error
}

func (client *Client) ExportJobRuns(ctx context.Context) error {
	rows := make([]models.JobRun, 0)

	query := client.Query("SELECT * FROM `ci_data.ReleaseJobRuns` AS jobRuns INNER JOIN `ci_data.ReleaseTags` AS releaseTags ON jobRuns.releaseTag = releaseTags.releaseTag WHERE releaseTags.releaseTime > @releaseTime")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "releaseTime",
			Value: client.mostRecent,
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

	return client.postgres.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, client.postgres.BatchSize).Error
}
