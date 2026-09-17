package dailysummary

import (
	"fmt"
	"sync"
	"testing"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

type fakeStore struct {
	releases    []string
	releasesErr error
	replaceErr  error

	mu    sync.Mutex
	calls []replaceCall
}

type replaceCall struct {
	start, end civil.Date
	release    string
}

func (f *fakeStore) Releases() ([]string, error) {
	if f.releases != nil {
		return f.releases, f.releasesErr
	}
	return []string{"4.22", "5.0"}, f.releasesErr
}

func (f *fakeStore) ReplaceRangeForRelease(start, end civil.Date, release string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, replaceCall{start: start, end: end, release: release})
	return f.replaceErr
}

func (f *fakeStore) getCalls() []replaceCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]replaceCall, len(f.calls))
	copy(result, f.calls)
	return result
}

func TestBackfill_UsesExplicitDateRange(t *testing.T) {
	start := civil.Date{Year: 2026, Month: 7, Day: 1}
	end := civil.Date{Year: 2026, Month: 7, Day: 3}
	store := &fakeStore{}

	err := backfillSummaries(store, start, end)

	require.NoError(t, err)
	calls := store.getCalls()
	require.Len(t, calls, 6, "3 days × 2 releases = 6 calls")

	type dateRelease struct {
		date    civil.Date
		release string
	}
	seen := sets.New[dateRelease]()
	for _, c := range calls {
		assert.Equal(t, c.start, c.end, "each call should be a single day")
		seen.Insert(dateRelease{date: c.start, release: c.release})
	}
	for date := start; !date.After(end); date = date.AddDays(1) {
		for _, rel := range []string{"4.22", "5.0"} {
			assert.True(t, seen.Has(dateRelease{date: date, release: rel}),
				"missing call for date %s release %s", date, rel)
		}
	}
}

func TestBackfill_PropagatesReplaceError(t *testing.T) {
	start := civil.Date{Year: 2026, Month: 7, Day: 1}
	end := civil.Date{Year: 2026, Month: 7, Day: 1}
	store := &fakeStore{replaceErr: fmt.Errorf("disk full")}

	err := backfillSummaries(store, start, end)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}
