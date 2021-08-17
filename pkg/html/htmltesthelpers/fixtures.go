package htmltesthelpers

import (
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func GetJobResult(jobName string) sippyprocessingv1.JobResult {
	return sippyprocessingv1.JobResult{
		Name:      jobName,
		Successes: 2,
		Failures:  1,
		StepRegistryMetrics: sippyprocessingv1.StepRegistryMetrics{
			MultistageName: "e2e-aws",
			StageResults: map[string]sippyprocessingv1.StageResult{
				"ipi-install": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						Successes:      1,
						Failures:       0,
						PassPercentage: 100,
					},
					Runs: 1,
				},
			},
		},
		TestResults: []sippyprocessingv1.TestResult{
			{
				Name:           "operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test",
				Successes:      1,
				Failures:       0,
				PassPercentage: 100,
			},
			{
				Name:           "unrelated-passing-test",
				Successes:      1,
				Failures:       0,
				PassPercentage: 100,
			},
			{
				Name:           "unrelated-failing-test",
				Successes:      0,
				Failures:       1,
				PassPercentage: 0,
			},
		},
	}
}

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
		TopLevelStepRegistryMetrics: GetTopLevelStepRegistryMetrics(jobName),
	}
}

func GetTopLevelStepRegistryMetrics(jobName string) sippyprocessingv1.TopLevelStepRegistryMetrics {
	return sippyprocessingv1.TopLevelStepRegistryMetrics{
		ByMultistageName: GetByMultistageName(),
		ByStageName:      GetByStageName(),
		ByJobName:        GetByJobName(jobName),
	}
}

func GetByJobName(jobName string) map[string]sippyprocessingv1.ByJobName {
	return map[string]sippyprocessingv1.ByJobName{
		jobName: sippyprocessingv1.ByJobName{
			JobName:             jobName,
			StepRegistryMetrics: GetByMultistageName()["e2e-aws"],
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
					OriginalTestName: "operator.Run multi-stage test e2e-aws - e2e-aws-openshift-e2e-test container test",
					Runs:             1,
				},
				"ipi-install": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: "operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test",
					Runs:             1,
				},
				"aws-specific": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "aws-specific",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: "operator.Run multi-stage test e2e-aws - e2e-aws-aws-specific container test",
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
					OriginalTestName: "operator.Run multi-stage test e2e-gcp - e2e-gcp-openshift-e2e-test container test",
					Runs:             1,
				},
				"ipi-install": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: "operator.Run multi-stage test e2e-gcp - e2e-gcp-ipi-install container test",
					Runs:             1,
				},
				"gcp-specific": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "gcp-specific",
						PassPercentage: 100,
						Successes:      1,
					},
					OriginalTestName: "operator.Run multi-stage test e2e-gcp - e2e-gcp-gcp-specific container test",
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
					OriginalTestName: "operator.Run multi-stage test e2e-aws - e2e-aws-openshift-e2e-test container test",
					Runs:             1,
				},
				"e2e-gcp": sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "openshift-e2e-test",
						Successes:      1,
						PassPercentage: 100,
					},
					OriginalTestName: "operator.Run multi-stage test e2e-gcp - e2e-gcp-openshift-e2e-test container test",
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
					OriginalTestName: "operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test",
					Runs:             1,
				},
				"e2e-gcp": sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						Successes:      1,
						PassPercentage: 100,
					},
					OriginalTestName: "operator.Run multi-stage test e2e-gcp - e2e-gcp-ipi-install container test",
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
					OriginalTestName: "operator.Run multi-stage test e2e-aws - e2e-aws-aws-specific container test",
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
					OriginalTestName: "operator.Run multi-stage test e2e-gcp - e2e-gcp-gcp-specific container test",
					Runs:             1,
				},
			},
		},
	}
}
