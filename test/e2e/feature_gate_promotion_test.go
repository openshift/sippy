package e2e

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeatureGatePromotionInDetail(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NetworkSegmentation?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching feature gate detail")

	require.NotNil(t, fg.Promotion, "promotion field should not be nil")
	assert.NotNil(t, fg.Promotion.ResultsByVariant, "variants should not be nil")
}

func TestFeatureGatePromotionVariantResults(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NetworkSegmentation?release="+util.Release, &fg)
	require.NoError(t, err)
	require.NotNil(t, fg.Promotion)

	for _, v := range fg.Promotion.ResultsByVariant {
		assert.NotEmpty(t, v.Variants["Platform"], "variant should have a platform")
		assert.NotEmpty(t, v.Variants["Architecture"], "variant should have an architecture")
		assert.NotEmpty(t, v.Variants["Topology"], "variant should have a topology")
	}
}

func TestFeatureGatePromotionInstallGate(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/AWSDualStackInstall?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching Install gate detail")

	assert.Equal(t, "AWSDualStackInstall", fg.FeatureGate)
	require.NotNil(t, fg.Promotion, "promotion field should not be nil for Install gate")
}

func TestFeatureGatePromotionCapabilityRegressionsLink(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NetworkSegmentation?release="+util.Release, &fg)
	require.NoError(t, err)

	require.NotNil(t, fg.Links)
	link, ok := fg.Links["capability_regressions"]
	assert.True(t, ok, "missing capability_regressions link")
	assert.Contains(t, link, "/api/tests")
	assert.Contains(t, link, "Capability%3ANetworkSegmentation")
}
