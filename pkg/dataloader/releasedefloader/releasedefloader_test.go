package releasedefloader

import (
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
)

func TestReleaseRowToDefinition(t *testing.T) {
	tests := []struct {
		name                 string
		row                  sippyv1.ReleaseRow
		expectedRelease      string
		expectedMajor        int
		expectedMinor        int
		expectedPatch        *int
		expectedPrevious     string
		expectedProduct      string
		expectedStatus       string
		expectedHasGA        bool
		expectedHasDevel     bool
		expectedCapabilities pq.StringArray
	}{
		{
			name: "fully populated release",
			row: sippyv1.ReleaseRow{
				Release:         "4.22",
				Major:           4,
				Minor:           22,
				PreviousRelease: bigquery.NullString{Valid: true, StringVal: "4.21"},
				Product:         bigquery.NullString{Valid: true, StringVal: "OCP"},
				ReleaseStatus:   bigquery.NullString{Valid: true, StringVal: "Full Support"},
				GADate:          bigquery.NullDate{Valid: true, Date: civil.Date{Year: 2026, Month: 6, Day: 9}},
				DevelStartDate:  civil.Date{Year: 2025, Month: 12, Day: 1},
				Capabilities:    []sippyv1.ReleaseCapability{"metrics", "componentReadiness"},
			},
			expectedRelease:      "4.22",
			expectedMajor:        4,
			expectedMinor:        22,
			expectedPrevious:     "4.21",
			expectedProduct:      "OCP",
			expectedStatus:       "Full Support",
			expectedHasGA:        true,
			expectedHasDevel:     true,
			expectedCapabilities: pq.StringArray{"componentReadiness", "metrics"},
		},
		{
			name: "in-development release with no GA date",
			row: sippyv1.ReleaseRow{
				Release:        "5.0",
				Major:          5,
				Minor:          0,
				Product:        bigquery.NullString{Valid: true, StringVal: "OCP"},
				ReleaseStatus:  bigquery.NullString{Valid: true, StringVal: "Development"},
				DevelStartDate: civil.Date{Year: 2026, Month: 1, Day: 15},
				Capabilities:   []sippyv1.ReleaseCapability{"componentReadiness"},
			},
			expectedRelease:      "5.0",
			expectedMajor:        5,
			expectedMinor:        0,
			expectedProduct:      "OCP",
			expectedStatus:       "Development",
			expectedHasGA:        false,
			expectedHasDevel:     true,
			expectedCapabilities: pq.StringArray{"componentReadiness"},
		},
		{
			name: "null optional fields",
			row: sippyv1.ReleaseRow{
				Release:         "automation",
				PreviousRelease: bigquery.NullString{Valid: false},
				Product:         bigquery.NullString{Valid: false},
				ReleaseStatus:   bigquery.NullString{Valid: false},
				GADate:          bigquery.NullDate{Valid: false},
			},
			expectedRelease:      "automation",
			expectedHasGA:        false,
			expectedHasDevel:     false,
			expectedCapabilities: pq.StringArray{},
		},
		{
			name: "release with patch version",
			row: sippyv1.ReleaseRow{
				Release: "4.22.1",
				Major:   4,
				Minor:   22,
				Patch:   bigquery.NullInt64{Valid: true, Int64: 1},
			},
			expectedRelease:      "4.22.1",
			expectedMajor:        4,
			expectedMinor:        22,
			expectedPatch:        intPtr(1),
			expectedHasDevel:     false,
			expectedCapabilities: pq.StringArray{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def := ReleaseRowToDefinition(tc.row)

			assert.Equal(t, tc.expectedRelease, def.Release)
			assert.Equal(t, tc.expectedMajor, def.Major)
			assert.Equal(t, tc.expectedMinor, def.Minor)
			assert.Equal(t, tc.expectedPatch, def.Patch)
			assert.Equal(t, tc.expectedPrevious, def.PreviousRelease)
			assert.Equal(t, tc.expectedProduct, def.Product)
			assert.Equal(t, tc.expectedStatus, def.Status)

			if tc.expectedHasGA {
				assert.NotNil(t, def.GADate)
				assert.Equal(t, time.UTC, def.GADate.Location())
			} else {
				assert.Nil(t, def.GADate)
			}

			if tc.expectedHasDevel {
				assert.NotNil(t, def.DevelopmentStartDate)
				assert.Equal(t, time.UTC, def.DevelopmentStartDate.Location())
			} else {
				assert.Nil(t, def.DevelopmentStartDate)
			}

			assert.Equal(t, tc.expectedCapabilities, def.Capabilities)
		})
	}
}

func intPtr(i int) *int {
	return &i
}
