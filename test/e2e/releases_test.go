package e2e

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleasesAPI(t *testing.T) {
	var releases api.Releases
	err := util.SippyRequest("/api/releases", &releases)
	require.NoError(t, err)

	t.Logf("found %d releases", len(releases.Releases))
	assert.Greater(t, len(releases.Releases), 0, "no releases returned")
}

func TestReleaseHealth(t *testing.T) {
	var health api.Health
	err := util.SippyRequest("/api/health?release="+util.Release, &health)
	require.NoError(t, err)

	assert.Greater(t, health.Indicators["bootstrap"].Current.Runs, 0, "no bootstrap runs")
	assert.Greater(t, health.Indicators["infrastructure"].Current.Runs, 0, "no infrastructure runs")
	assert.Greater(t, health.Indicators["install"].Current.Runs, 0, "no install runs")
	assert.Greater(t, health.Indicators["installConfig"].Current.Runs, 0, "no installConfig runs")
	assert.Greater(t, health.Indicators["installOther"].Current.Runs, 0, "no installOther runs")
	assert.Greater(t, health.Indicators["tests"].Current.Runs, 0, "no tests runs")
	assert.Greater(t, health.Indicators["upgrade"].Current.Runs, 0, "no upgrade runs")

	assert.False(t, health.LastUpdated.IsZero())
}
