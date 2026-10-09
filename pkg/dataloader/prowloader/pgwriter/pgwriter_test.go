package pgwriter

import (
	"strings"
	"testing"
	"time"
)

func TestFormatRunPartitionKeysDeduplicatesReleaseAndDate(t *testing.T) {
	timestamp := time.Date(2025, time.September, 29, 12, 34, 56, 0, time.UTC)
	got := formatRunPartitionKeys([]JobRunResult{
		{Run: RunRow{ID: 1, ProwJobRelease: "4.22", Timestamp: timestamp}},
		{Run: RunRow{ID: 2, ProwJobRelease: "4.22", Timestamp: timestamp.Add(time.Hour)}},
		{Run: RunRow{ID: 3, ProwJobRelease: "4.21", Timestamp: timestamp}},
	})

	if strings.Count(got, `release="4.22" date=2025-09-29`) != 1 {
		t.Fatalf("expected one 4.22 partition key, got %q", got)
	}
	if strings.Count(got, `release="4.21" date=2025-09-29`) != 1 {
		t.Fatalf("expected one 4.21 partition key, got %q", got)
	}
	if !strings.Contains(got, "run_id=1") || !strings.Contains(got, "run_id=3") {
		t.Fatalf("expected representative run IDs, got %q", got)
	}
}
