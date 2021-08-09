package testreportconversion_test

import (
	"testing"
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/testgridanalysis/testreportconversion"
)

// TestGrid job names
const (
	awsJobName   string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws"
	azureJobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-azure"
	gcpJobName   string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-gcp"
)

// The original test names from TestGrid
const (
	e2eAwsStageOriginalTestName         string = "operator.Run multi-stage test e2e-aws"
	e2eAwsOriginalTestNameSpecificStage string = "operator.Run multi-stage test e2e-aws - e2e-aws-aws-specific-stage container test"
	e2eAwsOriginalTestNameIpiInstall    string = "operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test"
	e2eAwsOriginalTestNameE2ETest       string = "operator.Run multi-stage test e2e-aws - e2e-aws-openshift-e2e-test container test"

	e2eGcpStageOriginalTestName         string = "operator.Run multi-stage test e2e-gcp"
	e2eGcpOriginalTestNameSpecificStage string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-gcp-specific-stage container test"
	e2eGcpOriginalTestNameIpiInstall    string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-ipi-install container test"
	e2eGcpOriginalTestNameE2ETest       string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-openshift-e2e-test container test"

	e2eAzureStageOriginalTestName         string = "operator.Run multi-stage test e2e-azure"
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
				expectedByJobStepRegistryMetrics := map[string]sippyprocessingv1.StepRegistryMetrics{
					awsJobName: {
						MultistageName:   "e2e-aws",
						MultistageResult: getStageResult("e2e-aws", e2eAwsStageOriginalTestName, 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"aws-specific-stage": getStageResult("aws-specific-stage", e2eAwsOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					gcpJobName: {
						MultistageName:   "e2e-gcp",
						MultistageResult: getStageResult("e2e-gcp", e2eGcpStageOriginalTestName, 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"gcp-specific-stage": getStageResult("gcp-specific-stage", e2eGcpOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					azureJobName: {
						MultistageName:   "e2e-azure",
						MultistageResult: getStageResult("e2e-azure", e2eAzureStageOriginalTestName, 1, 1, 50),
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
					})
				}
			},
		},
		{
			name: "ByMultistageName",
			testFunc: func(t *testing.T, report sippyprocessingv1.TestReport) {
				expectedByMultistageName := map[string]sippyprocessingv1.StepRegistryMetrics{
					"e2e-aws": sippyprocessingv1.StepRegistryMetrics{
						MultistageName:   "e2e-aws",
						MultistageResult: getStageResult("e2e-aws", e2eAwsStageOriginalTestName, 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"aws-specific-stage": getStageResult("aws-specific-stage", e2eAwsOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					"e2e-azure": sippyprocessingv1.StepRegistryMetrics{
						MultistageName:   "e2e-azure",
						MultistageResult: getStageResult("e2e-azure", e2eAzureStageOriginalTestName, 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"azure-specific-stage": getStageResult("azure-specific-stage", e2eAzureOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":          getStageResult("ipi-install", e2eAzureOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test":   getStageResult("openshift-e2e-test", e2eAzureOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					"e2e-gcp": sippyprocessingv1.StepRegistryMetrics{
						MultistageName:   "e2e-gcp",
						MultistageResult: getStageResult("e2e-gcp", e2eGcpStageOriginalTestName, 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"gcp-specific-stage": getStageResult("gcp-specific-stage", e2eGcpOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 1, 50),
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
				expectedByStageName := map[string]sippyprocessingv1.ByStageName{
					// These only run twice; one success and one failure.
					"aws-specific-stage": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("aws-specific-stage", "", 1, 1, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-aws": getStageResult("aws-specific-stage", e2eAwsOriginalTestNameSpecificStage, 1, 1, 50),
						},
					},
					"azure-specific-stage": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("azure-specific-stage", "", 1, 1, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-azure": getStageResult("azure-specific-stage", e2eAzureOriginalTestNameSpecificStage, 1, 1, 50),
						},
					},
					"gcp-specific-stage": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("gcp-specific-stage", "", 1, 1, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-gcp": getStageResult("gcp-specific-stage", e2eGcpOriginalTestNameSpecificStage, 1, 1, 50),
						},
					},
					// These stages run multiple times:
					// One success, one failure = 2
					// 2 * len(["aws", "azure", "gcp"]) = 6
					"ipi-install": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("ipi-install", "", 3, 3, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-aws":   getStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 1, 50),
							"e2e-azure": getStageResult("ipi-install", e2eAzureOriginalTestNameIpiInstall, 1, 1, 50),
							"e2e-gcp":   getStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 1, 50),
						},
					},
					"openshift-e2e-test": sippyprocessingv1.ByStageName{
						Aggregated: getStageResult("openshift-e2e-test", "", 3, 3, 50),
						ByMultistageName: map[string]sippyprocessingv1.StageResult{
							"e2e-aws":   getStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 1, 50),
							"e2e-azure": getStageResult("openshift-e2e-test", e2eAzureOriginalTestNameE2ETest, 1, 1, 50),
							"e2e-gcp":   getStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 1, 50),
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
		{
			name: "ByVariant",
			testFunc: func(t *testing.T, report sippyprocessingv1.TestReport) {
				expectedByVariantName := map[string]sippyprocessingv1.StepRegistryMetrics{
					"aws": sippyprocessingv1.StepRegistryMetrics{
						MultistageName:   "e2e-aws",
						MultistageResult: getStageResult("e2e-aws", e2eAwsStageOriginalTestName, 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"aws-specific-stage": getStageResult("aws-specific-stage", e2eAwsOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					"azure": sippyprocessingv1.StepRegistryMetrics{
						MultistageName:   "e2e-azure",
						MultistageResult: getStageResult("e2e-azure", e2eAzureStageOriginalTestName, 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"azure-specific-stage": getStageResult("azure-specific-stage", e2eAzureOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":          getStageResult("ipi-install", e2eAzureOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test":   getStageResult("openshift-e2e-test", e2eAzureOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
					"gcp": sippyprocessingv1.StepRegistryMetrics{
						MultistageName:   "e2e-gcp",
						MultistageResult: getStageResult("e2e-gcp", e2eGcpStageOriginalTestName, 1, 1, 50),
						StageResults: map[string]sippyprocessingv1.StageResult{
							"gcp-specific-stage": getStageResult("gcp-specific-stage", e2eGcpOriginalTestNameSpecificStage, 1, 1, 50),
							"ipi-install":        getStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 1, 50),
							"openshift-e2e-test": getStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 1, 50),
						},
					},
				}

				// All of the variants are in a list, so we must iterate over it as oppoesd to looking it up in a map.
				for _, variant := range report.ByVariant {
					expected, ok := expectedByVariantName[variant.VariantName]

					// If we're not expecting a variant, skip it because ByVariant has
					// an entry for each known variant (single-node, ipi, upi, etc.),
					// not just the ones found in the RawData.
					if !ok {
						continue
					}

					t.Run(variant.VariantName, func(t *testing.T) {
						// Each variant job result contains multiple JobResults, so iterate over them
						for _, jobResult := range variant.JobResults {
							// Since we only have a single job name in each ByVariant result,
							// we can directly make our assertion.
							assertStepRegistryMetricsEqual(t, jobResult.StepRegistryMetrics, expected)
						}
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

	haveLen := len(have.ByMultistageName)
	wantLen := len(want.ByMultistageName)

	if haveLen != wantLen {
		t.Errorf("have (%d) / want (%d) size mismatch", haveLen, wantLen)
	}

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

	haveLen := len(have.StageResults)
	wantLen := len(want.StageResults)

	if haveLen != wantLen {
		t.Errorf("have (%d) / want (%d) size mismatch", haveLen, wantLen)
	}

	if have.MultistageName != want.MultistageName {
		t.Errorf("expected multistage name to be %s, got: %s", want.MultistageName, have.MultistageName)
	}

	assertStageResultsEqual(t, have.MultistageResult, want.MultistageResult)

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
		t.Errorf("expected stage result to have %d successes, got: %d", want.Successes, have.Successes)
	}

	if have.Failures != want.Failures {
		t.Errorf("expected stage result to have %d failures, got: %d", want.Failures, have.Failures)
	}

	// TODO: Determine if we should allow step registry metrics to be flaky.
	if have.Flakes != want.Flakes {
		t.Errorf("expected stage result to have %d flakes, got: %d", want.Flakes, have.Flakes)
	}

	if have.PassPercentage != want.PassPercentage {
		t.Errorf("expected stage result to have %0.2f pass percentage, got: %0.2f", want.PassPercentage, have.PassPercentage)
	}

	if have.OriginalTestName != want.OriginalTestName {
		t.Errorf("expected stage result to have original test name %s, got: %s", want.OriginalTestName, have.OriginalTestName)
	}

	haveCount := have.Successes + have.Failures + have.Flakes
	wantCount := want.Successes + want.Failures + want.Flakes

	if haveCount != wantCount {
		t.Errorf("expected to have a job run count of %d, got: %d", wantCount, haveCount)
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
	}
}

//nolint:dupl // Duplication is fine in this context since the test fixture becomes more expressive.
func getRawData() testgridanalysisapi.RawData {
	return testgridanalysisapi.RawData{
		JobResults: map[string]testgridanalysisapi.RawJobResult{
			awsJobName: {
				JobName:        awsJobName,
				TestGridJobURL: "https://testgrid",
				JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
					"https://prowurl1": testgridanalysisapi.RawJobRunResult{
						Job:                    awsJobName,
						JobRunURL:              "https://prowurl1",
						Succeeded:              true,
						StepRegistryItemStates: getStepRegistryItemStates("e2e-aws", testgridanalysisapi.Success),
					},
					"https://prowurl2": testgridanalysisapi.RawJobRunResult{
						Job:                    awsJobName,
						JobRunURL:              "https://prowurl2",
						Failed:                 true,
						StepRegistryItemStates: getStepRegistryItemStates("e2e-aws", testgridanalysisapi.Failure),
					},
				},
			},
			azureJobName: {
				JobName:        azureJobName,
				TestGridJobURL: "https://testgrid",
				JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
					"https://prowurl3": testgridanalysisapi.RawJobRunResult{
						Job:                    azureJobName,
						JobRunURL:              "https://prowurl3",
						Succeeded:              true,
						StepRegistryItemStates: getStepRegistryItemStates("e2e-azure", testgridanalysisapi.Success),
					},
					"https://prowurl4": testgridanalysisapi.RawJobRunResult{
						Job:                    azureJobName,
						JobRunURL:              "https://prowurl4",
						Failed:                 true,
						StepRegistryItemStates: getStepRegistryItemStates("e2e-azure", testgridanalysisapi.Failure),
					},
				},
			},
			gcpJobName: {
				JobName:        gcpJobName,
				TestGridJobURL: "https://testgrid",
				JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
					"https://prowurl5": testgridanalysisapi.RawJobRunResult{
						Job:                    gcpJobName,
						JobRunURL:              "https://prowurl5",
						Succeeded:              true,
						StepRegistryItemStates: getStepRegistryItemStates("e2e-gcp", testgridanalysisapi.Success),
					},
					"https://prowurl6": testgridanalysisapi.RawJobRunResult{
						Job:                    gcpJobName,
						JobRunURL:              "https://prowurl6",
						Failed:                 true,
						StepRegistryItemStates: getStepRegistryItemStates("e2e-gcp", testgridanalysisapi.Failure),
					},
				},
			},
		},
	}
}

func getStepRegistryItemStates(multistageName, state string) testgridanalysisapi.StepRegistryItemStates {
	itemStates := map[string]testgridanalysisapi.StepRegistryItemStates{
		"e2e-aws": {
			MultistageName: "e2e-aws",
			MultistageState: testgridanalysisapi.StageState{
				Name:             "e2e-aws",
				State:            state,
				OriginalTestName: e2eAwsStageOriginalTestName,
			},
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
			MultistageState: testgridanalysisapi.StageState{
				Name:             "e2e-azure",
				State:            state,
				OriginalTestName: e2eAzureStageOriginalTestName,
			},
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
			MultistageState: testgridanalysisapi.StageState{
				Name:             "e2e-gcp",
				State:            state,
				OriginalTestName: e2eGcpStageOriginalTestName,
			},
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
