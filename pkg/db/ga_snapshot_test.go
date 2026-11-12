package db

import (
	"testing"
	"time"
)

func TestGAWindowCoverageEnd(t *testing.T) {
	day := func(n int) time.Time {
		return time.Date(2026, time.November, n, 0, 0, 0, 0, time.UTC)
	}

	tests := []struct {
		name     string
		ranges   []partitionRange
		start    time.Time
		end      time.Time
		wantEnd  time.Time
		wantFull bool
	}{
		{
			name: "full coverage",
			ranges: []partitionRange{
				{Start: day(1), End: day(2)},
				{Start: day(2), End: day(4)},
			},
			start:    day(1),
			end:      day(4),
			wantEnd:  day(4),
			wantFull: true,
		},
		{
			name: "leading gap",
			ranges: []partitionRange{
				{Start: day(2), End: day(4)},
			},
			start:   day(1),
			end:     day(4),
			wantEnd: day(1),
		},
		{
			name: "interior gap",
			ranges: []partitionRange{
				{Start: day(1), End: day(2)},
				{Start: day(3), End: day(4)},
			},
			start:   day(1),
			end:     day(4),
			wantEnd: day(2),
		},
		{
			name: "overlap extends coverage",
			ranges: []partitionRange{
				{Start: day(1), End: day(3)},
				{Start: day(2), End: day(4)},
			},
			start:    day(1),
			end:      day(4),
			wantEnd:  day(4),
			wantFull: true,
		},
		{
			name: "range ending before covered point",
			ranges: []partitionRange{
				{Start: day(1), End: day(2)},
				{Start: day(2), End: day(4)},
			},
			start:    day(3),
			end:      day(4),
			wantEnd:  day(4),
			wantFull: true,
		},
		{
			name: "range ending exactly at window end",
			ranges: []partitionRange{
				{Start: day(1), End: day(3)},
			},
			start:    day(1),
			end:      day(3),
			wantEnd:  day(3),
			wantFull: true,
		},
		{
			name:    "empty ranges",
			start:   day(1),
			end:     day(3),
			wantEnd: day(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEnd, gotFull := gaWindowCoverageEnd(tt.ranges, tt.start, tt.end)
			if !gotEnd.Equal(tt.wantEnd) {
				t.Fatalf("coverage end = %s, want %s", gotEnd, tt.wantEnd)
			}
			if gotFull != tt.wantFull {
				t.Fatalf("full coverage = %t, want %t", gotFull, tt.wantFull)
			}
		})
	}
}
