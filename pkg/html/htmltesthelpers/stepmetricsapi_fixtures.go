package htmltesthelpers

import (
	"github.com/openshift/sippy/pkg/api/stepmetrics"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func GetAllMultistageResponse() stepmetrics.Response {
	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release:           "4.9",
			MultistageJobName: stepmetrics.All,
		},
		MultistageDetails: map[string]stepmetrics.MultistageDetails{
			"e2e-aws": {
				Name: "e2e-aws",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("e2e-aws", "", 1, 0),
					Previous:   GetStageResult("e2e-aws", "", 1, 0),
				},
				StepDetails: map[string]stepmetrics.StepDetail{
					"aws-specific": stepmetrics.StepDetail{
						Name: "aws-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("aws-specific", e2eAwsOriginalTestNameSpecificStage, 1, 0),
							Previous:   GetStageResult("aws-specific", e2eAwsOriginalTestNameSpecificStage, 1, 0),
						},
					},
					"ipi-install": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 0),
							Previous:   GetStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 0),
						},
					},
					"openshift-e2e-test": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 0),
							Previous:   GetStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 0),
						},
					},
				},
			},
			"e2e-gcp": {
				Name: "e2e-gcp",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("e2e-gcp", "", 1, 0),
					Previous:   GetStageResult("e2e-gcp", "", 1, 0),
				},
				StepDetails: map[string]stepmetrics.StepDetail{
					"gcp-specific": stepmetrics.StepDetail{
						Name: "gcp-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("gcp-specific", e2eGcpOriginalTestNameSpecificStage, 1, 0),
							Previous:   GetStageResult("gcp-specific", e2eGcpOriginalTestNameSpecificStage, 1, 0),
						},
					},
					"ipi-install": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 0),
							Previous:   GetStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 0),
						},
					},
					"openshift-e2e-test": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 0),
							Previous:   GetStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 0),
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
			Release:           "4.9",
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
			Release:  "4.9",
			StepName: stepmetrics.All,
		},
		StepDetails: map[string]stepmetrics.StepDetails{
			"openshift-e2e-test": {
				Name: "openshift-e2e-test",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("openshift-e2e-test", "", 2, 0),
					Previous:   GetStageResult("openshift-e2e-test", "", 2, 0),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-aws": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 0),
							Previous:   GetStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 0),
						},
					},
					"e2e-gcp": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 0),
							Previous:   GetStageResult("openshift-e2e-test", e2eGcpOriginalTestNameE2ETest, 1, 0),
						},
					},
				},
			},
			"ipi-install": {
				Name: "ipi-install",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("ipi-install", "", 2, 0),
					Previous:   GetStageResult("ipi-install", "", 2, 0),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-aws": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 0),
							Previous:   GetStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 0),
						},
					},
					"e2e-gcp": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 0),
							Previous:   GetStageResult("ipi-install", e2eGcpOriginalTestNameIpiInstall, 1, 0),
						},
					},
				},
			},
			"aws-specific": {
				Name: "aws-specific",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("aws-specific", "", 1, 0),
					Previous:   GetStageResult("aws-specific", "", 1, 0),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-aws": stepmetrics.StepDetail{
						Name: "aws-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("aws-specific", e2eAwsOriginalTestNameSpecificStage, 1, 0),
							Previous:   GetStageResult("aws-specific", e2eAwsOriginalTestNameSpecificStage, 1, 0),
						},
					},
				},
			},
			"gcp-specific": {
				Name: "gcp-specific",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("gcp-specific", "", 1, 0),
					Previous:   GetStageResult("gcp-specific", "", 1, 0),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-gcp": stepmetrics.StepDetail{
						Name: "gcp-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("gcp-specific", e2eGcpOriginalTestNameSpecificStage, 1, 0),
							Previous:   GetStageResult("gcp-specific", e2eGcpOriginalTestNameSpecificStage, 1, 0),
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
			Release:  "4.9",
			StepName: "aws-specific",
		},
		StepDetails: map[string]stepmetrics.StepDetails{
			"aws-specific": {
				Name: "aws-specific",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("aws-specific", "", 1, 0),
					Previous:   GetStageResult("aws-specific", "", 1, 0),
				},
				ByMultistage: map[string]stepmetrics.StepDetail{
					"e2e-aws": stepmetrics.StepDetail{
						Name: "aws-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("aws-specific", e2eAwsOriginalTestNameSpecificStage, 1, 0),
							Previous:   GetStageResult("aws-specific", e2eAwsOriginalTestNameSpecificStage, 1, 0),
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
			Release:  "4.9",
			StepName: stepName,
		},
		StepDetails: map[string]stepmetrics.StepDetails{
			stepName: resp.StepDetails[stepName],
		},
	}
}

func GetByJobNameResponse() stepmetrics.Response {
	return stepmetrics.Response{
		Request: stepmetrics.Request{
			Release: "4.9",
			JobName: "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws",
		},
		MultistageDetails: map[string]stepmetrics.MultistageDetails{
			"e2e-aws": {
				Name: "e2e-aws",
				Trend: stepmetrics.Trend{
					Trajectory: stepmetrics.TrendTrajectoryFlat,
					Delta:      0,
					Current:    GetStageResult("e2e-aws", "", 1, 0),
					Previous:   GetStageResult("e2e-aws", "", 1, 0),
				},
				StepDetails: map[string]stepmetrics.StepDetail{
					"aws-specific": stepmetrics.StepDetail{
						Name: "aws-specific",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("aws-specific", e2eAwsOriginalTestNameSpecificStage, 1, 0),
							Previous:   GetStageResult("aws-specific", e2eAwsOriginalTestNameSpecificStage, 1, 0),
						},
					},
					"ipi-install": stepmetrics.StepDetail{
						Name: "ipi-install",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 0),
							Previous:   GetStageResult("ipi-install", e2eAwsOriginalTestNameIpiInstall, 1, 0),
						},
					},
					"openshift-e2e-test": stepmetrics.StepDetail{
						Name: "openshift-e2e-test",
						Trend: stepmetrics.Trend{
							Trajectory: stepmetrics.TrendTrajectoryFlat,
							Delta:      0,
							Current:    GetStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 0),
							Previous:   GetStageResult("openshift-e2e-test", e2eAwsOriginalTestNameE2ETest, 1, 0),
						},
					},
				},
			},
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
