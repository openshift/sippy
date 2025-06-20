package regressionallowances

import (
	"math"
	"reflect"
	"testing"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/regressionallowances"
	"github.com/stretchr/testify/assert"
)

func Test_PreAnalysis(t *testing.T) {
	test1ID := "test1ID"
	test2ID := "test2ID"
	variants := map[string]string{
		"Arch":     "amd64",
		"Platform": "aws",
	}
	regressionGetter := func(releaseString string, variant crtype.ColumnIdentification, testID string) *regressionallowances.IntentionalRegression {
		if releaseString == "4.18" && reflect.DeepEqual(variant.Variants, variants) && testID == test1ID {
			return &regressionallowances.IntentionalRegression{
				TestID:             test1ID,
				TestName:           "test 1",
				Variant:            variant,
				PreviousSuccesses:  100,
				PreviousFailures:   0,
				PreviousFlakes:     0,
				RegressedSuccesses: 75,
				RegressedFailures:  25,
				RegressedFlakes:    0,
			}
		}
		if releaseString == "4.19" && reflect.DeepEqual(variant.Variants, variants) && testID == test2ID {
			return &regressionallowances.IntentionalRegression{
				TestID:             test2ID,
				TestName:           "test 2",
				Variant:            variant,
				PreviousSuccesses:  0,
				PreviousFailures:   0,
				PreviousFlakes:     0,
				RegressedSuccesses: 90,
				RegressedFailures:  10,
				RegressedFlakes:    0,
			}
		}
		return nil
	}
	reqOpts419 := crtype.RequestOptions{
		SampleRelease: crtype.RequestReleaseOptions{Release: "4.19"},
		BaseRelease:   crtype.RequestReleaseOptions{Release: "4.18"},
		AdvancedOption: crtype.RequestAdvancedOptions{
			IncludeMultiReleaseAnalysis: true,
			PassRateRequiredNewTests:    95,
		},
	}
	reqOpts419Fallback := reqOpts419
	reqOpts419Fallback.TestIDOptions = []crtype.RequestTestIdentificationOptions{{BaseOverrideRelease: "4.17"}}
	reqOpts420Fallback := reqOpts419
	reqOpts420Fallback.SampleRelease.Release = "4.20"
	reqOpts420Fallback.BaseRelease.Release = "4.19"

	test1Key := crtype.ReportTestIdentification{
		RowIdentification: crtype.RowIdentification{
			TestName: "test 1",
			TestID:   test1ID,
		},
		ColumnIdentification: crtype.ColumnIdentification{
			Variants: variants,
		},
	}

	test2Key := crtype.ReportTestIdentification{
		RowIdentification: crtype.RowIdentification{
			TestName: "test 2",
			TestID:   test2ID,
		},
		ColumnIdentification: crtype.ColumnIdentification{
			Variants: variants,
		},
	}

	tests := []struct {
		name             string
		testKey          crtype.ReportTestIdentification
		reqOpts          crtype.RequestOptions
		regressionGetter func(releaseString string, variant crtype.ColumnIdentification, testID string) *regressionallowances.IntentionalRegression
		testStatus       *crtype.ReportTestStats
		expectedStatus   *crtype.ReportTestStats
	}{
		{
			name:             "swap base stats using regression allowance",
			reqOpts:          reqOpts419,
			testKey:          test1Key,
			regressionGetter: regressionGetter,
			testStatus:       buildTestStatus(100, 75, 0, "4.18"),
			expectedStatus:   buildTestStatus(100, 100, 0, "4.17"),
		},
		{
			name:             "do not swap base stats for regression allowance if fallback is active",
			reqOpts:          reqOpts419Fallback,
			testKey:          test1Key,
			regressionGetter: regressionGetter,
			testStatus:       buildTestStatus(100, 75, 0, "4.18"),
			expectedStatus:   buildTestStatus(100, 75, 0, "4.18"),
		},
		{
			name:             "also do not swap base stats with regression allowance with no history",
			reqOpts:          reqOpts420Fallback,
			testKey:          test2Key,
			regressionGetter: regressionGetter,
			testStatus:       buildTestStatus(100, 85, 0, "4.19"),
			expectedStatus:   buildTestStatus(100, 85, 0, "4.19"),
		},
		{
			name:             "do not swap base stats if no regression allowance",
			reqOpts:          reqOpts419,
			testKey:          test2Key,
			regressionGetter: regressionGetter,
			testStatus:       buildTestStatus(100, 75, 0, "4.18"),
			expectedStatus:   buildTestStatus(100, 75, 0, "4.18"),
		},
		{
			name:             "sample stats with regression allowance against basis should adjust pity",
			reqOpts:          reqOpts419,
			testKey:          test2Key, // has a regression allowance for 90% pass rate in 4.19
			regressionGetter: regressionGetter,
			testStatus:       buildTestStatus2(100, 99, 1, "4.18", "4.19", 20, 0, 0),
			expectedStatus:   buildTestStatus2(100, 99, 1, "4.18", "4.19", 20, 10, 0),
		},
		{
			name:             "sample stats with regression allowance and no basis should adjust pass rate",
			reqOpts:          reqOpts419,
			testKey:          test2Key, // has a regression allowance for 90% pass rate in 4.19, 5 less than the 95% required
			regressionGetter: regressionGetter,
			// note that the base stats are used only to detect that there was no basis, and sample stats are not used at all in the adjustment
			testStatus:     buildTestStatus2(0, 0, 0, "4.18", "4.19", 0, 0, 0),
			expectedStatus: buildTestStatus2(0, 0, 0, "4.18", "4.19", 0, 0, -5),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rfb := NewRegressionAllowancesMiddleware(test.reqOpts)
			rfb.regressionGetterFunc = test.regressionGetter
			err := rfb.PreAnalysis(test.testKey, test.testStatus)
			assert.NoError(t, err)
			test.testStatus.Explanations = nil // ignore explanations generated in the test
			maskFLOPError(&test.expectedStatus.RequiredPassRateAdjustment, &test.testStatus.RequiredPassRateAdjustment)
			maskFLOPError(&test.expectedStatus.PityAdjustment, &test.testStatus.PityAdjustment)
			assert.Equal(t, *test.expectedStatus, *test.testStatus)
		})
	}
}

func maskFLOPError(f1, f2 *float64) {
	if math.Abs(*f1-*f2) < 0.0000001 {
		*f2 = *f1
	}
}

func buildTestStatus(total, success, flake int, baseRelease string) *crtype.ReportTestStats {
	fails := total - success - flake
	ts := &crtype.ReportTestStats{
		BaseStats: &crtype.TestDetailsReleaseStats{
			Release: baseRelease,
			TestDetailsTestStats: crtype.TestDetailsTestStats{
				FailureCount: fails,
				SuccessCount: success,
				FlakeCount:   flake,
				SuccessRate:  crtype.CalculatePassRate(success, fails, flake, false),
			},
		},
	}
	return ts
}

//nolint:unparam
func buildTestStatus2(total, success, flake int, baseRelease, sampleRelease string, regressed int, pityAdjust, passRateAdjust float64) *crtype.ReportTestStats {
	fails := total - success - flake
	ts := buildTestStatus(total, success, flake, baseRelease) // set up the base stats as before

	fails += regressed // set up sample stats as base with regressed included
	success -= regressed
	ts.SampleStats = crtype.TestDetailsReleaseStats{
		Release: sampleRelease,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			FailureCount: fails,
			SuccessCount: success,
			FlakeCount:   flake,
			SuccessRate:  crtype.CalculatePassRate(success, fails, flake, false),
		},
	}
	ts.PityAdjustment = pityAdjust
	ts.RequiredPassRateAdjustment = passRateAdjust
	return ts
}
