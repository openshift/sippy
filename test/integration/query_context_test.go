package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// TestQueryFunctionsHonorCanceledContext verifies that query functions issue their
// SQL with the caller's context, so a canceled HTTP request stops the database work
// instead of letting it run to completion.
func TestQueryFunctionsHonorCanceledContext(t *testing.T) {
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	boundary := time.Date(2024, 6, 8, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	sample, base := testsReportPeriodsOnly()

	// Seed rows for lookups that return an error when nothing matches, so the
	// live-context baseline succeeds and only cancellation can cause failure.
	dbc := intutil.NewTestDB(t, pgContainer)
	intutil.CreateReleaseDefinition(t, dbc, "4.16", 4, 16)
	job := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-4.16", "4.16", nil)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.16", start.Add(24*time.Hour), true, v1.JobSucceeded)
	runID := int64(run.ID) //nolint:gosec // G115: prow_job_runs.id is a PostgreSQL serial, always within int64 range

	tests := []struct {
		name string
		run  func(ctx context.Context, dbc *db.DB) error
	}{
		{
			name: "JobReports",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.JobReports(ctx, dbc, &filter.FilterOptions{Filter: &filter.Filter{}}, "4.16", start, boundary, end)
				return err
			},
		},
		{
			name: "VariantReports",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.VariantReports(ctx, dbc, "4.16", start, boundary, end)
				return err
			},
		},
		{
			name: "BuildClusterHealth",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.BuildClusterHealth(ctx, dbc, "4.16", start, boundary, end)
				return err
			},
		},
		{
			name: "ListFilteredJobIDs",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.ListFilteredJobIDs(ctx, dbc, "4.16", &filter.Filter{}, start, boundary, end, 10, "name", apitype.SortAscending)
				return err
			},
		},
		{
			name: "TestOutputs",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.TestOutputs(ctx, dbc, "4.16", "some-test", nil, nil, 10)
				return err
			},
		},
		{
			name: "TestReportQueryCollapsed resolves dates with the caller context",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.TestReportQueryCollapsed(ctx, dbc, "4.16", sample, base, nil, nil, nil)
				return err
			},
		},
		{
			name: "CurrentActiveRelease",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.CurrentActiveRelease(ctx, dbc)
				return err
			},
		},
		{
			name: "GetLastPayloadTags",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.GetLastPayloadTags(ctx, dbc.DB, "4.16", "nightly", "amd64", end)
				return err
			},
		},
		{
			name: "LookupProwJobRunPartitionKeys",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.LookupProwJobRunPartitionKeys(ctx, dbc.DB, runID)
				return err
			},
		},
		{
			name: "CountRegressionFailuresAfter keeps the caller cancellation under its own timeout",
			run: func(ctx context.Context, dbc *db.DB) error {
				_, err := query.CountRegressionFailuresAfter(ctx, dbc, 1, start)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.run(t.Context(), dbc), "query should succeed with a live context")

			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			err := tt.run(ctx, dbc)
			require.Error(t, err, "query should fail once the context is canceled")
			assert.ErrorIs(t, err, context.Canceled)
		})
	}
}

// TestReturnedQueryBuildersCarryCallerContext verifies that query builders handed
// back to callers keep the caller's context, so canceling the request also cancels
// the deferred execution.
func TestReturnedQueryBuildersCarryCallerContext(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	sample, base := testsReportPeriodsOnly()
	reportEnd := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		build func(ctx context.Context) (scan func() error, err error)
	}{
		{
			name: "TestReportQuery",
			build: func(ctx context.Context) (func() error, error) {
				q, err := query.TestReportQuery(ctx, dbc, "4.16", sample, base, query.TestNameMatches{})
				if err != nil {
					return nil, err
				}
				return func() error { return q.Scan(&[]map[string]any{}).Error }, nil
			},
		},
		{
			name: "TestReportQueryCollapsed",
			build: func(ctx context.Context) (func() error, error) {
				q, err := query.TestReportQueryCollapsed(ctx, dbc, "4.16", sample, base, nil, nil, nil)
				if err != nil {
					return nil, err
				}
				return func() error { return q.Scan(&[]map[string]any{}).Error }, nil
			},
		},
		{
			name: "UncollapsedTestReportWithStats",
			build: func(ctx context.Context) (func() error, error) {
				q, _, err := query.UncollapsedTestReportWithStats(ctx, dbc, "4.16", sample, base, nil, nil, nil, nil)
				if err != nil {
					return nil, err
				}
				return func() error { return q.Scan(&[]map[string]any{}).Error }, nil
			},
		},
		{
			name: "GetTestFailuresForPayloadStream",
			build: func(ctx context.Context) (func() error, error) {
				q := query.GetTestFailuresForPayloadStream(ctx, dbc.DB, "4.16", "nightly", "amd64", reportEnd, "")
				return func() error { return q.Scan(&[]map[string]any{}).Error }, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			scan, err := tt.build(ctx)
			require.NoError(t, err, "building the query should succeed with a live context")

			cancel()
			err = scan()
			require.Error(t, err, "executing the built query should fail once the caller context is canceled")
			assert.ErrorIs(t, err, context.Canceled)
		})
	}
}

// testsReportPeriodsOnly returns the fixed sample and base ranges shared by the
// Tests-report integration tests.
func testsReportPeriodsOnly() (sample, base query.DateRange) {
	sample, base, _, _, _ = testsReportPeriods()
	return sample, base
}
