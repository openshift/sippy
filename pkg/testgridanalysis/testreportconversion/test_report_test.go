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

const (
	awsJobName   string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws"
	azureJobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-azure"
	gcpJobName   string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-gcp"
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
						MultistageResult: getStageResult("e2e-aws", 1, 1, 50),
						StageResults: []sippyprocessingv1.StageResult{
							getStageResult("aws-specific-stage", 1, 1, 50),
							getStageResult("ipi-install", 1, 1, 50),
							getStageResult("openshift-e2e-test", 1, 1, 50),
						},
					},
					gcpJobName: {
						MultistageName:   "e2e-gcp",
						MultistageResult: getStageResult("e2e-gcp", 1, 1, 50),
						StageResults: []sippyprocessingv1.StageResult{
							getStageResult("gcp-specific-stage", 1, 1, 50),
							getStageResult("ipi-install", 1, 1, 50),
							getStageResult("openshift-e2e-test", 1, 1, 50),
						},
					},
					azureJobName: {
						MultistageName:   "e2e-azure",
						MultistageResult: getStageResult("e2e-azure", 1, 1, 50),
						StageResults: []sippyprocessingv1.StageResult{
							getStageResult("azure-specific-stage", 1, 1, 50),
							getStageResult("ipi-install", 1, 1, 50),
							getStageResult("openshift-e2e-test", 1, 1, 50),
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
				expectedByMultistageName := map[string]map[string]sippyprocessingv1.StageResult{
					"e2e-aws": map[string]sippyprocessingv1.StageResult{
						"aws-specific-stage": getStageResult("aws-specific-stage", 1, 1, 50),
						"ipi-install":        getStageResult("ipi-install", 1, 1, 50),
						"openshift-e2e-test": getStageResult("openshift-e2e-test", 1, 1, 50),
					},
					"e2e-azure": map[string]sippyprocessingv1.StageResult{
						"azure-specific-stage": getStageResult("azure-specific-stage", 1, 1, 50),
						"ipi-install":          getStageResult("ipi-install", 1, 1, 50),
						"openshift-e2e-test":   getStageResult("openshift-e2e-test", 1, 1, 50),
					},
					"e2e-gcp": map[string]sippyprocessingv1.StageResult{
						"gcp-specific-stage": getStageResult("gcp-specific-stage", 1, 1, 50),
						"ipi-install":        getStageResult("ipi-install", 1, 1, 50),
						"openshift-e2e-test": getStageResult("openshift-e2e-test", 1, 1, 50),
					},
				}

				for multistageName := range expectedByMultistageName {
					for stageName, expectedStageResult := range expectedByMultistageName[multistageName] {
						t.Run(multistageName+" "+stageName, func(t *testing.T) {
							assertStageResultsEqual(t, report.TopLevelStepRegistryMetrics.ByMultistageName[multistageName][stageName], expectedStageResult)
						})
					}
				}
			},
		},
		{
			name: "ByStageName",
			testFunc: func(t *testing.T, report sippyprocessingv1.TestReport) {
				expectedByStageName := map[string]sippyprocessingv1.StageResult{
					// These only run twice; one success and one failure.
					"aws-specific-stage":   getStageResult("aws-specific-stage", 1, 1, 50),
					"azure-specific-stage": getStageResult("azure-specific-stage", 1, 1, 50),
					"gcp-specific-stage":   getStageResult("gcp-specific-stage", 1, 1, 50),
					// These stages run multiple times:
					// One success, one failure = 2
					// 2 * len(["aws", "azure", "gcp"]) = 6
					"ipi-install":        getStageResult("ipi-install", 3, 3, 50),
					"openshift-e2e-test": getStageResult("openshift-e2e-test", 3, 3, 50),
				}

				for stageName, expectedStageResult := range expectedByStageName {
					t.Run(stageName, func(t *testing.T) {
						assertStageResultsEqual(t, report.TopLevelStepRegistryMetrics.ByStageName[stageName], expectedStageResult)
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
						MultistageResult: getStageResult("e2e-aws", 1, 1, 50),
						StageResults: []sippyprocessingv1.StageResult{
							getStageResult("aws-specific-stage", 1, 1, 50),
							getStageResult("ipi-install", 1, 1, 50),
							getStageResult("openshift-e2e-test", 1, 1, 50),
						},
					},
					"azure": sippyprocessingv1.StepRegistryMetrics{
						MultistageName:   "e2e-azure",
						MultistageResult: getStageResult("e2e-azure", 1, 1, 50),
						StageResults: []sippyprocessingv1.StageResult{
							getStageResult("azure-specific-stage", 1, 1, 50),
							getStageResult("ipi-install", 1, 1, 50),
							getStageResult("openshift-e2e-test", 1, 1, 50),
						},
					},
					"gcp": sippyprocessingv1.StepRegistryMetrics{
						MultistageName:   "e2e-gcp",
						MultistageResult: getStageResult("e2e-gcp", 1, 1, 50),
						StageResults: []sippyprocessingv1.StageResult{
							getStageResult("gcp-specific-stage", 1, 1, 50),
							getStageResult("ipi-install", 1, 1, 50),
							getStageResult("openshift-e2e-test", 1, 1, 50),
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

	// Order does not matter, so convert this into a map keyed by name for easier comparison.
	expectedStages := getStageResultsByName(want.StageResults)

	for _, stageResult := range have.StageResults {
		if _, ok := expectedStages[stageResult.Name]; !ok {
			t.Errorf("expected to have a stageresult with name: %s", stageResult.Name)
		}

		assertStageResultsEqual(t, stageResult, expectedStages[stageResult.Name])
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

	haveCount := have.Successes + have.Failures + have.Flakes
	wantCount := want.Successes + want.Failures + want.Flakes

	if haveCount != wantCount {
		t.Errorf("expected to have a job run count of %d, got: %d", wantCount, haveCount)
	}
}

// Helpers
//nolint:unparam // The caller should specify passPercentage
func getStageResult(name string, successes, failures int, passPercentage float64) sippyprocessingv1.StageResult {
	// Allows us to pack this into a single line without affecting readability and epxressiveness.
	return sippyprocessingv1.StageResult{
		TestResult: sippyprocessingv1.TestResult{
			Name:           name,
			Successes:      successes,
			Failures:       failures,
			PassPercentage: passPercentage,
		},
	}
}

func getStageResultsByName(stageResults []sippyprocessingv1.StageResult) map[string]sippyprocessingv1.StageResult {
	byName := map[string]sippyprocessingv1.StageResult{}

	for _, stageResult := range stageResults {
		byName[stageResult.Name] = stageResult
	}

	return byName
}

//nolint:dupl // Duplication is fine in this context since the test fixture becomes more expressive.
func getRawData() testgridanalysisapi.RawData {
	return testgridanalysisapi.RawData{
		JobResults: map[string]testgridanalysisapi.RawJobResult{
			awsJobName: {
				JobName:        awsJobName,
				TestGridJobURL: "https://testgrid",
				TestResults: map[string]testgridanalysisapi.RawTestResult{
					// We're expecting to get the original test name that we were previously ignoring.
					// TODO: Determine if we still want to ignore these in a test context or use this as a special case.
					"operator.Run multi-stage test e2e-aws": {
						Name:      "operator.Run multi-stage test e2e-aws",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-aws - e2e-aws-aws-specific-stage container test": {
						Name:      "operator.Run multi-stage test e2e-aws - e2e-aws-aws-specific-stage container test",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test": {
						Name:      "operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-aws - e2e-aws-openshift-e2e-test container test": {
						Name:      "operator.Run multi-stage test e2e-aws - e2e-aws-openshift-e2e-test container test",
						Successes: 1,
						Failures:  1,
					},
					"Overall": {
						Name:      "Overall",
						Successes: 1,
						Failures:  1,
					},
				},
				JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
					"https://prowurl1": testgridanalysisapi.RawJobRunResult{
						Job:       awsJobName,
						JobRunURL: "https://prowurl1",
						Succeeded: true,
						StepRegistryItemStates: testgridanalysisapi.StepRegistryItemStates{
							MultistageName: "e2e-aws",
							MultistageState: testgridanalysisapi.StageState{
								Name:  "e2e-aws",
								State: testgridanalysisapi.Success,
							},
							States: []testgridanalysisapi.StageState{
								{
									Name:  "aws-specific-stage",
									State: testgridanalysisapi.Success,
								},
								{
									Name:  "ipi-install",
									State: testgridanalysisapi.Success,
								},
								{
									Name:  "openshift-e2e-test",
									State: testgridanalysisapi.Success,
								},
							},
						},
					},
					"https://prowurl2": testgridanalysisapi.RawJobRunResult{
						Job:       awsJobName,
						JobRunURL: "https://prowurl2",
						Failed:    true,
						StepRegistryItemStates: testgridanalysisapi.StepRegistryItemStates{
							MultistageName: "e2e-aws",
							MultistageState: testgridanalysisapi.StageState{
								Name:  "e2e-aws",
								State: testgridanalysisapi.Failure,
							},
							States: []testgridanalysisapi.StageState{
								{
									Name:  "aws-specific-stage",
									State: testgridanalysisapi.Failure,
								},
								{
									Name:  "ipi-install",
									State: testgridanalysisapi.Failure,
								},
								{
									Name:  "openshift-e2e-test",
									State: testgridanalysisapi.Failure,
								},
							},
						},
						TestFailures: 4,
						FailedTestNames: []string{
							"operator.Run multi-stage test e2e-aws - e2e-aws-aws-specific-stage container test",
							"operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test",
							"operator.Run multi-stage test e2e-aws - e2e-aws-openshift-e2e-test container test",
							"Overall", // If the job failed, Overall fails too.
						},
					},
				},
			},
			azureJobName: {
				JobName:        azureJobName,
				TestGridJobURL: "https://testgrid",
				TestResults: map[string]testgridanalysisapi.RawTestResult{
					"operator.Run multi-stage test e2e-azure": {
						Name:      "operator.Run multi-stage test e2e-azure",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-azure - e2e-azure-azure-specific-stage container test": {
						Name:      "operator.Run multi-stage test e2e-azure - e2e-azure-azure-specific-stage container test",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-azure - e2e-azure-ipi-install container test": {
						Name:      "operator.Run multi-stage test e2e-azure - e2e-azure-ipi-install container test",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-azure - e2e-azure-openshift-e2e-test container test": {
						Name:      "operator.Run multi-stage test e2e-azure - e2e-azure-openshift-e2e-test container test",
						Successes: 1,
						Failures:  1,
					},
					"Overall": {
						Name:      "Overall",
						Successes: 1,
						Failures:  1,
					},
				},
				JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
					"https://prowurl3": testgridanalysisapi.RawJobRunResult{
						Job:       azureJobName,
						JobRunURL: "https://prowurl3",
						Succeeded: true,
						StepRegistryItemStates: testgridanalysisapi.StepRegistryItemStates{
							MultistageName: "e2e-azure",
							MultistageState: testgridanalysisapi.StageState{
								Name:  "e2e-azure",
								State: testgridanalysisapi.Success,
							},
							States: []testgridanalysisapi.StageState{
								{
									Name:  "azure-specific-stage",
									State: testgridanalysisapi.Success,
								},
								{
									Name:  "ipi-install",
									State: testgridanalysisapi.Success,
								},
								{
									Name:  "openshift-e2e-test",
									State: testgridanalysisapi.Success,
								},
							},
						},
					},
					"https://prowurl4": testgridanalysisapi.RawJobRunResult{
						Job:       azureJobName,
						JobRunURL: "https://prowurl4",
						Failed:    true,
						StepRegistryItemStates: testgridanalysisapi.StepRegistryItemStates{
							MultistageName: "e2e-azure",
							MultistageState: testgridanalysisapi.StageState{
								Name:  "e2e-azure",
								State: testgridanalysisapi.Failure,
							},
							States: []testgridanalysisapi.StageState{
								{
									Name:  "azure-specific-stage",
									State: testgridanalysisapi.Failure,
								},
								{
									Name:  "ipi-install",
									State: testgridanalysisapi.Failure,
								},
								{
									Name:  "openshift-e2e-test",
									State: testgridanalysisapi.Failure,
								},
							},
						},
						TestFailures: 4,
						FailedTestNames: []string{
							"operator.Run multi-stage test e2e-azure - e2e-azure-azure-specific-stage container test",
							"operator.Run multi-stage test e2e-azure - e2e-azure-ipi-install container test",
							"operator.Run multi-stage test e2e-azure - e2e-azure-openshift-e2e-test container test",
							"Overall",
						},
					},
				},
			},
			gcpJobName: {
				JobName:        gcpJobName,
				TestGridJobURL: "https://testgrid",
				TestResults: map[string]testgridanalysisapi.RawTestResult{
					"operator.Run multi-stage test e2e-gcp": {
						Name:      "operator.Run multi-stage test e2e-gcp",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-gcp - e2e-gcp-gcp-specific-stage container test": {
						Name:      "operator.Run multi-stage test e2e-gcp - e2e-gcp-gcp-specific-stage container test",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-gcp - e2e-gcp-ipi-install container test": {
						Name:      "operator.Run multi-stage test e2e-gcp - e2e-gcp-ipi-install container test",
						Successes: 1,
						Failures:  1,
					},
					"operator.Run multi-stage test e2e-gcp - e2e-gcp-openshift-e2e-test container test": {
						Name:      "operator.Run multi-stage test e2e-gcp - e2e-gcp-openshift-e2e-test container test",
						Successes: 1,
						Failures:  1,
					},
					"Overall": {
						Name:      "Overall",
						Successes: 1,
						Failures:  1,
					},
				},
				JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
					"https://prowurl5": testgridanalysisapi.RawJobRunResult{
						Job:       gcpJobName,
						JobRunURL: "https://prowurl5",
						Succeeded: true,
						StepRegistryItemStates: testgridanalysisapi.StepRegistryItemStates{
							MultistageName: "e2e-gcp",
							MultistageState: testgridanalysisapi.StageState{
								Name:  "e2e-gcp",
								State: testgridanalysisapi.Success,
							},
							States: []testgridanalysisapi.StageState{
								{
									Name:  "gcp-specific-stage",
									State: testgridanalysisapi.Success,
								},
								{
									Name:  "ipi-install",
									State: testgridanalysisapi.Success,
								},
								{
									Name:  "openshift-e2e-test",
									State: testgridanalysisapi.Success,
								},
							},
						},
					},
					"https://prowurl6": testgridanalysisapi.RawJobRunResult{
						Job:       gcpJobName,
						JobRunURL: "https://prowurl6",
						Failed:    true,
						StepRegistryItemStates: testgridanalysisapi.StepRegistryItemStates{
							MultistageName: "e2e-gcp",
							MultistageState: testgridanalysisapi.StageState{
								Name:  "e2e-gcp",
								State: testgridanalysisapi.Failure,
							},
							States: []testgridanalysisapi.StageState{
								{
									Name:  "gcp-specific-stage",
									State: testgridanalysisapi.Failure,
								},
								{
									Name:  "ipi-install",
									State: testgridanalysisapi.Failure,
								},
								{
									Name:  "openshift-e2e-test",
									State: testgridanalysisapi.Failure,
								},
							},
						},
						TestFailures: 4,
						FailedTestNames: []string{
							"operator.Run multi-stage test e2e-gcp - e2e-gcp-gcp-specific-stage container test",
							"operator.Run multi-stage test e2e-gcp - e2e-gcp-ipi-install container test",
							"operator.Run multi-stage test e2e-gcp - e2e-gcp-openshift-e2e-test container test",
							"Overall",
						},
					},
				},
			},
		},
	}
}
