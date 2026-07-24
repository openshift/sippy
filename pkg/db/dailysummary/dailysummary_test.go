package dailysummary

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

type fakeStore struct {
	maxSummary    *civil.Date
	maxSummaryErr error
	releases      []string
	releasesErr   error
	aggregateErr  error

	mu    sync.Mutex
	calls []aggregateCall
}

type aggregateCall struct {
	start, end            civil.Date
	release               string
	skipConflictDetection bool
}

func (f *fakeStore) MaxSummaryDate() (*civil.Date, error) {
	return f.maxSummary, f.maxSummaryErr
}

func (f *fakeStore) Releases() ([]string, error) {
	if f.releases != nil {
		return f.releases, f.releasesErr
	}
	return []string{"4.22", "5.0"}, f.releasesErr
}

func (f *fakeStore) AggregateRangeForRelease(start, end civil.Date, release string, skipConflictDetection bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, aggregateCall{start: start, end: end, release: release, skipConflictDetection: skipConflictDetection})
	return f.aggregateErr
}

func (f *fakeStore) getCalls() []aggregateCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]aggregateCall, len(f.calls))
	copy(result, f.calls)
	return result
}

func (f *fakeStore) allSkippedConflictDetection() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if !c.skipConflictDetection {
			return false
		}
	}
	return true
}

func (f *fakeStore) noneSkippedConflictDetection() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.skipConflictDetection {
			return false
		}
	}
	return true
}

func TestRefresh_Incremental(t *testing.T) {
	today := civil.DateOf(time.Now().UTC())
	maxDate := today.AddDays(-3)
	store := &fakeStore{maxSummary: &maxDate}

	_, err := refreshSummaries(store)

	require.NoError(t, err)
	// 4 days (maxDate through today) × 2 releases
	assert.Len(t, store.getCalls(), 4*2)
	assert.True(t, store.noneSkippedConflictDetection())
}

func TestRefresh_FutureMaxDateStartsAtYesterday(t *testing.T) {
	future := civil.Date{Year: 2099, Month: 1, Day: 1}
	store := &fakeStore{maxSummary: &future}

	_, err := refreshSummaries(store)

	require.NoError(t, err)
	calls := store.getCalls()
	require.NotEmpty(t, calls)
	today := civil.DateOf(time.Now().UTC())
	yesterday := today.AddDays(-1)
	assert.Equal(t, yesterday, calls[0].start)
	assert.Equal(t, today, calls[len(calls)-1].end)
}

func TestRefresh_EmptyTableUsesDefaultLookbackAndSkipsConflictDetection(t *testing.T) {
	store := &fakeStore{}

	_, err := refreshSummaries(store)

	require.NoError(t, err)
	calls := store.getCalls()
	require.NotEmpty(t, calls)
	today := civil.DateOf(time.Now().UTC())
	daysBefore := today.DaysSince(calls[0].start)
	assert.InDelta(t, defaultLookbackDays, daysBefore, 1)
	assert.True(t, store.allSkippedConflictDetection())
}

func TestRefresh_IncrementalUsesUpsert(t *testing.T) {
	today := civil.DateOf(time.Now().UTC())
	maxDate := today.AddDays(-2)
	store := &fakeStore{maxSummary: &maxDate}

	_, err := refreshSummaries(store)

	require.NoError(t, err)
	// 3 days × 2 releases
	assert.Len(t, store.getCalls(), 3*2)
	assert.True(t, store.noneSkippedConflictDetection())
}

func TestBackfill_UsesExplicitDateRange(t *testing.T) {
	start := civil.Date{Year: 2026, Month: 7, Day: 1}
	end := civil.Date{Year: 2026, Month: 7, Day: 3}
	store := &fakeStore{}

	err := backfillSummaries(store, start, end)

	require.NoError(t, err)
	calls := store.getCalls()
	// 3 days × 2 releases = 6 calls, each single-day
	require.Len(t, calls, 6)
	assert.Equal(t, start, calls[0].start)
	assert.Equal(t, start, calls[0].end)
	assert.Equal(t, end, calls[len(calls)-1].start)
	assert.Equal(t, end, calls[len(calls)-1].end)
}

func TestRefresh_MaxSummaryDateError(t *testing.T) {
	store := &fakeStore{maxSummaryErr: fmt.Errorf("connection refused")}

	_, err := refreshSummaries(store)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max summary date")
	assert.Empty(t, store.getCalls())
}

func TestRefresh_ReleasesError(t *testing.T) {
	store := &fakeStore{releasesErr: fmt.Errorf("connection refused")}

	_, err := refreshSummaries(store)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "querying releases")
	assert.Empty(t, store.getCalls())
}

func TestRefresh_AggregateError(t *testing.T) {
	maxDate := civil.Date{Year: 2026, Month: 6, Day: 17}
	store := &fakeStore{maxSummary: &maxDate, aggregateErr: fmt.Errorf("disk full")}

	_, err := refreshSummaries(store)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "aggregating release")
}

func TestRefresh_NoReleases(t *testing.T) {
	store := &fakeStore{releases: []string{}}

	_, err := refreshSummaries(store)

	require.NoError(t, err)
	assert.Empty(t, store.getCalls())
}

func TestRefresh_ProcessesAllReleasesPerDay(t *testing.T) {
	releases := []string{"4.18", "4.19", "4.20", "4.21", "4.22", "5.0"}
	today := civil.DateOf(time.Now().UTC())
	maxDate := today.AddDays(-3)
	store := &fakeStore{maxSummary: &maxDate, releases: releases}

	_, err := refreshSummaries(store)

	require.NoError(t, err)
	calls := store.getCalls()
	// 4 days (maxDate through today) × 6 releases = 24 calls
	assert.Len(t, calls, 4*len(releases))
	seen := sets.New[string]()
	for _, call := range calls {
		seen.Insert(call.release)
		assert.Equal(t, call.start, call.end, "each call should be a single day")
	}
	for _, rel := range releases {
		assert.True(t, seen.Has(rel), "release %s not processed", rel)
	}
}
