package fisherexact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware/analysis"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

// baseStats constructs a *testdetails.ReleaseStats with the given pass/fail counts.
func baseStats(success, failure int) *testdetails.ReleaseStats {
	return &testdetails.ReleaseStats{
		Stats: crtest.NewTestStats(success, failure, 0, false),
	}
}

// sampleStats constructs a testdetails.ReleaseStats with the given pass/fail counts.
func sampleStats(success, failure int) testdetails.ReleaseStats {
	return testdetails.ReleaseStats{
		Stats: crtest.NewTestStats(success, failure, 0, false),
	}
}

func TestFisherExactAnalyze(t *testing.T) {
	tests := []struct {
		name                string
		opts                reqopts.Advanced
		inputStats          *testdetails.TestComparison
		wantHandled         bool
		wantStatus          crtest.Status
		wantFisherZero      bool // true: assert FisherExact == 0.0
		wantExplanations    int  // expected len(Explanations); -1 means skip check
		explanationContains string
		wantConfidence      int // if non-zero, assert testStats.RequiredConfidence == this after call
	}{
		{
			name: "sample total=0, IgnoreMissing=false → MissingSample",
			opts: reqopts.Advanced{IgnoreMissing: false, Confidence: 95},
			inputStats: &testdetails.TestComparison{
				SampleStats: sampleStats(0, 0),
				BaseStats:   baseStats(900, 100),
			},
			wantHandled:         true,
			wantStatus:          crtest.MissingSample,
			wantFisherZero:      true,
			wantExplanations:    1,
			explanationContains: analysis.ExplanationNoRegression,
		},
		{
			name: "sample total=0, IgnoreMissing=true → NotSignificant",
			opts: reqopts.Advanced{IgnoreMissing: true, Confidence: 95},
			inputStats: &testdetails.TestComparison{
				SampleStats: sampleStats(0, 0),
				BaseStats:   baseStats(900, 100),
			},
			wantHandled:         true,
			wantStatus:          crtest.NotSignificant,
			wantFisherZero:      true,
			wantExplanations:    1,
			explanationContains: analysis.ExplanationNoRegression,
		},
		{
			name: "minimum-failure triage: failures below threshold → NotSignificant, no Fisher",
			// sample has 5 failures, MinimumFailure=10 → early return
			opts: reqopts.Advanced{Confidence: 95, MinimumFailure: 10, PityFactor: 5},
			inputStats: &testdetails.TestComparison{
				RequiredConfidence: 95,
				SampleStats:        sampleStats(95, 5),
				BaseStats:          baseStats(80, 20),
			},
			wantHandled:      true,
			wantStatus:       crtest.NotSignificant,
			wantFisherZero:   true,
			wantExplanations: 0,
		},
		{
			name: "improved performance, Fisher significant → SignificantImprovement",
			// base 50%, sample 90%: massive improvement, Fisher will be significant at 95%
			opts: reqopts.Advanced{Confidence: 95},
			inputStats: &testdetails.TestComparison{
				RequiredConfidence: 95,
				SampleStats:        sampleStats(900, 100),
				BaseStats:          baseStats(500, 500),
			},
			wantHandled:      true,
			wantStatus:       crtest.SignificantImprovement,
			wantFisherZero:   false,
			wantExplanations: 0,
		},
		{
			name: "degraded within pity → NotSignificant, no Fisher call",
			// base 95%, sample 93%: drop 2% < PityFactor 5% + PityAdjustment 1% = 6%
			opts: reqopts.Advanced{Confidence: 95, PityFactor: 5},
			inputStats: &testdetails.TestComparison{
				RequiredConfidence: 95,
				PityAdjustment:     1.0,
				SampleStats:        sampleStats(93, 7),
				BaseStats:          baseStats(95, 5),
			},
			wantHandled:      true,
			wantStatus:       crtest.NotSignificant,
			wantFisherZero:   false, // FisherExact set to the computed (zero) val at end
			wantExplanations: 0,
		},
		{
			name: "degraded beyond pity, drop 13% → SignificantRegression",
			// base 95%, sample 82%: drop 13% < 15%, Fisher significant at 95%
			opts: reqopts.Advanced{Confidence: 95, PityFactor: 5},
			inputStats: &testdetails.TestComparison{
				RequiredConfidence: 95,
				SampleStats:        sampleStats(820, 180),
				BaseStats:          baseStats(950, 50),
			},
			wantHandled:         true,
			wantStatus:          crtest.SignificantRegression,
			wantFisherZero:      false,
			wantExplanations:    3,
			explanationContains: "Significant",
		},
		{
			name: "degraded beyond pity, drop 25% → ExtremeRegression",
			// base 95%, sample 70%: drop 25% > 15%, Fisher significant at 95%
			opts: reqopts.Advanced{Confidence: 95, PityFactor: 5},
			inputStats: &testdetails.TestComparison{
				RequiredConfidence: 95,
				SampleStats:        sampleStats(700, 300),
				BaseStats:          baseStats(950, 50),
			},
			wantHandled:         true,
			wantStatus:          crtest.ExtremeRegression,
			wantFisherZero:      false,
			wantExplanations:    3,
			explanationContains: "Extreme",
		},
		{
			name: "RequiredConfidence=0 inherits opts.Confidence",
			// Use missing-sample to keep the outcome predictable; focus is the confidence field.
			opts: reqopts.Advanced{IgnoreMissing: true, Confidence: 97},
			inputStats: &testdetails.TestComparison{
				RequiredConfidence: 0,
				SampleStats:        sampleStats(0, 0),
			},
			wantHandled:      true,
			wantStatus:       crtest.NotSignificant,
			wantFisherZero:   true,
			wantExplanations: 1, // missing-sample path always appends ExplanationNoRegression
			wantConfidence:   97,
		},
		{
			name: "RequiredConfidence pre-set is not overwritten by opts.Confidence",
			opts: reqopts.Advanced{IgnoreMissing: true, Confidence: 95},
			inputStats: &testdetails.TestComparison{
				RequiredConfidence: 90,
				SampleStats:        sampleStats(0, 0),
			},
			wantHandled:      true,
			wantStatus:       crtest.NotSignificant,
			wantFisherZero:   true,
			wantExplanations: 1,
			wantConfidence:   90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := NewFisherExactMiddleware(reqopts.RequestOptions{
				AdvancedOption: tt.opts,
			})

			handled, err := mw.Analyze(crtest.Identification{}, tt.inputStats)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHandled, handled)
			assert.Equal(t, tt.wantStatus, tt.inputStats.ReportStatus)

			require.NotNil(t, tt.inputStats.FisherExact)
			if tt.wantFisherZero {
				assert.Equal(t, 0.0, *tt.inputStats.FisherExact)
			} else {
				assert.GreaterOrEqual(t, *tt.inputStats.FisherExact, 0.0)
			}

			if tt.wantExplanations >= 0 {
				assert.Len(t, tt.inputStats.Explanations, tt.wantExplanations)
			}
			if tt.explanationContains != "" {
				assert.True(t, func() bool {
					for _, e := range tt.inputStats.Explanations {
						if len(e) >= len(tt.explanationContains) {
							for i := 0; i <= len(e)-len(tt.explanationContains); i++ {
								if e[i:i+len(tt.explanationContains)] == tt.explanationContains {
									return true
								}
							}
						}
					}
					return false
				}(), "expected an explanation containing %q, got: %v", tt.explanationContains, tt.inputStats.Explanations)
			}
			if tt.wantConfidence != 0 {
				assert.Equal(t, tt.wantConfidence, tt.inputStats.RequiredConfidence)
			}
		})
	}
}
