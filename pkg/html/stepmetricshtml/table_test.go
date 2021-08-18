package stepmetricshtml_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

const (
	jobName string = "periodic-ci-openshift-release-master-ci-4.9-e2e-aws"
	release string = "4.9"
)

type tableTestCase struct {
	name             string
	response         stepmetrics.Response
	expectedContents []string
	expectedURLs     []stepmetricshtml.URLGenerator
	tableFunc        func(stepmetrics.Response) (string, error)
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
				"All Multistage Job Names",
				"Multistage Job Name",
				"<td>e2e-aws",
				"<td>e2e-gcp",
				"100.00% (1 runs)",
				"href=\"#AllMultistageJobNames\"",
				"id=\"AllMultistageJobNames\"",
			},
			tableFunc: stepmetricshtml.AllMultistages,
		},
		{
			name:         "specific multistage name - e2e-aws",
			response:     htmltesthelpers.GetSpecificMultistageResponse("e2e-aws"),
			expectedURLs: getExpectedURLsForMultistage("e2e-aws"),
			expectedContents: []string{
				"All Step Names for Multistage Job e2e-aws",
				"Step Name",
				"<td>aws-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"100.00% (1 runs)",
				"href=\"#AllStepNamesForMultistageJobE2eAws\"",
				"id=\"AllStepNamesForMultistageJobE2eAws\"",
			},
			tableFunc: stepmetricshtml.MultistageDetail,
		},
		{
			name:         "all step names",
			response:     htmltesthelpers.GetAllStepsResponse(),
			expectedURLs: getExpectedURLsForAllSteps(),
			expectedContents: []string{
				"Step Metrics For All",
				"Step Name",
				"<td>aws-specific",
				"<td>gcp-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"100.00% (1 runs)",
				"100.00% (2 runs)",
				"href=\"#StepMetricsForAllSteps\"",
				"id=\"StepMetricsForAllSteps\"",
			},
			tableFunc: stepmetricshtml.AllSteps,
		},
		{
			name:         "specific step name - openshift-e2e-test",
			response:     htmltesthelpers.GetSpecificStepNameResponse("openshift-e2e-test"),
			expectedURLs: getExpectedURLsForStep("openshift-e2e-test"),
			expectedContents: []string{
				"Step Metrics For openshift-e2e-test By Multistage Job Name",
				"Multistage Job Name",
				"<td>e2e-aws",
				"<td>e2e-gcp",
				"100.00% (1 runs)",
				"href=\"#StepMetricsForOpenshiftE2eTestByMultistageJobName\"",
				"id=\"StepMetricsForOpenshiftE2eTestByMultistageJobName\"",
			},
			tableFunc: stepmetricshtml.StepDetail,
		},
		{
			name:         "specific step name - aws-specific",
			response:     htmltesthelpers.GetSpecificStepNameResponse("aws-specific"),
			expectedURLs: getExpectedURLsForStep("aws-specific"),
			expectedContents: []string{
				"Step Metrics For aws-specific By Multistage Job Name",
				"Multistage Job Name",
				"<td>e2e-aws",
				"100.00% (1 runs)",
				"href=\"#StepMetricsForAwsSpecificByMultistageJobName\"",
				"id=\"StepMetricsForAwsSpecificByMultistageJobName\"",
			},
			tableFunc: stepmetricshtml.StepDetail,
		},
		{
			name:         "by job name",
			response:     htmltesthelpers.GetByJobNameResponse(),
			expectedURLs: getExpectedURLsForJobName(),
			expectedContents: []string{
				htmltesthelpers.GetByJobNameResponse().Request.JobName,
				"Step Name",
				"Multistage Job Name",
				"<td>aws-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"100.00% (1 runs)",
				"<td>e2e-aws",
			},
			tableFunc: stepmetricshtml.ByJob,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			expectedContents := append([]string{}, testCase.expectedContents...)

			for _, u := range testCase.expectedURLs {
				expectedContents = append(expectedContents, u.URL().String())
				expectedContents = append(expectedContents, u.ToHTML())
			}

			result, err := testCase.tableFunc(testCase.response)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			rendered, err := stepmetricshtml.RenderResponse(testCase.response, time.Now())
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			fmt.Println(rendered)

			if !strings.Contains(rendered, result) {
				t.Errorf("result not in rendered")
			}

			for _, item := range expectedContents {
				if !strings.Contains(result, item) {
					t.Errorf("expected to contain %s", item)
				}

				if !strings.Contains(rendered, item) {
					t.Errorf("expected rendered table to contain %s", item)
				}
			}
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

func getExpectedURLsForJobName() []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	byJobName := htmltesthelpers.GetByJobName(jobName)[jobName]

	for _, stageResult := range byJobName.StepRegistryMetrics.StageResults {
		urls = append(urls,
			stepmetricshtml.StepRegistryURL{
				Reference: stageResult.Name,
			},
			stepmetricshtml.CISearchURL{
				Release: release,
				Search:  stageResult.OriginalTestName,
			},
			stepmetricshtml.SippyURL{
				Release:  release,
				StepName: stageResult.Name,
			},
		)
	}

	return urls
}

func getExpectedURLsForSteps(stepNames []string) []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	for _, stepName := range stepNames {
		urls = append(urls, getExpectedURLsForStep(stepName)...)
	}

	return urls
}

func getExpectedURLsForStep(stepName string) []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	byMultistageName := htmltesthelpers.GetByStageName()[stepName].ByMultistageName

	for multistageName, multistageResult := range byMultistageName {
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
