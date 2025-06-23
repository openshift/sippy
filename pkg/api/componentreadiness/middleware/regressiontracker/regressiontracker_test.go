package regressiontracker

import (
	"database/sql"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegressionTracker_PostAnalysis(t *testing.T) {
	baseRelease := "4.19"
	sampleRelease := "4.18"
	testKey := crtype.ReportTestIdentification{
		RowIdentification: crtype.RowIdentification{
			Component:  "foo",
			Capability: "bar",
			TestName:   "foobar test 1",
			TestSuite:  "foo",
			TestID:     "foobartest1",
		},
		ColumnIdentification: crtype.ColumnIdentification{
			Variants: map[string]string{
				"foo": "bar",
			},
		},
	}
	variantsStrSlice := utils.VariantsMapToStringSlice(testKey.Variants)
	mw := RegressionTracker{
		reqOptions: reqopts.RequestOptions{
			BaseRelease: reqopts.RequestReleaseOptions{
				Release: baseRelease,
				Start:   time.Time{},
				End:     time.Time{},
			},
			SampleRelease: reqopts.RequestReleaseOptions{
				Release: sampleRelease,
				Start:   time.Time{},
				End:     time.Time{},
			},
			AdvancedOption: reqopts.RequestAdvancedOptions{
				Confidence: 95,
			},
		},
	}
	daysAgo5 := time.Now().UTC().Add(-5 * 24 * time.Hour)
	daysAgo4 := time.Now().UTC().Add(-4 * 24 * time.Hour)
	daysAgo3 := time.Now().UTC().Add(-3 * 24 * time.Hour)
	daysAgo2 := time.Now().UTC().Add(-2 * 24 * time.Hour)
	tests := []struct {
		name                      string
		testStats                 crtype.ReportTestStats
		openRegression            models.TestRegression
		expectStatus              crtype.Status
		expectedExplanationsCount int
	}{
		{
			name: "triaged regression",
			testStats: crtype.ReportTestStats{
				ReportStatus: crtype.ExtremeRegression,
				Explanations: []string{},
				LastFailure:  &daysAgo4,
			},
			openRegression: models.TestRegression{
				ID:       0,
				View:     "",
				Release:  "",
				TestID:   testKey.TestID,
				TestName: testKey.TestName,
				Variants: variantsStrSlice,
				Opened:   daysAgo5,
				Closed: sql.NullTime{
					Time:  time.Time{},
					Valid: false,
				},
				Triages: []models.Triage{
					{
						ID:          42,
						CreatedAt:   daysAgo4,
						UpdatedAt:   daysAgo4,
						URL:         "https://example.com/foobar",
						Description: "foobar",
						Type:        "product",
						Resolved:    sql.NullTime{},
					},
				},
			},
			expectStatus:              crtype.ExtremeTriagedRegression,
			expectedExplanationsCount: 1,
		},
		{
			name: "triage resolved waiting to clear",
			testStats: crtype.ReportTestStats{
				ReportStatus: crtype.ExtremeRegression,
				Explanations: []string{},
				LastFailure:  &daysAgo4,
			},
			openRegression: models.TestRegression{
				ID:       0,
				View:     "",
				Release:  "",
				TestID:   testKey.TestID,
				TestName: testKey.TestName,
				Variants: variantsStrSlice,
				Opened:   daysAgo5,
				Closed: sql.NullTime{
					Time:  time.Time{},
					Valid: false,
				},
				Triages: []models.Triage{
					{
						ID:          42,
						CreatedAt:   daysAgo4,
						UpdatedAt:   daysAgo4,
						URL:         "https://example.com/foobar",
						Description: "foobar",
						Type:        "product",
						Resolved: sql.NullTime{
							Time:  daysAgo3,
							Valid: true,
						},
					},
				},
			},
			expectStatus:              crtype.FixedRegression,
			expectedExplanationsCount: 1,
		},
		{
			name: "triage resolved but has failed since",
			testStats: crtype.ReportTestStats{
				ReportStatus: crtype.ExtremeRegression,
				Explanations: []string{},
				LastFailure:  &daysAgo2,
			},
			openRegression: models.TestRegression{
				ID:       0,
				View:     "",
				Release:  "",
				TestID:   testKey.TestID,
				TestName: testKey.TestName,
				Variants: variantsStrSlice,
				Opened:   daysAgo5,
				Closed: sql.NullTime{
					Time:  time.Time{},
					Valid: false,
				},
				Triages: []models.Triage{
					{
						ID:          42,
						CreatedAt:   daysAgo4,
						UpdatedAt:   daysAgo4,
						URL:         "https://example.com/foobar",
						Description: "foobar",
						Type:        "product",
						Resolved: sql.NullTime{
							Time:  daysAgo3,
							Valid: true,
						},
					},
				},
			},
			expectStatus:              crtype.FailedFixedRegression,
			expectedExplanationsCount: 1,
		},
		{
			name: "triage resolved and has cleared entirely",
			testStats: crtype.ReportTestStats{
				ReportStatus: crtype.SignificantImprovement,
				Explanations: []string{},
				LastFailure:  nil,
			},
			openRegression: models.TestRegression{
				ID:       0,
				View:     "",
				Release:  "",
				TestID:   testKey.TestID,
				TestName: testKey.TestName,
				Variants: variantsStrSlice,
				Opened:   daysAgo5,
				Closed: sql.NullTime{
					Time:  time.Time{},
					Valid: false,
				},
				Triages: []models.Triage{
					{
						ID:          42,
						CreatedAt:   daysAgo4,
						UpdatedAt:   daysAgo4,
						URL:         "https://example.com/foobar",
						Description: "foobar",
						Type:        "product",
						Resolved: sql.NullTime{
							Time:  daysAgo3,
							Valid: true,
						},
					},
				},
			},
			expectStatus:              crtype.SignificantImprovement,
			expectedExplanationsCount: 0,
		},
		{
			name: "triage resolved no longer significant but failures since resolution time",
			testStats: crtype.ReportTestStats{
				ReportStatus: crtype.NotSignificant,
				Explanations: []string{},
				LastFailure:  &daysAgo2,
			},
			openRegression: models.TestRegression{
				ID:       0,
				View:     "",
				Release:  "",
				TestID:   testKey.TestID,
				TestName: testKey.TestName,
				Variants: variantsStrSlice,
				Opened:   daysAgo5,
				Closed: sql.NullTime{
					Time:  time.Time{},
					Valid: false,
				},
				Triages: []models.Triage{
					{
						ID:          42,
						CreatedAt:   daysAgo4,
						UpdatedAt:   daysAgo4,
						URL:         "https://example.com/foobar",
						Description: "foobar",
						Type:        "product",
						Resolved: sql.NullTime{
							Time:  daysAgo3,
							Valid: true,
						},
					},
				},
			},
			expectStatus:              crtype.NotSignificant,
			expectedExplanationsCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw.openRegressions = []*models.TestRegression{&tt.openRegression}
			mw.hasLoadedRegressions = true
			mw.log = logrus.New()
			err := mw.PostAnalysis(testKey, &tt.testStats)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedExplanationsCount, len(tt.testStats.Explanations), tt.testStats.Explanations)
			assert.Equal(t, tt.expectStatus, tt.testStats.ReportStatus)

		})
	}
}
