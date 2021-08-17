package stepmetricshtml_test

import (
	"net/http/httptest"
	"testing"

	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

type tableTestCase struct {
	name             string
	request          stepmetricshtml.Request
	expectedContents []string
	expectedURLs     []stepmetricshtml.URLGenerator
}

func TestPrintTable(t *testing.T) {
	testCases := []tableTestCase{
		{
			name: "all multistage jobs",
			request: stepmetricshtml.Request{
				MultistageJobName: stepmetricshtml.All,
			},
			expectedURLs: []stepmetricshtml.URLGenerator{
				stepmetricshtml.StepRegistryURL{
					Search: "e2e-aws",
				},
				stepmetricshtml.StepRegistryURL{
					Search: "e2e-gcp",
				},
				stepmetricshtml.SippyURL{
					Release:           release,
					MultistageJobName: "e2e-aws",
				},
				stepmetricshtml.SippyURL{
					Release:           release,
					MultistageJobName: "e2e-gcp",
				},
			},
			expectedContents: []string{
				"All Multistage Jobs",
				"<td>e2e-aws",
				"<td>e2e-gcp",
				"100.00% (1 runs)",
			},
		},
		{
			name: "specific multistage name",
			request: stepmetricshtml.Request{
				MultistageJobName: "e2e-aws",
			},
			expectedURLs: getExpectedURLsForMultistage("e2e-aws"),
			expectedContents: []string{
				"All Step Names For Multistage Job e2e-aws",
			},
		},
		{
			name: "all step names",
			request: stepmetricshtml.Request{
				StepName: stepmetricshtml.All,
			},
			expectedURLs: getExpectedURLsForAllSteps(),
			expectedContents: []string{
				"Step Metrics For All",
				"<td>aws-specific",
				"<td>gcp-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"100.00% (1 runs)",
			},
		},
		{
			name: "specific step name - openshift-e2e-test",
			request: stepmetricshtml.Request{
				StepName: "openshift-e2e-test",
			},
			expectedURLs: getExpectedURLsForStep("openshift-e2e-test"),
			expectedContents: []string{
				"Step Metrics For openshift-e2e-test By Multistage Job Name",
				"<td>e2e-aws",
				"<td>e2e-gcp",
				"100.00% (1 runs)",
			},
		},
		{
			name: "specific step name - aws-specific",
			request: stepmetricshtml.Request{
				StepName: "aws-specific",
			},
			expectedURLs: getExpectedURLsForStep("aws-specific"),
			expectedContents: []string{
				"Step Metrics For aws-specific By Multistage Job Name",
				"<td>e2e-aws",
				"100.00% (1 runs)",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testFunc := func(r *httptest.ResponseRecorder) {
				table := stepmetricshtml.NewStepMetricsHTMLTable(
					htmltesthelpers.GetTestReport(jobName, "test-name", release),
					htmltesthelpers.GetTestReport(jobName, "test-name", release),
				)
				table.Render(r, testCase.request)
			}

			expectedContents := append([]string{}, testCase.expectedContents...)

			for _, u := range testCase.expectedURLs {
				expectedContents = append(expectedContents, u.URL().String())
			}

			htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
			htmltesthelpers.PrintHTML(t, testFunc)
		})
	}
}

func getExpectedURLsForAllSteps() []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	for stepName := range htmltesthelpers.GetByStageName() {
		urls = append(urls,
			stepmetricshtml.StepRegistryURL{
				Reference: stepName,
			},
			stepmetricshtml.SippyURL{
				Release:  release,
				StepName: stepName,
			},
		)
	}

	return urls
}

func getExpectedURLsForMultistage(multistageName string) []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	for _, stageResult := range htmltesthelpers.GetByMultistageName()[multistageName].StageResults {
		urls = append(urls,
			stepmetricshtml.SippyURL{
				Release:  release,
				StepName: stageResult.Name,
			},
			stepmetricshtml.CISearchURL{
				Release: release,
				Search:  stageResult.OriginalTestName,
			},
			stepmetricshtml.StepRegistryURL{
				Reference: stageResult.Name,
			},
		)
	}

	return urls
}

func getExpectedURLsForStep(stepName string) []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	for multistageName, multistageResult := range htmltesthelpers.GetByStageName()[stepName].ByMultistageName {
		urls = append(urls,
			stepmetricshtml.StepRegistryURL{
				Search: multistageName,
			},
			stepmetricshtml.CISearchURL{
				Release: release,
				Search:  multistageResult.OriginalTestName,
			},
			stepmetricshtml.SippyURL{
				Release:           release,
				MultistageJobName: multistageName,
			},
		)
	}

	return urls
}
