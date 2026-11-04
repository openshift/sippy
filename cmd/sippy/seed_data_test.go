package main

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/util"
)

func TestReleaseTimeWindowAt4_22(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "just after 04:00 boundary ends at 04:00",
			now:       time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC),
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC),
		},
		{
			name:      "before first boundary of the day ends at previous day 16:00",
			now:       time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 3, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "exactly on 16:00 boundary",
			now:       time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "late evening ends at 16:00",
			now:       time.Date(2026, 11, 4, 23, 59, 0, 0, time.UTC),
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "non-UTC input is normalized",
			now:       time.Date(2026, 11, 4, 3, 30, 0, 0, time.FixedZone("EST", -5*3600)),
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := releaseTimeWindowAt("4.22", tt.now)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
		})
	}
}

func TestSeededTestStatusCounts(t *testing.T) {
	tests := []struct {
		name   string
		counts testCount
	}{
		{name: "extreme regression arm64", counts: testCount{200, 170, 0}},
		{name: "extreme regression amd64", counts: testCount{200, 140, 0}},
		{name: "all passing", counts: testCount{100, 100, 0}},
		{name: "all failing", counts: testCount{10, 0, 0}},
		{name: "with flakes", counts: testCount{100, 90, 7}},
		{name: "single run failing", counts: testCount{1, 0, 0}},
		{name: "zero runs", counts: testCount{0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[int]int{}
			for i := 0; i < tt.counts.total; i++ {
				got[seededTestStatus(i, tt.counts)]++
			}
			assert.Equal(t, tt.counts.success-tt.counts.flake, got[int(v1.TestStatusSuccess)], "passes")
			assert.Equal(t, tt.counts.flake, got[int(v1.TestStatusFlake)], "flakes")
			assert.Equal(t, tt.counts.total-tt.counts.success, got[int(v1.TestStatusFailure)], "failures")
		})
	}
}

// Every synthetic spec must still produce exactly its declared counts.
func TestSeededTestStatusMatchesSyntheticSpecs(t *testing.T) {
	for _, ts := range syntheticTests {
		for jobTpl, releaseCounts := range ts.jobCounts {
			for release, counts := range releaseCounts {
				failures := 0
				for i := 0; i < counts.total; i++ {
					if seededTestStatus(i, counts) == int(v1.TestStatusFailure) {
						failures++
					}
				}
				assert.Equal(t, counts.total-counts.success, failures, "%s %s %s", ts.testID, jobTpl, release)
			}
		}
	}
}

// Failures must be spread across the window, not concentrated in the latest runs.
func TestSeededTestStatusSpreadsFailures(t *testing.T) {
	counts := testCount{200, 170, 0}
	const slice = 40
	for from := 0; from+slice <= counts.total; from += slice {
		failures := 0
		for i := from; i < from+slice; i++ {
			if seededTestStatus(i, counts) == int(v1.TestStatusFailure) {
				failures++
			}
		}
		assert.Equal(t, 6, failures, "runs [%d,%d)", from, from+slice)
	}
}

func TestSeededRunTimestamps(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC)
	ts := seededRunTimestamps(start, end, 202)
	require.Len(t, ts, 202)
	assert.Equal(t, start, ts[0])
	for _, tm := range ts {
		assert.False(t, tm.Before(start))
		assert.True(t, tm.Before(end))
	}
	assert.Nil(t, seededRunTimestamps(start, end, 0))
}

// TestSeeded4_22SampleConsistentAcrossSurfaces models how the component report
// (whole UTC days through the sample end date) and test details (exact run
// timestamps up to the rounded sample end) count the seeded 4.22 results, and
// checks both see the same intended counts for the life of the seed: at seed
// time, and later the same day as rounding boundaries pass.
func TestSeeded4_22SampleConsistentAcrossSurfaces(t *testing.T) {
	const runCount = 200
	counts := testCount{200, 170, 0} // test-extreme-regression on aws/arm64/ovn
	seedTimes := []time.Time{
		time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC), // reported reproduction: sample end 04:00
		time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 15, 59, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 23, 0, 0, 0, time.UTC),
	}
	laterOffsets := []time.Duration{0, time.Hour, 5 * time.Hour}

	for _, seedAt := range seedTimes {
		start, end := releaseTimeWindowAt("4.22", seedAt)
		timestamps := seededRunTimestamps(start, end, runCount+2)

		for _, later := range laterOffsets {
			queryAt := seedAt.Add(later)
			if civil.DateOf(queryAt) != civil.DateOf(seedAt) {
				continue // the start-of-window day moves at midnight; reseed is expected
			}
			t.Run(seedAt.Format(time.RFC3339)+"+"+later.String(), func(t *testing.T) {
				sampleStart := util.AdjustReleaseTime(queryAt, true, "3", 0, 0)
				sampleEnd := util.TruncateAligned(queryAt, flags.DefaultCRTimeRoundingFactor, flags.DefaultCRTimeRoundingOffset)
				startDate, endDate := civil.DateOf(sampleStart), civil.DateOf(sampleEnd)

				var reportPass, reportFail, detailsPass, detailsFail int
				for i := 0; i < counts.total; i++ {
					failed := seededTestStatus(i, counts) == int(v1.TestStatusFailure)
					d := civil.DateOf(timestamps[i])
					if !d.Before(startDate) && !d.After(endDate) {
						if failed {
							reportFail++
						} else {
							reportPass++
						}
					}
					if !timestamps[i].Before(sampleStart) && !timestamps[i].After(sampleEnd) {
						if failed {
							detailsFail++
						} else {
							detailsPass++
						}
					}
				}
				assert.Equal(t, counts.success, reportPass, "report passes")
				assert.Equal(t, counts.total-counts.success, reportFail, "report failures")
				assert.Equal(t, reportPass, detailsPass, "details passes")
				assert.Equal(t, reportFail, detailsFail, "details failures")
			})
		}
	}
}
