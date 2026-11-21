package gateststatus

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
)

func TestGASnapshotCutoff(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want civil.Date
	}{
		{name: "before two complete days", now: time.Date(2026, 11, 21, 23, 59, 59, 0, time.UTC), want: civil.Date{Year: 2026, Month: 11, Day: 18}},
		{name: "two complete days", now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), want: civil.Date{Year: 2026, Month: 11, Day: 19}},
		{name: "calendar day is UTC", now: time.Date(2026, 11, 21, 20, 0, 0, 0, time.FixedZone("west", -5*60*60)), want: civil.Date{Year: 2026, Month: 11, Day: 19}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gaSnapshotCutoff(tt.now); got != tt.want {
				t.Errorf("GA cutoff = %v, want %v", got, tt.want)
			}
		})
	}
}
