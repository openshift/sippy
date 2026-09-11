package integration

// Quay product-release coverage for the additive lifecycle selector (pkg/api/lifecycle_tests.go).
// Kept out of lifecycle_reports_test.go because it exercises apitype.Health.Links and the
// product install/upgrade testcases, neither of which exist at base commit 74967418d; that file
// is copied verbatim into a base worktree to regenerate TestLifecycleReportsMatchBase's goldens,
// so it must compile there unchanged.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// lifecycleTestsResponse mirrors the JSON body of /api/install and /api/upgrade
// (pkg/api/install.go, pkg/api/upgrade.go).
type lifecycleTestsResponse struct {
	ColumnNames []string                           `json:"column_names"`
	Tests       map[string]map[string]apitype.Test `json:"tests"`
	Links       map[string]string                  `json:"links"`
}

func installTestRow(t *testing.T, tests map[string]map[string]apitype.Test, name string) apitype.Test {
	t.Helper()
	variants, ok := tests[name]
	require.True(t, ok, "missing test row %q", name)
	row, ok := variants["All"]
	require.True(t, ok, "missing All aggregate for %q", name)
	return row
}

func TestLifecycleReportsQuay(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	seedLifecycleFixture(t, dbc, true)

	wantOCPInstallRuns, _, _ := coreTestCounts(1, 0) // testInstallOverall, aws/ovn job only (upgrade-minor excluded)

	t.Run("install", func(t *testing.T) {
		var resp lifecycleTestsResponse
		require.NoError(t, json.Unmarshal(fetchInstallReport(t, dbc, releaseQuay), &resp))

		quayRow := installTestRow(t, resp.Tests, testQuayInstall)
		ocpRow := installTestRow(t, resp.Tests, testInstallOverall)
		assert.Equal(t, quayInstallRuns, quayRow.CurrentRuns)
		assert.Equal(t, int(wantOCPInstallRuns), ocpRow.CurrentRuns)
		assert.NotEqual(t, quayRow.CurrentRuns, ocpRow.CurrentRuns, "quay and OpenShift install rows must keep their own denominators")
		assert.NotContains(t, resp.ColumnNames, "upgrade-minor", "install excludes the upgrade-minor job entirely")
		assert.NotEmpty(t, resp.Links, "product install response must carry HATEOAS links")
	})

	t.Run("upgrade", func(t *testing.T) {
		var resp lifecycleTestsResponse
		require.NoError(t, json.Unmarshal(fetchUpgradeReport(t, dbc, releaseQuay), &resp))

		quayRow := installTestRow(t, resp.Tests, testQuayUpgrade)
		sippyRow := installTestRow(t, resp.Tests, testSippyUpgrade)
		assert.Equal(t, quayUpgradeRuns, quayRow.CurrentRuns)
		assert.NotEqual(t, quayRow.CurrentRuns, sippyRow.CurrentRuns)
		assert.NotEmpty(t, resp.Links)
	})

	t.Run("health", func(t *testing.T) {
		var health apitype.Health
		require.NoError(t, json.Unmarshal(fetchHealthReport(t, dbc, releaseQuay), &health))

		for _, key := range []string{"install", "upgrade", "productInstall", "productUpgrade", "bootstrap", "installConfig", "installOther"} {
			_, ok := health.Indicators[key]
			assert.True(t, ok, "missing health indicator %q", key)
		}

		assert.Equal(t, int(wantOCPInstallRuns), health.Indicators["install"].CurrentRuns, "install indicator stays OpenShift-only")
		assert.Equal(t, quayInstallRuns, health.Indicators["productInstall"].CurrentRuns)

		require.Len(t, health.Links, 5)
		for _, key := range []string{"install", "upgrade", "health", "product_install_test", "product_upgrade_test"} {
			assert.Contains(t, health.Links, key)
		}
	})
}

func TestLifecycleReportsQuayMissingUpgrade(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	seedLifecycleFixture(t, dbc, false)

	var health apitype.Health
	require.NoError(t, json.Unmarshal(fetchHealthReport(t, dbc, releaseQuay), &health))

	_, hasProductInstall := health.Indicators["productInstall"]
	_, hasProductUpgrade := health.Indicators["productUpgrade"]
	assert.True(t, hasProductInstall, "product install already reported")
	assert.False(t, hasProductUpgrade, "product upgrade has not reported yet, so absent data must stay absent")

	assert.Contains(t, health.Links, "product_upgrade_test", "links describe the resource, not the data")

	for _, release := range []string{releaseOCPModern, releaseOCPLegacy, releaseAROStage, releaseOKD} {
		body := string(fetchHealthReport(t, dbc, release))
		assert.NotContains(t, body, "links", "non-product health response must not carry lifecycle links")
		assert.NotContains(t, body, "productInstall", "non-product health response must not carry product indicators")
	}
}
