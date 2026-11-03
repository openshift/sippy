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

func (s *recordingStore) Releases() ([]string, error) {
	return []string{"4.22", "Presubmits", "4.21"}, nil
}

func (s *recordingStore) UpdateDateForRelease(_ civil.Date, release string) error {
	s.calls = append(s.calls, release)
	return nil
}

func TestBackfillDateRange_ExcludesReleases(t *testing.T) {
	date := civil.Date{Year: 2026, Month: 7, Day: 1}
	store := &recordingStore{}

	err := backfillDateRange(store, date, date, "Presubmits", "4.21")

	require.NoError(t, err)
	assert.Equal(t, []string{"4.22"}, store.calls)
}
