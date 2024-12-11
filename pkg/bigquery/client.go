package bigquery

import (
	"context"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"github.com/openshift/sippy/pkg/apis/cache"
)

type Client struct {
	BQ      *bigquery.Client
	Cache   cache.Cache
	Dataset string
}

func New(ctx context.Context, credentialFile, project, dataset string, c cache.Cache) (*Client, error) {
	bqc, err := bigquery.NewClient(ctx, project, option.WithCredentialsFile(credentialFile))
	if err != nil {
		return nil, err
	}
	// Enable Storage API usage for fetching data
	err = bqc.EnableStorageReadClient(context.Background(), option.WithCredentialsFile(credentialFile))
	if err != nil {
		return nil, errors.WithMessage(err, "couldn't enable storage API")
	}

	return &Client{
		BQ:      bqc,
		Cache:   c,
		Dataset: dataset,
	}, nil
}

// LoggedRead is a wrapper around the bigquery Read method that logs the query being executed
func LoggedRead(ctx context.Context, q *bigquery.Query) (*bigquery.RowIterator, error) {
	log.Debugf("Querying BQ with Parameters: %v\n%v", q.Parameters, q.QueryConfig.Q)
	return q.Read(ctx)
}
