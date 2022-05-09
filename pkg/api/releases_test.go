package api

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/assert"
)

func TestScanReleaseHealthForRHCOSVersionMisMatches(t *testing.T) {
	tests := []struct {
		name             string
		releaseHealth    []api.ReleaseHealthReport
		expectedWarnings []string
	}{
		{
			name: "single stream os version match",
			releaseHealth: []api.ReleaseHealthReport{
				buildFakeReleaseHealthReport("4.11", "411.85.202203212232-0"),
			},
		},
		{
			name: "single stream os version mismatch",
			releaseHealth: []api.ReleaseHealthReport{
				buildFakeReleaseHealthReport("4.11", "410.85.202203212232-0"),
			},
			expectedWarnings: []string{
				"OS version 410.85.202203212232-0 does not match OpenShift release 4.11",
			},
		},
		{
			name: "single stream os version parse error",
			releaseHealth: []api.ReleaseHealthReport{
				buildFakeReleaseHealthReport("4.11", "foobar"),
			},
			expectedWarnings: []string{
				"unable to parse OpenShift version from OS version foobar",
			},
		},
		{
			name: "multi stream os version mismatch",
			releaseHealth: []api.ReleaseHealthReport{
				buildFakeReleaseHealthReport("4.11", "411.85.202203212232-0"), // one good
				buildFakeReleaseHealthReport("4.11", "410.85.202203212232-0"),
				buildFakeReleaseHealthReport("4.11", "412.85.202203212232-0"),
				buildFakeReleaseHealthReport("4.11", "413.85.202203212232-0"),
			},
			expectedWarnings: []string{
				"OS version 410.85.202203212232-0 does not match OpenShift release 4.11",
				"OS version 412.85.202203212232-0 does not match OpenShift release 4.11",
				"OS version 413.85.202203212232-0 does not match OpenShift release 4.11",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			warnings := ScanReleaseHealthForRHCOSVersionMisMatches(tc.releaseHealth)
			assert.ElementsMatch(t, tc.expectedWarnings, warnings, "unexpected warnings")
		})
	}
}

func buildFakeReleaseHealthReport(release, osVersion string) api.ReleaseHealthReport {
	return api.ReleaseHealthReport{
		ReleaseTag: models.ReleaseTag{
			Release:          release,
			CurrentOSVersion: osVersion,
		},
	}
}
