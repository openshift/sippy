package stepmetricshtml_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
	"github.com/openshift/sippy/pkg/util/sets"
)

func TestHTTPQuery(t *testing.T) {
	testCases := []struct {
		name                         string
		request                      stepmetricshtml.Request
		expectedValidateError        string
		expectedValidateReportsError string
	}{
		{
			name: "sunny day",
			request: stepmetricshtml.Request{
				Release:           "4.9",
				MultistageJobName: "e2e-aws",
			},
		},
		{
			name: "all multistage job names",
			request: stepmetricshtml.Request{
				Release:           "4.9",
				MultistageJobName: stepmetricshtml.All,
			},
		},
		{
			name: "missing release",
			request: stepmetricshtml.Request{
				MultistageJobName: "e2e-aws",
			},
			expectedValidateError: "missing release",
		},
		{
			name: "unknown release",
			request: stepmetricshtml.Request{
				Release:           "unknown-release",
				MultistageJobName: "e2e-aws",
			},
			expectedValidateError: "invalid release unknown-release",
		},
		{
			name: "unknown multistage job name",
			request: stepmetricshtml.Request{
				Release:           "4.9",
				MultistageJobName: "unknown-multistage-name",
			},
			expectedValidateReportsError: "invalid multistage job name unknown-multistage-name",
		},
		{
			name: "all step names",
			request: stepmetricshtml.Request{
				Release:  "4.9",
				StepName: stepmetricshtml.All,
			},
		},
		{
			name: "unknown step name",
			request: stepmetricshtml.Request{
				Release:  "4.9",
				StepName: "unknown-step-name",
			},
			expectedValidateReportsError: "unknown step name unknown-step-name",
		},
		{
			name: "unknown variant",
			request: stepmetricshtml.Request{
				Release:  "4.9",
				StepName: "gcp-specific",
				Variant:  "unknown-variant",
			},
			expectedValidateError: "unknown variant unknown-variant",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			knownReleases := sets.NewString("4.9")

			httpQuery := stepmetricshtml.NewStepMetricsHTTPQuery(getHTTPRequest(testCase.request))

			validateErr := httpQuery.Validate(knownReleases)
			if validateErr != nil && testCase.expectedValidateError != validateErr.Error() {
				t.Errorf("expected error %s, got: %s", testCase.expectedValidateError, validateErr)
			}

			current := htmltesthelpers.GetTestReport("a-job-name", "test-name", "4.9")
			previous := htmltesthelpers.GetTestReport("a-job-name", "test-name", "4.9")

			validateReportErr := httpQuery.ValidateFromReports(current, previous)
			if validateReportErr != nil && testCase.expectedValidateReportsError != validateReportErr.Error() {
				t.Errorf("expected error %s, got: %s", testCase.expectedValidateReportsError, validateReportErr)
			}

			if httpQuery.Request() != testCase.request {
				t.Errorf("requests do not match, have: %v, want: %v", httpQuery.Request(), testCase.request)
			}
		})
	}
}

func getHTTPRequest(req stepmetricshtml.Request) *http.Request {
	valMap := map[string]string{
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

	u := &url.URL{
		RawQuery: values.Encode(),
	}

	return &http.Request{
		URL: u,
	}
}
