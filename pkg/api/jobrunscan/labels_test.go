package jobrunscan

import (
	"encoding/json"
	"testing"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

func TestNormalizeLabelBugs(t *testing.T) {
	label := jobrunscan.Label{}

	normalizeLabelBugs(&label)

	encoded, err := json.Marshal(label)
	if err != nil {
		t.Fatalf("marshalling label: %v", err)
	}
	if string(encoded) == "" || !json.Valid(encoded) {
		t.Fatalf("expected valid JSON, got %q", encoded)
	}
	var response map[string]any
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("unmarshalling label response: %v", err)
	}
	bugs, ok := response["bugs"].([]any)
	if !ok || len(bugs) != 0 {
		t.Fatalf("expected bugs to be an empty array, got %#v", response["bugs"])
	}
}

func TestValidateLabelJiraKeys(t *testing.T) {
	tests := []struct {
		name    string
		bugs    pq.StringArray
		wantErr bool
	}{
		{name: "no bugs"},
		{name: "one bug", bugs: pq.StringArray{"OCPBUGS-12345"}},
		{name: "multiple projects", bugs: pq.StringArray{"TRT-2896", "RHOAIENG-42"}},
		{name: "lowercase project", bugs: pq.StringArray{"Ocpbugs-12345"}, wantErr: true},
		{name: "missing issue number", bugs: pq.StringArray{"OCPBUGS"}, wantErr: true},
		{name: "full URL", bugs: pq.StringArray{"https://redhat.atlassian.net/browse/OCPBUGS-12345"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			label := jobrunscan.Label{
				LabelContent: jobrunscan.LabelContent{
					ID:         "TestLabel",
					LabelTitle: "Test label",
					Bugs:       test.bugs,
				},
			}
			err := validateLabel(label)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateLabel() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}
