package alltestspassrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/analysis"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

func TestAllTestsPassRateAnalyze(t *testing.T) {
	tests := []struct {
		name                   string
		passRateRequiredAll    int
		minimumFailure         int
		successCount           int
		failureCount           int
		expectHandled          bool
		expectRegressionStatus bool
	}{
		{
			name:                "PassRateRequiredAllTests=0 returns not handled, testStats unchanged",
			passRateRequiredAll: 0,
			successCount:        3,
			failureCount:        3,
			expectHandled:       false,
		},
		{
			name:                   "PassRateRequiredAllTests>0 with passing run returns handled and NotSignificant",
			passRateRequiredAll:    95,
			successCount:           10,
			failureCount:           0,
			expectHandled:          true,
			expectRegressionStatus: false,
		},
		{
			name:                   "PassRateRequiredAllTests>0 with failing run returns handled and regression",
			passRateRequiredAll:    95,
			minimumFailure:         1,
			successCount:           93,
			failureCount:           7,
			expectHandled:          true,
			expectRegressionStatus: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := NewAllTestsPassRateMiddleware(reqopts.RequestOptions{
				AdvancedOption: reqopts.Advanced{
					PassRateRequiredAllTests: tt.passRateRequiredAll,
					MinimumFailure:           tt.minimumFailure,
				},
			})

			testStats := &testdetails.TestComparison{
				SampleStats: testdetails.ReleaseStats{
					Stats: crtest.NewTestStats(tt.successCount, tt.failureCount, 0, false),
				},
			}

			handled, err := mw.Analyze(crtest.Identification{}, testStats)
			require.NoError(t, err)
			assert.Equal(t, tt.expectHandled, handled)

			if !tt.expectHandled {
				// testStats must not be modified
				assert.Equal(t, crtest.Status(0), testStats.ReportStatus)
				assert.Empty(t, testStats.Explanations)
				return
			}

			// analysis.BuildPassRateTestStats must have been applied
			if tt.expectRegressionStatus {
				assert.Less(t, int(testStats.ReportStatus), 0, "expected a regression status (negative)")
				assert.Equal(t, crtest.PassRate, testStats.Comparison)
			} else {
				assert.Equal(t, crtest.NotSignificant, testStats.ReportStatus)
				assert.Contains(t, testStats.Explanations, analysis.ExplanationNoRegression)
			}
		})
	}
}
