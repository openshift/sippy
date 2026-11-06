package regressioncacheloader

import (
	"context"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
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

// reportWithFixedRegression builds the smallest report that still lists a regressed test. The
// test is already marked fixed so view processing stops before the test details query.
func reportWithFixedRegression(testID string) *crtype.ComponentReport {
	return &crtype.ComponentReport{
		Rows: []crtype.ReportRow{{
			Columns: []crtype.ReportColumn{{
				RegressedTests: []crtype.ReportTestSummary{{
					Identification: crtest.Identification{
						RowIdentification: crtest.RowIdentification{TestID: testID, TestName: testID},
					},
					TestComparison: testdetails.TestComparison{ReportStatus: crtest.FixedRegression},
				}},
			}},
		}},
	}
}

// fakeRegressionStore records the regression lifecycle calls the loader makes, so tests can
// assert whether a release had its regressions closed. It is a stand-in for the loader's own
// RegressionStore interface, not for a storage client.
type fakeRegressionStore struct {
	componentreadiness.RegressionStore // unused methods panic if ever called

	// current holds each release's regressions.
	current             map[string][]*models.TestRegression
	closedIDs           []uint
	upsertedViews       []string
	resolveTriagesCalls int
}

func (f *fakeRegressionStore) ListCurrentRegressionsForRelease(release string) ([]*models.TestRegression, error) {
	return f.current[release], nil
}

func (f *fakeRegressionStore) OpenRegression(view crview.View, regressedTest crtype.ReportTestSummary) (*models.TestRegression, error) {
	reg := &models.TestRegression{
		ID:      uint(100 + len(f.current[view.SampleRelease.Name])),
		Release: view.SampleRelease.Name,
		TestID:  regressedTest.TestID,
		Opened:  time.Now(),
	}
	return reg, nil
}

func (f *fakeRegressionStore) UpdateRegression(reg *models.TestRegression) error {
	if reg.Closed.Valid {
		f.closedIDs = append(f.closedIDs, reg.ID)
	}
	return nil
}

func (f *fakeRegressionStore) UpsertRegressionView(_ uint, viewName string) error {
	f.upsertedViews = append(f.upsertedViews, viewName)
	return nil
}

func (f *fakeRegressionStore) DeactivateRolledOffViews(_ []uint, _ map[uint][]string) error {
	return nil
}

func (f *fakeRegressionStore) ResolveTriages() error {
	f.resolveTriagesCalls++
	return nil
}

// TestLoadClosesRolledOffRegressions covers the normal roll off path: a report that still
// finds regressions closes the ones it no longer lists.
func TestLoadClosesRolledOffRegressions(t *testing.T) {
	const release = "4.16"

	store := &fakeRegressionStore{current: map[string][]*models.TestRegression{
		release: {{ID: 1, Release: release, TestID: "rolled-off"}},
	}}
	loader := &RegressionCacheLoader{
		views:           []crview.View{trackingView("a-view", release)},
		regressionStore: store,
		logger:          log.WithField("loader", "regression-cache-test"),
		generateReport: func(_ context.Context, _ *componentreadiness.ComponentReportGenerator) (*crtype.ComponentReport, error) {
			return reportWithFixedRegression("still-regressed"), nil
		},
	}

	loader.Load()

	assert.Empty(t, loader.Errors())
	assert.Equal(t, []uint{1}, store.closedIDs, "a report with regressions still closes rolled off ones")
}
