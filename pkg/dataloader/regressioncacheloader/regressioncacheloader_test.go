package regressioncacheloader

import (
	"context"
	"errors"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/db/models"
)

// TestNew_NilDataProvider verifies that New rejects a nil dataProvider. Without
// this guard a caller would receive a valid-looking loader that later panics when
// Load dereferences the provider via dataProvider.Cache().
func TestNew_NilDataProvider(t *testing.T) {
	loader, err := New(nil, nil, nil, nil, nil, 0, 0, nil)
	require.Error(t, err)
	assert.Nil(t, loader)
	assert.Contains(t, err.Error(), "dataProvider must not be nil")
}

// trackingView builds a regression tracking view whose release window needs no release
// configuration, keeping the tests focused on view processing rather than release lookups.
func trackingView(name, release string) crview.View {
	view := crview.View{
		Name: name,
		BaseRelease: reqopts.RelativeRelease{
			Release:       reqopts.Release{Name: "4.15"},
			RelativeStart: "now-30d",
			RelativeEnd:   "now",
		},
		SampleRelease: reqopts.RelativeRelease{
			Release:       reqopts.Release{Name: release},
			RelativeStart: "now-7d",
			RelativeEnd:   "now",
		},
	}
	view.RegressionTracking.Enabled = true
	return view
}

// fakeRegressionStore records the regression lifecycle calls the loader makes, so tests
// can assert which releases had their closing skipped. It is a stand-in for the loader's
// own RegressionStore interface, not for a storage client.
type fakeRegressionStore struct {
	componentreadiness.RegressionStore // unused methods panic if ever called

	current             map[string][]*models.TestRegression
	closedIDs           []uint
	resolveTriagesCalls int
}

func (f *fakeRegressionStore) ListCurrentRegressionsForRelease(release string) ([]*models.TestRegression, error) {
	return f.current[release], nil
}

func (f *fakeRegressionStore) UpdateRegression(reg *models.TestRegression) error {
	if reg.Closed.Valid {
		f.closedIDs = append(f.closedIDs, reg.ID)
	}
	return nil
}

func (f *fakeRegressionStore) DeactivateRolledOffViews(_ []uint, _ map[uint][]string) error {
	return nil
}

func (f *fakeRegressionStore) ResolveTriages() error {
	f.resolveTriagesCalls++
	return nil
}

// TestLoadPerViewTimeout covers the regression where every view shared one deadline:
// the slow views ahead consumed it, and the views behind them failed instantly with
// context.DeadlineExceeded, so their releases never got their regressions updated.
// With a per-view deadline, a view that times out must not consume the budget of the
// views after it, and only its own release must skip regression closing.
func TestLoadPerViewTimeout(t *testing.T) {
	const (
		perViewTimeout = 200 * time.Millisecond
		slowRelease    = "4.16"
		fastRelease    = "4.17"
	)

	store := &fakeRegressionStore{current: map[string][]*models.TestRegression{
		slowRelease: {{ID: 1, Release: slowRelease}},
		fastRelease: {{ID: 2, Release: fastRelease}},
	}}

	type viewCall struct {
		release string
		budget  time.Duration
	}
	var calls []viewCall

	loader := &RegressionCacheLoader{
		views: []crview.View{
			trackingView("slow-view", slowRelease),
			trackingView("fast-view", fastRelease),
		},
		regressionStore: store,
		viewTimeout:     perViewTimeout,
		logger:          log.WithField("loader", "regression-cache-test"),
	}
	loader.generateReport = func(ctx context.Context, generator *componentreadiness.ComponentReportGenerator) (*crtype.ComponentReport, error) {
		release := generator.ReqOptions.SampleRelease.Name
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "expected a deadline on the view context")
		calls = append(calls, viewCall{release: release, budget: time.Until(deadline)})
		if release == slowRelease {
			<-ctx.Done() // a query that outlives this view's budget
			return nil, ctx.Err()
		}
		return &crtype.ComponentReport{}, nil
	}

	loader.Load()

	// The view after the one that timed out still ran, and with its own full budget.
	require.Len(t, calls, 2)
	assert.Equal(t, slowRelease, calls[0].release)
	assert.Equal(t, fastRelease, calls[1].release)
	for _, call := range calls {
		assert.Greater(t, call.budget, perViewTimeout/2,
			"view for release %s should get its own timeout budget, had %s", call.release, call.budget)
	}

	// The timed out view is reported as a deadline error naming the view.
	require.Len(t, loader.Errors(), 1)
	err := loader.Errors()[0]
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "slow-view")

	// Closing is skipped for the release that had errors, but happens for the healthy one.
	assert.NotContains(t, store.closedIDs, uint(1), "regressions must not be closed for a release that had errors")
	assert.Contains(t, store.closedIDs, uint(2), "healthy release should still close rolled off regressions")

	// Triage resolution is global, so it is skipped while any release had errors.
	assert.Zero(t, store.resolveTriagesCalls)
}

// TestProcessViewTimeout verifies processView only reports a timeout when a deadline
// expired, so ordinary failures keep their original cause, and that it names the deadline
// that actually passed rather than always blaming the view.
func TestProcessViewTimeout(t *testing.T) {
	view := trackingView("a-view", "4.16")
	view.RegressionTracking.Enabled = false // keep the test on report generation

	tests := []struct {
		name        string
		generate    reportGeneratorFunc
		parentSpent bool
		wantErr     string
		wantTimeout bool
	}{
		{
			name: "report generation outlives the view timeout",
			generate: func(ctx context.Context, _ *componentreadiness.ComponentReportGenerator) (*crtype.ComponentReport, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			wantErr:     "a-view exceeded the 50ms per-view timeout",
			wantTimeout: true,
		},
		{
			name: "load cycle deadline already spent is not blamed on the view",
			generate: func(ctx context.Context, _ *componentreadiness.ComponentReportGenerator) (*crtype.ComponentReport, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			parentSpent: true,
			wantErr:     "a-view exceeded the load cycle timeout",
			wantTimeout: true,
		},
		{
			name: "report generation fails for another reason",
			generate: func(_ context.Context, _ *componentreadiness.ComponentReportGenerator) (*crtype.ComponentReport, error) {
				return nil, errors.New("query failed")
			},
			wantErr: "query failed",
		},
		{
			name: "report generation succeeds",
			generate: func(_ context.Context, _ *componentreadiness.ComponentReportGenerator) (*crtype.ComponentReport, error) {
				return &crtype.ComponentReport{}, nil
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loader := &RegressionCacheLoader{
				viewTimeout:    50 * time.Millisecond,
				generateReport: tc.generate,
				logger:         log.WithField("loader", "regression-cache-test"),
			}

			parent := context.Background()
			if tc.parentSpent {
				// A load cycle whose own deadline passed before this view was reached.
				expired, cancel := context.WithDeadline(parent, time.Now().Add(-time.Minute))
				defer cancel()
				parent = expired
			}

			_, err := loader.processView(parent, view, cache.RequestOptions{},
				loader.logger.WithField("view", view.Name))

			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Equal(t, tc.wantTimeout, errors.Is(err, context.DeadlineExceeded))
		})
	}
}
