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
		name                  string
		jobNames              []string
		testVariantsPassRates []apitype.Test
		testJobNamePassRates  []apitype.Test

		includeVariantsAnalysis bool
		includeJobNamesAnalysis bool
		expectedTestRisks       map[string]apitype.RiskLevel
		expectedOverallRisk     apitype.RiskLevel
	}{
		{
			name:                    "max test risk level high",
			includeVariantsAnalysis: true,
			testVariantsPassRates: []apitype.Test{
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
			name:                    "max test risk level low jobnames",
			includeJobNamesAnalysis: true,
			jobNames:                []string{"jobname1"},
			testJobNamePassRates: []apitype.Test{
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
			name:                    "max test risk level medium jobnames fallback unknown",
			includeJobNamesAnalysis: true,
			includeVariantsAnalysis: true,
			jobNames:                []string{"jobname1"},
			testVariantsPassRates: []apitype.Test{
				{
					Name:                  "test3",
					CurrentPassPercentage: 100.0,
				},
				{
					Name:                  "test4",
					CurrentPassPercentage: 85, // hack to tell the setup to not return results for this test
				},
			},
			testJobNamePassRates: []apitype.Test{
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
				"test4": apitype.FailureRiskLevelMedium,
				"test3": apitype.FailureRiskLevelMedium,
			},
			expectedOverallRisk: apitype.FailureRiskLevelMedium,
		},
		{
			name:                    "max test risk level medium jobnames",
			includeJobNamesAnalysis: true,
			jobNames:                []string{"jobname1"},

			testJobNamePassRates: []apitype.Test{
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
			name:                    "max test risk level medium variants",
			includeJobNamesAnalysis: true,
			includeVariantsAnalysis: true,
			testJobNamePassRates: []apitype.Test{
				{
					Name:                  "test3",
					CurrentPassPercentage: 100.0,
				},
				{
					Name:                  "test4",
					CurrentPassPercentage: 85, // hack to tell the setup to not return results for this test
				},
			},
			testVariantsPassRates: []apitype.Test{
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
			name:                    "max test risk level unknown",
			includeVariantsAnalysis: true,
			testVariantsPassRates: []apitype.Test{
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
			name:                    "max test risk level none",
			includeVariantsAnalysis: true,
			testVariantsPassRates:   []apitype.Test{},
			expectedTestRisks:       map[string]apitype.RiskLevel{},
			expectedOverallRisk:     apitype.FailureRiskLevelNone,
		},
		{
			name:                    "max test risk level high with mass failures",
			includeVariantsAnalysis: true,
			testVariantsPassRates: func() []apitype.Test {
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

			var tests []apitype.Test

			if tc.testVariantsPassRates != nil {
				tests = tc.testVariantsPassRates
			} else {
				tests = tc.testJobNamePassRates
			}

			for _, t := range tests {
				fakeProwJobRun.Tests = append(fakeProwJobRun.Tests, models.ProwJobRunTest{
					Test:   models.Test{Name: t.Name},
					Suite:  models.Suite{Name: t.SuiteName},
					Status: 12,
				})
			}

			// Fake test results lookup func:
			var testResultsJobNamesLookupFunc testResultsByJobNameFunc
			if tc.includeJobNamesAnalysis {
				testResultsJobNamesLookupFunc = func(testName string, jobNames []string) (*apitype.Test, error) {
					for _, tpr := range tc.testJobNamePassRates {
						if tpr.Name == testName && tpr.CurrentPassPercentage > 0 {
							tpr.CurrentRuns = 100
							return &tpr, nil
						}
					}
					return nil, nil
				}
			}

			var testResultsVariantsLookupFunc testResultsByVariantsFunc
			if tc.includeVariantsAnalysis {
				testResultsVariantsLookupFunc = func(testName string, release, suite string, variants []string, jobNames []string) (*apitype.Test, error) {
					for _, tpr := range tc.testVariantsPassRates {
						if tpr.Name == testName && tpr.CurrentPassPercentage > 0 {
							tpr.CurrentRuns = 100
							return &tpr, nil
						}
					}
					return nil, nil
				}
			}

			result, err := runJobRunAnalysis(nil, fakeProwJobRun, "4.12", 5, 5, false, tc.jobNames, log.WithField("jobRunID", "test"), testResultsJobNamesLookupFunc, testResultsVariantsLookupFunc, false)

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

func TestSubSliceEqual(t *testing.T) {

	tests := []struct {
		name            string
		variants        []string
		testRunVariants []string

		expectedMatch bool
	}{
		{
			name:            "match azure",
			variants:        []string{"ovn", "ha"},
			testRunVariants: []string{"ha", "ovn", "azure"},
			expectedMatch:   true,
		},
		{
			name:            "match aws",
			variants:        []string{"ovn", "ha"},
			testRunVariants: []string{"ha", "ovn", "aws"},
			expectedMatch:   true,
		},
		{
			name:            "no match",
			variants:        []string{"ovn", "ha"},
			testRunVariants: []string{"ha", "sdn", "azure"},
			expectedMatch:   false,
		},
		{
			name:            "match",
			variants:        []string{"ovn", "ha"},
			testRunVariants: []string{"ha", "ovn", "azure"},
			expectedMatch:   true,
		},
		{
			name:            "smaller",
			variants:        []string{"ovn", "ha"},
			testRunVariants: []string{"ha"},
			expectedMatch:   false,
		},
		{
			name:          "missing test run",
			variants:      []string{"ovn", "ha"},
			expectedMatch: false,
		},
		{
			name:            "missing variants",
			testRunVariants: []string{"ha"},
			expectedMatch:   false,
		},
	}

	for _, tc := range tests {

		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedMatch, stringSubSlicesEqual(tc.variants, tc.testRunVariants), "%s did not match expected", tc.name)
		})
	}
}

func TestSelectRiskAnalysisResult(t *testing.T) {

	tests := []struct {
		name              string
		expectedResponse  []string
		expectedRiskLevel int
		jobNames          []string
		compareRelease    string
		variantsRiskLevel *apitype.Test
		jobNamesRiskLevel *apitype.Test
	}{
		{
			name:              "matching risk levels",
			variantsRiskLevel: &apitype.Test{CurrentPassPercentage: 50, CurrentRuns: 100},
			jobNamesRiskLevel: &apitype.Test{CurrentPassPercentage: 50, CurrentRuns: 100},
			expectedResponse:  []string{"This test has passed 50.00% of 100 runs on jobs [jobname-4.14 jobname-4.13] in the last 14 days."},
			expectedRiskLevel: apitype.FailureRiskLevelLow.Level,
			compareRelease:    "4.14",
			jobNames:          []string{"jobname-4.14", "jobname-4.13"},
		},
		{
			name:              "mismatch risk levels bias to jobNames",
			variantsRiskLevel: &apitype.Test{CurrentPassPercentage: 100, CurrentRuns: 100},
			jobNamesRiskLevel: &apitype.Test{CurrentPassPercentage: 50, CurrentRuns: 100},
			expectedResponse:  []string{"This test has passed 50.00% of 100 runs on jobs [jobname-4.14 jobname-4.13] in the last 14 days."},
			expectedRiskLevel: apitype.FailureRiskLevelLow.Level,
			compareRelease:    "4.14",
			jobNames:          []string{"jobname-4.14", "jobname-4.13"},
		},
		{
			name:              "mismatch risk levels bias to variants",
			variantsRiskLevel: &apitype.Test{CurrentPassPercentage: 100, CurrentRuns: 100, Variants: []string{"aws", "ovn"}},
			jobNamesRiskLevel: &apitype.Test{CurrentPassPercentage: 50, CurrentRuns: 0},
			expectedResponse:  []string{"This test has passed 100.00% of 100 runs on release 4.14 [aws ovn] in the last week."},
			expectedRiskLevel: apitype.FailureRiskLevelHigh.Level,
			compareRelease:    "4.14",
			jobNames:          []string{},
		},
		{
			name:              "no runs",
			variantsRiskLevel: &apitype.Test{CurrentPassPercentage: 100, CurrentRuns: 0, Variants: []string{"aws", "ovn"}},
			jobNamesRiskLevel: &apitype.Test{CurrentPassPercentage: 50, CurrentRuns: 0},
			expectedResponse:  []string{"Analysis was not performed for this test due to lack of current runs"},
			expectedRiskLevel: apitype.FailureRiskLevelUnknown.Level,
			compareRelease:    "4.14",
			jobNames:          []string{},
		},
		{
			name:              "job name risk level",
			jobNamesRiskLevel: &apitype.Test{CurrentPassPercentage: 85, CurrentRuns: 100},
			expectedResponse:  []string{"This test has passed 85.00% of 100 runs on jobs [jobname-4.14 jobname-4.13] in the last 14 days."},
			expectedRiskLevel: apitype.FailureRiskLevelMedium.Level,
			compareRelease:    "4.14",
			jobNames:          []string{"jobname-4.14", "jobname-4.13"},
		},
		{
			name:              "variants risk level",
			variantsRiskLevel: &apitype.Test{CurrentPassPercentage: 100, CurrentRuns: 100, Variants: []string{"aws", "ovn"}},
			expectedResponse:  []string{"This test has passed 100.00% of 100 runs on release 4.14 [aws ovn] in the last week."},
			expectedRiskLevel: apitype.FailureRiskLevelHigh.Level,
			compareRelease:    "4.14",
			jobNames:          []string{"jobname-4.14", "jobname-4.13"},
		},
	}

	for _, tc := range tests {

		t.Run(tc.name, func(t *testing.T) {
			analysis := selectRiskAnalysisResult(tc.jobNamesRiskLevel, tc.variantsRiskLevel, tc.jobNames, tc.compareRelease)
			assert.Equal(t, tc.expectedRiskLevel, analysis.Level.Level, "%s risk level did not match expected", tc.name)
			assert.Equal(t, tc.expectedResponse, analysis.Reasons, "%s response did not match expected", tc.name)
		})
	}
}
