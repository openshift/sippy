package bigqueryanalysis

import (
	"context"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	v1 "github.com/openshift/sippy/pkg/apis/bigquery/v1"
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

func (client *Client) GetJobs(ctx context.Context, releases []string) ([]v1.Job, error) {
	jobs := make([]v1.Job, 0)
	query := client.Query(`
		SELECT
			*
		FROM ` + "`ci_data.Jobs`" + `
		WHERE
			Release IN UNNEST(@releases)
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "releases",
			Value: releases,
		},
	}

	it, err := query.Read(ctx)
	if err != nil {
		return jobs, err
	}

	for {
		job := v1.Job{}
		err := it.Next(&job)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return jobs, err
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}
