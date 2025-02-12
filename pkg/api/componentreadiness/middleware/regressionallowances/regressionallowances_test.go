package regressionallowances

import (
	"encoding/json"
	"reflect"
	"testing"

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
				JiraComponent:      "foo",
				TestID:             test1ID,
				TestName:           "test1",
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
	test1Variants := []string{"Arch:amd64", "Platform:aws"}
	test1Key := crtype.TestWithVariantsKey{
		TestID:   test1ID,
		Variants: variants,
	}
	test1KeyBytes, err := json.Marshal(test1Key)
	test1KeyStr := string(test1KeyBytes)
	assert.NoError(t, err)

	test2ID := "test2ID"
	test2Key := crtype.TestWithVariantsKey{
		TestID:   test2ID,
		Variants: variants,
	}
	test2KeyBytes, err := json.Marshal(test2Key)
	test2KeyStr := string(test2KeyBytes)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		reqOpts        crtype.RequestOptions
		baseStatus     map[string]crtype.TestStatus
		expectedStatus map[string]crtype.TestStatus
	}{
		{
			name:    "swap base stats using regression allowance",
			reqOpts: reqOpts419,
			baseStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 75, 0, nil),
			},
			expectedStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 100, 0, nil),
			},
		},
		{
			name:    "do not swap base stats if no regression allowance",
			reqOpts: reqOpts419,
			baseStatus: map[string]crtype.TestStatus{
				test2KeyStr: buildTestStatus("test2", test1Variants, 100, 75, 0, nil),
			},
			expectedStatus: map[string]crtype.TestStatus{
				test2KeyStr: buildTestStatus("test2", test1Variants, 100, 75, 0, nil),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rfb := NewRegressionAllowancesMiddleware(test.reqOpts)
			rfb.regressionGetterFunc = regressionGetter
			baseStatus, _, err := rfb.Transform(test.baseStatus, map[string]crtype.TestStatus{})
			assert.NoError(t, err)
			assert.Equal(t, test.expectedStatus, baseStatus)
		})
	}
}

func buildTestStatus(testName string, variants []string, total, success, flake int, release *crtype.Release) crtype.TestStatus {
	return crtype.TestStatus{
		TestName:     testName,
		TestSuite:    "conformance",
		Component:    "foo",
		Capabilities: nil,
		Variants:     variants,
		TotalCount:   total,
		SuccessCount: success,
		FlakeCount:   flake,
		Release:      release,
	}
}
