package jobrunscan

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"

	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

func TestValidateReEvalRequest(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty IDs",
			ids:     []string{},
			wantErr: true,
			errMsg:  "prow_job_build_ids is required",
		},
		{
			name:    "nil IDs",
			ids:     nil,
			wantErr: true,
			errMsg:  "prow_job_build_ids is required",
		},
		{
			name:    "valid single ID",
			ids:     []string{"1234567890"},
			wantErr: false,
		},
		{
			name:    "valid multiple IDs",
			ids:     []string{"111", "222", "333"},
			wantErr: false,
		},
		{
			name:    "non-numeric ID",
			ids:     []string{"abc"},
			wantErr: true,
			errMsg:  "invalid prow_job_build_id",
		},
		{
			name:    "exceeds max batch size",
			ids:     makeIDs(51),
			wantErr: true,
			errMsg:  "maximum 50 job runs per request",
		},
		{
			name:    "at max batch size",
			ids:     makeIDs(50),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateReEvalRequest(tt.ids)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateReEvalRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if got := err.Error(); !strings.Contains(got, tt.errMsg) {
					t.Errorf("error %q does not contain %q", got, tt.errMsg)
				}
			}
		})
	}
}

func TestFilterImplementedSymptoms(t *testing.T) {
	withLabels := pq.StringArray{"label-a"}
	symptoms := []jobrunscan.Symptom{
		{SymptomContent: jobrunscan.SymptomContent{ID: "s1", MatcherType: jobrunscan.MatcherTypeString, LabelIDs: withLabels}},
		{SymptomContent: jobrunscan.SymptomContent{ID: "s2", MatcherType: jobrunscan.MatcherTypeRegex, LabelIDs: withLabels}},
		{SymptomContent: jobrunscan.SymptomContent{ID: "s3", MatcherType: jobrunscan.MatcherTypeFile, LabelIDs: withLabels}},
		{SymptomContent: jobrunscan.SymptomContent{ID: "s4", MatcherType: jobrunscan.MatcherTypeCEL, LabelIDs: withLabels}},
		{SymptomContent: jobrunscan.SymptomContent{ID: "s5", MatcherType: "unknown", LabelIDs: withLabels}},
		{SymptomContent: jobrunscan.SymptomContent{ID: "s6", MatcherType: jobrunscan.MatcherTypeString}},
	}

	filtered := filterRelevantSymptoms(symptoms)
	if len(filtered) != 3 {
		t.Fatalf("expected 3 relevant symptoms, got %d", len(filtered))
	}

	ids := map[string]bool{}
	for _, s := range filtered {
		ids[s.ID] = true
	}
	for _, id := range []string{"s1", "s2", "s3"} {
		if !ids[id] {
			t.Errorf("expected symptom %q to be included", id)
		}
	}
	for _, id := range []string{"s4", "s5", "s6"} {
		if ids[id] {
			t.Errorf("expected symptom %q to be excluded", id)
		}
	}
}

func TestMergeLabels(t *testing.T) {
	tests := []struct {
		name         string
		manualLabels []string
		bqLabels     []models.JobRunLabel
		wantLabels   []string
	}{
		{
			name:         "empty both",
			manualLabels: nil,
			bqLabels:     nil,
			wantLabels:   nil,
		},
		{
			name:         "manual only",
			manualLabels: []string{"FlakeDetected", "InfraFailure"},
			bqLabels:     nil,
			wantLabels:   []string{"FlakeDetected", "InfraFailure"},
		},
		{
			name:         "symptom only",
			manualLabels: nil,
			bqLabels: []models.JobRunLabel{
				{Label: "DNSTimeout"},
				{Label: "NetworkError"},
			},
			wantLabels: []string{"DNSTimeout", "NetworkError"},
		},
		{
			name:         "mixed with dedup",
			manualLabels: []string{"InfraFailure", "DNSTimeout"},
			bqLabels: []models.JobRunLabel{
				{Label: "DNSTimeout"},
				{Label: "NetworkError"},
			},
			wantLabels: []string{"DNSTimeout", "InfraFailure", "NetworkError"},
		},
		{
			name:         "duplicate symptom labels deduplicated",
			manualLabels: nil,
			bqLabels: []models.JobRunLabel{
				{Label: "SameLabel", SymptomID: "s1"},
				{Label: "SameLabel", SymptomID: "s2"},
			},
			wantLabels: []string{"SameLabel"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeLabels(tt.manualLabels, tt.bqLabels)
			if !stringSliceEqual(got, tt.wantLabels) {
				t.Errorf("mergeLabels() = %v, want %v", got, tt.wantLabels)
			}
		})
	}
}

func TestUniqueSymptomsMatched(t *testing.T) {
	matches := []symptomMatch{
		{symptom: jobrunscan.Symptom{SymptomContent: jobrunscan.SymptomContent{ID: "s1"}}},
		{symptom: jobrunscan.Symptom{SymptomContent: jobrunscan.SymptomContent{ID: "s1"}}},
		{symptom: jobrunscan.Symptom{SymptomContent: jobrunscan.SymptomContent{ID: "s2"}}},
	}
	got := uniqueSymptomsMatched(matches)
	want := []string{"s1", "s2"}
	if !stringSliceEqual(got, want) {
		t.Errorf("uniqueSymptomsMatched() = %v, want %v", got, want)
	}

	if got := uniqueSymptomsMatched(nil); len(got) != 0 {
		t.Errorf("uniqueSymptomsMatched(nil) = %v, want empty", got)
	}
}

func TestUniqueLabels(t *testing.T) {
	labels := []models.JobRunLabel{
		{Label: "A"},
		{Label: "B"},
		{Label: "A"},
		{Label: "C"},
		{Label: "B"},
	}
	got := uniqueLabels(labels)
	want := []string{"A", "B", "C"}
	if !stringSliceEqual(got, want) {
		t.Errorf("uniqueLabels() = %v, want %v", got, want)
	}
}

func TestJobRunPathFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "standard URL",
			url:  "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.17-e2e-aws-ovn/1234567890",
			want: "logs/periodic-ci-openshift-release-master-nightly-4.17-e2e-aws-ovn/1234567890/",
		},
		{
			name: "URL with trailing slash",
			url:  "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/some-job/999/",
			want: "logs/some-job/999/",
		},
		{
			name: "no bucket root in URL",
			url:  "https://example.com/some/other/path",
			want: "",
		},
		{
			name: "empty URL",
			url:  "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobRunPathFromURL(tt.url); got != tt.want {
				t.Errorf("jobRunPathFromURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContentMatcherForSymptom(t *testing.T) {
	tests := []struct {
		name    string
		symptom jobrunscan.SymptomContent
		wantNil bool
		wantErr bool
	}{
		{
			name: "string matcher",
			symptom: jobrunscan.SymptomContent{
				MatcherType: jobrunscan.MatcherTypeString,
				MatchString: "error connecting",
			},
			wantNil: false,
		},
		{
			name: "regex matcher",
			symptom: jobrunscan.SymptomContent{
				MatcherType: jobrunscan.MatcherTypeRegex,
				MatchString: "timeout.*dns",
			},
			wantNil: false,
		},
		{
			name: "invalid regex",
			symptom: jobrunscan.SymptomContent{
				ID:          "bad",
				MatcherType: jobrunscan.MatcherTypeRegex,
				MatchString: "[invalid",
			},
			wantErr: true,
		},
		{
			name: "file existence matcher",
			symptom: jobrunscan.SymptomContent{
				MatcherType: jobrunscan.MatcherTypeFile,
			},
			wantNil: true,
		},
		{
			name: "unsupported matcher type",
			symptom: jobrunscan.SymptomContent{
				ID:          "test",
				MatcherType: "jq",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := ContentMatcherForSymptom(tt.symptom)
			if (err != nil) != tt.wantErr {
				t.Errorf("ContentMatcherForSymptom() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && (matcher == nil) != tt.wantNil {
				t.Errorf("ContentMatcherForSymptom() matcher nil = %v, wantNil %v", matcher == nil, tt.wantNil)
			}
		})
	}
}

func TestBuildOutputs(t *testing.T) {
	re := &ReEvaluator{gcsBucket: "test-bucket"}
	matches := []symptomMatch{
		{
			symptom: jobrunscan.Symptom{SymptomContent: jobrunscan.SymptomContent{
				ID:          "DNSTimeout",
				Summary:     "DNS Timeout detected",
				MatcherType: jobrunscan.MatcherTypeString,
				FilePattern: "**/build-log.txt",
				MatchString: "dns timeout",
				LabelIDs:    []string{"InfraFailure"},
			}},
			fileMatch: "artifacts/build-log.txt",
			textMatch: "dns timeout occurred at 12:34",
		},
	}

	jobRun := &models.ProwJobRun{
		URL: "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/test-job/12345",
	}

	bqLabels, bucketLabels, err := re.buildOutputs(matches, "12345", jobRun)
	if err != nil {
		t.Fatalf("buildOutputs() error = %v", err)
	}

	if len(bqLabels) != 1 {
		t.Fatalf("expected 1 BQ label, got %d", len(bqLabels))
	}

	bl := bqLabels[0]
	if bl.ID != "12345" {
		t.Errorf("BQ label ID = %q, want %q", bl.ID, "12345")
	}
	if bl.Label != "InfraFailure" {
		t.Errorf("BQ label Label = %q, want %q", bl.Label, "InfraFailure")
	}
	if bl.SourceTool != reEvalSourceTool {
		t.Errorf("BQ label SourceTool = %q, want %q", bl.SourceTool, reEvalSourceTool)
	}
	if bl.SymptomID != "DNSTimeout" {
		t.Errorf("BQ label SymptomID = %q, want %q", bl.SymptomID, "DNSTimeout")
	}
	if bl.StartTime == (civil.DateTime{}) {
		t.Error("BQ label StartTime should not be zero")
	}

	if len(bucketLabels) != 1 {
		t.Fatalf("expected 1 bucket label, got %d", len(bucketLabels))
	}
	gcs := bucketLabels[0]
	if gcs.Symptom.ID != "DNSTimeout" {
		t.Errorf("bucket label symptom ID = %q, want %q", gcs.Symptom.ID, "DNSTimeout")
	}
	if gcs.FileMatch != "artifacts/build-log.txt" {
		t.Errorf("bucket label FileMatch = %q, want %q", gcs.FileMatch, "artifacts/build-log.txt")
	}
	if gcs.TextMatch != "dns timeout occurred at 12:34" {
		t.Errorf("bucket label TextMatch = %q", gcs.TextMatch)
	}
	if gcs.Bucket != "test-bucket" {
		t.Errorf("bucket label Bucket = %q, want %q", gcs.Bucket, "test-bucket")
	}
	expectedPath := "logs/test-job/12345/"
	if gcs.JobRunPath != expectedPath {
		t.Errorf("bucket label JobRunPath = %q, want %q", gcs.JobRunPath, expectedPath)
	}
}

func TestBuildOutputsMultipleLabels(t *testing.T) {
	re := &ReEvaluator{gcsBucket: "test-bucket"}
	matches := []symptomMatch{
		{
			symptom: jobrunscan.Symptom{SymptomContent: jobrunscan.SymptomContent{
				ID:       "MultiLabel",
				LabelIDs: []string{"Label1", "Label2"},
			}},
			fileMatch: "test-file.txt",
		},
	}
	jobRun := &models.ProwJobRun{
		URL: "https://example.com/gs/test-platform-results/logs/job/1/",
	}

	bqLabels, bucketLabels, err := re.buildOutputs(matches, "1", jobRun)
	if err != nil {
		t.Fatalf("buildOutputs() error = %v", err)
	}
	if len(bqLabels) != 2 {
		t.Fatalf("expected 2 BQ labels, got %d", len(bqLabels))
	}
	if len(bucketLabels) != 2 {
		t.Fatalf("expected 2 bucket labels, got %d", len(bucketLabels))
	}
}

// helpers

func makeIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%d", 1000000000+i)
	}
	return ids
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
