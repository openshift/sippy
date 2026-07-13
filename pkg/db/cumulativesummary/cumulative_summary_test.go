package cumulativesummary

import (
	"fmt"
	"testing"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	maxCumulativeSummary    *civil.Date
	maxCumulativeSummaryErr error
	maxDailySummaryDate     *civil.Date
	maxDailySummaryDateErr  error
	updatedDates            []civil.Date
	updateDateErr           error
}

func (f *fakeStore) MaxCumulativeSummaryDate() (*civil.Date, error) {
	return f.maxCumulativeSummary, f.maxCumulativeSummaryErr
}

func (f *fakeStore) MaxDailySummaryDate() (*civil.Date, error) {
	return f.maxDailySummaryDate, f.maxDailySummaryDateErr
}

func (f *fakeStore) Releases() ([]string, error) {
	return []string{"4.22", "5.0"}, nil
}

func (f *fakeStore) UpdateDateForRelease(date civil.Date, _ string) error {
	f.updatedDates = append(f.updatedDates, date)
	return f.updateDateErr
}

var today = civil.Date{Year: 2026, Month: 7, Day: 9}

const numReleases = 2

func TestRefresh_NoCumulativeDataUsesEarliestChanged(t *testing.T) {
	maxDailySummaryDate := today
	earliestChanged := today.AddDays(-1)
	store := &fakeStore{maxDailySummaryDate: &maxDailySummaryDate}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.NoError(t, err)
	require.NotEmpty(t, store.updatedDates)
	assert.Equal(t, earliestChanged, store.updatedDates[0])
}

func TestRefresh_FillsGapFromMaxCumulative(t *testing.T) {
	maxCum := today.AddDays(-4)
	maxDailySummaryDate := today.AddDays(-2)
	earliestChanged := today.AddDays(-2)
	store := &fakeStore{maxCumulativeSummary: &maxCum, maxDailySummaryDate: &maxDailySummaryDate}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.NoError(t, err)
	assert.Len(t, store.updatedDates, 2*numReleases)
	assert.Equal(t, today.AddDays(-3), store.updatedDates[0])
}

func TestRefresh_StartsFromEarliestChanged(t *testing.T) {
	maxDailySummaryDate := today.AddDays(-2)
	earliestChanged := today.AddDays(-3)
	store := &fakeStore{maxDailySummaryDate: &maxDailySummaryDate}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.NoError(t, err)
	assert.Len(t, store.updatedDates, 2*numReleases)
	assert.Equal(t, today.AddDays(-3), store.updatedDates[0])
}

func TestRefresh_FillsSmallGap(t *testing.T) {
	maxCum := today.AddDays(-6)
	maxDailySummaryDate := today.AddDays(-2)
	earliestChanged := today.AddDays(-3)
	store := &fakeStore{maxCumulativeSummary: &maxCum, maxDailySummaryDate: &maxDailySummaryDate}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.NoError(t, err)
	assert.Equal(t, today.AddDays(-5), store.updatedDates[0])
	assert.Equal(t, today.AddDays(-2), store.updatedDates[len(store.updatedDates)-1])
}

func TestRefresh_CapsGapAt14Days(t *testing.T) {
	maxCum := today.AddDays(-30)
	maxDailySummaryDate := today
	earliestChanged := today.AddDays(-1)
	store := &fakeStore{maxCumulativeSummary: &maxCum, maxDailySummaryDate: &maxDailySummaryDate}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.NoError(t, err)
	assert.Equal(t, today.AddDays(-14), store.updatedDates[0])
}

func TestRefresh_IncrementalFromMaxPlusOne(t *testing.T) {
	maxCum := today.AddDays(-3)
	maxDailySummaryDate := today.AddDays(-2)
	earliestChanged := today.AddDays(-3)
	store := &fakeStore{maxCumulativeSummary: &maxCum, maxDailySummaryDate: &maxDailySummaryDate}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.NoError(t, err)
	assert.Len(t, store.updatedDates, 2*numReleases)
	assert.Equal(t, today.AddDays(-3), store.updatedDates[0])
}

func TestRefresh_AlreadyUpToDate(t *testing.T) {
	maxCum := today.AddDays(-2)
	maxDailySummaryDate := today.AddDays(-2)
	earliestChanged := today.AddDays(-2)
	store := &fakeStore{maxCumulativeSummary: &maxCum, maxDailySummaryDate: &maxDailySummaryDate}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.NoError(t, err)
	assert.Len(t, store.updatedDates, 1*numReleases)
	assert.Equal(t, today.AddDays(-2), store.updatedDates[0])
}

func TestRefresh_MaxCumulativeSummaryDateError(t *testing.T) {
	earliestChanged := today.AddDays(-3)
	store := &fakeStore{maxCumulativeSummaryErr: fmt.Errorf("connection refused")}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max cumulative summary date")
}

func TestRefresh_UpdateDateError(t *testing.T) {
	maxCum := today.AddDays(-4)
	maxDailySummaryDate := today.AddDays(-2)
	earliestChanged := today.AddDays(-3)
	store := &fakeStore{maxCumulativeSummary: &maxCum, maxDailySummaryDate: &maxDailySummaryDate, updateDateErr: fmt.Errorf("disk full")}

	_, err := doRefreshWithToday(store, earliestChanged, today)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "updating cumulative summaries")
}
