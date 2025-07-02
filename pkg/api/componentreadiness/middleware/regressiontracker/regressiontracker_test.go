package regressiontracker

import (
	"database/sql"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegressionTracker_PostAnalysis(t *testing.T) {
	baseRelease := "4.19"
	sampleRelease := "4.18"
	testKey := crtest.Identification{
		RowIdentification: crtest.RowIdentification{
			Component:  "foo",
			Capability: "bar",
			TestName:   "foobar test 1",
			TestSuite:  "foo",
			TestID:     "foobartest1",
		},
		ColumnIdentification: crtest.ColumnIdentification{
			Variants: map[string]string{
				"foo": "bar",
			},
		},
	}
	variantsStrSlice := utils.VariantsMapToStringSlice(testKey.Variants)
	mw := RegressionTracker{
		reqOptions: reqopts.RequestOptions{
			BaseRelease: reqopts.Release{
				Name:  baseRelease,
				Start: time.Time{},
				End:   time.Time{},
			},
			SampleRelease: reqopts.Release{
				Name:  sampleRelease,
				Start: time.Time{},
				End:   time.Time{},
			},
			AdvancedOption: reqopts.Advanced{
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
		testStats                 testdetails.TestComparison
		openRegression            models.TestRegression
		expectStatus              crtest.Status
		expectedExplanationsCount int
		expectedTriages           []models.Triage
	}{
		{
			name: "triaged regression",
			testStats: testdetails.TestComparison{
				ReportStatus: crtest.ExtremeRegression,
				Explanations: []string{},
				LastFailure:  &daysAgo4,
				Regression:   &models.TestRegression{},
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
			expectStatus:              crtest.ExtremeTriagedRegression,
			expectedExplanationsCount: 1,
			expectedTriages: []models.Triage{
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
		{
			name: "triage resolved waiting to clear",
			testStats: testdetails.TestComparison{
				ReportStatus: crtest.ExtremeRegression,
				Explanations: []string{},
				LastFailure:  &daysAgo4,
				Regression:   &models.TestRegression{},
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
			expectStatus:              crtest.FixedRegression,
			expectedExplanationsCount: 1,
			expectedTriages: []models.Triage{
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
		{
			name: "triage resolved but has failed since",
			testStats: testdetails.TestComparison{
				ReportStatus: crtest.ExtremeRegression,
				Explanations: []string{},
				LastFailure:  &daysAgo2,
				Regression:   &models.TestRegression{},
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
			expectStatus:              crtest.FailedFixedRegression,
			expectedExplanationsCount: 1,
			expectedTriages: []models.Triage{
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
		{
			name: "triage resolved and has cleared entirely",
			testStats: testdetails.TestComparison{
				ReportStatus: crtest.SignificantImprovement,
				Explanations: []string{},
				LastFailure:  nil,
				Regression:   &models.TestRegression{},
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
			expectStatus:              crtest.SignificantImprovement,
			expectedExplanationsCount: 0,
		},
		{
			name: "triage resolved no longer significant but failures since resolution time",
			testStats: testdetails.TestComparison{
				ReportStatus: crtest.NotSignificant,
				Explanations: []string{},
				LastFailure:  &daysAgo2,
				Regression:   &models.TestRegression{},
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
			expectStatus:              crtest.NotSignificant,
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
			assert.Equal(t, tt.expectedTriages, tt.testStats.Regression.Triages)

		})
	}
}
