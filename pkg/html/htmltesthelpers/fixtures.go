package htmltesthelpers

import (
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

const (
	release string = "4.9"
)

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

func GetTestReport(jobName, testName, release string) sippyprocessingv1.TestReport {
	return sippyprocessingv1.TestReport{
		Release:   release,
		Timestamp: time.Now(),
		ByTest: []sippyprocessingv1.FailingTestResult{
			{
				TestName: testName,
				TestResultAcrossAllJobs: sippyprocessingv1.TestResult{
					Name:           testName,
					Successes:      0,
					Failures:       1,
					PassPercentage: 0,
				},
				JobResults: []sippyprocessingv1.FailingTestJobResult{
					{
						Name:           jobName,
						TestFailures:   1,
						TestSuccesses:  0,
						PassPercentage: 0,
					},
				},
			},
			{
				TestName: "failing-test-1",
				TestResultAcrossAllJobs: sippyprocessingv1.TestResult{
					Name:           "failing-test-1",
					Successes:      0,
					Failures:       1,
					PassPercentage: 0,
				},
				JobResults: []sippyprocessingv1.FailingTestJobResult{
					{
						Name:           jobName,
						TestFailures:   1,
						TestSuccesses:  0,
						PassPercentage: 0,
					},
				},
			},
		},
		ByJob: []sippyprocessingv1.JobResult{
			{
				Name:    "job-name",
				Variant: "aws",
				TestResults: []sippyprocessingv1.TestResult{
					{
						Name:           testName,
						Successes:      0,
						Failures:       1,
						PassPercentage: 0,
					},
					{
						Name:           "failing-test-1",
						Successes:      0,
						Failures:       1,
						PassPercentage: 0,
					},
				},
			},
		},
		TopLevelStepRegistryMetrics: GetTopLevelStepRegistryMetrics(),
	}
}

func GetTopLevelStepRegistryMetrics() sippyprocessingv1.TopLevelStepRegistryMetrics {
	return sippyprocessingv1.TopLevelStepRegistryMetrics{
		ByMultistageName: GetByMultistageName(),
		ByStageName:      GetByStageName(),
		ByJobName:        GetByJobName(),
	}
}

func GetByJobName() map[string]sippyprocessingv1.ByJobName {
	return map[string]sippyprocessingv1.ByJobName{
		AwsJobName: sippyprocessingv1.ByJobName{
			JobName:             AwsJobName,
			StepRegistryMetrics: GetByMultistageName()["e2e-aws"],
		},
		GcpJobName: sippyprocessingv1.ByJobName{
			JobName:             GcpJobName,
			StepRegistryMetrics: GetByMultistageName()["e2e-gcp"],
		},
	}
}

func GetByMultistageName() map[string]sippyprocessingv1.StepRegistryMetrics {
	return map[string]sippyprocessingv1.StepRegistryMetrics{
		"e2e-aws": sippyprocessingv1.StepRegistryMetrics{
			MultistageName: "e2e-aws",
			Aggregated: sippyprocessingv1.StageResult{
				TestResult: sippyprocessingv1.TestResult{
					Name:           "e2e-aws",
					Successes:      1,
					PassPercentage: 100,
				},
				Runs: 1,
			},
			StageResults: map[string]sippyprocessingv1.StageResult{
				"openshift-e2e-test": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "openshift-e2e-test",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: e2eAwsOriginalTestNameE2ETest,
					Runs:             1,
				},
				"ipi-install": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: e2eAwsOriginalTestNameIpiInstall,
					Runs:             1,
				},
				"aws-specific": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "aws-specific",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: e2eAwsOriginalTestNameSpecificStage,
					Runs:             1,
				},
			},
		},
		"e2e-gcp": sippyprocessingv1.StepRegistryMetrics{
			MultistageName: "e2e-gcp",
			Aggregated: sippyprocessingv1.StageResult{
				TestResult: sippyprocessingv1.TestResult{
					Name:           "e2e-gcp",
					Successes:      1,
					PassPercentage: 100,
				},
				Runs: 1,
			},
			StageResults: map[string]sippyprocessingv1.StageResult{
				"openshift-e2e-test": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "openshift-e2e-test",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: e2eGcpOriginalTestNameE2ETest,
					Runs:             1,
				},
				"ipi-install": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: e2eGcpOriginalTestNameIpiInstall,
					Runs:             1,
				},
				"gcp-specific": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "gcp-specific",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: e2eGcpOriginalTestNameSpecificStage,
					Runs:             1,
				},
			},
		},
	}
}

func GetByStageName() map[string]sippyprocessingv1.ByStageName {
	return map[string]sippyprocessingv1.ByStageName{
		"openshift-e2e-test": {
			Aggregated: sippyprocessingv1.StageResult{
				TestResult: sippyprocessingv1.TestResult{
					Name:           "openshift-e2e-test",
					Successes:      2,
					PassPercentage: 100,
				},
				Runs: 2,
			},
			ByMultistageName: map[string]sippyprocessingv1.StageResult{
				"e2e-aws": sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "openshift-e2e-test",
						Successes:      1,
						PassPercentage: 100,
					},
					OriginalTestName: e2eAwsOriginalTestNameE2ETest,
					Runs:             1,
				},
				"e2e-gcp": sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "openshift-e2e-test",
						Successes:      1,
						PassPercentage: 100,
					},
					OriginalTestName: e2eGcpOriginalTestNameE2ETest,
					Runs:             1,
				},
			},
		},
		"ipi-install": {
			Aggregated: sippyprocessingv1.StageResult{
				TestResult: sippyprocessingv1.TestResult{
					Name:           "ipi-install",
					Successes:      2,
					PassPercentage: 100,
				},
				Runs: 2,
			},
			ByMultistageName: map[string]sippyprocessingv1.StageResult{
				"e2e-aws": sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						Successes:      1,
						PassPercentage: 100,
					},
					OriginalTestName: e2eAwsOriginalTestNameIpiInstall,
					Runs:             1,
				},
				"e2e-gcp": sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						Successes:      1,
						PassPercentage: 100,
					},
					OriginalTestName: e2eGcpOriginalTestNameIpiInstall,
					Runs:             1,
				},
			},
		},
		"aws-specific": {
			Aggregated: sippyprocessingv1.StageResult{
				TestResult: sippyprocessingv1.TestResult{
					Name:           "aws-specific",
					Successes:      1,
					PassPercentage: 100,
				},
				Runs: 1,
			},
			ByMultistageName: map[string]sippyprocessingv1.StageResult{
				"e2e-aws": sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "aws-specific",
						Successes:      1,
						PassPercentage: 100,
					},
					OriginalTestName: e2eAwsOriginalTestNameSpecificStage,
					Runs:             1,
				},
			},
		},
		"gcp-specific": {
			Aggregated: sippyprocessingv1.StageResult{
				TestResult: sippyprocessingv1.TestResult{
					Name:           "gcp-specific",
					Successes:      1,
					PassPercentage: 100,
				},
				Runs: 1,
			},
			ByMultistageName: map[string]sippyprocessingv1.StageResult{
				"e2e-gcp": sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "gcp-specific",
						Successes:      1,
						PassPercentage: 100,
					},
					OriginalTestName: e2eGcpOriginalTestNameSpecificStage,
					Runs:             1,
				},
			},
		},
	}
}
