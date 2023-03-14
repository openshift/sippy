package api

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/models"
)

func TestScanReleaseHealthForRHCOSVersionMisMatches(t *testing.T) {
	tests := []struct {
		name             string
		releaseHealth    []apitype.ReleaseHealthReport
		expectedWarnings []string
	}{
		{
			name: "single stream os version match",
			releaseHealth: []apitype.ReleaseHealthReport{
				buildFakeReleaseHealthReport("411.85.202203212232-0"),
			},
		},
		{
			name: "single stream os version mismatch",
			releaseHealth: []apitype.ReleaseHealthReport{
				buildFakeReleaseHealthReport("410.85.202203212232-0"),
			},
			expectedWarnings: []string{
				"OS version 410.85.202203212232-0 does not match OpenShift release 4.11",
			},
		},
		{
			name: "single stream os version parse error",
			releaseHealth: []apitype.ReleaseHealthReport{
				buildFakeReleaseHealthReport("foobar"),
			},
			expectedWarnings: []string{
				"unable to parse OpenShift version from OS version foobar",
			},
		},
		{
			name: "multi stream os version mismatch",
			releaseHealth: []apitype.ReleaseHealthReport{
				buildFakeReleaseHealthReport("411.85.202203212232-0"), // one good
				buildFakeReleaseHealthReport("410.85.202203212232-0"),
				buildFakeReleaseHealthReport("412.85.202203212232-0"),
				buildFakeReleaseHealthReport("413.85.202203212232-0"),
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

func buildFakeReleaseHealthReport(osVersion string) apitype.ReleaseHealthReport {
	return apitype.ReleaseHealthReport{
		ReleaseTag: models.ReleaseTag{
			Release:          "4.11",
			CurrentOSVersion: osVersion,
		},
	}
}
