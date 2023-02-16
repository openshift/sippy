package api

import (
	"fmt"
	"testing"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunJobAnalysis(t *testing.T) {

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
		{
			name: "max test risk level low",
			testPassRates: []apitype.Test{
				{
					Name:                  "test1",
					CurrentPassPercentage: 21.0,
				},
				{
					Name:                  "test2",
					CurrentPassPercentage: 5.0,
				},
				{
					Name:                  "test3",
					CurrentPassPercentage: 60.0,
				},
			},
			expectedTestRisks: map[string]apitype.RiskLevel{
				"test1": apitype.FailureRiskLevelLow,
				"test2": apitype.FailureRiskLevelLow,
				"test3": apitype.FailureRiskLevelLow,
			},
			expectedOverallRisk: apitype.FailureRiskLevelLow,
		},
		{
			name: "max test risk level medium",
			testPassRates: []apitype.Test{
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
				"test4": apitype.FailureRiskLevelUnknown,
				"test3": apitype.FailureRiskLevelMedium,
			},
			expectedOverallRisk: apitype.FailureRiskLevelMedium,
		},
		{
			name: "max test risk level unknown",
			testPassRates: []apitype.Test{
				{
					Name:                  "test3",
					CurrentPassPercentage: 50.0,
				},
				{
					Name:                  "test4",
					CurrentPassPercentage: -1, // hack to tell the setup to not return results for this test
				},
			},
			expectedTestRisks: map[string]apitype.RiskLevel{
				"test4": apitype.FailureRiskLevelUnknown,
				"test3": apitype.FailureRiskLevelLow,
			},
			expectedOverallRisk: apitype.FailureRiskLevelUnknown,
		},
		{
			name:                "max test risk level none",
			testPassRates:       []apitype.Test{},
			expectedTestRisks:   map[string]apitype.RiskLevel{},
			expectedOverallRisk: apitype.FailureRiskLevelNone,
		},
		{
			name: "max test risk level high with mass failures",
			testPassRates: func() []apitype.Test {
				fts := []apitype.Test{}
				// One more than allowed. All low risk failures, but because so many we want to see high
				// risk on the job.
				for i := 0; i < 21; i++ {
					fts = append(fts, apitype.Test{Name: fmt.Sprintf("test%d", i), CurrentPassPercentage: 2.0})
				}
				return fts
			}(),
			// We do not expect individual test analysis on this many failures:
			expectedTestRisks:   map[string]apitype.RiskLevel{},
			expectedOverallRisk: apitype.FailureRiskLevelHigh,
		},
	}
	for _, tc := range tests {

		t.Run(tc.name, func(t *testing.T) {
			fakeProwJobRun := buildFakeProwJobRun()
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
						tpr.CurrentRuns = 100
						return &tpr, nil
					}
				}
				return nil, nil
			}

			result, err := runJobRunAnalysis(fakeProwJobRun, "4.12", 5, 5, log.WithField("jobRunID", "test"), testResultsLookupFunc)
			require.NoError(t, err)
			assert.Equal(t, len(tc.expectedTestRisks), len(result.Tests))
			for testName, expectedRisk := range tc.expectedTestRisks {
				actualTestRisk := getTestRisk(result, testName)
				if !assert.NotNil(t, actualTestRisk, "no test risk for test: %s", testName) {
					continue
				}
				assert.Equal(t, expectedRisk, actualTestRisk.Risk.Level, "unexpected risk level for test: %s", testName)
			}

			assert.Equal(t, tc.expectedOverallRisk, result.OverallRisk.Level, "unexpected overall risk for test: %s", tc.name)

		})
	}
}

func buildFakeProwJobRun() *models.ProwJobRun {
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
	return fakeProwJobRun
}

func getTestRisk(result apitype.ProwJobRunRiskAnalysis, testName string) *apitype.ProwJobRunTestRiskAnalysis {
	for _, ta := range result.Tests {
		if ta.Name == testName {
			return &ta
		}
	}
	return nil

}
