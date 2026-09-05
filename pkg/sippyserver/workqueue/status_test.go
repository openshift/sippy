package workqueue

import "testing"

func TestOverallStatus(t *testing.T) {
	tests := []struct {
		name   string
		counts ItemStateCounts
		want   BatchStatus
	}{
		{
			name:   "empty batch",
			counts: ItemStateCounts{Total: 0},
			want:   BatchStatusComplete,
		},
		{
			name:   "all completed",
			counts: ItemStateCounts{Total: 5, Completed: 5},
			want:   BatchStatusComplete,
		},
		{
			name:   "all failed",
			counts: ItemStateCounts{Total: 3, Failed: 3},
			want:   BatchStatusFailed,
		},
		{
			name:   "mixed terminal",
			counts: ItemStateCounts{Total: 5, Completed: 3, Failed: 2},
			want:   BatchStatusComplete,
		},
		{
			name:   "some running",
			counts: ItemStateCounts{Total: 5, Completed: 2, Running: 2, Pending: 1},
			want:   BatchStatusRunning,
		},
		{
			name:   "all pending",
			counts: ItemStateCounts{Total: 5, Pending: 5},
			want:   BatchStatusPending,
		},
		{
			name:   "running with failures",
			counts: ItemStateCounts{Total: 10, Completed: 3, Failed: 1, Running: 4, Pending: 2},
			want:   BatchStatusRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OverallStatus(tt.counts)
			if got != tt.want {
				t.Errorf("OverallStatus(%+v) = %q, want %q", tt.counts, got, tt.want)
			}
		})
	}
}
