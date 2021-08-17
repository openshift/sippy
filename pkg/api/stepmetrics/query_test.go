package stepmetrics_test

import (
	"net/url"
	"testing"

	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/api/stepmetrics"
)

func TestValidateQuery(t *testing.T) {
	testCases := []struct {
		name          string
		request       stepmetrics.Request
		expectedError string
	}{
		{
			name: "sunny day",
			request: stepmetrics.Request{
				Release:           "4.9",
				MultistageJobName: "e2e-aws",
			},
		},
		{
			name: "all multistage job names",
			request: stepmetrics.Request{
				Release:           "4.9",
				MultistageJobName: stepmetrics.All,
			},
		},
		{
			name: "unknown multistage job name",
			request: stepmetrics.Request{
				Release:           "4.9",
				MultistageJobName: "unknown-multistage-name",
			},
			expectedError: "invalid multistage job name unknown-multistage-name",
		},
		{
			name: "all step names",
			request: stepmetrics.Request{
				Release:  "4.9",
				StepName: stepmetrics.All,
			},
		},
		{
			name: "unknown step name",
			request: stepmetrics.Request{
				Release:  "4.9",
				StepName: "unknown-step-name",
			},
			expectedError: "unknown step name unknown-step-name",
		},
		{
			name: "unknown variant",
			request: stepmetrics.Request{
				Release:  "4.9",
				StepName: "gcp-specific",
				Variant:  "unknown-variant",
			},
			expectedError: "unknown variant unknown-variant",
		},
		{
			name: "unknown job",
			request: stepmetrics.Request{
				Release: "4.9",
				JobName: "unknown-job-name",
			},
			expectedError: "unknown job name unknown-job-name",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			opts := stepmetrics.RequestOpts{
				URLValues: getURLValues(testCase.request),
				Current:   htmltesthelpers.GetTestReport("a-job-name", "test-name", "4.9"),
				Previous:  htmltesthelpers.GetTestReport("a-job-name", "test-name", "4.9"),
			}

			req, err := stepmetrics.ValidateRequest(opts)
			if testCase.expectedError != "" && err == nil {
				t.Errorf("expected error: %s, got nil", testCase.expectedError)
			}

			if err != nil && testCase.expectedError != err.Error() {
				t.Errorf("expected error %s, got: %s", testCase.expectedError, err)
			}

			if err == nil && req != testCase.request {
				t.Errorf("requests do not match, have: %v, want: %v", req, testCase.request)
			}
		})
	}
}

func getURLValues(req stepmetrics.Request) url.Values {
	valMap := map[string]string{
		"jobName":           req.JobName,
		"release":           req.Release,
		"multistageJobName": req.MultistageJobName,
		"stepName":          req.StepName,
		"variant":           req.Variant,
	}

	values := url.Values{}

	for k, v := range valMap {
		if v != "" {
			values.Add(k, v)
		}
	}

	return values
}
