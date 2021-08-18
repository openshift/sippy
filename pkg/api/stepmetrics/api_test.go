package stepmetrics_test

import (
	"testing"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	jobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws"
	release string = "4.9"
)

type apiTestCase struct {
	name             string
	request          stepmetrics.Request
	expectedResponse stepmetrics.Response
}

func TestStepMetricsAPI(t *testing.T) {
	testCases := []apiTestCase{
		{
			name: "all multistage jobs",
			request: stepmetrics.Request{
				Release:           release,
				MultistageJobName: stepmetrics.All,
			},
			expectedResponse: htmltesthelpers.GetAllMultistageResponse(),
		},
		{
			name: "specific multistage job name - e2e-aws",
			request: stepmetrics.Request{
				MultistageJobName: "e2e-aws",
				Release:           "4.9",
			},
			expectedResponse: htmltesthelpers.GetSpecificMultistageResponse("e2e-aws"),
		},
		{
			name: "specific multistage job name - e2e-gcp",
			request: stepmetrics.Request{
				MultistageJobName: "e2e-gcp",
				Release:           "4.9",
			},
			expectedResponse: htmltesthelpers.GetSpecificMultistageResponse("e2e-gcp"),
		},
		{
			name: "all step names",
			request: stepmetrics.Request{
				Release:  release,
				StepName: stepmetrics.All,
			},
			expectedResponse: htmltesthelpers.GetAllStepsResponse(),
		},
		{
			name: "specific step name - openshift-e2e-test",
			request: stepmetrics.Request{
				Release:  release,
				StepName: "openshift-e2e-test",
			},
			expectedResponse: htmltesthelpers.GetSpecificStepNameResponse("openshift-e2e-test"),
		},
		{
			name: "specific step name - ipi-install",
			request: stepmetrics.Request{
				Release:  release,
				StepName: "ipi-install",
			},
			expectedResponse: htmltesthelpers.GetSpecificStepNameResponse("ipi-install"),
		},
		{
			name: "specific step name - aws-specific",
			request: stepmetrics.Request{
				Release:  release,
				StepName: "aws-specific",
			},
			expectedResponse: htmltesthelpers.GetSpecificStepNameResponse("aws-specific"),
		},
		{
			name: "by job name",
			request: stepmetrics.Request{
				Release: release,
				JobName: jobName,
			},
			expectedResponse: htmltesthelpers.GetByJobNameResponse(),
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

			if resp.Request != testCase.expectedResponse.Request {
				t.Errorf("expected request to be: %v, got: %v", testCase.request, resp.Request)
			}

			assertAllMultistageDetails(t, resp.MultistageDetails, testCase.expectedResponse.MultistageDetails)
			assertAllStepDetails(t, resp.StepDetails, testCase.expectedResponse.StepDetails)
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

	assertStageResultsEqual(t, have.Current, want.Current)
	assertStageResultsEqual(t, have.Previous, want.Previous)
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
