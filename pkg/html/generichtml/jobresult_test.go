package generichtml_test

import (
	"strings"
	"testing"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
)

func TestJobResultRenderer(t *testing.T) {
	sectionBlock := "i dunno what this is..."

	release := "4.9"
	jobName := "job-name"

	renderer := generichtml.NewJobResultRendererFromJobResult(
		sectionBlock,
		getJobResult(jobName),
		release)

	result := renderer.ToHTML()

	expectedContents := []string{
		release,
		jobName,
	}

	for _, item := range expectedContents {
		if !strings.Contains(result, item) {
			t.Errorf("result did not contain: %s", item)
		}
	}
}

func getJobResult(jobName string) sippyprocessingv1.JobResult {
	return sippyprocessingv1.JobResult{
		Name:      jobName,
		Successes: 2,
		Failures:  1,
		StepRegistryMetrics: sippyprocessingv1.StepRegistryMetrics{
			MultistageName: "e2e-aws",
			StageResults: map[string]sippyprocessingv1.StageResult{
				"ipi-install": {
					TestResult: sippyprocessingv1.TestResult{
						Name:           "ipi-install",
						Successes:      1,
						Failures:       0,
						PassPercentage: 100,
					},
					Runs: 1,
				},
			},
		},
		TestResults: []sippyprocessingv1.TestResult{
			{
				Name:           "unrelated-passing-test",
				Successes:      1,
				Failures:       0,
				PassPercentage: 100,
			},
			{
				Name:           "unrelated-failing-test",
				Successes:      0,
				Failures:       1,
				PassPercentage: 0,
			},
		},
	}
}
