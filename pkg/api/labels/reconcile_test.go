package labels

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDeriveJobRunPathFromURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantDir string
		wantErr bool
	}{
		{
			name:    "release job",
			url:     "https://prow.example/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.18-e2e-aws/123456/",
			wantDir: "logs/periodic-ci-openshift-release-master-ci-4.18-e2e-aws/123456/",
		},
		{
			name:    "pull job",
			url:     "https://prow.example/view/gs/test-platform-results/pr-logs/pull/openshift/origin/123/pull-ci-openshift-origin-main-e2e-aws/987654/",
			wantDir: "pr-logs/pull/openshift/origin/123/pull-ci-openshift-origin-main-e2e-aws/987654/",
		},
		{name: "missing GCS path", url: "https://prow.example/view/job/123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, err := DeriveJobRunPathFromURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DeriveJobRunPathFromURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && gotDir != tt.wantDir {
				t.Fatalf("DeriveJobRunPathFromURL() = %q, want %q", gotDir, tt.wantDir)
			}
		})
	}
}

func TestParseLabelFile(t *testing.T) {
	data, err := json.Marshal(map[string]any{
		"symptom_label_v1": map[string]any{
			"label": map[string]any{
				"id":          "KnownFlake",
				"label_title": "Known flaky test",
				"explanation": "test explanation",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := ParseLabelFile(data)
	if err != nil {
		t.Fatalf("ParseLabelFile() error = %v", err)
	}
	want := DiscoveredLabel{ID: "KnownFlake", Title: "Known flaky test", Explanation: "test explanation"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseLabelFile() = %#v, want %#v", got, want)
	}

	for _, malformed := range [][]byte{[]byte("{"), []byte(`{"symptom_label_v1":null}`), []byte(`{"symptom_label_v1":{"label":{"id":""}}}`)} {
		if _, err := ParseLabelFile(malformed); err == nil {
			t.Errorf("ParseLabelFile(%s) succeeded, want error", malformed)
		}
	}
}

func TestDiffLabelsFullSet(t *testing.T) {
	current := map[string]DiscoveredLabel{
		"retain": {ID: "retain", Title: "same"},
		"update": {ID: "update", Title: "old"},
		"remove": {ID: "remove", Title: "gone"},
	}
	desired := map[string]DiscoveredLabel{
		"retain": {ID: "retain", Title: "same"},
		"update": {ID: "update", Title: "new"},
		"add":    {ID: "add", Title: "new"},
	}

	got := DiffLabels(current, desired)
	want := LabelDiff{
		Add:    []DiscoveredLabel{{ID: "add", Title: "new"}},
		Update: []DiscoveredLabel{{ID: "update", Title: "new"}},
		Remove: []string{"remove"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiffLabels() = %#v, want %#v", got, want)
	}

	empty := DiffLabels(current, nil)
	if len(empty.Add) != 0 || len(empty.Update) != 0 || !reflect.DeepEqual(empty.Remove, []string{"remove", "retain", "update"}) {
		t.Fatalf("DiffLabels(empty) = %#v, want all current labels removed", empty)
	}
}
