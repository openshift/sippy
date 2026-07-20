package e2e

import (
	"testing"

	"github.com/openshift/sippy/pkg/api/featuregatepromotion"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeatureGatePromotionAPI(t *testing.T) {
	var status featuregatepromotion.PromotionStatus
	err := util.SippyGet("/api/feature_gates/promotion?release="+util.Release+"&feature_gate=NetworkSegmentation", &status)
	require.NoError(t, err, "error fetching feature gate promotion status")

	assert.Equal(t, "NetworkSegmentation", status.FeatureGate)
	assert.Equal(t, util.Release, status.Release)
	assert.NotNil(t, status.Variants, "variants should not be nil")
}

func TestFeatureGatePromotionHATEOASLinks(t *testing.T) {
	var status featuregatepromotion.PromotionStatus
	err := util.SippyGet("/api/feature_gates/promotion?release="+util.Release+"&feature_gate=NetworkSegmentation", &status)
	require.NoError(t, err)

	require.NotNil(t, status.Links, "Links map should not be nil")

	featureGateLink, ok := status.Links["feature_gate"]
	assert.True(t, ok, "missing feature_gate link")
	assert.Contains(t, featureGateLink, "/api/feature_gates")
	assert.Contains(t, featureGateLink, "NetworkSegmentation")
}

func TestFeatureGatePromotionVariantResults(t *testing.T) {
	var status featuregatepromotion.PromotionStatus
	err := util.SippyGet("/api/feature_gates/promotion?release="+util.Release+"&feature_gate=NetworkSegmentation", &status)
	require.NoError(t, err)

	for _, v := range status.Variants {
		assert.NotEmpty(t, v.Cloud, "variant should have a cloud")
		assert.NotEmpty(t, v.Architecture, "variant should have an architecture")
		assert.NotEmpty(t, v.Topology, "variant should have a topology")
	}
}

func TestFeatureGatePromotionInstallGate(t *testing.T) {
	var status featuregatepromotion.PromotionStatus
	err := util.SippyGet("/api/feature_gates/promotion?release="+util.Release+"&feature_gate=AWSDualStackInstall", &status)
	require.NoError(t, err, "error fetching Install gate promotion status")

	assert.Equal(t, "AWSDualStackInstall", status.FeatureGate)
	assert.Equal(t, util.Release, status.Release)
}

func TestFeatureGatePromotionMissingParams(t *testing.T) {
	t.Run("missing release", func(t *testing.T) {
		var status featuregatepromotion.PromotionStatus
		err := util.SippyGet("/api/feature_gates/promotion?feature_gate=NetworkSegmentation", &status)
		assert.Error(t, err, "should fail without release param")
	})

	t.Run("missing feature_gate", func(t *testing.T) {
		var status featuregatepromotion.PromotionStatus
		err := util.SippyGet("/api/feature_gates/promotion?release="+util.Release, &status)
		assert.Error(t, err, "should fail without feature_gate param")
	})
}
