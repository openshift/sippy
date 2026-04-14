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
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/cache/redis"
	"github.com/openshift/sippy/pkg/dataloader/regressioncacheloader"
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

func TestRegressionCacheLoader(t *testing.T) {
	credFile := os.Getenv("GCS_SA_JSON_PATH")
	if credFile == "" {
		t.Skip("GCS_SA_JSON_PATH not set, skipping regression cache loader test")
	}

	dbc := util.CreateE2EPostgresConnection(t)

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
	crFlags.ComponentReadinessViewsFile = "../../../config/e2e-views.yaml"
	sippyViews, err := crFlags.ParseViewsFile()
	require.NoError(t, err, "error parsing e2e views")
	require.Greater(t, len(sippyViews.ComponentReadiness), 0, "no views found in e2e-views.yaml")

	// Get release configs from BigQuery
	releaseConfigs, err := api.GetReleasesFromBigQuery(ctx, bqClient)
	require.NoError(t, err, "error getting releases from bigquery")

	// Build a regression store
	regressionStore := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	// Create and run the loader
	loader := regressioncacheloader.New(
		dbc, bqClient,
		&configv1.SippyConfig{},
		sippyViews.ComponentReadiness,
		releaseConfigs,
		4*time.Hour, // default CRTimeRoundingFactor
		regressionStore,
		nil, // no variant junit table overrides for e2e
	)

	t.Log("running regression cache loader...")
	loader.Load()
	require.Empty(t, loader.Errors(), "regression cache loader had errors: %v", loader.Errors())
	t.Log("regression cache loader completed successfully")

	// Fetch views from the API for use in subtests
	var views []crview.View
	err = util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error fetching views from API")
	require.Greater(t, len(views), 0, "no views returned")

	t.Run("component report served from cache", func(t *testing.T) {
		start := time.Now()
		var report componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", views[0].Name), &report)
		elapsed := time.Since(start)
		require.NoError(t, err, "error making component readiness request")

		t.Logf("component report request took %s", elapsed)
		assert.Less(t, elapsed, 10*time.Second,
			"component report request took too long (%s), may indicate cache primer failure or cache key mismatch", elapsed)
		assert.Greater(t, len(report.Rows), 25,
			"component report does not have the rows we would expect")
	})
}
