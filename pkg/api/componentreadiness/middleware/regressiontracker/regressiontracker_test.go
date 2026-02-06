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
				Release:  sampleRelease,
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
				Release:  sampleRelease,
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
				Release:  sampleRelease,
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
				Release:  sampleRelease,
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
				Release:  sampleRelease,
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

func TestRegressionTracker_PreAnalysis_Adjustments(t *testing.T) {
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

	tests := []struct {
		name                         string
		hasOpenRegression            bool
		expectedPityAdjustment       float64
		expectedMinFailureAdjustment int
		expectedRequiredConfidence   int
	}{
		{
			name:                         "no open regression - no adjustments",
			hasOpenRegression:            false,
			expectedPityAdjustment:       0,
			expectedMinFailureAdjustment: 0,
			expectedRequiredConfidence:   95, // default confidence
		},
		{
			name:                         "has open regression - adjustments applied",
			hasOpenRegression:            true,
			expectedPityAdjustment:       openRegressionPityAdjustment,            // -2
			expectedMinFailureAdjustment: openRegressionMinimumFailureAdjustment,  // -1
			expectedRequiredConfidence:   95 - openRegressionConfidenceAdjustment, // 90
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				log: logrus.New(),
			}

			// Set up test stats
			testStats := &testdetails.TestComparison{
				ReportStatus:             crtest.SignificantRegression,
				RequiredConfidence:       95,
				PityAdjustment:           0,
				MinimumFailureAdjustment: 0,
			}

			// Set up open regressions if needed
			if tt.hasOpenRegression {
				openRegression := &models.TestRegression{
					ID:       1,
					Release:  sampleRelease,
					TestID:   testKey.TestID,
					TestName: testKey.TestName,
					Variants: variantsStrSlice,
					Opened:   time.Now().UTC().Add(-5 * 24 * time.Hour),
					Closed:   sql.NullTime{Valid: false},
				}
				mw.openRegressions = []*models.TestRegression{openRegression}
				mw.hasLoadedRegressions = true
			} else {
				mw.openRegressions = []*models.TestRegression{}
				mw.hasLoadedRegressions = true
			}

			// Run PreAnalysis
			err := mw.PreAnalysis(testKey, testStats)
			require.NoError(t, err)

			// Verify adjustments
			assert.Equal(t, tt.expectedPityAdjustment, testStats.PityAdjustment,
				"PityAdjustment should match expected value")
			assert.Equal(t, tt.expectedMinFailureAdjustment, testStats.MinimumFailureAdjustment,
				"MinimumFailureAdjustment should match expected value")
			assert.Equal(t, tt.expectedRequiredConfidence, testStats.RequiredConfidence,
				"RequiredConfidence should match expected value")

			// Verify regression is set when there's an open regression
			if tt.hasOpenRegression {
				assert.NotNil(t, testStats.Regression, "Regression should be set when there's an open regression")
				assert.Equal(t, testKey.TestID, testStats.Regression.TestID, "Regression TestID should match")
			} else {
				assert.Nil(t, testStats.Regression, "Regression should be nil when there's no open regression")
			}
		})
	}
}

func TestRegressionTracker_PreAnalysis_RegressionMatching(t *testing.T) {
	baseRelease := "4.19"
	sampleRelease := "4.18"

	tests := []struct {
		name                string
		testKey             crtest.Identification
		openRegressions     []*models.TestRegression
		expectRegressionSet bool
		expectedTestID      string
	}{
		{
			name: "exact match - regression should be set",
			testKey: crtest.Identification{
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
			},
			openRegressions: []*models.TestRegression{
				{
					ID:          1,
					Release:     sampleRelease,
					BaseRelease: baseRelease,
					TestID:      "foobartest1",
					TestName:    "foobar test 1",
					Variants:    []string{"foo:bar"},
					Opened:      time.Now().UTC().Add(-5 * 24 * time.Hour),
					Closed:      sql.NullTime{Valid: false},
				},
			},
			expectRegressionSet: true,
			expectedTestID:      "foobartest1",
		},
		{
			name: "test ID mismatch - regression should not be set",
			testKey: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "foo",
					Capability: "bar",
					TestName:   "foobar test 1",
					TestSuite:  "foo",
					TestID:     "differenttest1",
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{
						"foo": "bar",
					},
				},
			},
			openRegressions: []*models.TestRegression{
				{
					ID:       1,
					Release:  sampleRelease,
					TestID:   "foobartest1",
					TestName: "foobar test 1",
					Variants: []string{"foo:bar"},
					Opened:   time.Now().UTC().Add(-5 * 24 * time.Hour),
					Closed:   sql.NullTime{Valid: false},
				},
			},
			expectRegressionSet: false,
		},
		{
			name: "variant mismatch - regression should not be set",
			testKey: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "foo",
					Capability: "bar",
					TestName:   "foobar test 1",
					TestSuite:  "foo",
					TestID:     "foobartest1",
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{
						"foo": "different",
					},
				},
			},
			openRegressions: []*models.TestRegression{
				{
					ID:       1,
					Release:  sampleRelease,
					TestID:   "foobartest1",
					TestName: "foobar test 1",
					Variants: []string{"foo:bar"},
					Opened:   time.Now().UTC().Add(-5 * 24 * time.Hour),
					Closed:   sql.NullTime{Valid: false},
				},
			},
			expectRegressionSet: false,
		},
		{
			name: "release mismatch - regression should not be set",
			testKey: crtest.Identification{
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
			},
			openRegressions: []*models.TestRegression{
				{
					ID:       1,
					Release:  sampleRelease,
					TestID:   "differenttest1", // Different test ID
					TestName: "different test",
					Variants: []string{"foo:bar"},
					Opened:   time.Now().UTC().Add(-5 * 24 * time.Hour),
					Closed:   sql.NullTime{Valid: false},
				},
				{
					ID:       2,
					Release:  "4.17", // Different release; FindOpenRegression matches by sampleRelease
					TestID:   "foobartest1",
					TestName: "foobar test 1",
					Variants: []string{"foo:bar"},
					Opened:   time.Now().UTC().Add(-5 * 24 * time.Hour),
					Closed:   sql.NullTime{Valid: false},
				},
			},
			expectRegressionSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				openRegressions:      tt.openRegressions,
				hasLoadedRegressions: true,
				log:                  logrus.New(),
			}

			testStats := &testdetails.TestComparison{
				ReportStatus:             crtest.SignificantRegression,
				RequiredConfidence:       95,
				PityAdjustment:           0,
				MinimumFailureAdjustment: 0,
			}

			err := mw.PreAnalysis(tt.testKey, testStats)
			require.NoError(t, err)

			if tt.expectRegressionSet {
				assert.NotNil(t, testStats.Regression, "Regression should be set")
				assert.Equal(t, tt.expectedTestID, testStats.Regression.TestID, "Regression TestID should match")
				// Verify adjustments are applied
				assert.Equal(t, float64(openRegressionPityAdjustment), testStats.PityAdjustment)
				assert.Equal(t, openRegressionMinimumFailureAdjustment, testStats.MinimumFailureAdjustment)
				assert.Equal(t, 95-openRegressionConfidenceAdjustment, testStats.RequiredConfidence)
			} else {
				assert.Nil(t, testStats.Regression, "Regression should not be set")
				// Verify no adjustments are applied
				assert.Equal(t, float64(0), testStats.PityAdjustment)
				assert.Equal(t, 0, testStats.MinimumFailureAdjustment)
				assert.Equal(t, 95, testStats.RequiredConfidence)
			}
		})
	}
}

func TestFindOpenRegression(t *testing.T) {
	sampleRelease := "4.22"
	baseRelease := "4.21"
	testID := "test-id-1"
	variants := map[string]string{"arch": "amd64"}

	tests := []struct {
		name            string
		regressions     []*models.TestRegression
		wantMatch       bool
		wantRelease     string
		wantBaseRelease string
	}{
		{
			name: "match when sample release, testID and variants match",
			regressions: []*models.TestRegression{
				{
					ID:          1,
					Release:     sampleRelease,
					BaseRelease: baseRelease,
					TestID:      testID,
					Variants:    []string{"arch:amd64"},
				},
			},
			wantMatch:       true,
			wantRelease:     sampleRelease,
			wantBaseRelease: baseRelease,
		},
		{
			name: "no match when sample release differs",
			regressions: []*models.TestRegression{
				{
					ID:          1,
					Release:     "4.20",
					BaseRelease: "4.19",
					TestID:      testID,
					Variants:    []string{"arch:amd64"},
				},
			},
			wantMatch: false,
		},
		{
			name: "no match when testID differs",
			regressions: []*models.TestRegression{
				{
					ID:          1,
					Release:     sampleRelease,
					BaseRelease: baseRelease,
					TestID:      "other-test",
					Variants:    []string{"arch:amd64"},
				},
			},
			wantMatch: false,
		},
		{
			name: "no match when variants differ",
			regressions: []*models.TestRegression{
				{
					ID:          1,
					Release:     sampleRelease,
					BaseRelease: baseRelease,
					TestID:      testID,
					Variants:    []string{"arch:arm64"},
				},
			},
			wantMatch: false,
		},
		{
			name: "returns first when multiple match",
			regressions: []*models.TestRegression{
				{
					ID:          2,
					Release:     sampleRelease,
					BaseRelease: baseRelease,
					TestID:      testID,
					Variants:    []string{"arch:amd64"},
				},
				{
					ID:          1,
					Release:     sampleRelease,
					BaseRelease: baseRelease,
					TestID:      testID,
					Variants:    []string{"arch:amd64"},
				},
			},
			wantMatch:       true,
			wantRelease:     sampleRelease,
			wantBaseRelease: baseRelease,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindOpenRegression(sampleRelease, testID, variants, tt.regressions)
			if !tt.wantMatch {
				assert.Nil(t, got, "expected no match")
				return
			}
			require.NotNil(t, got, "expected a match")
			assert.Equal(t, tt.wantRelease, got.Release)
			assert.Equal(t, tt.wantBaseRelease, got.BaseRelease)
			assert.Equal(t, testID, got.TestID)
		})
	}
}
