package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/test/e2e/util"
)

func TestReleasesAPI(t *testing.T) {
	var releases api.Releases
	err := util.SippyGet("/api/releases", &releases)
	require.NoError(t, err)

	t.Logf("found %d releases", len(releases.Releases))
	assert.Greater(t, len(releases.Releases), 0, "no releases returned")
}

func TestReleaseHealth(t *testing.T) {
	var health api.Health
	err := util.SippyGet("/api/health?release="+util.Release, &health)
	require.NoError(t, err)

	assert.Greater(t, health.Indicators["bootstrap"].CurrentRuns, 0, "no bootstrap runs")
	assert.Greater(t, health.Indicators["infrastructure"].CurrentRuns, 0, "no infrastructure runs")
	assert.Greater(t, health.Indicators["install"].CurrentRuns, 0, "no install runs")
	assert.Greater(t, health.Indicators["installConfig"].CurrentRuns, 0, "no installConfig runs")
	assert.Greater(t, health.Indicators["installOther"].CurrentRuns, 0, "no installOther runs")
	assert.Greater(t, health.Indicators["tests"].CurrentRuns, 0, "no tests runs")
	assert.Greater(t, health.Indicators["upgrade"].CurrentRuns, 0, "no upgrade runs")

	assert.False(t, health.LastUpdated.IsZero())
}
