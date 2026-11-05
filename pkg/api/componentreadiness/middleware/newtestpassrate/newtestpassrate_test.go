package newtestpassrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

func TestNewTestPassRateAnalyze(t *testing.T) {
	existingBase := &testdetails.ReleaseStats{
		Stats: crtest.NewTestStats(80, 20, 0, false),
	}

	tests := []struct {
		name             string
		passRateNewTests int
		passRateAllTests int
		baseStats        *testdetails.ReleaseStats
		sampleSuccess    int
		sampleFailure    int
		wantHandled      bool
		wantStatus       crtest.Status
	}{
		{
			name:             "PassRateRequiredNewTests=0 → not handled",
			passRateNewTests: 0,
			baseStats:        nil,
			sampleSuccess:    5,
			sampleFailure:    5,
			wantHandled:      false,
			wantStatus:       0, // unchanged zero value
		},
		{
			name:             "BaseStats non-nil with runs → not handled",
			passRateNewTests: 95,
			baseStats:        existingBase,
			sampleSuccess:    8,
			sampleFailure:    2,
			wantHandled:      false,
			wantStatus:       0,
		},
		{
			name: "new test, passing sample, PassRateRequiredAllTests=0 → MissingBasis",
			// 10 successes, 0 failures → 100% pass rate > 95% required → NotSignificant from
			// BuildPassRateTestStats, then overridden to MissingBasis because PassRateRequiredAllTests==0.
			passRateNewTests: 95,
			passRateAllTests: 0,
			baseStats:        nil,
			sampleSuccess:    10,
			sampleFailure:    0,
			wantHandled:      true,
			wantStatus:       crtest.MissingBasis,
		},
		{
			name: "new test, passing sample, PassRateRequiredAllTests>0 → NotSignificant",
			// Same passing stats but PassRateRequiredAllTests is set, so MissingBasis override is skipped.
			passRateNewTests: 95,
			passRateAllTests: 90,
			baseStats:        nil,
			sampleSuccess:    10,
			sampleFailure:    0,
			wantHandled:      true,
			wantStatus:       crtest.NotSignificant,
		},
		{
			name: "new test, failing sample → regression status",
			// 3 successes, 4 failures: Total=7 (≥6), pass rate ≈42.8% < 95% required,
			// drop > allowed → ExtremeRegression from BuildPassRateTestStats.
			passRateNewTests: 95,
			passRateAllTests: 0,
			baseStats:        nil,
			sampleSuccess:    3,
			sampleFailure:    4,
			wantHandled:      true,
			wantStatus:       crtest.ExtremeRegression,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := NewNewTestPassRateMiddleware(reqopts.RequestOptions{
				AdvancedOption: reqopts.Advanced{
					PassRateRequiredNewTests: tt.passRateNewTests,
					PassRateRequiredAllTests: tt.passRateAllTests,
				},
			})

			testStats := &testdetails.TestComparison{
				BaseStats: tt.baseStats,
				SampleStats: testdetails.ReleaseStats{
					Stats: crtest.NewTestStats(tt.sampleSuccess, tt.sampleFailure, 0, false),
				},
			}

			handled, err := mw.Analyze(crtest.Identification{}, testStats)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHandled, handled)
			assert.Equal(t, tt.wantStatus, testStats.ReportStatus)
		})
	}
}
