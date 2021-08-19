package stepmetrics_test

import (
	"net/url"
	"testing"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/api/stepmetrics/fixtures"
)

func TestValidateQuery(t *testing.T) {
	testCases := []struct {
		name            string
		request         stepmetrics.Request
		expectedRequest stepmetrics.Request
		expectedError   string
		validators      map[string]stepmetrics.Validator
	}{
		{
			name: "specific multistage job name",
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
		{
			name: "ui sets default",
			request: stepmetrics.Request{
				Release: "4.9",
			},
			validators: map[string]stepmetrics.Validator{
				"UI": stepmetrics.ValidateUIRequest,
			},
			expectedRequest: stepmetrics.Request{
				Release:           "4.9",
				MultistageJobName: stepmetrics.All,
			},
		},
		{
			name: "api emits error",
			request: stepmetrics.Request{
				Release: "4.9",
			},
			validators: map[string]stepmetrics.Validator{
				"API": stepmetrics.ValidateAPIRequest,
			},
			expectedError: "missing multistage job name or step name",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			opts := stepmetrics.RequestOpts{
				URLValues: getURLValues(testCase.request),
				Current:   fixtures.GetTestReport("a-job-name", "test-name", "4.9"),
				Previous:  fixtures.GetTestReport("a-job-name", "test-name", "4.9"),
			}

			// If the test case doesn't specify which validator to run, run both
			if len(testCase.validators) == 0 {
				testCase.validators = map[string]stepmetrics.Validator{
					"UI":  stepmetrics.ValidateUIRequest,
					"API": stepmetrics.ValidateAPIRequest,
				}
			}

			for name, validator := range testCase.validators {
				t.Run(name, func(t *testing.T) {
					req, err := validator(opts)

					if testCase.expectedError != "" && err == nil {
						t.Errorf("expected error: %s, got nil", testCase.expectedError)
					}

					if err != nil && testCase.expectedError != err.Error() {
						t.Errorf("expected error %s, got: %s", testCase.expectedError, err)
					}

					// If the test case doesn't specify if we have an expected request,
					// use the one we provided since it should match.
					emptyRequest := stepmetrics.Request{}
					if testCase.expectedRequest == emptyRequest {
						testCase.expectedRequest = testCase.request
					}

					if err == nil && req != testCase.expectedRequest {
						t.Errorf("requests do not match, have: %v, want: %v", req, testCase.expectedRequest)
					}
				})
			}
		})
	}
}

func getURLValues(req stepmetrics.Request) url.Values {
	valMap := map[string]string{
		"jobName":           req.JobName,
		"multistageJobName": req.MultistageJobName,
		"release":           req.Release,
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
