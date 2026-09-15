package jobrunannotator

import (
	"strings"
	"testing"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

func TestGenerateHTMLSummaryIncludesJiraLinks(t *testing.T) {
	labels := map[string][]JobRunBucketLabelContainer{
		"KnownFailure": {
			{V1: &JobRunBucketLabel{
				Label: jobrunscan.LabelContent{
					ID:          "KnownFailure",
					LabelTitle:  "Known failure",
					Explanation: "A known failure mode.",
					Bugs:        pq.StringArray{"OCPBUGS-12345", "TRT-2896"},
				},
				Symptom: jobrunscan.SymptomContent{Summary: "Known symptom"},
			}},
		},
	}

	html := generateHTMLSummary(labels)

	for _, expected := range []string{
		"Bugs:",
		`href="https://redhat.atlassian.net/browse/OCPBUGS-12345"`,
		`href="https://redhat.atlassian.net/browse/TRT-2896"`,
		`rel="noopener noreferrer"`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("expected generated HTML to contain %q, got %s", expected, html)
		}
	}
}
