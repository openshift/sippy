package componentreadiness

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/cache/redis"
	"github.com/openshift/sippy/pkg/dataloader/regressioncacheloader"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentReadinessViews(t *testing.T) {
	var views []crview.View
	err := util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error making http request")
	t.Logf("found %d views", len(views))
	require.Greater(t, len(views), 0, "no views returned, check server cli params")
}

func TestCapabilitiesFilter(t *testing.T) {
	var views []crview.View
	err := util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error fetching views")
	require.Greater(t, len(views), 0, "no views returned")

	viewName := views[0].Name

	// Fetch without capabilities filter
	var unfilteredReport componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", viewName), &unfilteredReport)
	require.NoError(t, err, "error fetching unfiltered component report")
	require.Greater(t, len(unfilteredReport.Rows), 0, "unfiltered report has no rows")

	// Fetch with testCapabilities=install filter
	var filteredReport componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&testCapabilities=install", viewName), &filteredReport)
	require.NoError(t, err, "error fetching filtered component report")

	// The filtered report should have fewer rows since only tests with
	// the "install" capability are included.
	assert.Less(t, len(filteredReport.Rows), len(unfilteredReport.Rows),
		"capabilities filter did not reduce the number of rows (got %d filtered vs %d unfiltered)",
		len(filteredReport.Rows), len(unfilteredReport.Rows))

	// Collect component names from each report to verify filtering
	unfilteredComponents := map[string]bool{}
	for _, row := range unfilteredReport.Rows {
		unfilteredComponents[row.Component] = true
	}
	filteredComponents := map[string]bool{}
	for _, row := range filteredReport.Rows {
		filteredComponents[row.Component] = true
	}

	// The unfiltered report should have components that the filtered
	// report does not (e.g., components with only "cap1" tests).
	assert.Greater(t, len(unfilteredComponents), len(filteredComponents),
		"filter should exclude components that have no tests with the install capability")

	t.Logf("unfiltered: %d components, filtered (install): %d components",
		len(unfilteredComponents), len(filteredComponents))
}

func TestRegressionCacheLoader(t *testing.T) {
	credFile := os.Getenv("GCS_SA_JSON_PATH")
	if credFile == "" {
		t.Skip("GCS_SA_JSON_PATH not set, skipping regression cache loader test")
	}

	dbc := util.CreateE2EPostgresConnection(t)

	// PostgreSQL is required: the test body reads releases from Postgres, builds a
	// Postgres regression store, and queries dbc.DB directly.
	if dbc == nil {
		t.Skip("PostgreSQL is required for this regression cache loader test")
	}

	// Set up Redis cache client
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:23479"
	}
	cacheClient, err := redis.NewRedisCache(redisURL)
	require.NoError(t, err, "error connecting to redis")

	// Set up BigQuery client
	ctx := context.Background()
	opCtx, ctx := bqcachedclient.OpCtxForCronEnv(ctx, "e2e")
	bqClient, err := bqcachedclient.New(ctx, opCtx, cacheClient,
		credFile, "openshift-gce-devel", "ci_analysis_us",
		"openshift-ci-data-analysis.ci_data.Releases")
	require.NoError(t, err, "error creating bigquery client")

	// Parse the e2e views
	crFlags := flags.NewComponentReadinessFlags()
	crFlags.ComponentReadinessViewsFile = "../../../config/seed-views.yaml"
	sippyViews, err := crFlags.ParseViewsFile()
	require.NoError(t, err, "error parsing seed views")
	require.Greater(t, len(sippyViews.ComponentReadiness), 0, "no views found in seed-views.yaml")

	// Get release configs from PostgreSQL
	releaseConfigs, err := api.GetReleasesFromDB(ctx, dbc)
	require.NoError(t, err, "error getting releases from postgres")

	// Build a regression store
	regressionStore := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	// Build the data provider using the default provider selection, which
	// cascades to whichever backend is configured (BigQuery, Postgres, or both).
	dataProvider, err := flags.NewDataProvider("default", bqClient, dbc, cacheClient)
	require.NoError(t, err, "error creating data provider")

	// Create and run the loader
	loader, err := regressioncacheloader.New(
		dbc, dataProvider,
		&configv1.SippyConfig{},
		sippyViews.ComponentReadiness,
		releaseConfigs,
		12*time.Hour, // default CRTimeRoundingFactor
		4*time.Hour,  // default CRTimeRoundingOffset
		regressionStore,
	)
	require.NoError(t, err)

	t.Log("running regression cache loader...")
	loader.Load()
	require.Empty(t, loader.Errors(), "regression cache loader had errors: %v", loader.Errors())
	t.Log("regression cache loader completed successfully")

	// Fetch views from the API for use in subtests
	var views []crview.View
	err = util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error fetching views from API")
	require.Greater(t, len(views), 0, "no views returned")

	// Fetch the component report once for use in subtests
	var report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", views[0].Name), &report)
	require.NoError(t, err, "error fetching component report")

	t.Run("component report served from cache", func(t *testing.T) {
		start := time.Now()
		var cachedReport componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", views[0].Name), &cachedReport)
		elapsed := time.Since(start)
		require.NoError(t, err, "error making component readiness request")

		t.Logf("component report request took %s", elapsed)
		assert.Less(t, elapsed, 10*time.Second,
			"component report request took too long (%s), may indicate cache primer failure or cache key mismatch", elapsed)
		assert.Greater(t, len(cachedReport.Rows), 25,
			"component report does not have the rows we would expect")
	})

	t.Run("regressions tracked with job runs", func(t *testing.T) {
		// Collect unresolved regressed tests from the report, matching the loader's logic
		var regressedTests []componentreport.ReportTestSummary
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				for _, reg := range col.RegressedTests {
					if reg.ReportStatus < crtest.FixedRegression {
						regressedTests = append(regressedTests, reg)
					}
				}
			}
		}
		t.Logf("found %d unresolved regressed tests in report", len(regressedTests))
		if len(regressedTests) == 0 {
			t.Skip("no regressed tests in report, nothing to verify")
		}

		release := sippyViews.ComponentReadiness[0].SampleRelease.Name

		for _, regTest := range regressedTests {
			// Look up the regression in the database by test_id and release
			var dbReg models.TestRegression
			res := dbc.DB.
				Where("test_id = ? AND release = ?", regTest.TestID, release).
				First(&dbReg)
			require.NoError(t, res.Error,
				"regression for test %s (%s) not found in db", regTest.TestName, regTest.TestID)

			assert.Equal(t, regTest.TestName, dbReg.TestName)
			assert.Equal(t, regTest.Component, dbReg.Component)
			assert.Equal(t, regTest.Capability, dbReg.Capability)
			assert.False(t, dbReg.Closed.Valid,
				"regression for %s should be open", regTest.TestName)

			// Verify job runs were tracked for this regression
			var jobRuns []models.RegressionJobRun
			res = dbc.DB.Where("regression_id = ?", dbReg.ID).Find(&jobRuns)
			require.NoError(t, res.Error,
				"error querying job runs for regression %d (%s)", dbReg.ID, regTest.TestName)
			assert.Greater(t, len(jobRuns), 2,
				"regression %d (%s) should have at least three failed job run tracked", dbReg.ID, regTest.TestName)

			// Every tracked job run should have basic fields populated
			for _, jr := range jobRuns {
				assert.NotEmpty(t, jr.ProwJobRunID, "job run missing ProwJobRunID")
				assert.NotEmpty(t, jr.ProwJobName, "job run missing ProwJobName")
				assert.False(t, jr.StartTime.IsZero(), "job run missing StartTime")
			}

			t.Logf("regression %d: test=%s, job_runs=%d", dbReg.ID, regTest.TestName, len(jobRuns))
		}
	})
}
