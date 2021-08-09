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
			StageResults: []sippyprocessingv1.StageResult{
				{
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						Successes:      1,
						Failures:       0,
						PassPercentage: 100,
					},
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
		TopLevelStepRegistryMetrics: sippyprocessingv1.TopLevelStepRegistryMetrics{
			ByStageName: map[string]sippyprocessingv1.StageResult{
				"openshift-e2e-test": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "openshift-e2e-test",
						PassPercentage: 100,
						Successes:      2,
					},
				},
				"ipi-install": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						PassPercentage: 100,
						Successes:      2,
					},
				},
				"aws-specific-step": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "aws-specific-step",
						PassPercentage: 100,
						Successes:      1,
					},
				},
				"gcp-specific-step": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "gcp-specific-step",
						PassPercentage: 100,
						Successes:      1,
					},
				},
			},
			ByMultistageName: map[string]map[string]sippyprocessingv1.StageResult{
				"e2e-aws": map[string]sippyprocessingv1.StageResult{
					"openshift-e2e-test": {
						TestResult: sippyprocessingv1.TestResult{
							Name:           "openshift-e2e-test",
							PassPercentage: 100,
							Successes:      1,
						},
					},
					"ipi-install": {
						TestResult: sippyprocessingv1.TestResult{
							Name:           "ipi-install",
							PassPercentage: 100,
							Successes:      1,
						},
					},
					"aws-specific-step": {
						TestResult: sippyprocessingv1.TestResult{
							Name:           "aws-specific-step",
							PassPercentage: 100,
							Successes:      1,
						},
					},
				},
				"e2e-gcp": map[string]sippyprocessingv1.StageResult{
					"openshift-e2e-test": {
						TestResult: sippyprocessingv1.TestResult{
							Name:           "openshift-e2e-test",
							PassPercentage: 100,
							Successes:      1,
						},
					},
					"ipi-install": {
						TestResult: sippyprocessingv1.TestResult{
							Name:           "ipi-install",
							PassPercentage: 100,
							Successes:      1,
						},
					},
					"gcp-specific-step": {
						TestResult: sippyprocessingv1.TestResult{
							Name:           "gcp-specific-step",
							PassPercentage: 100,
							Successes:      1,
						},
					},
				},
			},
		},
	}
}
