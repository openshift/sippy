package api

import (
	"testing"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunJobAnalysis(t *testing.T) {

	fakeProwJobRun := &models.ProwJobRun{
		ProwJob: models.ProwJob{
			Name:        "fake-prow-job",
			Release:     "4.12",
			Variants:    []string{"var1", "var2"},
			TestGridURL: "https://example.com/foo",
			Bugs: []models.Bug{
				{
					ID:              500,
					Key:             "OCPBUGS-500",
					Status:          "New",
					Summary:         "This isn't a real bug",
					AffectsVersions: []string{},
					FixVersions:     []string{},
				},
			},
		},
		ProwJobID:             1000000000,
		URL:                   "https://example.com/run/1000000000",
		Tests:                 []models.ProwJobRunTest{}, // will be populated in the test cases
		Failed:                true,
		InfrastructureFailure: false,
		Succeeded:             false,
		OverallResult:         "f",
	}

	tests := []struct {
		name          string
		testPassRates []apitype.Test

		expectedTestRisks   map[string]apitype.RiskLevel
		expectedOverallRisk apitype.RiskLevel
	}{
		{
			name: "max test risk level high",
			testPassRates: []apitype.Test{
				{
					Name:                  "test1",
					CurrentPassPercentage: 21.0,
				},
				{
					Name:                  "test2",
					CurrentPassPercentage: 99.0,
				},
				{
					Name:                  "test3",
					CurrentPassPercentage: 85.0,
				},
				{
					Name:                  "test4",
					CurrentPassPercentage: -1, // hack to tell the setup to not return results for this test
				},
			},
			expectedTestRisks: map[string]apitype.RiskLevel{
				"test1": apitype.FailureRiskLevelLow,
				"test2": apitype.FailureRiskLevelHigh,
				"test3": apitype.FailureRiskLevelMedium,
				"test4": apitype.FailureRiskLevelUnknown,
			},
			expectedOverallRisk: apitype.FailureRiskLevelHigh,
		},
	}
	for _, tc := range tests {

		// Assume to build out the failed tests as those we provided pass rates for.
		for _, t := range tc.testPassRates {
			fakeProwJobRun.Tests = append(fakeProwJobRun.Tests, models.ProwJobRunTest{
				Test:   models.Test{Name: t.Name},
				Suite:  models.Suite{Name: t.SuiteName},
				Status: 12,
			})
		}

		// Fake test results lookup func:
		testResultsLookupFunc := func(testName, release, suite string, variants []string) (*apitype.Test, error) {
			for _, tpr := range tc.testPassRates {
				if tpr.Name == testName && tpr.CurrentPassPercentage > 0 {
					return &tpr, nil
				}
			}
			return nil, nil
		}

		result, err := runJobRunAnalysis(fakeProwJobRun, "4.12", testResultsLookupFunc)
		require.NoError(t, err)
		for testName, expectedRisk := range tc.expectedTestRisks {
			actualTestRisk := getTestRisk(result, testName)
			if !assert.NotNil(t, actualTestRisk, "no test risk for test: %s", testName) {
				continue
			}
			assert.Equal(t, expectedRisk, actualTestRisk.Risk.Level, "unexpected risk level for test: %s", testName)
		}

		assert.Equal(t, tc.expectedOverallRisk, result.OverallRisk.Level)
	}
}

func getTestRisk(result apitype.ProwJobRunFailureAnalysis, testName string) *apitype.ProwJobRunTestFailureAnalysis {
	for _, ta := range result.Tests {
		if ta.Name == testName {
			return &ta
		}
	}
	return nil

}
