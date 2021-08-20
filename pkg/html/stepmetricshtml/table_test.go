package stepmetricshtml_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/api/stepmetrics/fixtures"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

const (
	release string = "4.9"
)

type tableTestCase struct {
	name             string
	request          stepmetrics.Request
	expectedContents []string
	expectedURLs     []stepmetricshtml.URLGenerator
}

func (tc tableTestCase) allExpectedContents() []string {
	expectedContents := append([]string{}, tc.expectedContents...)

	for _, u := range tc.expectedURLs {
		expectedContents = append(expectedContents, u.URL().String())
		expectedContents = append(expectedContents, u.ToHTML())
	}

	return expectedContents
}

func TestPrintTableRaw(t *testing.T) {
	testCases := []tableTestCase{
		{
			name: "all multistage jobs",
			request: stepmetrics.Request{
				MultistageJobName: stepmetrics.All,
				Release:           fixtures.Release,
			},
			expectedURLs: getExpectedURLsForAllMultistages(),
			expectedContents: []string{
				"All Multistage Job Names",
				"Multistage Job Name",
				"<td>e2e-aws",
				"<td>e2e-gcp",
				"50.00% (2 runs)",
				"href=\"#AllMultistageJobNames\"",
				"id=\"AllMultistageJobNames\"",
			},
		},
		{
			name: "specific multistage name - e2e-aws",
			request: stepmetrics.Request{
				MultistageJobName: "e2e-aws",
				Release:           fixtures.Release,
			},
			expectedURLs: getExpectedURLsForMultistage("e2e-aws"),
			expectedContents: []string{
				"All Step Names for Multistage Job e2e-aws",
				"Step Name",
				"<td>aws-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"50.00% (2 runs)",
				"href=\"#AllStepNamesForMultistageJobE2eAws\"",
				"id=\"AllStepNamesForMultistageJobE2eAws\"",
			},
		},
		{
			name: "all step names",
			request: stepmetrics.Request{
				Release:  fixtures.Release,
				StepName: stepmetrics.All,
			},
			expectedURLs: getExpectedURLsForAllSteps(),
			expectedContents: []string{
				"Step Metrics For All",
				"Step Name",
				"<td>aws-specific",
				"<td>gcp-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"50.00% (2 runs)",
				"50.00% (4 runs)",
				"href=\"#StepMetricsForAllSteps\"",
				"id=\"StepMetricsForAllSteps\"",
			},
		},
		{
			name: "specific step name - openshift-e2e-test",
			request: stepmetrics.Request{
				Release:  fixtures.Release,
				StepName: "openshift-e2e-test",
			},
			expectedURLs: getExpectedURLsForStep("openshift-e2e-test"),
			expectedContents: []string{
				"Step Metrics For openshift-e2e-test By Multistage Job Name",
				"Multistage Job Name",
				"<td>e2e-aws",
				"<td>e2e-gcp",
				"50.00% (2 runs)",
				"href=\"#StepMetricsForOpenshiftE2eTestByMultistageJobName\"",
				"id=\"StepMetricsForOpenshiftE2eTestByMultistageJobName\"",
			},
		},
		{
			name: "specific step name - aws-specific",
			request: stepmetrics.Request{
				Release:  fixtures.Release,
				StepName: "aws-specific",
			},
			expectedURLs: getExpectedURLsForStep("aws-specific"),
			expectedContents: []string{
				"Step Metrics For aws-specific By Multistage Job Name",
				"Multistage Job Name",
				"<td>e2e-aws",
				"50.00% (2 runs)",
				"href=\"#StepMetricsForAwsSpecificByMultistageJobName\"",
				"id=\"StepMetricsForAwsSpecificByMultistageJobName\"",
			},
		},
		{
			name: "by job name",
			request: stepmetrics.Request{
				Release: fixtures.Release,
				JobName: fixtures.AwsJobName,
			},
			expectedURLs: getExpectedURLsForJobName(),
			expectedContents: []string{
				fixtures.AwsJobName,
				"Step Name",
				"Multistage Job Name",
				"<td>aws-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"50.00% (2 runs)",
				"<td>e2e-aws",
			},
		},
		{
			name: "all jobs",
			request: stepmetrics.Request{
				Release: fixtures.Release,
				JobName: stepmetrics.All,
			},
			expectedURLs: getExpectedURLsForJobName(),
			expectedContents: []string{
				fixtures.AwsJobName,
				fixtures.GcpJobName,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tableReq := stepmetricshtml.NewTableRequest(
				fixtures.GetTestReport(fixtures.AwsJobName, "a-test", release),
				fixtures.GetTestReport(fixtures.AwsJobName, "a-test", release),
				testCase.request,
			)

			rendered, err := stepmetricshtml.RenderRequest(tableReq, time.Now())
			if err != nil {
				t.Errorf("expected no errors, got: %s", err)
			}

			assertContains(t, rendered, testCase.allExpectedContents())
		})
	}
}

func TestPrintLandingPage(t *testing.T) {
	testCases := []tableTestCase{
		{
			name:         "landing page",
			expectedURLs: append(getExpectedURLsForAllSteps(), getExpectedURLsForAllMultistages()...),
			expectedContents: []string{
				fmt.Sprintf(generichtml.HTMLPageStart, "Step Metrics For 4.9"),
				fmt.Sprintf(generichtml.HTMLPageEnd, time.Now().Format("Jan 2 15:04 2006 MST")),
				"All Multistage Job Names",
				"Multistage Job Name",
				"<td>e2e-aws",
				"<td>e2e-gcp",
				"href=\"#AllMultistageJobNames\"",
				"id=\"AllMultistageJobNames\"",
				"Step Metrics For All",
				"Step Name",
				"<td>aws-specific",
				"<td>gcp-specific",
				"<td>openshift-e2e-test",
				"<td>ipi-install",
				"50.00% (2 runs)",
				"50.00% (4 runs)",
				"href=\"#StepMetricsForAllSteps\"",
				"id=\"StepMetricsForAllSteps\"",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tableReq := stepmetricshtml.NewTableRequest(
				fixtures.GetTestReport(fixtures.AwsJobName, "a-test", release),
				fixtures.GetTestReport(fixtures.AwsJobName, "a-test", release),
				stepmetrics.Request{
					Release: fixtures.Release,
				})

			out := stepmetricshtml.PrintLandingPage(tableReq, time.Now())

			assertContains(t, out, testCase.allExpectedContents())
		})
	}
}

// Assertions
func assertContains(t *testing.T, result string, expectedContents []string) {
	t.Helper()

	for _, item := range expectedContents {
		if !strings.Contains(result, item) {
			t.Errorf("expected result to contain: %s", item)
		}
	}
}

func getExpectedURLsForAllMultistages() []stepmetricshtml.URLGenerator {
	return []stepmetricshtml.URLGenerator{
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
		stepmetricshtml.CISearchURL{
			Release: release,
			Search:  "operator.Run multi-stage test e2e-aws",
		},
		stepmetricshtml.CISearchURL{
			Release: release,
			Search:  "operator.Run multi-stage test e2e-gcp",
		},
	}
}

func getExpectedURLsForAllSteps() []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	for stepName := range fixtures.GetByStageName() {
		urls = append(urls,
			stepmetricshtml.StepRegistryURL{
				Reference: stepName,
			},
			stepmetricshtml.SippyURL{
				Release:  release,
				StepName: stepName,
			},
			stepmetricshtml.CISearchURL{
				Release:     release,
				SearchRegex: fmt.Sprintf(`operator\.Run multi-stage test .*-%s container test`, stepName),
			},
		)
	}

	return urls
}

func getExpectedURLsForMultistage(multistageName string) []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	for _, stageResult := range fixtures.GetByMultistageName()[multistageName].StageResults {
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

	byJobName := fixtures.GetByJobName()[fixtures.AwsJobName]

	for _, stageResult := range byJobName.StepRegistryMetrics.StageResults {
		urls = append(urls,
			stepmetricshtml.StepRegistryURL{
				Reference: stageResult.Name,
			},
			stepmetricshtml.CISearchURL{
				Search:   stageResult.OriginalTestName,
				JobRegex: fmt.Sprintf("^%s$", regexp.QuoteMeta(fixtures.AwsJobName)),
			},
			stepmetricshtml.SippyURL{
				Release:  release,
				StepName: stageResult.Name,
			},
		)
	}

	return urls
}

func getExpectedURLsForStep(stepName string) []stepmetricshtml.URLGenerator {
	urls := []stepmetricshtml.URLGenerator{}

	byMultistageName := fixtures.GetByStageName()[stepName].ByMultistageName

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
