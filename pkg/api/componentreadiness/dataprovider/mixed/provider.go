package mixed

import (
	"context"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/bigquery"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
)

var _ dataprovider.DataProvider = &MixedProvider{}

// MixedProvider wraps both a BigQuery and PostgreSQL provider, routing
// release metadata queries to PostgreSQL and everything else to BigQuery.
// When reqOptions.DataSource is "postgres", test status queries route to PG instead.
type MixedProvider struct {
	bq *bigquery.BigQueryProvider
	pg *postgres.PostgresProvider
}

func NewMixedProvider(bqClient *bqcachedclient.Client, dbc *db.DB, cacheClient cache.Cache) *MixedProvider {
	return &MixedProvider{
		bq: bigquery.NewBigQueryProvider(bqClient),
		pg: postgres.NewPostgresProvider(dbc, cacheClient),
	}
}

func (p *MixedProvider) providerFor(reqOptions reqopts.RequestOptions) dataprovider.DataProvider {
	if reqOptions.DataSource == "postgres" {
		return p.pg
	}
	return p.bq
}

func (p *MixedProvider) Cache() cache.Cache {
	return p.bq.Cache()
}

func (p *MixedProvider) QueryReleases(ctx context.Context) ([]v1.Release, error) {
	return p.pg.QueryReleases(ctx)
}

func (p *MixedProvider) QueryReleaseDates(ctx context.Context, reqOptions reqopts.RequestOptions) ([]crtest.ReleaseTimeRange, []error) {
	return p.pg.QueryReleaseDates(ctx, reqOptions)
}

func (p *MixedProvider) QueryJobVariants(ctx context.Context, reqOptions reqopts.RequestOptions) (crtest.JobVariants, []error) {
	return p.providerFor(reqOptions).QueryJobVariants(ctx, reqOptions)
}

func (p *MixedProvider) QueryUniqueVariantValues(ctx context.Context, reqOptions reqopts.RequestOptions, field string, nested bool) ([]string, error) {
	return p.providerFor(reqOptions).QueryUniqueVariantValues(ctx, reqOptions, field, nested)
}

func (p *MixedProvider) QueryBaseTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions) (map[string]crstatus.TestStatus, []error) {
	return p.providerFor(reqOptions).QueryBaseTestStatus(ctx, reqOptions)
}

func (p *MixedProvider) QuerySampleTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions, includeVariants map[string][]string, start, end time.Time) (map[string]crstatus.TestStatus, []error) {
	return p.providerFor(reqOptions).QuerySampleTestStatus(ctx, reqOptions, includeVariants, start, end)
}

func (p *MixedProvider) QueryBaseJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions) (map[string][]crstatus.TestJobRunRows, []error) {
	return p.providerFor(reqOptions).QueryBaseJobRunTestStatus(ctx, reqOptions)
}

func (p *MixedProvider) QuerySampleJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions, includeVariants map[string][]string, start, end time.Time) (map[string][]crstatus.TestJobRunRows, []error) {
	return p.providerFor(reqOptions).QuerySampleJobRunTestStatus(ctx, reqOptions, includeVariants, start, end)
}

func (p *MixedProvider) QueryJobRuns(ctx context.Context, reqOptions reqopts.RequestOptions, release string, start, end time.Time) (map[string]dataprovider.JobRunStats, error) {
	return p.providerFor(reqOptions).QueryJobRuns(ctx, reqOptions, release, start, end)
}

func (p *MixedProvider) QueryJobVariantValues(ctx context.Context, reqOptions reqopts.RequestOptions, jobNames, variantKeys []string) (map[string]map[string]string, error) {
	return p.providerFor(reqOptions).QueryJobVariantValues(ctx, reqOptions, jobNames, variantKeys)
}

func (p *MixedProvider) LookupJobVariants(ctx context.Context, reqOptions reqopts.RequestOptions, jobName string) (map[string]string, error) {
	return p.providerFor(reqOptions).LookupJobVariants(ctx, reqOptions, jobName)
}
