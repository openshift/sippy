package stepmetricshtml_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

const (
	release string = "4.9"
	jobName string = "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws"
)

type tableTestCase struct {
	name             string
	response         stepmetrics.Response
	expectedContents []string
	expectedURLs     []stepmetricshtml.URLGenerator
}

func TestPrintTable(t *testing.T) {
	testCases := []tableTestCase{
		{
			name:     "all multistage jobs",
			response: htmltesthelpers.GetAllMultistageResponse(),
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
			name:         "specific multistage name - e2e-aws",
			response:     htmltesthelpers.GetSpecificMultistageResponse("e2e-aws"),
			expectedURLs: getExpectedURLsForMultistage("e2e-aws"),
			expectedContents: []string{
				"All Step Names for Multistage Job e2e-aws",
				"<td>aws-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"100.00% (1 runs)",
			},
		},
		{
			name:         "all step names",
			response:     htmltesthelpers.GetAllStepsResponse(),
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
			name:         "specific step name - openshift-e2e-test",
			response:     htmltesthelpers.GetSpecificStepNameResponse("openshift-e2e-test"),
			expectedURLs: getExpectedURLsForStep("openshift-e2e-test"),
			expectedContents: []string{
				"Step Metrics For openshift-e2e-test By Multistage Job Name",
				"<td>e2e-aws",
				"<td>e2e-gcp",
				"100.00% (1 runs)",
			},
		},
		{
			name:         "specific step name - aws-specific",
			response:     htmltesthelpers.GetSpecificStepNameResponse("aws-specific"),
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
			table := stepmetricshtml.NewStepMetricsHTMLTable(release, time.Now())

			expectedContents := append([]string{}, testCase.expectedContents...)

			for _, u := range testCase.expectedURLs {
				expectedContents = append(expectedContents, u.URL().String())
			}

			result := ""

			if testCase.response.Request.MultistageJobName == stepmetrics.All {
				result = table.AllMultistages(testCase.response).ToHTML()
			} else if testCase.response.Request.MultistageJobName != "" {
				result = table.MultistageDetail(testCase.response).ToHTML()
			}

			if testCase.response.Request.StepName == stepmetrics.All {
				result = table.AllStages(testCase.response).ToHTML()
			} else if testCase.response.Request.StepName != "" {
				result = table.StageDetail(testCase.response).ToHTML()
			}

			for _, item := range expectedContents {
				if !strings.Contains(result, item) {
					t.Errorf("expected to contain %s", item)
				}
			}

			testFunc := func(r *httptest.ResponseRecorder) {
				if err := table.RenderResponse(r, testCase.response); err != nil {
					t.Errorf("expected no errors, got: %s", err)
				}
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

func getStageResult(name, originalName string, passes, fails int) sippyprocessingv1.StageResult {
	return sippyprocessingv1.StageResult{
		TestResult: sippyprocessingv1.TestResult{
			Name:           name,
			Successes:      passes,
			Failures:       fails,
			PassPercentage: float64(passes) / float64(passes+fails) * 100,
		},
		OriginalTestName: originalName,
		Runs:             passes + fails,
	}
}
