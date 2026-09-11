package api

// DB-free coverage for the lifecycle test selection (lifecycle_tests.go) and the response
// builder functions extracted from install.go, upgrade.go and health.go.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testidentification"
)

// baseSelection reproduces, verbatim, the pre-lifecycle-tests.go test-name selection logic, before
// lifecycleTestsForRelease existed. For a non-product release this must equal
// lifecycleTestsForRelease(release) field by field: the additive product selector in
// lifecycle_tests.go must change nothing for OpenShift-only releases.
func baseSelection(release string) lifecycleTestSelection {
	installExactNames := sets.New[string]()
	installPrefixes := sets.New(testidentification.OperatorInstallPrefix)
	infraTestName := testidentification.InfrastructureTestName
	installTestName := testidentification.InstallTestName
	if useNewInstallTest(release) {
		installPrefixes.Insert(testidentification.InstallTestNamePrefix)
		infraTestName = testidentification.NewInfrastructureTestName
		installTestName = testidentification.NewInstallTestName
	} else {
		installExactNames.Insert(testidentification.InstallTestName)
	}

	return lifecycleTestSelection{
		InstallExactNames: installExactNames,
		InstallPrefixes:   installPrefixes,

		UpgradeExactNames: sets.New(testidentification.UpgradeTestName),
		UpgradePrefixes:   sets.New(testidentification.OperatorUpgradePrefix),
		UpgradeSubstrings: sets.New(
			testidentification.OperatorsUpgradedTest,
			testidentification.APIsRemainAvailTest,
			testidentification.MachineConfigsUpgradedTest,
			testidentification.CVOAcknowledgesUpgradeTest,
		),

		HealthInstallTestName: installTestName,
		HealthUpgradeTestName: testidentification.UpgradeTestName,
		HealthInfraTestName:   infraTestName,
	}
}

func TestLifecycleTestsForReleaseMatchesBaseForNonProductReleases(t *testing.T) {
	for _, release := range []string{"4.21", "4.10", "4.21-okd", "aro-stage"} {
		t.Run(release, func(t *testing.T) {
			want := baseSelection(release)
			got := lifecycleTestsForRelease(release)

			assert.True(t, want.InstallExactNames.Equal(got.InstallExactNames), "InstallExactNames")
			assert.True(t, want.InstallPrefixes.Equal(got.InstallPrefixes), "InstallPrefixes")
			assert.True(t, want.UpgradeExactNames.Equal(got.UpgradeExactNames), "UpgradeExactNames")
			assert.True(t, want.UpgradePrefixes.Equal(got.UpgradePrefixes), "UpgradePrefixes")
			assert.True(t, want.UpgradeSubstrings.Equal(got.UpgradeSubstrings), "UpgradeSubstrings")
			assert.Equal(t, want.HealthInstallTestName, got.HealthInstallTestName)
			assert.Equal(t, want.HealthUpgradeTestName, got.HealthUpgradeTestName)
			assert.Equal(t, want.HealthInfraTestName, got.HealthInfraTestName)
			assert.Empty(t, got.ProductInstallTestName)
			assert.Empty(t, got.ProductUpgradeTestName)
		})
	}
}

// fixtureTest builds a minimal apitype.Test row; only the fields the install/upgrade/health
// bodies expose are set, everything else stays at its zero value.
func fixtureTest(name string, runs, successes int) apitype.Test {
	return apitype.Test{
		Name:             name,
		SuiteName:        "openshift-tests",
		CurrentRuns:      runs,
		CurrentSuccesses: successes,
		CurrentFailures:  runs - successes,
	}
}

func TestInstallReportSummary(t *testing.T) {
	variantColumns := sets.New("All", "aws")
	tests := map[string]map[string]apitype.Test{
		testidentification.NewInstallTestName: {
			"All": fixtureTest(testidentification.NewInstallTestName, 100, 95),
			"aws": fixtureTest(testidentification.NewInstallTestName, 50, 48),
		},
		testidentification.OperatorInstallPrefix + "kube-apiserver": {
			"All": fixtureTest(testidentification.OperatorInstallPrefix+"kube-apiserver", 100, 99),
		},
	}

	got, err := json.Marshal(installReportSummary("4.21", lifecycleTestsForRelease("4.21"), variantColumns, tests))
	require.NoError(t, err)

	want := `{"column_names":["All","aws"],"description":"Install Rates by Operator by Variant","tests":{"install should succeed: overall":{"All":{"name":"install should succeed: overall","suite_name":"openshift-tests","variants":null,"jira_component":"","jira_component_id":0,"current_successes":95,"current_failures":5,"current_flakes":0,"current_pass_percentage":0,"current_failure_percentage":0,"current_flake_percentage":0,"current_working_percentage":0,"current_runs":100,"previous_successes":0,"previous_failures":0,"previous_flakes":0,"previous_pass_percentage":0,"previous_failure_percentage":0,"previous_flake_percentage":0,"previous_working_percentage":0,"previous_runs":0,"net_failure_improvement":0,"net_flake_improvement":0,"net_working_improvement":0,"net_improvement":0,"tags":null,"open_bugs":0},"aws":{"name":"install should succeed: overall","suite_name":"openshift-tests","variants":null,"jira_component":"","jira_component_id":0,"current_successes":48,"current_failures":2,"current_flakes":0,"current_pass_percentage":0,"current_failure_percentage":0,"current_flake_percentage":0,"current_working_percentage":0,"current_runs":50,"previous_successes":0,"previous_failures":0,"previous_flakes":0,"previous_pass_percentage":0,"previous_failure_percentage":0,"previous_flake_percentage":0,"previous_working_percentage":0,"previous_runs":0,"net_failure_improvement":0,"net_flake_improvement":0,"net_working_improvement":0,"net_improvement":0,"tags":null,"open_bugs":0}},"operator install kube-apiserver":{"All":{"name":"operator install kube-apiserver","suite_name":"openshift-tests","variants":null,"jira_component":"","jira_component_id":0,"current_successes":99,"current_failures":1,"current_flakes":0,"current_pass_percentage":0,"current_failure_percentage":0,"current_flake_percentage":0,"current_working_percentage":0,"current_runs":100,"previous_successes":0,"previous_failures":0,"previous_flakes":0,"previous_pass_percentage":0,"previous_failure_percentage":0,"previous_flake_percentage":0,"previous_working_percentage":0,"previous_runs":0,"net_failure_improvement":0,"net_flake_improvement":0,"net_working_improvement":0,"net_improvement":0,"tags":null,"open_bugs":0}}},"title":"Install Rates by Operator"}`

	assert.JSONEq(t, want, string(got), "no links key for a non-product release")
}

func TestUpgradeReportSummary(t *testing.T) {
	variantColumns := sets.New("All", "gcp")
	tests := map[string]map[string]apitype.Test{
		testidentification.UpgradeTestName: {
			"All": fixtureTest(testidentification.UpgradeTestName, 80, 76),
		},
	}

	got, err := json.Marshal(upgradeReportSummary("4.21", lifecycleTestsForRelease("4.21"), variantColumns, tests))
	require.NoError(t, err)

	want := `{"column_names":["All","gcp"],"description":"Upgrade Rates by Operator by Variant","tests":{"[sig-sippy] upgrade should work":{"All":{"name":"[sig-sippy] upgrade should work","suite_name":"openshift-tests","variants":null,"jira_component":"","jira_component_id":0,"current_successes":76,"current_failures":4,"current_flakes":0,"current_pass_percentage":0,"current_failure_percentage":0,"current_flake_percentage":0,"current_working_percentage":0,"current_runs":80,"previous_successes":0,"previous_failures":0,"previous_flakes":0,"previous_pass_percentage":0,"previous_failure_percentage":0,"previous_flake_percentage":0,"previous_working_percentage":0,"previous_runs":0,"net_failure_improvement":0,"net_flake_improvement":0,"net_working_improvement":0,"net_improvement":0,"tags":null,"open_bugs":0}}},"title":"Upgrade Rates by Operator"}`

	assert.JSONEq(t, want, string(got), "no links key for a non-product release")
}

func TestReleaseHealthResponse(t *testing.T) {
	lastUpdated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	indicators := map[string]apitype.Test{
		"infrastructure": fixtureTest(testidentification.NewInfrastructureTestName, 10, 9),
		"install":        fixtureTest(testidentification.NewInstallTestName, 10, 8),
	}

	got := releaseHealthResponse("4.21", lifecycleTestsForRelease("4.21"), indicators, lastUpdated, sippyprocessingv1.Statistics{}, sippyprocessingv1.Statistics{}, nil)

	body, err := json.Marshal(got)
	require.NoError(t, err)

	want := `{"indicators":{"infrastructure":{"name":"install should succeed: infrastructure","suite_name":"openshift-tests","variants":null,"jira_component":"","jira_component_id":0,"current_successes":9,"current_failures":1,"current_flakes":0,"current_pass_percentage":0,"current_failure_percentage":0,"current_flake_percentage":0,"current_working_percentage":0,"current_runs":10,"previous_successes":0,"previous_failures":0,"previous_flakes":0,"previous_pass_percentage":0,"previous_failure_percentage":0,"previous_flake_percentage":0,"previous_working_percentage":0,"previous_runs":0,"net_failure_improvement":0,"net_flake_improvement":0,"net_working_improvement":0,"net_improvement":0,"tags":null,"open_bugs":0},"install":{"name":"install should succeed: overall","suite_name":"openshift-tests","variants":null,"jira_component":"","jira_component_id":0,"current_successes":8,"current_failures":2,"current_flakes":0,"current_pass_percentage":0,"current_failure_percentage":0,"current_flake_percentage":0,"current_working_percentage":0,"current_runs":10,"previous_successes":0,"previous_failures":0,"previous_flakes":0,"previous_pass_percentage":0,"previous_failure_percentage":0,"previous_flake_percentage":0,"previous_working_percentage":0,"previous_runs":0,"net_failure_improvement":0,"net_flake_improvement":0,"net_working_improvement":0,"net_improvement":0,"tags":null,"open_bugs":0}},"variants":{"current":{"success":0,"failed":0,"unstable":0},"previous":{"success":0,"failed":0,"unstable":0}},"last_updated":"2026-01-02T03:04:05Z","promotions":null,"warnings":null,"current_statistics":{"mean":0,"standard_deviation":0,"quartiles":null,"p95":0,"histogram":null},"previous_statistics":{"mean":0,"standard_deviation":0,"quartiles":null,"p95":0,"histogram":null}}`

	assert.JSONEq(t, want, string(body), "Health.Links must be absent for a non-product release")
}

func TestLifecycleReportSummariesForQuayIncludeLinks(t *testing.T) {
	const release = "quay-3.18"
	sel := lifecycleTestsForRelease(release)
	require.NotEmpty(t, sel.ProductInstallTestName, "quay-3.18 must resolve as a product release")

	t.Run("install", func(t *testing.T) {
		tests := map[string]map[string]apitype.Test{
			sel.ProductInstallTestName: {"All": fixtureTest(sel.ProductInstallTestName, 4242, 4200)},
		}
		got, err := json.Marshal(installReportSummary(release, sel, sets.New("All"), tests))
		require.NoError(t, err)

		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(got, &decoded))
		links, ok := decoded["links"].(map[string]interface{})
		require.True(t, ok, "product install response must carry HATEOAS links")
		assert.Equal(t, "/api/install?release=quay-3.18", links["install"])
		assert.Equal(t, "/api/upgrade?release=quay-3.18", links["upgrade"])
		assert.Equal(t, "/api/health?release=quay-3.18", links["health"])
		assert.Contains(t, links, "product_install_test")
		assert.Contains(t, links, "product_upgrade_test")
	})

	t.Run("upgrade", func(t *testing.T) {
		tests := map[string]map[string]apitype.Test{
			sel.ProductUpgradeTestName: {"All": fixtureTest(sel.ProductUpgradeTestName, 3131, 3100)},
		}
		got, err := json.Marshal(upgradeReportSummary(release, sel, sets.New("All"), tests))
		require.NoError(t, err)

		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(got, &decoded))
		assert.NotEmpty(t, decoded["links"], "product upgrade response must carry HATEOAS links")
	})

	t.Run("health", func(t *testing.T) {
		indicators := map[string]apitype.Test{
			"productInstall": fixtureTest(sel.ProductInstallTestName, 4242, 4200),
			"productUpgrade": fixtureTest(sel.ProductUpgradeTestName, 3131, 3100),
		}
		health := releaseHealthResponse(release, sel, indicators, time.Now().UTC(), sippyprocessingv1.Statistics{}, sippyprocessingv1.Statistics{}, nil)

		require.Len(t, health.Links, 5)
		for _, key := range []string{"install", "upgrade", "health", "product_install_test", "product_upgrade_test"} {
			assert.Contains(t, health.Links, key)
		}
	})
}
