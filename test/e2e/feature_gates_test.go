package e2e

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeatureGatesAPI(t *testing.T) {
	var gates []api.FeatureGate
	err := util.SippyGet("/api/feature_gates?release="+util.Release, &gates)
	require.NoError(t, err, "error fetching feature gates")
	require.Greater(t, len(gates), 0, "no feature gates returned")
	t.Logf("found %d feature gates", len(gates))

	gatesByName := make(map[string]api.FeatureGate)
	for _, g := range gates {
		gatesByName[g.FeatureGate] = g
	}

	t.Run("NetworkSegmentation gate exists with correct data", func(t *testing.T) {
		fg, ok := gatesByName["NetworkSegmentation"]
		require.True(t, ok, "NetworkSegmentation feature gate not found")
		assert.Equal(t, util.Release, fg.Release)
		assert.Greater(t, fg.UniqueTestCount, int64(0), "expected tests for NetworkSegmentation")
		assert.NotEmpty(t, fg.Enabled, "expected enabled topologies")
	})

	t.Run("AWSDualStackInstall gate exists", func(t *testing.T) {
		fg, ok := gatesByName["AWSDualStackInstall"]
		require.True(t, ok, "AWSDualStackInstall feature gate not found")
		assert.Equal(t, util.Release, fg.Release)
	})
}

func TestFeatureGatesHATEOASLinks(t *testing.T) {
	var gates []api.FeatureGate
	err := util.SippyGet("/api/feature_gates?release="+util.Release, &gates)
	require.NoError(t, err, "error fetching feature gates")
	require.Greater(t, len(gates), 0, "no feature gates returned")

	for _, fg := range gates {
		t.Run(fg.FeatureGate+" has list HATEOAS links", func(t *testing.T) {
			require.NotNil(t, fg.Links, "Links map should not be nil")

			uiDetail, ok := fg.Links["ui_detail"]
			assert.True(t, ok, "missing ui_detail link")
			assert.Contains(t, uiDetail, "/feature_gates/"+util.Release+"/"+fg.FeatureGate)

			apiDetail, ok := fg.Links["api_detail"]
			assert.True(t, ok, "missing api_detail link")
			assert.Contains(t, apiDetail, "/api/feature_gates/"+fg.FeatureGate)
			assert.Contains(t, apiDetail, "release="+util.Release)

			assert.Len(t, fg.Links, 2, "list API should only have ui_detail and api_detail links")
		})
	}
}

func TestFeatureGateDetailAPI(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NetworkSegmentation?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching feature gate detail")

	assert.Equal(t, "NetworkSegmentation", fg.FeatureGate)
	assert.Equal(t, util.Release, fg.Release)
	assert.Greater(t, fg.UniqueTestCount, int64(0), "expected tests for NetworkSegmentation")

	require.NotNil(t, fg.Links, "Links map should not be nil")

	gateTests, ok := fg.Links["gate_tests"]
	assert.True(t, ok, "missing gate_tests link")
	assert.Contains(t, gateTests, "/api/tests?release="+util.Release)
	assert.Contains(t, gateTests, "FeatureGate%3ANetworkSegmentation")

	_, hasInstall := fg.Links["install_tests"]
	assert.False(t, hasInstall, "NetworkSegmentation should not have install_tests link")

	jobTests, ok := fg.Links["gate_job_tests"]
	assert.True(t, ok, "missing gate_job_tests link")
	assert.Contains(t, jobTests, "/api/tests?release="+util.Release)
	assert.Contains(t, jobTests, "Capability%3ANetworkSegmentation")

	capReg, ok := fg.Links["capability_regressions"]
	assert.True(t, ok, "missing capability_regressions link")
	assert.Contains(t, capReg, "/api/tests")

	uiDetail, ok := fg.Links["ui_detail"]
	assert.True(t, ok, "missing ui_detail link")
	assert.Contains(t, uiDetail, "/feature_gates/"+util.Release+"/NetworkSegmentation")
}

func TestFeatureGateDetailInstallGate(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/AWSDualStackInstall?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching Install gate detail")

	assert.Equal(t, "AWSDualStackInstall", fg.FeatureGate)
	require.NotNil(t, fg.Links)

	_, ok := fg.Links["install_tests"]
	assert.True(t, ok, "Install gate should have install_tests link")
}

func TestFeatureGateDetailNotFound(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NonExistentGate12345?release="+util.Release, &fg)
	assert.Error(t, err, "should return error for non-existent gate")
}

func TestFeatureGatesAnnotationLinkFollowable(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NetworkSegmentation?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching NetworkSegmentation detail")

	link := fg.Links["gate_tests"]
	require.NotEmpty(t, link)

	var tests []api.Test
	err = util.SippyGetAbsolute(link, &tests)
	require.NoError(t, err, "failed to follow gate_tests link")
	assert.Greater(t, len(tests), 0, "expected tests when following gate_tests link for NetworkSegmentation")
	for _, test := range tests {
		assert.Contains(t, test.Name, "FeatureGate:NetworkSegmentation", "test name should contain the feature gate annotation")
	}
}

func TestFeatureGatesInstallTestsLinkFollowable(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/AWSDualStackInstall?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching AWSDualStackInstall detail")

	link := fg.Links["install_tests"]
	require.NotEmpty(t, link)

	var tests []api.Test
	err = util.SippyGetAbsolute(link, &tests)
	require.NoError(t, err, "failed to follow install_tests link")
	assert.Greater(t, len(tests), 0, "expected tests when following install_tests link for AWSDualStackInstall")
	for _, test := range tests {
		assert.Contains(t, test.Name, "install should succeed",
			"install gate tests should contain install tests")
	}
}

func TestFeatureGatesFilterByName(t *testing.T) {
	var gates []api.FeatureGate
	filterJSON := `{"items":[{"columnField":"feature_gate","operatorValue":"equals","value":"NetworkSegmentation"}]}`
	err := util.SippyGet("/api/feature_gates?release="+util.Release+"&filter="+filterJSON, &gates)
	require.NoError(t, err, "error filtering feature gates")
	require.Len(t, gates, 1, "expected exactly one feature gate")
	assert.Equal(t, "NetworkSegmentation", gates[0].FeatureGate)
	assert.NotNil(t, gates[0].Links)
}
