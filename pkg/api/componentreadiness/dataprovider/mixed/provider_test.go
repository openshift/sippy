package mixed

import (
	"context"
	"testing"
	"time"

	bqprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/bigquery"
	pgprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/stretchr/testify/require"
)

type testCache struct{ name string }

func (c *testCache) Get(context.Context, string, time.Duration) ([]byte, error) {
	return []byte(c.name), nil
}
func (c *testCache) Set(context.Context, string, []byte, time.Duration) error { return nil }

func TestProviderForDefaultsToPostgres(t *testing.T) {
	provider := NewMixedProvider(&bqcachedclient.Client{}, &db.DB{}, nil)

	tests := []struct {
		name       string
		dataSource string
		wantType   any
	}{
		{name: "default", wantType: (*pgprovider.PostgresProvider)(nil)},
		{name: "postgres", dataSource: reqopts.DataSourcePostgres, wantType: (*pgprovider.PostgresProvider)(nil)},
		{name: "bigquery", dataSource: reqopts.DataSourceBigQuery, wantType: (*bqprovider.BigQueryProvider)(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerForRequest := provider.providerFor(reqopts.RequestOptions{DataSource: tt.dataSource})
			require.IsType(t, tt.wantType, providerForRequest)
		})
	}
}

func TestCacheUsesPostgresProviderCache(t *testing.T) {
	bqCache := &testCache{name: "bigquery"}
	pgCache := &testCache{name: "postgres"}
	provider := NewMixedProvider(&bqcachedclient.Client{Cache: bqCache}, &db.DB{}, pgCache)

	require.Same(t, pgCache, provider.Cache())
}
