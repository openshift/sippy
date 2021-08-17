package testreportconversion_test

import (
	"fmt"
	"testing"
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/testgridanalysis/testreportconversion"
	"github.com/openshift/sippy/pkg/util/sets"
)

// TestGrid job names
const (
	awsCiJobName      string = "periodic-ci-openshift-release-master-ci-4.9-e2e-aws"
	awsNightlyJobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws"

	azureCiJobName      string = "periodic-ci-openshift-release-master-ci-4.9-e2e-azure"
	azureNightlyJobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-azure"

	gcpCiJobName      string = "periodic-ci-openshift-release-master-ci-4.9-e2e-gcp"
	gcpNightlyJobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-gcp"
)

// The original test names from TestGrid
const (
	e2eAwsOriginalTestNameSpecificStage string = "operator.Run multi-stage test e2e-aws - e2e-aws-aws-specific-stage container test"
	e2eAwsOriginalTestNameIpiInstall    string = "operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test"
	e2eAwsOriginalTestNameE2ETest       string = "operator.Run multi-stage test e2e-aws - e2e-aws-openshift-e2e-test container test"

	e2eGcpOriginalTestNameSpecificStage string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-gcp-specific-stage container test"
	e2eGcpOriginalTestNameIpiInstall    string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-ipi-install container test"
	e2eGcpOriginalTestNameE2ETest       string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-openshift-e2e-test container test"

	e2eAzureOriginalTestNameSpecificStage string = "operator.Run multi-stage test e2e-azure - e2e-azure-azure-specific-stage container test"
	e2eAzureOriginalTestNameIpiInstall    string = "operator.Run multi-stage test e2e-azure - e2e-azure-ipi-install container test"
	e2eAzureOriginalTestNameE2ETest       string = "operator.Run multi-stage test e2e-azure - e2e-azure-openshift-e2e-test container test"
)

func TestPrepareTestReportWithStepMetrics(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(*testing.T, sippyprocessingv1.TestReport)
	}{
		{
			name: "ByJob",
			testFunc: func(t *testing.T, report sippyprocessingv1.TestReport) {
				// For each job name, we expect to find two runs, one successful, one failure.
				// We also expect to find a similar aggregation for the top-level multistage itself.
				//
				// It should be noted that a multistage job name can run across
				// multiple jobs with different names, as shown here. For this
				// aggregation, we only want to aggregate by job name.
				expectedByJobStepRegistryMetrics := map[string]sippyprocessingv1.StepRegistryMetrics{
					awsNightlyJobName: {
						MultistageName: "e2e-aws",
						Aggregated:     getStageResult("e2e-aws", "", 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"aws-specific-stage": getStageResult("aws-specific-stage", e2eAwsOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					awsCiJobName: {
						MultistageName: "e2e-aws",
						Aggregated:     getStageResult("e2e-aws", "", 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"aws-specific-stage": getStageResult("aws-specific-stage", e2eAwsOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					gcpNightlyJobName: {
						MultistageName: "e2e-gcp",
						Aggregated:     getStageResult("e2e-gcp", "", 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"gcp-specific-stage": getStageResult("gcp-specific-stage", e2eGcpOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					gcpCiJobName: {
						MultistageName: "e2e-gcp",
						Aggregated:     getStageResult("e2e-gcp", "", 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"gcp-specific-stage": getStageResult("gcp-specific-stage", e2eGcpOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					azureNightlyJobName: {
						MultistageName: "e2e-azure",
						Aggregated:     getStageResult("e2e-azure", "", 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"azure-specific-stage": getStageResult("azure-specific-stage", e2eAzureOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":          getStageResult("ipi-install", e2eAzureOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test":   getStageResult("openshift-e2e-test", e2eAzureOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					azureCiJobName: {
						MultistageName: "e2e-azure",
						Aggregated:     getStageResult("e2e-azure", "", 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"azure-specific-stage": getStageResult("azure-specific-stage", e2eAzureOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":          getStageResult("ipi-install", e2eAzureOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test":   getStageResult("openshift-e2e-test", e2eAzureOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
				}

				for _, job := range report.ByJob {
					t.Run(job.Name, func(t *testing.T) {
						assertStepRegistryMetricsEqual(t, job.StepRegistryMetrics, expectedByJobStepRegistryMetrics[job.Name])
						assertStepRegistryMetricsEqual(t, report.TopLevelStepRegistryMetrics.ByJobName[job.Name].StepRegistryMetrics, expectedByJobStepRegistryMetrics[job.Name])
					})
				}
			},
		},
		{
			name: "ByMultistageName",
			testFunc: func(t *testing.T, report sippyprocessingv1.TestReport) {
				expectedByMultistageName := map[string]sippyprocessingv1.StepRegistryMetrics{
					// These are aggregated by the multistage job name (e.g., e2e-aws, e2e-azure, e2e-gcp), regardless of job name.

					// It is worth noting that some jobs (most notably the
					// periodic-ci-openshift-release-master-ci-4.9-e2e-* and
					// periodic-ci-openshift-release-master-nightly-4.9-e2e-* series use
					// the same multistage jobs. Because of this, our top-level
					// aggregation should take that into account.

					// We expect to find four total runs (two successes, two failures),
					// with the top-level multistage results being aggregated similarly.
					"e2e-aws": sippyprocessingv1.StepRegistryMetrics{
						MultistageName: "e2e-aws",
						Aggregated:     getStageResult("e2e-aws", "", 2, 2, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"aws-specific-stage": getStageResult("aws-specific-stage", e2eAwsOriginalTestNameSpecificStage, 2, 2, 50),
							"ipi-install":        getStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 2, 2, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 2, 2, 50),
						},
					},
					"e2e-azure": sippyprocessingv1.StepRegistryMetrics{
						MultistageName: "e2e-azure",
						Aggregated:     getStageResult("e2e-azure", "", 2, 2, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"azure-specific-stage": getStageResult("azure-specific-stage", e2eAzureOriginalTestNameSpecificStage, 2, 2, 50),
							"ipi-install":          getStageResult("ipi-install", e2eAzureOriginalTestNameIpiInstall, 2, 2, 50),
							"openshift-e2e-test":   getStageResult("openshift-e2e-test", e2eAzureOriginalTestNameE2ETest, 2, 2, 50),
						},
					},
					"e2e-gcp": sippyprocessingv1.StepRegistryMetrics{
						MultistageName: "e2e-gcp",
						Aggregated:     getStageResult("e2e-gcp", "", 2, 2, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"gcp-specific-stage": getStageResult("gcp-specific-stage", e2eGcpOriginalTestNameSpecificStage, 2, 2, 50),
							"ipi-install":        getStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 2, 2, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 2, 2, 50),
						},
					},
				}

				for multistageName, expectedStageResult := range expectedByMultistageName {
					t.Run(multistageName, func(t *testing.T) {
						assertStepRegistryMetricsEqual(t, report.TopLevelStepRegistryMetrics.ByMultistageName[multistageName], expectedStageResult)
					})
				}
			},
		},
		{
			name: "ByStageName",
			testFunc: func(t *testing.T, report sippyprocessingv1.TestReport) {
				// These are aggregated by the individual stage names (e.g., openshift-e2e-test, ipi-install, etc.)
				//
				// These are specific to a given multistage job name. We expect to find
				// four total runs (two successes, two failures) with a similarly
				// incremented top-level aggregation.
				expectedByStageName := map[string]sippyprocessingv1.ByStageName{
					// These run four times; two successes and two failures.
					"aws-specific-stage": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("aws-specific-stage", "", 2, 2, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-aws": getStageResult("aws-specific-stage", e2eAwsOriginalTestNameSpecificStage, 2, 2, 50),
						},
					},
					"azure-specific-stage": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("azure-specific-stage", "", 2, 2, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-azure": getStageResult("azure-specific-stage", e2eAzureOriginalTestNameSpecificStage, 2, 2, 50),
						},
					},
					"gcp-specific-stage": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("gcp-specific-stage", "", 2, 2, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-gcp": getStageResult("gcp-specific-stage", e2eGcpOriginalTestNameSpecificStage, 2, 2, 50),
						},
					},
					// For multistage-agnostic tests, (openshift-e2e-test, ipi-install,
					// etc.), we expect to find two successes and two failures, with the
					// top-level being aggregated similarly.
					// These stages run multiple times:
					// Two successes, two failures = 4
					// 2 * len(["aws", "azure", "gcp"]) = 6
					"ipi-install": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("ipi-install", "", 6, 6, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-aws":   getStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 2, 2, 50),
							"e2e-azure": getStageResult("ipi-install", e2eAzureOriginalTestNameIpiInstall, 2, 2, 50),
							"e2e-gcp":   getStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 2, 2, 50),
						},
					},
					"openshift-e2e-test": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("openshift-e2e-test", "", 6, 6, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-aws":   getStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 2, 2, 50),
							"e2e-azure": getStageResult("openshift-e2e-test", e2eAzureOriginalTestNameE2ETest, 2, 2, 50),
							"e2e-gcp":   getStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 2, 2, 50),
						},
					},
				}

				for stageName, byStageResult := range expectedByStageName {
					t.Run(stageName, func(t *testing.T) {
						assertByStageNameEqual(t, report.TopLevelStepRegistryMetrics.ByStageName[stageName], byStageResult)
					})
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			report := testreportconversion.PrepareTestReport(
				"4.9",
				getRawData(),
				testidentification.NewOpenshiftVariantManager(),
				buganalysis.NewNoOpBugCache(),
				"4.9",
				0,
				0.95,
				0,
				[]string{},
				time.Now(),
				0,
			)

			testCase.testFunc(t, report)
		})
	}
}

// Assertions
func assertByStageNameEqual(t *testing.T, have, want sippyprocessingv1.ByStageName) {
	t.Helper()

	assertKeysEqual(t, have.ByMultistageName, want.ByMultistageName)

	assertStageResultsEqual(t, have.Aggregated, want.Aggregated)

	for stageName, stageResult := range have.ByMultistageName {
		if _, ok := have.ByMultistageName[stageName]; !ok {
			t.Errorf("expected to find stageresult with name %s", stageName)
		}

		assertStageResultsEqual(t, stageResult, want.ByMultistageName[stageName])
	}
}

func assertStepRegistryMetricsEqual(t *testing.T, have, want sippyprocessingv1.StepRegistryMetrics) {
	t.Helper()

	assertKeysEqual(t, have.StageResults, want.StageResults)

	if have.MultistageName != want.MultistageName {
		t.Errorf("expected multistage name to be %s, got: %s", want.MultistageName, have.MultistageName)
	}

	assertStageResultsEqual(t, have.Aggregated, want.Aggregated)

	for stageName, stageResult := range want.StageResults {
		if _, ok := have.StageResults[stageName]; !ok {
			t.Errorf("expected to have a stageresult with name: %s", stageResult.Name)
		}

		assertStageResultsEqual(t, have.StageResults[stageName], want.StageResults[stageName])
	}
}

func assertStageResultsEqual(t *testing.T, have, want sippyprocessingv1.StageResult) {
	t.Helper()

	if have.Name != want.Name {
		t.Errorf("expected stage result to have name %s, got: %s", want.Name, have.Name)
	}

	if have.Successes != want.Successes {
		t.Errorf("expected stage result %s to have %d successes, got: %d", have.Name, want.Successes, have.Successes)
	}

	if have.Failures != want.Failures {
		t.Errorf("expected stage result %s to have %d failures, got: %d", have.Name, want.Failures, have.Failures)
	}

	// TODO: Determine if we should allow step registry metrics to be flaky.
	if have.Flakes != want.Flakes {
		t.Errorf("expected stage result %s to have %d flakes, got: %d", have.Name, want.Flakes, have.Flakes)
	}

	if have.PassPercentage != want.PassPercentage {
		t.Errorf("expected stage result %s to have %0.2f pass percentage, got: %0.2f", have.Name, want.PassPercentage, have.PassPercentage)
	}

	if have.OriginalTestName != want.OriginalTestName {
		t.Errorf("expected stage result %s to have original test name %s, got: %s", have.Name, want.OriginalTestName, have.OriginalTestName)
	}

	if have.Runs != want.Runs {
		t.Errorf("expected stage result %s to have %d runs, got: %d", have.Name, want.Runs, have.Runs)
	}

	haveCount := have.Successes + have.Failures + have.Flakes
	wantCount := want.Successes + want.Failures + want.Flakes

	if haveCount != wantCount {
		t.Errorf("expected stage result %s to have a job run count of %d, got: %d", have.Name, wantCount, haveCount)
	}
}

func assertKeysEqual(t *testing.T, have, want interface{}) {
	haveSet := sets.StringKeySet(have)
	wantSet := sets.StringKeySet(want)

	if !haveSet.Equal(wantSet) {
		t.Errorf("key mismatch, expected: %v, got: %v", wantSet.List(), haveSet.List())
	}
}

// Helpers
//nolint:unparam // The caller should specify passPercentage
func getStageResult(name, originalTestName string, successes, failures int, passPercentage float64) sippyprocessingv1.StageResult {
	// Allows us to pack this into a single line without affecting readability and expressiveness.
	return sippyprocessingv1.StageResult{
		TestResult: sippyprocessingv1.TestResult{
			Name:           name,
			Successes:      successes,
			Failures:       failures,
			PassPercentage: passPercentage,
		},
		OriginalTestName: originalTestName,
		Runs:             successes + failures,
	}
}

func getRawJobResult(jobName, multistageJobName string, prowURLCount int) testgridanalysisapi.RawJobResult {
	prowURL1 := fmt.Sprintf("https://prowurl%d", prowURLCount)
	prowURL2 := fmt.Sprintf("https://prowurl%d", prowURLCount+1)

	return testgridanalysisapi.RawJobResult{
		JobName:        jobName,
		TestGridJobURL: "https://testgrid",
		JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
			prowURL1: testgridanalysisapi.RawJobRunResult{
				Job:                    jobName,
				JobRunURL:              prowURL1,
				Succeeded:              true,
				StepRegistryItemStates: getStepRegistryItemStates(multistageJobName, testgridanalysisapi.Success),
			},
			prowURL2: testgridanalysisapi.RawJobRunResult{
				Job:                    jobName,
				JobRunURL:              prowURL2,
				Failed:                 true,
				StepRegistryItemStates: getStepRegistryItemStates(multistageJobName, testgridanalysisapi.Failure),
			},
		},
	}
}

func getRawData() testgridanalysisapi.RawData {
	return testgridanalysisapi.RawData{
		JobResults: map[string]testgridanalysisapi.RawJobResult{
			// AWS Jobs
			awsNightlyJobName: getRawJobResult(awsNightlyJobName, "e2e-aws", 1),
			awsCiJobName:      getRawJobResult(awsCiJobName, "e2e-aws", 2),
			// Azure Jobs
			azureNightlyJobName: getRawJobResult(azureNightlyJobName, "e2e-azure", 3),
			azureCiJobName:      getRawJobResult(azureCiJobName, "e2e-azure", 4),
			// GCP Jobs
			gcpNightlyJobName: getRawJobResult(gcpNightlyJobName, "e2e-gcp", 5),
			gcpCiJobName:      getRawJobResult(gcpCiJobName, "e2e-gcp", 6),
		},
	}
}

func getStepRegistryItemStates(multistageName, state string) testgridanalysisapi.StepRegistryItemStates {
	itemStates := map[string]testgridanalysisapi.StepRegistryItemStates{
		"e2e-aws": {
			MultistageName: "e2e-aws",
			States: []testgridanalysisapi.StageState{
				{
					Name:             "aws-specific-stage",
					State:            state,
					OriginalTestName: e2eAwsOriginalTestNameSpecificStage,
				},
				{
					Name:             "ipi-install",
					State:            state,
					OriginalTestName: e2eAwsOriginalTestNameIpiInstall,
				},
				{
					Name:             "openshift-e2e-test",
					State:            state,
					OriginalTestName: e2eAwsOriginalTestNameE2ETest,
				},
			},
		},
		"e2e-azure": {
			MultistageName: "e2e-azure",
			States: []testgridanalysisapi.StageState{
				{
					Name:             "azure-specific-stage",
					State:            state,
					OriginalTestName: e2eAzureOriginalTestNameSpecificStage,
				},
				{
					Name:             "ipi-install",
					State:            state,
					OriginalTestName: e2eAzureOriginalTestNameIpiInstall,
				},
				{
					Name:             "openshift-e2e-test",
					State:            state,
					OriginalTestName: e2eAzureOriginalTestNameE2ETest,
				},
			},
		},
		"e2e-gcp": {
			MultistageName: "e2e-gcp",
			States: []testgridanalysisapi.StageState{
				{
					Name:             "gcp-specific-stage",
					State:            state,
					OriginalTestName: e2eGcpOriginalTestNameSpecificStage,
				},
				{
					Name:             "ipi-install",
					State:            state,
					OriginalTestName: e2eGcpOriginalTestNameIpiInstall,
				},
				{
					Name:             "openshift-e2e-test",
					State:            state,
					OriginalTestName: e2eGcpOriginalTestNameE2ETest,
				},
			},
		},
	}

	return itemStates[multistageName]
}
