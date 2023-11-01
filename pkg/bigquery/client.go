package bigquery

import (
	"cloud.google.com/go/bigquery"

	"github.com/openshift/sippy/pkg/apis/cache"
)

type Client struct {
	BQ    *bigquery.Client
	Cache cache.Cache
}

func New(bqc *bigquery.Client, c cache.Cache) *Client {
	return &Client{
		BQ:    bqc,
		Cache: c,
	}
}
