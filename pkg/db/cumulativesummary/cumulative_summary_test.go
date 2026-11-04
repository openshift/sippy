package cumulativesummary

import (
	"testing"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingStore struct {
	calls []string
}

func (s *recordingStore) UpdateDateForRelease(_ civil.Date, release string) error {
	s.calls = append(s.calls, release)
	return nil
}

func TestBackfillDateRange_UsesIncludedReleases(t *testing.T) {
	date := civil.Date{Year: 2026, Month: 7, Day: 1}
	store := &recordingStore{}

	err := backfillDateRange(store, date, date, []string{"4.22"})

	require.NoError(t, err)
	assert.Equal(t, []string{"4.22"}, store.calls)
}
