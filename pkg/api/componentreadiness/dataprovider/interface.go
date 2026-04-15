package dataprovider

import (
	"context"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
)

// TestStatusQuerier fetches aggregated test pass/fail counts.
type TestStatusQuerier interface {
	// QueryBaseTestStatus returns test status for the basis release.
	QueryBaseTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
		allJobVariants crtest.JobVariants) (map[string]crstatus.TestStatus, []error)

	// QuerySampleTestStatus returns test status for the sample release.
	QuerySampleTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
		allJobVariants crtest.JobVariants,
		includeVariants map[string][]string,
		start, end time.Time) (map[string]crstatus.TestStatus, []error)
}

// TestDetailsQuerier fetches per-job-run test breakdowns used for test details reports.
type TestDetailsQuerier interface {
	QueryBaseJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
		allJobVariants crtest.JobVariants) (map[string][]crstatus.TestJobRunRows, []error)

	QuerySampleJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
		allJobVariants crtest.JobVariants,
		includeVariants map[string][]string,
		start, end time.Time) (map[string][]crstatus.TestJobRunRows, []error)
}

// MetadataQuerier fetches reference data used to configure and parameterize reports.
type MetadataQuerier interface {
	// QueryJobVariants returns all variant names and their possible values.
	QueryJobVariants(ctx context.Context) (crtest.JobVariants, []error)

	// QueryReleaseDates returns the time ranges for each known release.
	QueryReleaseDates(ctx context.Context, reqOptions reqopts.RequestOptions) ([]crtest.ReleaseTimeRange, []error)

	// QueryReleases returns known release configurations.
	QueryReleases(ctx context.Context) ([]v1.Release, error)

	// QueryUniqueVariantValues returns distinct values for a variant column
	// from the past 60 days.
	QueryUniqueVariantValues(ctx context.Context, field string, nested bool) ([]string, error)
}

// JobQuerier fetches job-level data for the view-jobs and diagnose endpoints.
type JobQuerier interface {
	// QueryJobRuns returns pass/fail statistics per job for a release in a time window.
	QueryJobRuns(ctx context.Context, reqOptions reqopts.RequestOptions,
		allJobVariants crtest.JobVariants,
		release string, start, end time.Time) (map[string]JobRunStats, error)

	// QueryJobVariantValues returns variant key/value pairs for the given jobs.
	QueryJobVariantValues(ctx context.Context, jobNames []string,
		variantKeys []string) (map[string]map[string]string, error)

	// LookupJobVariants returns all variant key/value pairs for a single job.
	LookupJobVariants(ctx context.Context, jobName string) (map[string]string, error)
}

// DataProvider combines all query capabilities needed by Component Readiness.
type DataProvider interface {
	TestStatusQuerier
	TestDetailsQuerier
	MetadataQuerier
	JobQuerier

	// Cache returns the cache implementation for storing/retrieving computed results.
	Cache() cache.Cache
}

// JobRunStats contains pass/fail statistics for a single concrete job name.
// Defined here so both the interface and implementations share the same type.
type JobRunStats struct {
	JobName        string  `json:"job_name"`
	TotalRuns      int     `json:"total_runs"`
	SuccessfulRuns int     `json:"successful_runs"`
	PassRate       float64 `json:"pass_rate"`
}
