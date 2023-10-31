package bigquery

import (
	"cloud.google.com/go/bigquery"

	"github.com/openshift/sippy/pkg/apis/cache"
)

type Client struct {
	BQ    *bigquery.Client
	Cache cache.Cache
}

func New(client *bigquery.Client, cache cache.Cache) *Client {
	return &Client{
		BQ:    client,
		Cache: cache,
	}
}
