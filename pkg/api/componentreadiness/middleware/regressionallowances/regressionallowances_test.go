package regressionallowances

import (
	"reflect"
	"testing"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/regressionallowances"
	"github.com/stretchr/testify/assert"
)

func Test_Transform(t *testing.T) {
	test1ID := "test1ID"
	variants := map[string]string{
		"Arch":     "amd64",
		"Platform": "aws",
	}
	regressionGetter := func(releaseString string, variant crtype.ColumnIdentification, testID string) *regressionallowances.IntentionalRegression {
		if releaseString == "4.18" && reflect.DeepEqual(variant.Variants, variants) && testID == test1ID {
			return &regressionallowances.IntentionalRegression{
				JiraComponent:      "",
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
		return &regressionallowances.IntentionalRegression{}
	}
	reqOpts419 := crtype.RequestOptions{
		BaseRelease: crtype.RequestReleaseOptions{
			Release: "4.18",
		},
		AdvancedOption: crtype.RequestAdvancedOptions{IncludeMultiReleaseAnalysis: true},
	}

	test1Key := crtype.ReportTestIdentification{
		RowIdentification: crtype.RowIdentification{
			Component:  "",
			Capability: "",
			TestName:   "test 1",
			TestSuite:  "",
			TestID:     test1ID,
		},
		ColumnIdentification: crtype.ColumnIdentification{
			Variants: variants,
		},
	}

	test2ID := "test2ID"
	test2Key := crtype.ReportTestIdentification{
		RowIdentification: crtype.RowIdentification{
			Component:  "",
			Capability: "",
			TestName:   "test 2",
			TestSuite:  "",
			TestID:     test2ID,
		},
		ColumnIdentification: crtype.ColumnIdentification{
			Variants: variants,
		},
	}

	tests := []struct {
		name           string
		testKey        crtype.ReportTestIdentification
		reqOpts        crtype.RequestOptions
		baseStatus     *crtype.ReportTestStats
		expectedStatus *crtype.ReportTestStats
	}{
		{
			name:           "swap base stats using regression allowance",
			reqOpts:        reqOpts419,
			testKey:        test1Key,
			baseStatus:     buildTestStatus(100, 75, 0, "4.18"),
			expectedStatus: buildTestStatus(100, 100, 0, "4.17"),
		},
		{
			name:           "do not swap base stats if no regression allowance",
			reqOpts:        reqOpts419,
			testKey:        test2Key,
			baseStatus:     buildTestStatus(100, 75, 0, "4.18"),
			expectedStatus: buildTestStatus(100, 75, 0, "4.18"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rfb := NewRegressionAllowancesMiddleware(test.reqOpts)
			rfb.regressionGetterFunc = regressionGetter
			err := rfb.PreAnalysis(test.testKey, test.baseStatus)
			assert.NoError(t, err)
			assert.Equal(t, *test.expectedStatus, *test.baseStatus)
		})
	}
}

//nolint:unparam
func buildTestStatus(total, success, flake int, baseRelease string) *crtype.ReportTestStats {
	fails := total - success - flake
	ts := &crtype.ReportTestStats{
		BaseStats: &crtype.TestDetailsReleaseStats{
			Release: baseRelease,
			TestDetailsTestStats: crtype.TestDetailsTestStats{
				FailureCount: fails,
				SuccessCount: success,
				FlakeCount:   flake,
				SuccessRate:  utils.CalculatePassRate(success, fails, flake, false),
			},
		},
	}
	return ts
}
