package workqueue

import (
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river/rivertype"
)

func TestCategorizeState(t *testing.T) {
	tests := []struct {
		state rivertype.JobState
		want  stateCategory
	}{
		{rivertype.JobStateCompleted, stateCompleted},
		{rivertype.JobStateDiscarded, stateFailed},
		{rivertype.JobStateCancelled, stateFailed},
		{rivertype.JobStateRunning, stateRunning},
		{rivertype.JobStateAvailable, statePending},
		{rivertype.JobStateScheduled, statePending},
		{rivertype.JobStateRetryable, statePending},
		{rivertype.JobStatePending, statePending},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := categorizeState(tt.state); got != tt.want {
				t.Errorf("categorizeState(%q) = %d, want %d", tt.state, got, tt.want)
			}
		})
	}
}

func TestComputeBatchStatus(t *testing.T) {
	tests := []struct {
		name string
		resp *BatchStatusResponse
		want BatchStatus
	}{
		{
			name: "empty batch",
			resp: &BatchStatusResponse{Total: 0},
			want: BatchStatusComplete,
		},
		{
			name: "all completed",
			resp: &BatchStatusResponse{Total: 5, Completed: 5},
			want: BatchStatusComplete,
		},
		{
			name: "some still running",
			resp: &BatchStatusResponse{Total: 5, Completed: 3, Running: 2},
			want: BatchStatusRunning,
		},
		{
			name: "all failed",
			resp: &BatchStatusResponse{Total: 3, Failed: 3},
			want: BatchStatusFailed,
		},
		{
			name: "mixed completed and failed",
			resp: &BatchStatusResponse{Total: 5, Completed: 3, Failed: 2},
			want: BatchStatusComplete,
		},
		{
			name: "some pending",
			resp: &BatchStatusResponse{Total: 5, Completed: 2, Pending: 3},
			want: BatchStatusRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := &Batch{ID: uuid.New()}
			got := computeBatchStatus(batch, tt.resp)
			if got != tt.want {
				t.Errorf("computeBatchStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeBatchStatus_AlreadyFinalized(t *testing.T) {
	batch := &Batch{ID: uuid.New(), Status: BatchStatusFailed}
	now := batch.CreatedAt
	batch.CompletedAt = &now

	resp := &BatchStatusResponse{Total: 3, Completed: 3}
	got := computeBatchStatus(batch, resp)
	if got != BatchStatusFailed {
		t.Errorf("expected finalized status to be preserved, got %q", got)
	}
}
