package fixtures

import (
	"github.com/openshift/sippy/pkg/api/stepmetrics"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

// Release
const (
	Release string = "4.9"
)

// Job Names
const (
	AwsJobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws"
	GcpJobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-gcp"
)

// Original Test Names
const (
	E2eAwsOriginalTestNameSpecificStage string = "operator.Run multi-stage test e2e-aws - e2e-aws-aws-specific-stage container test"
	E2eAwsOriginalTestNameIpiInstall    string = "operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test"
	E2eAwsOriginalTestNameE2ETest       string = "operator.Run multi-stage test e2e-aws - e2e-aws-openshift-e2e-test container test"

	E2eGcpOriginalTestNameSpecificStage string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-gcp-specific-stage container test"
	E2eGcpOriginalTestNameIpiInstall    string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-ipi-install container test"
	E2eGcpOriginalTestNameE2ETest       string = "operator.Run multi-stage test e2e-gcp - e2e-gcp-openshift-e2e-test container test"
)

//nolint:dupl // Duplication is OK in this context
func GetAllMultistageResponse() stepmetrics.Response {
	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release:           Release,
			MultistageJobName: stepmetrics.All,
		},
		MultistageDetails: map[string]stepmetrics.MultistageDetails{
			"e2e-aws": {
				Name: "e2e-aws",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("e2e-aws", "", 1, 1),
					Previous:   GetStageResult("e2e-aws", "", 1, 1),
				},
				StepDetails: map[string]stepmetrics.StepDetail{
					"aws-specific": stepmetrics.StepDetail{
						Name: "aws-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
							Previous:   GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
						},
					},
					"ipi-install": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", E2eAwsOriginalTestNameIpiInstall, 1, 1),
							Previous:   GetStageResult("ipi-install", E2eAwsOriginalTestNameIpiInstall, 1, 1),
						},
					},
					"openshift-e2e-test": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", E2eAwsOriginalTestNameE2ETest, 1, 1),
							Previous:   GetStageResult("openshift-e2e-test", E2eAwsOriginalTestNameE2ETest, 1, 1),
						},
					},
				},
			},
			"e2e-gcp": {
				Name: "e2e-gcp",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("e2e-gcp", "", 1, 1),
					Previous:   GetStageResult("e2e-gcp", "", 1, 1),
				},
				StepDetails: map[string]stepmetrics.StepDetail{
					"gcp-specific": stepmetrics.StepDetail{
						Name: "gcp-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("gcp-specific", E2eGcpOriginalTestNameSpecificStage, 1, 1),
							Previous:   GetStageResult("gcp-specific", E2eGcpOriginalTestNameSpecificStage, 1, 1),
						},
					},
					"ipi-install": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", E2eGcpOriginalTestNameIpiInstall, 1, 1),
							Previous:   GetStageResult("ipi-install", E2eGcpOriginalTestNameIpiInstall, 1, 1),
						},
					},
					"openshift-e2e-test": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", E2eGcpOriginalTestNameE2ETest, 1, 1),
							Previous:   GetStageResult("openshift-e2e-test", E2eGcpOriginalTestNameE2ETest, 1, 1),
						},
					},
				},
			},
		},
	}
}

func GetSpecificMultistageResponse(multistageJobName string) stepmetrics.Response {
	resp := GetAllMultistageResponse()

	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release:           Release,
			MultistageJobName: multistageJobName,
		},
		MultistageDetails: map[string]stepmetrics.MultistageDetails{
			multistageJobName: resp.MultistageDetails[multistageJobName],
		},
	}
}

func GetAllStepsResponse() stepmetrics.Response {
	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release:  Release,
			StepName: stepmetrics.All,
		},
		StepDetails: map[string]stepmetrics.StepDetails{
			"openshift-e2e-test": {
				Name: "openshift-e2e-test",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("openshift-e2e-test", "", 2, 2),
					Previous:   GetStageResult("openshift-e2e-test", "", 2, 2),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-aws": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", E2eAwsOriginalTestNameE2ETest, 1, 1),
							Previous:   GetStageResult("openshift-e2e-test", E2eAwsOriginalTestNameE2ETest, 1, 1),
						},
					},
					"e2e-gcp": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", E2eGcpOriginalTestNameE2ETest, 1, 1),
							Previous:   GetStageResult("openshift-e2e-test", E2eGcpOriginalTestNameE2ETest, 1, 1),
						},
					},
				},
			},
			"ipi-install": {
				Name: "ipi-install",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("ipi-install", "", 2, 2),
					Previous:   GetStageResult("ipi-install", "", 2, 2),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-aws": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", E2eAwsOriginalTestNameIpiInstall, 1, 1),
							Previous:   GetStageResult("ipi-install", E2eAwsOriginalTestNameIpiInstall, 1, 1),
						},
					},
					"e2e-gcp": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", E2eGcpOriginalTestNameIpiInstall, 1, 1),
							Previous:   GetStageResult("ipi-install", E2eGcpOriginalTestNameIpiInstall, 1, 1),
						},
					},
				},
			},
			"aws-specific": {
				Name: "aws-specific",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("aws-specific", "", 1, 1),
					Previous:   GetStageResult("aws-specific", "", 1, 1),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-aws": stepmetrics.StepDetail{
						Name: "aws-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
							Previous:   GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
						},
					},
				},
			},
			"gcp-specific": {
				Name: "gcp-specific",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("gcp-specific", "", 1, 1),
					Previous:   GetStageResult("gcp-specific", "", 1, 1),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-gcp": stepmetrics.StepDetail{
						Name: "gcp-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("gcp-specific", E2eGcpOriginalTestNameSpecificStage, 1, 1),
							Previous:   GetStageResult("gcp-specific", E2eGcpOriginalTestNameSpecificStage, 1, 1),
						},
					},
				},
			},
		},
	}
}

func GetAwsSpecificStepNameResponse() stepmetrics.Response {
	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release:  Release,
			StepName: "aws-specific",
		},
		StepDetails: map[string]stepmetrics.StepDetails{
			"aws-specific": {
				Name: "aws-specific",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("aws-specific", "", 1, 1),
					Previous:   GetStageResult("aws-specific", "", 1, 1),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-aws": stepmetrics.StepDetail{
						Name: "aws-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
							Previous:   GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
						},
					},
				},
			},
		},
	}
}

func GetSpecificStepNameResponse(stepName string) stepmetrics.Response {
	resp := GetAllStepsResponse()

	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release:  Release,
			StepName: stepName,
		},
		StepDetails: map[string]stepmetrics.StepDetails{
			stepName: resp.StepDetails[stepName],
		},
	}
}

//nolint:dupl // Duplication is OK in this context
func GetAllJobsResponse() stepmetrics.Response {
	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release: Release,
			JobName: stepmetrics.All,
		},
		JobDetails: map[string]stepmetrics.JobDetails{
			AwsJobName: stepmetrics.JobDetails{
				JobName: AwsJobName,
				MultistageDetails: stepmetrics.MultistageDetails{
					Name: "e2e-aws",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
						Current:    GetStageResult("e2e-aws", "", 1, 1),
						Previous:   GetStageResult("e2e-aws", "", 1, 1),
					},
					StepDetails: map[string]stepmetrics.StepDetail{
						"aws-specific": stepmetrics.StepDetail{
							Name: "aws-specific",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
								Current:    GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
								Previous:   GetStageResult("aws-specific", E2eAwsOriginalTestNameSpecificStage, 1, 1),
							},
						},
						"ipi-install": stepmetrics.StepDetail{
							Name: "ipi-install",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
								Current:    GetStageResult("ipi-install", E2eAwsOriginalTestNameIpiInstall, 1, 1),
								Previous:   GetStageResult("ipi-install", E2eAwsOriginalTestNameIpiInstall, 1, 1),
							},
						},
						"openshift-e2e-test": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
								Current:    GetStageResult("openshift-e2e-test", E2eAwsOriginalTestNameE2ETest, 1, 1),
								Previous:   GetStageResult("openshift-e2e-test", E2eAwsOriginalTestNameE2ETest, 1, 1),
							},
						},
					},
				},
			},
			GcpJobName: stepmetrics.JobDetails{
				JobName: GcpJobName,
				MultistageDetails: stepmetrics.MultistageDetails{
					Name: "e2e-gcp",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
						Current:    GetStageResult("e2e-gcp", "", 1, 1),
						Previous:   GetStageResult("e2e-gcp", "", 1, 1),
					},
					StepDetails: map[string]stepmetrics.StepDetail{
						"gcp-specific": stepmetrics.StepDetail{
							Name: "gcp-specific",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
								Current:    GetStageResult("gcp-specific", E2eGcpOriginalTestNameSpecificStage, 1, 1),
								Previous:   GetStageResult("gcp-specific", E2eGcpOriginalTestNameSpecificStage, 1, 1),
							},
						},
						"ipi-install": stepmetrics.StepDetail{
							Name: "ipi-install",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
								Current:    GetStageResult("ipi-install", E2eGcpOriginalTestNameIpiInstall, 1, 1),
								Previous:   GetStageResult("ipi-install", E2eGcpOriginalTestNameIpiInstall, 1, 1),
							},
						},
						"openshift-e2e-test": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
								Current:    GetStageResult("openshift-e2e-test", E2eGcpOriginalTestNameE2ETest, 1, 1),
								Previous:   GetStageResult("openshift-e2e-test", E2eGcpOriginalTestNameE2ETest, 1, 1),
							},
						},
					},
				},
			},
		},
	}
}

func GetByJobNameResponse(jobName string) stepmetrics.Response {
	allJobs := GetAllJobsResponse()

	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release: Release,
			JobName: jobName,
		},
		JobDetails: map[string]stepmetrics.JobDetails{
			jobName: allJobs.JobDetails[jobName],
		},
	}
}

func GetStageResult(name, originalName string, passes, fails int) sippyprocessingv1.StageResult {
	return sippyprocessingv1.StageResult{
		TestResult: sippyprocessingv1.TestResult{
			Name:           name,
			Successes:      passes,
			Failures:       fails,
			PassPercentage: float64(passes) / float64(passes+fails) * 100.0,
		},
		OriginalTestName: originalName,
		Runs:             passes + fails,
	}
}
