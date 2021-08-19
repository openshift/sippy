package fixtures

import (
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

// TODO: Make this more reusable
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

func GetPreviousTopLevelStepRegistryMetrics() sippyprocessingv1.TopLevelStepRegistryMetrics {
	return GetTopLevelStepRegistryMetrics()
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
			Aggregated:     GetStageResult("e2e-aws", "", 1, 1),
			StageResults: map[string]sippyprocessingv1.StageResult{
				"openshift-e2e-test": GetStageResult("openshift-e2e-test", E2eAwsOriginalTestNameE2ETest, 1, 1),
				"ipi-install":        GetStageResult("ipi-install", E2eAwsOriginalTestNameIpiInstall, 1, 1),
				"aws-specific":       GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
			},
		},
		"e2e-gcp": sippyprocessingv1.StepRegistryMetrics{
			MultistageName: "e2e-gcp",
			Aggregated:     GetStageResult("e2e-gcp", "", 1, 1),
			StageResults: map[string]sippyprocessingv1.StageResult{
				"openshift-e2e-test": GetStageResult("openshift-e2e-test", E2eGcpOriginalTestNameE2ETest, 1, 1),
				"ipi-install":        GetStageResult("ipi-install", E2eGcpOriginalTestNameIpiInstall, 1, 1),
				"gcp-specific":       GetStageResult("gcp-specific", E2eGcpOriginalTestNameSpecificStage, 1, 1),
			},
		},
	}
}

func GetByStageName() map[string]sippyprocessingv1.ByStageName {
	return map[string]sippyprocessingv1.ByStageName{
		"openshift-e2e-test": {
			Aggregated: GetStageResult("openshift-e2e-test", "", 2, 2),
			ByMultistageName: map[string]sippyprocessingv1.StageResult{
				"e2e-aws": GetStageResult("openshift-e2e-test", E2eAwsOriginalTestNameE2ETest, 1, 1),
				"e2e-gcp": GetStageResult("openshift-e2e-test", E2eGcpOriginalTestNameE2ETest, 1, 1),
			},
		},
		"ipi-install": {
			Aggregated: GetStageResult("ipi-install", "", 2, 2),
			ByMultistageName: map[string]sippyprocessingv1.StageResult{
				"e2e-aws": GetStageResult("ipi-install", E2eAwsOriginalTestNameIpiInstall, 1, 1),
				"e2e-gcp": GetStageResult("ipi-install", E2eGcpOriginalTestNameIpiInstall, 1, 1),
			},
		},
		"aws-specific": {
			Aggregated: GetStageResult("aws-specific", "", 1, 1),
			ByMultistageName: map[string]sippyprocessingv1.StageResult{
				"e2e-aws": GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
			},
		},
		"gcp-specific": {
			Aggregated: GetStageResult("gcp-specific", "", 1, 1),
			ByMultistageName: map[string]sippyprocessingv1.StageResult{
				"e2e-gcp": GetStageResult("gcp-specific", E2eGcpOriginalTestNameSpecificStage, 1, 1),
			},
		},
	}
}
