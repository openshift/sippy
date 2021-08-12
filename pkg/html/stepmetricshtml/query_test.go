package stepmetricshtml_test

import (
	"testing"

	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
	"github.com/openshift/sippy/pkg/util/sets"
)

func TestSippyURL(t *testing.T) {
	testCases := []struct {
		name                         string
		query                        stepmetricshtml.SippyURL
		expectedValidateError        string
		expectedValidateReportsError string
	}{
		{
			name: "sunny day",
			query: stepmetricshtml.SippyURL{
				Release:           "4.9",
				MultistageJobName: "e2e-aws",
			},
		},
		{
			name: "all multistage job names",
			query: stepmetricshtml.SippyURL{
				Release:           "4.9",
				MultistageJobName: stepmetricshtml.All,
			},
		},
		{
			name: "missing release",
			query: stepmetricshtml.SippyURL{
				MultistageJobName: "e2e-aws",
			},
			expectedValidateError: "missing release",
		},
		{
			name: "unknown release",
			query: stepmetricshtml.SippyURL{
				Release:           "unknown-release",
				MultistageJobName: "e2e-aws",
			},
			expectedValidateError: "invalid release unknown-release",
		},
		{
			name: "unknown multistage job name",
			query: stepmetricshtml.SippyURL{
				Release:           "4.9",
				MultistageJobName: "unknown-multistage-name",
			},
			expectedValidateReportsError: "invalid multistage job name unknown-multistage-name",
		},
		{
			name: "all step names",
			query: stepmetricshtml.SippyURL{
				Release:  "4.9",
				StepName: stepmetricshtml.All,
			},
		},
		{
			name: "unknown step name",
			query: stepmetricshtml.SippyURL{
				Release:  "4.9",
				StepName: "unknown-step-name",
			},
			expectedValidateReportsError: "unknown step name unknown-step-name",
		},
		{
			name: "unknown variant",
			query: stepmetricshtml.SippyURL{
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

			httpQuery := stepmetricshtml.StepMetricsHTTPQuery{
				SippyURL: testCase.query,
			}

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
		})
	}
}
