package api

import (
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"

	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"

	"github.com/stretchr/testify/assert"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/models"
)

func TestTransformRelease(t *testing.T) {

	devStart420, _ := time.Parse(time.RFC3339, "2025-04-18T00:00:00.00Z")
	devStart419, _ := time.Parse(time.RFC3339, "2024-11-25T00:00:00.00Z")
	gaDate419, _ := time.Parse(time.RFC3339, "2025-05-09T00:00:00.00Z")

	tests := []struct {
		name            string
		releaseRow      sippyv1.ReleaseRow
		expectedRelease sippyv1.Release
	}{
		{
			name:            "release without devel start",
			releaseRow:      sippyv1.ReleaseRow{Release: "4.20", ReleaseStatus: bigquery.NullString{Valid: true, StringVal: "Development"}},
			expectedRelease: sippyv1.Release{Release: "4.20", Status: "Development"},
		},
		{
			name: "release with devel start",
			releaseRow: sippyv1.ReleaseRow{Release: "4.20", ReleaseStatus: bigquery.NullString{Valid: true, StringVal: "Development"}, DevelStartDate: civil.Date{
				Year:  2025,
				Month: 4,
				Day:   18,
			}},
			expectedRelease: sippyv1.Release{Release: "4.20", Status: "Development", DevelopmentStartDate: &devStart420},
		},
		{
			name: "release with ga date",
			releaseRow: sippyv1.ReleaseRow{Release: "4.19", ReleaseStatus: bigquery.NullString{Valid: true, StringVal: "Development"}, DevelStartDate: civil.Date{
				Year:  2024,
				Month: 11,
				Day:   25,
			}, GADate: bigquery.NullDate{
				Date: civil.Date{
					Year:  2025,
					Month: 5,
					Day:   9},
				Valid: true,
			}},
			expectedRelease: sippyv1.Release{Release: "4.19", Status: "Development", DevelopmentStartDate: &devStart419, GADate: &gaDate419},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			release := transformRelease(tc.releaseRow)
			assert.Equal(t, tc.expectedRelease.Release, release.Release, "unexpected release")
			assert.Equal(t, tc.expectedRelease.Status, release.Status, "unexpected status")
			assert.Equal(t, tc.expectedRelease.GADate, release.GADate, "unexpected status")
			assert.Equal(t, tc.expectedRelease.DevelopmentStartDate, release.DevelopmentStartDate, "unexpected devel start")
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
