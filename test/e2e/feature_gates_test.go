package e2e

import (
	"strings"
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
		t.Run(fg.FeatureGate+" has HATEOAS links", func(t *testing.T) {
			require.NotNil(t, fg.Links, "Links map should not be nil")

			fgTests, ok := fg.Links["gate_tests"]
			assert.True(t, ok, "missing gate_tests link")
			assert.Contains(t, fgTests, "/api/tests?release="+util.Release)
			assert.Contains(t, fgTests, "FeatureGate%3A"+fg.FeatureGate)

			if strings.Contains(fg.FeatureGate, "Install") {
				installTests, ok := fg.Links["install_tests"]
				assert.True(t, ok, "Install gate missing install_tests link")
				assert.Contains(t, installTests, "/api/tests?release="+util.Release)
				assert.Contains(t, installTests, "Capability%3A"+fg.FeatureGate)
				assert.Contains(t, installTests, "install+should+succeed")
			} else {
				_, ok := fg.Links["install_tests"]
				assert.False(t, ok, "non-Install gate should not have install_tests link")
			}

			jobTests, ok := fg.Links["gate_job_tests"]
			assert.True(t, ok, "missing gate_job_tests link")
			assert.Contains(t, jobTests, "/api/tests?release="+util.Release)
			assert.Contains(t, jobTests, "Capability%3A"+fg.FeatureGate)

			uiDetail, ok := fg.Links["ui_detail"]
			assert.True(t, ok, "missing ui_detail link")
			assert.Contains(t, uiDetail, "/feature_gates/"+util.Release+"/"+fg.FeatureGate)
		})
	}
}

func TestFeatureGatesAnnotationLinkFollowable(t *testing.T) {
	var gates []api.FeatureGate
	err := util.SippyGet("/api/feature_gates?release="+util.Release, &gates)
	require.NoError(t, err)

	gatesByName := make(map[string]api.FeatureGate)
	for _, g := range gates {
		gatesByName[g.FeatureGate] = g
	}

	fg, ok := gatesByName["NetworkSegmentation"]
	require.True(t, ok, "NetworkSegmentation not found")

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
	var gates []api.FeatureGate
	err := util.SippyGet("/api/feature_gates?release="+util.Release, &gates)
	require.NoError(t, err)

	gatesByName := make(map[string]api.FeatureGate)
	for _, g := range gates {
		gatesByName[g.FeatureGate] = g
	}

	fg, ok := gatesByName["AWSDualStackInstall"]
	require.True(t, ok, "AWSDualStackInstall not found")

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
