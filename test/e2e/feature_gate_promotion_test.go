package e2e

import (
	"strings"
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

	require.NotEmpty(t, fg.Promotion.ResultsByVariant, "expected at least one variant result")
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

func TestFeatureGatePromotionCapabilityRegressions_NonInstallGate(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NetworkSegmentation?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching feature gate detail")
	require.NotNil(t, fg.Promotion)

	regressions := fg.Promotion.CapabilityTestRegressions
	require.NotEmpty(t, regressions, "expected capability test regressions for NetworkSegmentation")

	activeCount := 0
	ignoredCount := 0
	for _, r := range regressions {
		assert.Less(t, r.WorkingPercentage, 92.0,
			"regression test %q should have working percentage below 92%%", r.TestName)
		if r.Ignored {
			ignoredCount++
			assert.NotEmpty(t, r.IgnoredReason, "ignored regression should have a reason")
			assert.Contains(t, r.TestName, "OCPFeatureGate:",
				"ignored regression should reference an OCPFeatureGate annotation")
		} else {
			activeCount++
		}
	}
	assert.Greater(t, activeCount, 0, "expected at least one active (non-ignored) regression")
	assert.Greater(t, ignoredCount, 0, "expected at least one ignored regression from unpromoted gate")
}

func TestFeatureGatePromotionCapabilityRegressions_InstallGate(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/AWSDualStackInstall?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching Install gate detail")
	require.NotNil(t, fg.Promotion)

	regressions := fg.Promotion.CapabilityTestRegressions
	require.NotEmpty(t, regressions, "expected capability test regressions for AWSDualStackInstall")

	for _, r := range regressions {
		assert.Less(t, r.WorkingPercentage, 92.0,
			"regression test %q should have working percentage below 92%%", r.TestName)
	}
}

func TestFeatureGatePromotionCapabilityRegressions_UnpromotedGateFiltered(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NetworkSegmentation?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching feature gate detail")
	require.NotNil(t, fg.Promotion)

	regressions := fg.Promotion.CapabilityTestRegressions

	for _, r := range regressions {
		if r.TestName == "[sig-network] [OCPFeatureGate:UnpromotedTestGate] should handle traffic correctly" {
			assert.True(t, r.Ignored, "test from unpromoted gate should be ignored")
			assert.Contains(t, r.IgnoredReason, "not yet promoted",
				"ignored reason should mention the gate is not yet promoted")
			return
		}
	}
	t.Fatal("expected to find regression for [OCPFeatureGate:UnpromotedTestGate] test")
}

func TestFeatureGatePromotionCapabilityRegressions_BlocksPromotion(t *testing.T) {
	var fg api.FeatureGate
	err := util.SippyGet("/api/feature_gates/NetworkSegmentation?release="+util.Release, &fg)
	require.NoError(t, err, "error fetching feature gate detail")
	require.NotNil(t, fg.Promotion)

	assert.False(t, fg.Promotion.Sufficient,
		"promotion should be insufficient when capability regressions exist")

	foundJobTestError := false
	for _, e := range fg.Promotion.Errors {
		if strings.Contains(e, "pass rate below 92%") {
			foundJobTestError = true
		}
	}
	assert.True(t, foundJobTestError,
		"errors should include a message about tests below 92%% pass rate, got: %v", fg.Promotion.Errors)
}
