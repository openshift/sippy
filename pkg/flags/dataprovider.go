package flags

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	bqprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/bigquery"
	mixedprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/mixed"
	pgprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
)

// NewDataProvider maps a data provider name and the available clients to a concrete
// component readiness DataProvider implementation.
//
// It lives in the flags package rather than the dataprovider package to avoid an import
// cycle: the bigquery, postgres, and mixed sub-packages already import the dataprovider
// package for the interface, so the parent package cannot import them back.
func NewDataProvider(name string, bigQueryClient *bqcachedclient.Client, dbc *db.DB, cacheClient cache.Cache) (dataprovider.DataProvider, error) {
	switch name {
	case "default":
		if bigQueryClient != nil && dbc != nil {
			return mixedprovider.NewMixedProvider(bigQueryClient, dbc, cacheClient), nil
		} else if bigQueryClient != nil {
			return bqprovider.NewBigQueryProvider(bigQueryClient), nil
		} else if dbc != nil {
			return pgprovider.NewPostgresProvider(dbc, cacheClient), nil
		}
		return nil, fmt.Errorf("default data provider requires at least one of BigQuery or PostgreSQL to be configured")

	case "bigquery":
		if bigQueryClient != nil {
			return bqprovider.NewBigQueryProvider(bigQueryClient), nil
		}
		return nil, fmt.Errorf("bigquery data provider requires google-service-account-credential-file to be configured")

	case "postgres":
		if dbc == nil {
			return nil, fmt.Errorf("postgres data provider requires a database connection")
		}
		log.Info("Using Postgres data provider for component readiness")
		return pgprovider.NewPostgresProvider(dbc, cacheClient), nil

	default:
		return nil, fmt.Errorf("unknown --data-provider %q, must be default, bigquery, or postgres", name)
	}
}
