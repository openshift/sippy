package generichtml_test

import (
	"strings"
	"testing"

	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
)

func TestJobResultRenderer(t *testing.T) {
	sectionBlock := "i dunno what this is..."

	release := "4.9"
	jobName := "job-name"

	renderer := generichtml.NewJobResultRendererFromJobResult(
		sectionBlock,
		htmltesthelpers.GetJobResult(jobName),
		release)

	result := renderer.ToHTML()

	expectedContents := []string{
		release,
		jobName,
		// "job-name",
		// "operator.Run multi-stage test e2e-aws - e2e-aws-ipi-install container test",
		// "unrelated-passing-test",
		// "unrelated-failing-test",
		// "e2e-aws",
		// "ipi-install",
	}

	for _, item := range expectedContents {
		if !strings.Contains(result, item) {
			t.Errorf("result did not contain: %s", item)
		}
	}
}
