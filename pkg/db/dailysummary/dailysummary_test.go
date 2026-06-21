package dailysummary

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	maxSummary    *time.Time
	maxSummaryErr error
	truncated     bool
	truncateErr   error
	releases      []string
	releasesErr   error
	aggregateErr  error

	mu    sync.Mutex
	calls []aggregateCall
}

type aggregateCall struct {
	start, end            time.Time
	release               string
	skipConflictDetection bool
}

func (f *fakeStore) MaxSummaryDate() (*time.Time, error) {
	return f.maxSummary, f.maxSummaryErr
}

func (f *fakeStore) Truncate() error {
	f.truncated = true
	return f.truncateErr
}

func (f *fakeStore) Releases() ([]string, error) {
	if f.releases != nil {
		return f.releases, f.releasesErr
	}
	return []string{"4.22", "5.0"}, f.releasesErr
}

func (f *fakeStore) AggregateRangeForRelease(start, end time.Time, release string, skipConflictDetection bool) error {
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
	maxDate := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{maxSummary: &maxDate}

	err := refreshSummaries(store, Options{})

	require.NoError(t, err)
	assert.Len(t, store.getCalls(), 2)
	assert.True(t, store.noneSkippedConflictDetection())
	assert.False(t, store.truncated)
}

func TestRefresh_IncrementalCapsAtYesterday(t *testing.T) {
	future := time.Now().AddDate(0, 0, 1)
	store := &fakeStore{maxSummary: &future}

	err := refreshSummaries(store, Options{})

	require.NoError(t, err)
	calls := store.getCalls()
	require.NotEmpty(t, calls)
	assert.True(t, calls[0].start.Before(future))
}

func TestRefresh_EmptyTableUsesDefaultLookbackAndSkipsConflictDetection(t *testing.T) {
	store := &fakeStore{}

	err := refreshSummaries(store, Options{})

	require.NoError(t, err)
	calls := store.getCalls()
	require.NotEmpty(t, calls)
	daysBefore := time.Since(calls[0].start).Hours() / 24
	assert.InDelta(t, defaultLookbackDays, daysBefore, 1)
	assert.True(t, store.allSkippedConflictDetection())
}

func TestRefresh_RebuildUsesInsert(t *testing.T) {
	store := &fakeStore{}

	err := refreshSummaries(store, Options{Rebuild: true})

	require.NoError(t, err)
	assert.True(t, store.truncated)
	assert.Len(t, store.getCalls(), 2)
	assert.True(t, store.allSkippedConflictDetection())
}

func TestRefresh_IncrementalUsesUpsert(t *testing.T) {
	maxDate := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{maxSummary: &maxDate}

	err := refreshSummaries(store, Options{})

	require.NoError(t, err)
	assert.Len(t, store.getCalls(), 2)
	assert.True(t, store.noneSkippedConflictDetection())
}

func TestRefresh_DateOverrides(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	maxDate := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{maxSummary: &maxDate}

	err := refreshSummaries(store, Options{StartOverride: &start, EndOverride: &end})

	require.NoError(t, err)
	calls := store.getCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, start, calls[0].start)
	assert.Equal(t, end, calls[0].end)
}

func TestRefresh_StartOverrideOnly(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	maxDate := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{maxSummary: &maxDate}

	err := refreshSummaries(store, Options{StartOverride: &start})

	require.NoError(t, err)
	calls := store.getCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, start, calls[0].start)
}

func TestRefresh_TruncateError(t *testing.T) {
	store := &fakeStore{truncateErr: fmt.Errorf("permission denied")}

	err := refreshSummaries(store, Options{Rebuild: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "truncating table")
	assert.Empty(t, store.getCalls())
}

func TestRefresh_MaxSummaryDateError(t *testing.T) {
	store := &fakeStore{maxSummaryErr: fmt.Errorf("connection refused")}

	err := refreshSummaries(store, Options{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max summary date")
	assert.Empty(t, store.getCalls())
}

func TestRefresh_ReleasesError(t *testing.T) {
	store := &fakeStore{releasesErr: fmt.Errorf("connection refused")}

	err := refreshSummaries(store, Options{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "querying releases")
	assert.Empty(t, store.getCalls())
}

func TestRefresh_AggregateError(t *testing.T) {
	maxDate := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{maxSummary: &maxDate, aggregateErr: fmt.Errorf("disk full")}

	err := refreshSummaries(store, Options{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "aggregating release")
}

func TestRefresh_NoReleases(t *testing.T) {
	store := &fakeStore{releases: []string{}}

	err := refreshSummaries(store, Options{})

	require.NoError(t, err)
	assert.Empty(t, store.getCalls())
}

func TestRefresh_ParallelProcessesAllReleases(t *testing.T) {
	releases := []string{"4.18", "4.19", "4.20", "4.21", "4.22", "5.0"}
	maxDate := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{maxSummary: &maxDate, releases: releases}

	err := refreshSummaries(store, Options{})

	require.NoError(t, err)
	calls := store.getCalls()
	assert.Len(t, calls, len(releases))
	seen := make(map[string]bool)
	for _, call := range calls {
		seen[call.release] = true
	}
	for _, rel := range releases {
		assert.True(t, seen[rel], "release %s not processed", rel)
	}
}
