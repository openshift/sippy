package generichtml_test

import (
	"strings"
	"testing"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
)

const release string = "4.9"

func TestJobAggregationResult(t *testing.T) {
	variantResults := sippyprocessingv1.VariantResults{}

	renderer := generichtml.NewJobAggregationResultRendererFromVariantResults("section-block-name", variantResults, release)

	expectedContents := []string{}

	result := renderer.ToHTML()

	for _, item := range expectedContents {
		if !strings.Contains(result, item) {
			t.Errorf("expected to contain %s", item)
		}
	}
}
