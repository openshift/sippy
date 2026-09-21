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

// TestGenerateHTMLSummaryWithoutJiraLinks covers legacy and explicitly empty bug lists.
func TestGenerateHTMLSummaryWithoutJiraLinks(t *testing.T) {
	for _, test := range []struct {
		name string
		bugs pq.StringArray
	}{
		{name: "legacy null bugs"},
		{name: "empty bugs", bugs: pq.StringArray{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			labels := map[string][]JobRunBucketLabelContainer{
				"KnownFailure": {
					{V1: &JobRunBucketLabel{
						Label: jobrunscan.LabelContent{
							ID:          "KnownFailure",
							LabelTitle:  "Known failure",
							Explanation: "A known failure mode.",
							Bugs:        test.bugs,
						},
						Symptom: jobrunscan.SymptomContent{Summary: "Known symptom"},
					}},
				},
			}
			html := generateHTMLSummary(labels)
			for _, expected := range []string{"Known failure", "A known failure mode.", "Known symptom"} {
				if !strings.Contains(html, expected) {
					t.Errorf("expected generated HTML to contain %q, got %s", expected, html)
				}
			}
			if strings.Contains(html, "Bugs:") || strings.Contains(html, jiraIssueURLPrefix) {
				t.Errorf("expected no Jira section, got %s", html)
			}
		})
	}
}
