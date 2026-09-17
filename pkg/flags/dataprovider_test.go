package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	bqprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/bigquery"
	mixedprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/mixed"
	pgprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
)

// TestNewDataProvider verifies that NewDataProvider maps a provider name plus the
// available clients to the correct concrete DataProvider implementation, and returns
// an error when the required clients are missing or the name is unknown.
func TestNewDataProvider(t *testing.T) {
	// The factory and the underlying provider constructors only check the clients for
	// nil vs. non-nil and store the pointers; they never dereference them. Empty structs
	// are therefore sufficient to exercise the selection logic with no real BigQuery or
	// Postgres connection. The cache is passed through untouched, so nil is fine.
	bqClient := &bqcachedclient.Client{}
	dbClient := &db.DB{}

	tests := []struct {
		name         string
		providerName string
		bqClient     *bqcachedclient.Client
		dbc          *db.DB
		wantErr      bool
		assertType   func(t *testing.T, dp dataprovider.DataProvider)
	}{
		{
			name:         "default with both clients returns mixed provider",
			providerName: "default",
			bqClient:     bqClient,
			dbc:          dbClient,
			assertType: func(t *testing.T, dp dataprovider.DataProvider) {
				_, ok := dp.(*mixedprovider.MixedProvider)
				assert.True(t, ok, "expected *mixed.MixedProvider, got %T", dp)
			},
		},
		{
			name:         "default with only bigquery client returns bigquery provider",
			providerName: "default",
			bqClient:     bqClient,
			dbc:          nil,
			assertType: func(t *testing.T, dp dataprovider.DataProvider) {
				_, ok := dp.(*bqprovider.BigQueryProvider)
				assert.True(t, ok, "expected *bigquery.BigQueryProvider, got %T", dp)
			},
		},
		{
			name:         "default with only postgres client returns postgres provider",
			providerName: "default",
			bqClient:     nil,
			dbc:          dbClient,
			assertType: func(t *testing.T, dp dataprovider.DataProvider) {
				_, ok := dp.(*pgprovider.PostgresProvider)
				assert.True(t, ok, "expected *postgres.PostgresProvider, got %T", dp)
			},
		},
		{
			name:         "default with neither client returns error",
			providerName: "default",
			bqClient:     nil,
			dbc:          nil,
			wantErr:      true,
		},
		{
			name:         "bigquery with bigquery client returns bigquery provider",
			providerName: "bigquery",
			bqClient:     bqClient,
			dbc:          nil,
			assertType: func(t *testing.T, dp dataprovider.DataProvider) {
				_, ok := dp.(*bqprovider.BigQueryProvider)
				assert.True(t, ok, "expected *bigquery.BigQueryProvider, got %T", dp)
			},
		},
		{
			name:         "bigquery without bigquery client returns error",
			providerName: "bigquery",
			bqClient:     nil,
			dbc:          dbClient,
			wantErr:      true,
		},
		{
			name:         "postgres with postgres client returns postgres provider",
			providerName: "postgres",
			bqClient:     nil,
			dbc:          dbClient,
			assertType: func(t *testing.T, dp dataprovider.DataProvider) {
				_, ok := dp.(*pgprovider.PostgresProvider)
				assert.True(t, ok, "expected *postgres.PostgresProvider, got %T", dp)
			},
		},
		{
			name:         "postgres without postgres client returns error",
			providerName: "postgres",
			bqClient:     bqClient,
			dbc:          nil,
			wantErr:      true,
		},
		{
			name:         "unknown provider name returns error",
			providerName: "nonsense",
			bqClient:     bqClient,
			dbc:          dbClient,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp, err := NewDataProvider(tt.providerName, tt.bqClient, tt.dbc, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, dp)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, dp)
			tt.assertType(t, dp)
		})
	}
}
