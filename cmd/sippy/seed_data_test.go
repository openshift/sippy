package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/flags"
)

// defaultRounding is the serving default, which seed-data also uses unless overridden.
var defaultRounding = flags.NewCRTimeRoundingFlags()

// testSeedViews loads the real seed views file so tests follow the view the seed reads.
func testSeedViews(t *testing.T) apitype.SippyViews {
	t.Helper()
	views, err := loadSeedViews(filepath.Join("..", "..", syntheticViewsFile))
	require.NoError(t, err)
	return views
}

// testSeedClock builds the seed clock the command would build at now with the given rounding.
func testSeedClock(t *testing.T, views apitype.SippyViews, now time.Time, factor, offset time.Duration) seedClock {
	t.Helper()
	clock, err := newSeedClock(now, &flags.CRTimeRoundingFlags{Factor: factor, Offset: offset}, views)
	require.NoError(t, err)
	return clock
}

// tallySeededStatuses counts the statuses seededTestStatus assigns across all of counts.total results.
func tallySeededStatuses(counts testCount) map[int]int {
	got := map[int]int{}
	for i := 0; i < counts.total; i++ {
		got[seededTestStatus(i, counts)]++
	}
	return got
}

// assertStatusTally checks a status tally holds exactly the passes, flakes and failures counts declares.
func assertStatusTally(t *testing.T, counts testCount, got map[int]int, surface string) {
	t.Helper()
	assert.Equal(t, counts.success-counts.flake, got[int(v1.TestStatusSuccess)], "%s passes", surface)
	assert.Equal(t, counts.flake, got[int(v1.TestStatusFlake)], "%s flakes", surface)
	assert.Equal(t, counts.total-counts.success, got[int(v1.TestStatusFailure)], "%s failures", surface)
}

// assertSeededStatusCounts checks seededTestStatus produces exactly the declared counts.
func assertSeededStatusCounts(t *testing.T, counts testCount) {
	t.Helper()
	assertStatusTally(t, counts, tallySeededStatuses(counts), "seeded")
}

func TestSeedClockWindow4_22(t *testing.T) {
	views := testSeedViews(t)
	tests := []struct {
		name      string
		now       time.Time
		factor    time.Duration
		offset    time.Duration
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "just after 04:00 boundary ends at 04:00",
			now:       time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC),
			factor:    defaultRounding.Factor,
			offset:    defaultRounding.Offset,
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC),
		},
		{
			name:      "before first boundary of the day ends at previous day 16:00",
			now:       time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC),
			factor:    defaultRounding.Factor,
			offset:    defaultRounding.Offset,
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 3, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "exactly on 16:00 boundary",
			now:       time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC),
			factor:    defaultRounding.Factor,
			offset:    defaultRounding.Offset,
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "late evening ends at 16:00",
			now:       time.Date(2026, 11, 4, 23, 59, 0, 0, time.UTC),
			factor:    defaultRounding.Factor,
			offset:    defaultRounding.Offset,
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC),
		},
		{
			name:      "non-UTC input is normalized",
			now:       time.Date(2026, 11, 4, 3, 30, 0, 0, time.FixedZone("EST", -5*3600)),
			factor:    defaultRounding.Factor,
			offset:    defaultRounding.Offset,
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC),
		},
		{
			name:      "daily rounding at midnight UTC ends at today 00:00",
			now:       time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC),
			factor:    24 * time.Hour,
			offset:    0,
			wantStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := testSeedClock(t, views, tt.now, tt.factor, tt.offset)
			assert.Equal(t, time.UTC, clock.now.Location(), "clock time must be normalized to UTC")
			start, end := clock.window("4.22")
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
		})
	}
}

func TestSeedClockWindowHistoricalReleases(t *testing.T) {
	views := testSeedViews(t)
	clock := testSeedClock(t, views, time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC), defaultRounding.Factor, defaultRounding.Offset)
	tests := []struct {
		release   string
		wantStart time.Time
		wantEnd   time.Time
	}{
		{release: "4.21", wantStart: time.Date(2026, 9, 5, 7, 0, 0, 0, time.UTC), wantEnd: time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC)},
		{release: "4.20", wantStart: time.Date(2026, 7, 7, 7, 0, 0, 0, time.UTC), wantEnd: time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC)},
		{release: "4.19", wantStart: time.Date(2026, 5, 8, 7, 0, 0, 0, time.UTC), wantEnd: time.Date(2026, 6, 7, 7, 0, 0, 0, time.UTC)},
		{release: "4.18", wantStart: time.Date(2026, 10, 21, 7, 0, 0, 0, time.UTC), wantEnd: time.Date(2026, 11, 4, 7, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.release, func(t *testing.T) {
			start, end := clock.window(tt.release)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
		})
	}
}

func TestNewSeedClockRequiresSampleView(t *testing.T) {
	_, err := newSeedClock(time.Now(), defaultRounding, apitype.SippyViews{})
	assert.ErrorContains(t, err, seedSampleView)
}

func TestSeededRegressionOpened(t *testing.T) {
	views := testSeedViews(t)
	tests := []struct {
		name       string
		now        time.Time
		release    string
		wantOpened time.Time
	}{
		{
			name:       "4.22 opens at the start of the three day sample window",
			now:        time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC),
			release:    "4.22",
			wantOpened: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "4.22 just after midnight still leaves whole days before today",
			now:        time.Date(2026, 11, 4, 0, 5, 0, 0, time.UTC),
			release:    "4.22",
			wantOpened: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "4.21 opens at the start of its historical window",
			now:        time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC),
			release:    "4.21",
			wantOpened: time.Date(2026, 9, 5, 7, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := testSeedClock(t, views, tt.now, defaultRounding.Factor, defaultRounding.Offset)
			opened := seededRegressionOpened(clock, tt.release)
			assert.Equal(t, tt.wantOpened, opened)

			// Force close requires opened < resolved, and a user picks a resolution date before today.
			today := civil.DateOf(tt.now.UTC()).In(time.UTC)
			resolved := today.Add(-24 * time.Hour)
			assert.True(t, opened.Before(resolved), "opened %v should precede representative resolution %v", opened, resolved)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSeededStatusCounts(t, tt.counts)
		})
	}
}

// With no results to distribute, the guard must report success rather than divide by zero.
func TestSeededTestStatusZeroRuns(t *testing.T) {
	assert.Equal(t, int(v1.TestStatusSuccess), seededTestStatus(0, testCount{0, 0, 0}))
}

// Every synthetic spec must still produce exactly its declared counts.
func TestSeededTestStatusMatchesSyntheticSpecs(t *testing.T) {
	for _, ts := range syntheticTests {
		for jobTpl, releaseCounts := range ts.jobCounts {
			for release, counts := range releaseCounts {
				t.Run(fmt.Sprintf("%s %s %s", ts.testID, jobTpl, release), func(t *testing.T) {
					assertSeededStatusCounts(t, counts)
				})
			}
		}
	}
}

// Failures and flakes must be spread across the window, not concentrated in the latest runs.
func TestSeededTestStatusSpreads(t *testing.T) {
	const slice int = 40
	tests := []struct {
		name         string
		counts       testCount
		wantFailures int
		wantFlakes   int
	}{
		{name: "failures only", counts: testCount{200, 170, 0}, wantFailures: 6, wantFlakes: 0},
		{name: "failures and flakes", counts: testCount{200, 170, 10}, wantFailures: 6, wantFlakes: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for from := 0; from+slice <= tt.counts.total; from += slice {
				got := map[int]int{}
				for i := from; i < from+slice; i++ {
					got[seededTestStatus(i, tt.counts)]++
				}
				assert.Equal(t, tt.wantFailures, got[int(v1.TestStatusFailure)], "failures in runs [%d,%d)", from, from+slice)
				assert.Equal(t, tt.wantFlakes, got[int(v1.TestStatusFlake)], "flakes in runs [%d,%d)", from, from+slice)
			}
		})
	}
}

func TestSeededRunTimestamps(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		count int
	}{
		{name: "runs plus infra runs", count: 200 + seededInfraRuns},
		{name: "single run", count: 1},
		{name: "zero runs", count: 0},
		{name: "negative count", count: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := seededRunTimestamps(start, end, tt.count)
			if tt.count <= 0 {
				assert.Nil(t, ts)
				return
			}
			require.Len(t, ts, tt.count)
			assert.Equal(t, start, ts[0])
			interval := end.Sub(start) / time.Duration(tt.count)
			for i, tm := range ts {
				assert.False(t, tm.Before(start), "run %d before start", i)
				assert.True(t, tm.Before(end), "run %d not before end", i)
				if i > 0 {
					assert.Equal(t, interval, tm.Sub(ts[i-1]), "spacing before run %d", i)
				}
			}
		})
	}
}

// TestSeedClockWindow4_22SampleConsistentAcrossSurfaces models how the component report
// (whole UTC days through the sample end date) and test details (exact run
// timestamps up to the rounded sample end) count the seeded 4.22 results, and
// checks both see the same intended counts for the life of the seed: at seed
// time, and later the same day as rounding boundaries pass. The server's sample
// window comes from the seed view through the server's own parser, so a change to
// the view or the parser that the seed does not follow fails here. The server and
// the seed share the rounding settings, so each config is applied to both.
func TestSeedClockWindow4_22SampleConsistentAcrossSurfaces(t *testing.T) {
	const runCount int = 200
	views := testSeedViews(t)
	sampleRelease, err := seedViewSampleRelease(views, seedSampleView)
	require.NoError(t, err)

	roundings := []struct {
		name   string
		factor time.Duration
		offset time.Duration
	}{
		{name: "default 12h/4h", factor: defaultRounding.Factor, offset: defaultRounding.Offset},
		{name: "daily 24h/0h", factor: 24 * time.Hour, offset: 0},
	}
	countCases := []struct {
		name   string
		counts testCount
	}{
		{name: "regression without flakes", counts: testCount{200, 170, 0}}, // test-extreme-regression on aws/arm64/ovn
		{name: "regression with flakes", counts: testCount{200, 170, 9}},
	}
	seedTimes := []time.Time{
		time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC), // reported reproduction: sample end 04:00
		time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 15, 59, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 4, 23, 0, 0, 0, time.UTC),
	}
	laterOffsets := []time.Duration{0, time.Hour, 5 * time.Hour}

	for _, rounding := range roundings {
		for _, cc := range countCases {
			counts := cc.counts
			for _, seedAt := range seedTimes {
				clock := testSeedClock(t, views, seedAt, rounding.factor, rounding.offset)
				start, end := clock.window(sampleRelease.Name)
				timestamps := seededRunTimestamps(start, end, runCount+seededInfraRuns)

				for _, later := range laterOffsets {
					queryAt := seedAt.Add(later)
					if civil.DateOf(queryAt) != civil.DateOf(seedAt) {
						continue // the start-of-window day moves at midnight; reseed is expected
					}
					name := rounding.name + "/" + cc.name + "/" + seedAt.Format(time.RFC3339) + "+" + later.String()
					t.Run(name, func(t *testing.T) {
						server, err := utils.GetViewReleaseOptionsAt(queryAt, nil, "sample", sampleRelease, rounding.factor, rounding.offset)
						require.NoError(t, err)
						assertSurfacesAgree(t, server, counts, timestamps)
					})
				}
			}
		}
	}
}

// assertSurfacesAgree tallies seeded results the way the report (whole days) and test details
// (exact timestamps) select them from the server's sample window, and checks both match counts.
func assertSurfacesAgree(t *testing.T, server reqopts.Release, counts testCount, timestamps []time.Time) {
	t.Helper()
	startDate, endDate := civil.DateOf(server.Start), civil.DateOf(server.End)
	report, details := map[int]int{}, map[int]int{}
	for i := 0; i < counts.total; i++ {
		status := seededTestStatus(i, counts)
		d := civil.DateOf(timestamps[i])
		if !d.Before(startDate) && !d.After(endDate) {
			report[status]++
		}
		if !timestamps[i].Before(server.Start) && !timestamps[i].After(server.End) {
			details[status]++
		}
	}
	assertStatusTally(t, counts, report, "report")
	assertStatusTally(t, counts, details, "details")
}
