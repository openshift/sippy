package stepmetrics_test

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	jobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws"
	release string = "4.9"
)

type apiTestCase struct {
	name                      string
	request                   stepmetrics.Request
	expectedMultistageDetails map[string]stepmetrics.MultistageDetails
	expectedStepDetails       map[string]stepmetrics.StepDetails
}

func TestStepMetricsAPI(t *testing.T) {
	testCases := []apiTestCase{
		{
			name: "all multistage jobs",
			request: stepmetrics.Request{
				Release:           release,
				MultistageJobName: stepmetrics.All,
			},
			expectedMultistageDetails: map[string]stepmetrics.MultistageDetails{
				"e2e-aws": {
					Name: "e2e-aws",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					StepDetails: map[string]stepmetrics.StepDetail{
						"aws-specific": stepmetrics.StepDetail{
							Name: "aws-specific",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"ipi-install": stepmetrics.StepDetail{
							Name: "ipi-install",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"openshift-e2e-test": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
				"e2e-gcp": {
					Name: "e2e-gcp",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					StepDetails: map[string]stepmetrics.StepDetail{
						"gcp-specific": stepmetrics.StepDetail{
							Name: "gcp-specific",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"ipi-install": stepmetrics.StepDetail{
							Name: "ipi-install",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"openshift-e2e-test": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
			},
		},
		{
			name: "specific multistage job name",
			request: stepmetrics.Request{
				MultistageJobName: "e2e-aws",
				Release:           "4.9",
			},
			expectedMultistageDetails: map[string]stepmetrics.MultistageDetails{
				"e2e-aws": {
					Name: "e2e-aws",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					StepDetails: map[string]stepmetrics.StepDetail{
						"aws-specific": stepmetrics.StepDetail{
							Name: "aws-specific",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"ipi-install": stepmetrics.StepDetail{
							Name: "ipi-install",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"openshift-e2e-test": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
			},
		},
		{
			name: "all step names",
			request: stepmetrics.Request{
				Release:  release,
				StepName: stepmetrics.All,
			},
			expectedStepDetails: map[string]stepmetrics.StepDetails{
				"openshift-e2e-test": {
					Name: "openshift-e2e-test",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					ByMultistage: map[string]stepmetrics.StepDetail{
						"e2e-aws": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"e2e-gcp": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
				"ipi-install": {
					Name: "ipi-install",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					ByMultistage: map[string]stepmetrics.StepDetail{
						"e2e-aws": stepmetrics.StepDetail{
							Name: "ipi-install",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"e2e-gcp": stepmetrics.StepDetail{
							Name: "ipi-install",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
				"aws-specific": {
					Name: "aws-specific",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					ByMultistage: map[string]stepmetrics.StepDetail{
						"e2e-aws": stepmetrics.StepDetail{
							Name: "aws-specific",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
				"gcp-specific": {
					Name: "gcp-specific",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					ByMultistage: map[string]stepmetrics.StepDetail{
						"e2e-gcp": stepmetrics.StepDetail{
							Name: "gcp-specific",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
			},
		},
		{
			name: "specific step name",
			request: stepmetrics.Request{
				Release:  release,
				StepName: "openshift-e2e-test",
			},
			expectedStepDetails: map[string]stepmetrics.StepDetails{
				"openshift-e2e-test": {
					Name: "openshift-e2e-test",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					ByMultistage: map[string]stepmetrics.StepDetail{
						"e2e-aws": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"e2e-gcp": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
			},
		},
		{
			name: "by job name",
			request: stepmetrics.Request{
				Release: release,
				JobName: jobName,
			},
			expectedMultistageDetails: map[string]stepmetrics.MultistageDetails{
				"e2e-aws": {
					Name: "e2e-aws",
					Trend: stepmetrics.Trend{
						Trajectory: stepmetrics.TrendTrajectoryFlat,
						Delta:      0,
					},
					StepDetails: map[string]stepmetrics.StepDetail{
						"aws-specific": stepmetrics.StepDetail{
							Name: "aws-specific",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"ipi-install": stepmetrics.StepDetail{
							Name: "ipi-install",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
						"openshift-e2e-test": stepmetrics.StepDetail{
							Name: "openshift-e2e-test",
							Trend: stepmetrics.Trend{
								Trajectory: stepmetrics.TrendTrajectoryFlat,
								Delta:      0,
							},
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			curr := htmltesthelpers.GetTestReport(jobName, "test-name", release)
			prev := htmltesthelpers.GetTestReport(jobName, "test-name", release)

			a := stepmetrics.NewStepMetricsAPI(curr, prev)

			resp, err := a.Fetch(testCase.request)
			if err != nil {
				t.Errorf("expected no errors, got: %s", err)
			}

			spew.Dump(resp)

			if resp.Request != testCase.request {
				t.Errorf("expected request to be: %v, got: %v", testCase.request, resp.Request)
			}

			if testCase.request.JobName != "" {
				assertAllMultistageDetails(t, resp.MultistageDetails, testCase.expectedMultistageDetails)
			}

			if testCase.request.MultistageJobName != "" {
				assertAllMultistageDetails(t, resp.MultistageDetails, testCase.expectedMultistageDetails)
			}

			if testCase.request.StepName != "" {
				assertAllStepDetails(t, resp.StepDetails, testCase.expectedStepDetails)
			}
		})
	}
}

func assertAllMultistageDetails(t *testing.T, have, want map[string]stepmetrics.MultistageDetails) {
	t.Helper()

	assertKeysEqual(t, have, want)

	for _, multistageDetails := range have {
		if _, ok := want[multistageDetails.Name]; !ok {
			t.Errorf("expected to find multistage details for: %s", multistageDetails.Name)
		}

		assertMultistageDetails(t, multistageDetails, want[multistageDetails.Name])
	}
}

func assertMultistageDetails(t *testing.T, have, want stepmetrics.MultistageDetails) {
	t.Helper()

	if have.Name != want.Name {
		t.Errorf("name mismatch, have: %s, want: %s", have.Name, want.Name)
	}

	assertTrend(t, have.Trend, want.Trend)

	for stageName := range want.StepDetails {
		if _, ok := have.StepDetails[stageName]; !ok {
			t.Errorf("missing step details for: %s", stageName)
		}

		assertStepDetail(t, have.StepDetails[stageName], want.StepDetails[stageName])
	}
}

func assertTrend(t *testing.T, have, want stepmetrics.Trend) {
	t.Helper()

	if have.Trajectory != want.Trajectory {
		t.Errorf("trajectory mismatch, have: %s, want: %s", have.Trajectory, want.Trajectory)
	}

	if have.Delta != want.Delta {
		t.Errorf("delta mismatch, have: %0.2f, want: %0.2f", have.Delta, want.Delta)
	}

	if have.Current.Name != have.Previous.Name {
		t.Errorf("trend name mismatch, current: %s, previous: %s", have.Current.Name, have.Previous.Name)
	}
}

func assertAllStepDetails(t *testing.T, have, want map[string]stepmetrics.StepDetails) {
	t.Helper()

	assertKeysEqual(t, have, want)

	for _, stepDetails := range have {
		if _, ok := want[stepDetails.Name]; !ok {
			t.Errorf("expected to find step details for: %s", stepDetails.Name)
		}

		assertStepDetails(t, stepDetails, want[stepDetails.Name])
	}
}

func assertStepDetails(t *testing.T, have, want stepmetrics.StepDetails) {
	t.Helper()

	if have.Name != want.Name {
		t.Errorf("name mismatch, have: %s, want: %s", have.Name, want.Name)
	}

	assertTrend(t, have.Trend, want.Trend)

	for multistageName := range want.ByMultistage {
		if _, ok := have.ByMultistage[multistageName]; !ok {
			t.Errorf("missing step details for multistage name: %s", multistageName)
		}

		assertStepDetail(t, have.ByMultistage[multistageName], want.ByMultistage[multistageName])
	}
}

func assertStepDetail(t *testing.T, have, want stepmetrics.StepDetail) {
	if have.Name != want.Name {
		t.Errorf("name mismatch, have: %s, want: %s", have.Name, want.Name)
	}

	assertTrend(t, have.Trend, want.Trend)
}

func assertKeysEqual(t *testing.T, have, want interface{}) {
	t.Helper()

	haveSet := sets.StringKeySet(have)
	wantSet := sets.StringKeySet(want)

	if !haveSet.Equal(wantSet) {
		t.Errorf("key mismatch, expected: %v, got: %v", wantSet.List(), haveSet.List())
	}
}
