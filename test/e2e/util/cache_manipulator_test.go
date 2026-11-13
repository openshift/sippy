package util

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db/models"
)

// TestCachePrimingReplacesStaleFixtures exercises the launcher's isolation boundary
// against the running e2e API and Redis, including fixtures left by interrupted runs.
func TestCachePrimingReplacesStaleFixtures(t *testing.T) {
	root := os.Getenv("SIPPY_E2E_REPO_ROOT")
	if root == "" {
		t.Skip("SIPPY_E2E_REPO_ROOT not set, skipping e2e cache priming")
	}
	cache, err := NewE2ECacheManipulator(Release)
	require.NoError(t, err)
	defer cache.Close()

	prime := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-c", "source scripts/e2e.sh; e2e_prime_component_readiness_cache")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "cache priming failed: %s", output)
	}
	// Run before closing the client, even when an assertion fails.
	defer prime()

	unrelatedKey := "_SIPPY_e2e-cache-priming-" + uuid.NewString()
	require.NoError(t, cache.client.Set(unrelatedKey, "untouched", 5*time.Minute).Err())
	defer func() { require.NoError(t, cache.client.Del(unrelatedKey).Err()) }()

	fixtures := make([]componentreport.ReportTestSummary, 2)
	fixtures[0].TestName, fixtures[0].ReportStatus = "TestSomthing", crtest.MissingSample
	fixtures[1].TestName, fixtures[1].ReportStatus = "CompletelyDifferentTest", crtest.NotSignificant
	for i := range fixtures {
		fixtures[i].TestID = fixtures[i].TestName
		fixtures[i].Component, fixtures[i].Capability = "e2e-cache-fixtures", "e2e-cache-fixtures"
		fixtures[i].Regression = &models.TestRegression{ID: uint(i + 1)}
	}
	for attempt := 0; attempt < 2; attempt++ {
		require.NoError(t, cache.AddTestRegressionsToReport(fixtures))
		_, cacheKey, err := cache.GetReport()
		require.NoError(t, err)
		// The previous primer used this ordinary GET, which keeps stale fixtures.
		var stale componentreport.ComponentReport
		require.NoError(t, SippyGet("/api/component_readiness?view="+Release+"-main", &stale))
		staleJSON, err := json.Marshal(stale)
		require.NoError(t, err)
		for _, fixture := range fixtures {
			require.True(t, strings.Contains(string(staleJSON), fixture.TestName), "fixture %s was not cached", fixture.TestName)
		}

		prime()
		fresh, refreshedKey, err := cache.GetReport()
		require.NoError(t, err)
		require.Equal(t, cacheKey, refreshedKey)
		freshJSON, err := json.Marshal(fresh)
		require.NoError(t, err)
		for _, fixture := range fixtures {
			require.False(t, strings.Contains(string(freshJSON), fixture.TestName), "attempt %d retained stale fixture %s", attempt+1, fixture.TestName)
		}
		value, err := cache.client.Get(unrelatedKey).Result()
		require.NoError(t, err)
		require.Equal(t, "untouched", value)
	}
}

func TestFindMainComponentReportCacheKey(t *testing.T) {
	mainKey := componentReportCacheKey(t, reqopts.TestFilters{}, "Network", "Platform", "Topology")
	filteredKey := componentReportCacheKey(t, reqopts.TestFilters{Capabilities: []string{"Networking"}}, "Network", "Platform", "Topology")
	gcpOnlyKey := componentReportCacheKey(t, reqopts.TestFilters{}, "Network")

	tests := []struct {
		name string
		keys []string
		want string
	}{
		{
			name: "main before gcp-only",
			keys: []string{mainKey, gcpOnlyKey},
			want: mainKey,
		},
		{
			name: "gcp-only before main",
			keys: []string{gcpOnlyKey, mainKey},
			want: mainKey,
		},
		{
			name: "filtered before main",
			keys: []string{filteredKey, mainKey},
			want: mainKey,
		},
		{
			name: "no main match",
			keys: []string{gcpOnlyKey},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := findMainComponentReportCacheKey(test.keys, Release); got != test.want {
				t.Fatalf("expected cache key %q, got %q", test.want, got)
			}
		})
	}
}

func componentReportCacheKey(t *testing.T, testFilters reqopts.TestFilters, columnGroupBy ...string) string {
	t.Helper()

	key, err := json.Marshal(componentreadiness.GeneratorCacheKey{
		BaseRelease:   reqopts.Release{Name: BaseRelease},
		SampleRelease: reqopts.Release{Name: Release},
		VariantOption: reqopts.Variants{
			ColumnGroupBy: sets.New(columnGroupBy...),
		},
		TestFilters: testFilters,
	})
	if err != nil {
		t.Fatalf("failed to marshal cache key: %v", err)
	}
	return "_SIPPY_cc:" + componentreadiness.ComponentReportCacheKeyPrefix + string(key)
}
