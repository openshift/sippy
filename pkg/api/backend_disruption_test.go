package api

import (
	"encoding/json"
	"testing"

	apitype "github.com/openshift/sippy/pkg/apis/api"
)

func TestBackendDisruptionRunsResultSerialization(t *testing.T) {
	result := apitype.BackendDisruptionRunsResult{
		Rows: []apitype.BackendDisruptionRunRow{
			{
				BackendName:        "kube-api-new-connections",
				DisruptionSeconds:  12,
				JobName:            "periodic-ci-openshift-release-master-ci-5.0-e2e-gcp-ovn-upgrade",
				JobRunName:         "2084247445587365888",
				JobRunStartTime:    "2026-08-01 14:23:00 UTC",
				JobRunEndTime:      "2026-08-01 16:45:00 UTC",
				Cluster:            "build01",
				ReleaseTag:         "5.0.0-0.ci-2026-08-01-142300",
				MasterNodesUpdated: "Y",
				JobRunStatus:       "failure",
			},
			{
				BackendName:       "kube-api-reused-connections",
				DisruptionSeconds: 0,
				JobName:           "periodic-ci-openshift-release-master-ci-5.0-e2e-gcp-ovn-upgrade",
				JobRunName:        "2084247445587365888",
				JobRunStartTime:   "2026-08-01 14:23:00 UTC",
				JobRunEndTime:     "2026-08-01 16:45:00 UTC",
				Cluster:           "build01",
				ReleaseTag:        "5.0.0-0.ci-2026-08-01-142300",
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal BackendDisruptionRunsResult: %v", err)
	}

	var decoded apitype.BackendDisruptionRunsResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal BackendDisruptionRunsResult: %v", err)
	}

	if len(decoded.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(decoded.Rows))
	}

	row := decoded.Rows[0]
	if row.BackendName != "kube-api-new-connections" {
		t.Errorf("BackendName = %q, want %q", row.BackendName, "kube-api-new-connections")
	}
	if row.DisruptionSeconds != 12 {
		t.Errorf("DisruptionSeconds = %d, want %d", row.DisruptionSeconds, 12)
	}
	if row.JobRunName != "2084247445587365888" {
		t.Errorf("JobRunName = %q, want %q", row.JobRunName, "2084247445587365888")
	}
	if row.MasterNodesUpdated != "Y" {
		t.Errorf("MasterNodesUpdated = %q, want %q", row.MasterNodesUpdated, "Y")
	}
	if row.JobRunStatus != "failure" {
		t.Errorf("JobRunStatus = %q, want %q", row.JobRunStatus, "failure")
	}

	emptyRow := decoded.Rows[1]
	if emptyRow.MasterNodesUpdated != "" {
		t.Errorf("empty MasterNodesUpdated = %q, want empty string", emptyRow.MasterNodesUpdated)
	}
	if emptyRow.JobRunStatus != "" {
		t.Errorf("empty JobRunStatus = %q, want empty string", emptyRow.JobRunStatus)
	}
}

func TestBackendDisruptionRunsResultEmpty(t *testing.T) {
	result := apitype.BackendDisruptionRunsResult{}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal empty result: %v", err)
	}

	var decoded apitype.BackendDisruptionRunsResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal empty result: %v", err)
	}

	if decoded.Rows != nil {
		t.Errorf("expected nil rows for empty result, got %v", decoded.Rows)
	}
}
