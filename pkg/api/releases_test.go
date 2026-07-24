package api

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db/models"
)

func TestDefinitionToRelease(t *testing.T) {
	ga := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	devStart := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		def      models.ReleaseDefinition
		expected sippyv1.Release
	}{
		{
			name: "all fields populated",
			def: models.ReleaseDefinition{
				Release:              "4.22",
				PreviousRelease:      "4.21",
				GADate:               &ga,
				DevelopmentStartDate: &devStart,
				Product:              "OCP",
				Status:               "Full Support",
				Capabilities:         pq.StringArray{"componentReadiness", "metrics", "payloadTags"},
			},
			expected: sippyv1.Release{
				Release:              "4.22",
				PreviousRelease:      "4.21",
				GADate:               &ga,
				DevelopmentStartDate: &devStart,
				Product:              "OCP",
				Status:               "Full Support",
				Capabilities: map[sippyv1.ReleaseCapability]bool{
					"componentReadiness": true,
					"metrics":            true,
					"payloadTags":        true,
				},
			},
		},
		{
			name: "nil GA date (in development)",
			def: models.ReleaseDefinition{
				Release:         "5.0",
				PreviousRelease: "4.22",
				Product:         "OCP",
				Status:          "Development",
				Capabilities:    pq.StringArray{"componentReadiness"},
			},
			expected: sippyv1.Release{
				Release:         "5.0",
				PreviousRelease: "4.22",
				Product:         "OCP",
				Status:          "Development",
				Capabilities:    map[sippyv1.ReleaseCapability]bool{"componentReadiness": true},
			},
		},
		{
			name: "empty capabilities",
			def: models.ReleaseDefinition{
				Release:      "automation",
				Product:      "OCP",
				Capabilities: pq.StringArray{},
			},
			expected: sippyv1.Release{
				Release:      "automation",
				Product:      "OCP",
				Capabilities: map[sippyv1.ReleaseCapability]bool{},
			},
		},
		{
			name: "nil capabilities",
			def: models.ReleaseDefinition{
				Release:      "3.11",
				Product:      "OCP",
				Capabilities: nil,
			},
			expected: sippyv1.Release{
				Release:      "3.11",
				Product:      "OCP",
				Capabilities: map[sippyv1.ReleaseCapability]bool{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := DefinitionToRelease(tc.def)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetReleasesFromDB_NilDB(t *testing.T) {
	_, err := GetReleasesFromDB(t.Context(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no database connection")
}

func TestGetReleaseDatesFromDB_NilDB(t *testing.T) {
	_, err := GetReleaseDatesFromDB(t.Context(), nil, reqopts.RequestOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no database connection")
}

func buildFakeReleaseHealthReport(osVersion string) apitype.ReleaseHealthReport {
	return apitype.ReleaseHealthReport{
		ReleaseTag: models.ReleaseTag{
			Release:          "4.11",
			CurrentOSVersion: osVersion,
		},
	}
}
