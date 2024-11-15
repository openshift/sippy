package bigquery

import (
	"context"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
	"google.golang.org/api/option"

	"github.com/openshift/sippy/pkg/apis/cache"
)

type Client struct {
	BQ              *bigquery.Client
	Cache           cache.Cache
	PersistentCache cache.Cache
	Dataset         string
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
