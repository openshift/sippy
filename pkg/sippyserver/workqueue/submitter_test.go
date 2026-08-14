package workqueue

import (
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river/rivertype"
)

func TestCountInsertResults(t *testing.T) {
	tests := []struct {
		name        string
		results     []*rivertype.JobInsertResult
		wantNew     int
		wantDeduped int
	}{
		{
			name:    "empty",
			results: nil,
		},
		{
			name: "all new",
			results: []*rivertype.JobInsertResult{
				{Job: &rivertype.JobRow{ID: 1}},
				{Job: &rivertype.JobRow{ID: 2}},
			},
			wantNew: 2,
		},
		{
			name: "all deduped",
			results: []*rivertype.JobInsertResult{
				{Job: &rivertype.JobRow{ID: 1}, UniqueSkippedAsDuplicate: true},
				{Job: &rivertype.JobRow{ID: 2}, UniqueSkippedAsDuplicate: true},
			},
			wantDeduped: 2,
		},
		{
			name: "mixed",
			results: []*rivertype.JobInsertResult{
				{Job: &rivertype.JobRow{ID: 1}},
				{Job: &rivertype.JobRow{ID: 2}, UniqueSkippedAsDuplicate: true},
				{Job: &rivertype.JobRow{ID: 3}},
			},
			wantNew:     2,
			wantDeduped: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNew, gotDeduped := countInsertResults(tt.results)
			if gotNew != tt.wantNew {
				t.Errorf("enqueued = %d, want %d", gotNew, tt.wantNew)
			}
			if gotDeduped != tt.wantDeduped {
				t.Errorf("deduped = %d, want %d", gotDeduped, tt.wantDeduped)
			}
		})
	}
}

func TestBuildBatchItems(t *testing.T) {
	batchID := uuid.New()
	results := []*rivertype.JobInsertResult{
		{Job: &rivertype.JobRow{ID: 100}},
		{Job: &rivertype.JobRow{ID: 200}, UniqueSkippedAsDuplicate: true},
	}
	keys := []string{"build-1", "build-2"}

	items := buildBatchItems(batchID, results, keys)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].BatchID != batchID {
		t.Errorf("items[0].BatchID = %v, want %v", items[0].BatchID, batchID)
	}
	if items[0].RiverJobID != 100 {
		t.Errorf("items[0].RiverJobID = %d, want 100", items[0].RiverJobID)
	}
	if items[0].ItemKey != "build-1" {
		t.Errorf("items[0].ItemKey = %q, want %q", items[0].ItemKey, "build-1")
	}
	if items[1].RiverJobID != 200 {
		t.Errorf("items[1].RiverJobID = %d, want 200", items[1].RiverJobID)
	}
}
