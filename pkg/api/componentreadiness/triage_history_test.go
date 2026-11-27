// Tests for historical (closed regression) test_details links: the sample window, basis resolution,
// per-view link selection in InjectRegressionHATEOASLinks, triage enrichment, and the request
// parsing round trip. General InjectRegressionHATEOASLinks behavior for current links is covered by
// TestInjectRegressionHATEOASLinks in triage_test.go.
package componentreadiness

import (
	"database/sql"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

// historyReleases mirrors the release configuration behind the 5.0-main view in TRT-3006.
func historyReleases() []v1.Release {
	ga422 := civil.Date{Year: 2026, Month: 6, Day: 9}
	ga421 := civil.Date{Year: 2026, Month: 2, Day: 3}
	return []v1.Release{
		{Release: "5.0", PreviousRelease: "4.22"},
		{Release: "4.22", PreviousRelease: "4.21", GADate: &ga422},
		{Release: "4.21", GADate: &ga421},
	}
}

// historyView builds a 5.0 view with the given basis expressions.
func historyView(name, baseStart, baseEnd string) crview.View {
	return crview.View{
		Name: name,
		BaseRelease: reqopts.RelativeRelease{
			Release:       reqopts.Release{Name: "4.22"},
			RelativeStart: baseStart,
			RelativeEnd:   baseEnd,
		},
		SampleRelease: reqopts.RelativeRelease{
			Release:       reqopts.Release{Name: "5.0"},
			RelativeStart: "now-7d",
			RelativeEnd:   "now",
		},
		VariantOptions: reqopts.Variants{
			ColumnGroupBy: sets.New[string]("Network", "Platform", "Topology"),
			DBGroupBy:     sets.New[string]("Architecture", "FeatureSet", "Installer", "Network", "Platform", "Suite", "Topology", "Upgrade", "LayeredProduct"),
			IncludeVariants: map[string][]string{
				"Platform": {"aws", "azure"},
			},
		},
		AdvancedOptions: reqopts.Advanced{
			Confidence:     95,
			PityFactor:     5,
			MinimumFailure: 3,
		},
		TestFilters: reqopts.TestFilters{
			Lifecycles: []string{"blocking"},
		},
		RegressionTracking: crview.RegressionTracking{Enabled: true},
	}
}

// regression49971 mirrors the closed reproducer regression from triage 792.
func regression49971() *models.TestRegression {
	return &models.TestRegression{
		ID:          49971,
		Release:     "5.0",
		BaseRelease: "4.22",
		Component:   "Networking / cluster-network-operator",
		Capability:  "Upgrade",
		TestID:      "openshift-tests-upgrade:abc123",
		TestName:    "sample test",
		Variants: pq.StringArray{
			"Architecture:amd64", "FeatureSet:default", "Installer:ipi", "LayeredProduct:none",
			"Network:ovn", "Platform:azure", "Suite:serial", "Topology:ha", "Upgrade:minor",
		},
		Opened:      time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC),
		Closed:      sql.NullTime{Time: time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC), Valid: true},
		LastFailure: sql.NullTime{Time: time.Date(2026, 9, 29, 1, 33, 26, 0, time.UTC), Valid: true},
		Views:       []models.RegressionView{{TestRegressionID: 49971, ViewName: "5.0-main", Active: false}},
	}
}

func TestHistoricalSampleWindow(t *testing.T) {
	tests := []struct {
		name        string
		lastFailure time.Time
		wantStart   time.Time
		wantEnd     time.Time
	}{
		{
			name:        "regression 49971 covers September 22 through 29",
			lastFailure: time.Date(2026, 9, 29, 1, 33, 26, 0, time.UTC),
			wantStart:   time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "failure just before midnight is included by the exclusive end",
			lastFailure: time.Date(2026, 9, 29, 23, 59, 59, 999, time.UTC),
			wantStart:   time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "failure exactly at midnight belongs to that date",
			lastFailure: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
			wantStart:   time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "non-UTC timestamps use the UTC calendar date",
			lastFailure: time.Date(2026, 9, 29, 22, 0, 0, 0, time.FixedZone("UTC-5", -5*3600)),
			wantStart:   time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "window crosses a month boundary",
			lastFailure: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
			wantStart:   time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := HistoricalSampleWindow(tt.lastFailure)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
			assert.False(t, tt.lastFailure.Before(start), "last failure must be at or after the start")
			assert.True(t, tt.lastFailure.Before(end), "last failure must be before the exclusive end")
		})
	}
}

func TestGenerateHistoricalTestDetailsURL(t *testing.T) {
	releases := historyReleases()
	gaView := historyView("5.0-main", "ga-30d", "ga")

	crossCompareView := historyView("5.0-arm64", "ga-30d", "ga")
	crossCompareView.VariantOptions.VariantCrossCompare = []string{"Architecture"}
	crossCompareView.VariantOptions.CompareVariants = map[string][]string{"Architecture": {"arm64"}}

	tests := []struct {
		name       string
		regression func() *models.TestRegression
		view       crview.View
		wantErr    error
		wantAnyErr bool
		want       map[string]string
		wantMulti  map[string][]string
		wantAbsent []string
	}{
		{
			name:       "GA basis keeps its configured meaning",
			regression: regression49971,
			view:       gaView,
			want: map[string]string{
				"view":            "5.0-main",
				"testId":          "openshift-tests-upgrade:abc123",
				"regressionId":    "49971",
				"component":       "Networking / cluster-network-operator",
				"capability":      "Upgrade",
				"sampleRelease":   "5.0",
				"sampleStartTime": "2026-09-22T00:00:00Z",
				"sampleEndTime":   "2026-09-30T00:00:00Z",
				"baseRelease":     "4.22",
				"baseStartTime":   "2026-05-10T00:00:00Z",
				"baseEndTime":     "2026-06-09T23:59:59Z",
				"confidence":      "95",
				"Architecture":    "amd64",
				"FeatureSet":      "default",
				"Installer":       "ipi",
				"LayeredProduct":  "none",
				"Network":         "ovn",
				"Platform":        "azure",
				"Suite":           "serial",
				"Topology":        "ha",
				"Upgrade":         "minor",
				"environment":     "Architecture:amd64 FeatureSet:default Installer:ipi LayeredProduct:none Network:ovn Platform:azure Suite:serial Topology:ha Upgrade:minor",
			},
			wantMulti: map[string][]string{
				"includeVariant": {"Platform:aws", "Platform:azure"},
				"testLifecycles": {"blocking"},
			},
			wantAbsent: []string{"testBasisRelease"},
		},
		{
			name:       "rolling basis resolves relative to the last failure",
			regression: regression49971,
			view:       historyView("5.0-rolling", "now-60d", "now-30d"),
			want: map[string]string{
				"baseStartTime":   "2026-07-31T00:00:00Z",
				"baseEndTime":     "2026-08-30T23:59:59Z",
				"sampleStartTime": "2026-09-22T00:00:00Z",
				"sampleEndTime":   "2026-09-30T00:00:00Z",
			},
		},
		{
			name:       "absolute basis is unchanged",
			regression: regression49971,
			view:       historyView("5.0-absolute", "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"),
			want: map[string]string{
				"baseStartTime": "2026-01-01T00:00:00Z",
				"baseEndTime":   "2026-02-01T00:00:00Z",
			},
		},
		{
			name: "basis release override is preserved",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.BaseRelease = "4.21"
				return r
			},
			view: gaView,
			want: map[string]string{
				"baseRelease":      "4.22",
				"testBasisRelease": "4.21",
			},
		},
		{
			name:       "cross-compare settings are retained",
			regression: regression49971,
			view:       crossCompareView,
			want: map[string]string{
				"view":                "5.0-arm64",
				"variantCrossCompare": "Architecture",
				"compareVariant":      "Architecture:arm64",
			},
		},
		{
			name: "force-closed regression still anchors on its last failure",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.ForceClosed = true
				r.LastFailure = sql.NullTime{Time: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), Valid: true}
				return r
			},
			view: gaView,
			want: map[string]string{
				"sampleStartTime": "2026-09-18T00:00:00Z",
				"sampleEndTime":   "2026-09-26T00:00:00Z",
			},
		},
		{
			name: "missing last failure is reported instead of substituting today",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.LastFailure = sql.NullTime{}
				return r
			},
			view:    gaView,
			wantErr: errMissingLastFailure,
		},
		{
			name: "spot-check regression has no historical link because its samples follow the current time",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.TestID = crtest.SpotCheckTestID("spotcheck-30d", "Networking", "ovn")
				return r
			},
			view:    gaView,
			wantErr: errSpotCheckHistory,
		},
		{
			name:       "basis GA date that cannot be resolved is an error",
			regression: regression49971,
			view: func() crview.View {
				v := historyView("5.0-unknown-basis", "ga-30d", "ga")
				v.BaseRelease.Name = "3.11"
				return v
			}(),
			wantAnyErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateHistoricalTestDetailsURL(tt.regression(), tt.view, releases, "http://frontend")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				// Expected gaps must wrap errNoHistoricalLink so they are logged at debug level.
				require.ErrorIs(t, err, errNoHistoricalLink)
				return
			}
			if tt.wantAnyErr {
				require.Error(t, err)
				require.NotErrorIs(t, err, errNoHistoricalLink, "unexpected failures must still be logged as errors")
				return
			}
			require.NoError(t, err)
			u, err := url.Parse(got)
			require.NoError(t, err)
			assert.Equal(t, "/api/component_readiness/test_details", u.Path)
			q := u.Query()
			for key, want := range tt.want {
				assert.Equal(t, want, q.Get(key), "query parameter %s", key)
			}
			for key, want := range tt.wantMulti {
				assert.ElementsMatch(t, want, q[key], "query parameter %s", key)
			}
			for _, key := range tt.wantAbsent {
				assert.False(t, q.Has(key), "query parameter %s should be absent", key)
			}
		})
	}

}

func TestInjectRegressionHATEOASLinksHistory(t *testing.T) {
	releases := historyReleases()
	unknownBasis := historyView("5.0-unknown-basis", "ga-30d", "ga")
	unknownBasis.BaseRelease.Name = "3.11"
	views := []crview.View{
		historyView("5.0-main", "ga-30d", "ga"),
		historyView("5.0-azure", "ga-30d", "ga"),
		unknownBasis,
	}
	closedAt := sql.NullTime{Time: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Valid: true}

	tests := []struct {
		name           string
		regression     func() *models.TestRegression
		wantHistorical []string
		wantCurrent    []string
		// wantUnavailable maps a view to the reason its closed association has no historical link.
		wantUnavailable map[string]string
	}{
		{
			name:           "closed regression with inactive view gets a historical link",
			regression:     regression49971,
			wantHistorical: []string{"5.0-main"},
		},
		{
			name: "closed regression in multiple views gets one historical link per view",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.Views = append(r.Views, models.RegressionView{TestRegressionID: r.ID, ViewName: "5.0-azure", Active: false})
				return r
			},
			wantHistorical: []string{"5.0-main", "5.0-azure"},
		},
		{
			name: "regression open elsewhere links currently and historically per view",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.Closed = sql.NullTime{}
				r.Views = []models.RegressionView{
					{TestRegressionID: r.ID, ViewName: "5.0-main", Active: true},
					{TestRegressionID: r.ID, ViewName: "5.0-azure", Active: false, ClosedAt: closedAt},
				}
				return r
			},
			wantCurrent:    []string{"5.0-main"},
			wantHistorical: []string{"5.0-azure"},
		},
		{
			name: "closed regression with a stale active flag is still historical",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.Views[0].Active = true
				return r
			},
			wantHistorical: []string{"5.0-main"},
		},
		{
			name: "force-closed regression gets a historical link",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.ForceClosed = true
				return r
			},
			wantHistorical: []string{"5.0-main"},
		},
		{
			name: "removed view produces no link and explains why",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.Views = []models.RegressionView{{TestRegressionID: r.ID, ViewName: "4.99-removed", Active: false}}
				return r
			},
			wantUnavailable: map[string]string{"4.99-removed": reasonViewNotConfigured},
		},
		{
			name: "removed view of an active association is not reported as a historical gap",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.Closed = sql.NullTime{}
				r.Views = []models.RegressionView{{TestRegressionID: r.ID, ViewName: "4.99-removed", Active: true}}
				return r
			},
		},
		{
			name: "closed regression without a last failure explains the missing link",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.LastFailure = sql.NullTime{}
				return r
			},
			wantUnavailable: map[string]string{"5.0-main": errMissingLastFailure.Reason},
		},
		{
			// 5.1-main with spotcheck-30d produces this combination: the spot-check regression closed
			// in main while azure still reports it.
			name: "spot-check regression open elsewhere links currently but never historically",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.TestID = crtest.SpotCheckTestID("spotcheck-30d", "Networking", "ovn")
				r.Closed = sql.NullTime{}
				r.Views = []models.RegressionView{
					{TestRegressionID: r.ID, ViewName: "5.0-main", Active: false, ClosedAt: closedAt},
					{TestRegressionID: r.ID, ViewName: "5.0-azure", Active: true},
				}
				return r
			},
			wantCurrent:     []string{"5.0-azure"},
			wantUnavailable: map[string]string{"5.0-main": errSpotCheckHistory.Reason},
		},
		{
			name: "basis that cannot be resolved for a closed association gets a generic reason",
			regression: func() *models.TestRegression {
				r := regression49971()
				r.Views = []models.RegressionView{{TestRegressionID: r.ID, ViewName: "5.0-unknown-basis", Active: false}}
				return r
			},
			wantUnavailable: map[string]string{"5.0-unknown-basis": reasonLinkFailed},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.regression()
			InjectRegressionHATEOASLinks(r, &RegressionLinkConfig{Views: views, Releases: releases, CRTimeRoundingFactor: 12 * time.Hour, CRTimeRoundingOffset: 4 * time.Hour}, "http://api", "http://frontend")
			assert.Equal(t, "http://api/api/component_readiness/regressions/49971", r.Links["self"])
			assert.Len(t, r.Links, 1+len(tt.wantHistorical)+len(tt.wantCurrent))
			if tt.wantUnavailable == nil {
				assert.Nil(t, r.HistoricalLinkUnavailable)
			} else {
				assert.Equal(t, tt.wantUnavailable, r.HistoricalLinkUnavailable)
			}
			for _, viewName := range tt.wantHistorical {
				link := r.Links["test_details:"+viewName]
				require.NotEmpty(t, link, "expected historical link for %s", viewName)
				u, err := url.Parse(link)
				require.NoError(t, err)
				assert.Equal(t, "49971", u.Query().Get("regressionId"))
				assert.Equal(t, "2026-09-30T00:00:00Z", u.Query().Get("sampleEndTime"))
				assert.Equal(t, viewName, u.Query().Get("view"))
			}
			for _, viewName := range tt.wantCurrent {
				link := r.Links["test_details:"+viewName]
				require.NotEmpty(t, link, "expected current link for %s", viewName)
				u, err := url.Parse(link)
				require.NoError(t, err)
				assert.False(t, u.Query().Has("regressionId"), "current links must not claim a historical regression")
				assert.NotEqual(t, "2026-09-30T00:00:00Z", u.Query().Get("sampleEndTime"))
			}
		})
	}
}

func TestInjectTriageRegressionLinks(t *testing.T) {
	releases := historyReleases()
	views := []crview.View{historyView("5.0-main", "ga-30d", "ga")}
	req := httptest.NewRequest("GET", "http://sippy.example/api/component_readiness/triages/792", nil)
	req.Header.Set("Origin", "https://frontend.example")

	tests := []struct {
		name       string
		linkConfig *RegressionLinkConfig
		wantLinks  bool
	}{
		{
			name:       "embedded regressions receive regression links",
			linkConfig: &RegressionLinkConfig{Views: views, Releases: releases},
			wantLinks:  true,
		},
		{
			name:       "nil config leaves regressions untouched",
			linkConfig: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triage := &models.Triage{ID: 792, Regressions: []models.TestRegression{*regression49971()}}
			injectTriageRegressionLinks(triage, tt.linkConfig, req)
			links := triage.Regressions[0].Links
			if !tt.wantLinks {
				assert.Nil(t, links)
				return
			}
			assert.Equal(t, "http://sippy.example/api/component_readiness/regressions/49971", links["self"])
			assert.Contains(t, links["test_details:5.0-main"], "https://frontend.example/api/component_readiness/test_details?")
		})
	}
}

// TestHistoricalURLSurvivesRequestParsing round-trips generated historical links through the request
// parser with production rounding, so explicit boundaries are not moved even when the exclusive end
// falls on today's UTC date.
func TestHistoricalURLSurvivesRequestParsing(t *testing.T) {
	releases := historyReleases()
	view := historyView("5.0-main", "ga-30d", "ga")
	allJobVariants := crtest.JobVariants{Variants: map[string][]string{
		"Architecture":   {"amd64", "arm64"},
		"FeatureSet":     {"default"},
		"Installer":      {"ipi"},
		"LayeredProduct": {"none"},
		"Network":        {"ovn"},
		"Platform":       {"aws", "azure"},
		"Suite":          {"serial"},
		"Topology":       {"ha"},
		"Upgrade":        {"minor"},
	}}
	now := time.Now().UTC()
	today := civil.DateOf(now)

	tests := []struct {
		name        string
		lastFailure time.Time
	}{
		{name: "last failure today", lastFailure: now.Add(-time.Second)},
		{name: "last failure yesterday ends at today's midnight", lastFailure: today.AddDays(-1).In(time.UTC).Add(12 * time.Hour)},
		{name: "last failure long ago", lastFailure: time.Date(2026, 9, 29, 1, 33, 26, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := regression49971()
			r.LastFailure = sql.NullTime{Time: tt.lastFailure, Valid: true}
			link, err := generateHistoricalTestDetailsURL(r, view, releases, "http://frontend")
			require.NoError(t, err)

			req := httptest.NewRequest("GET", link, nil)
			opts, _, err := utils.ParseComponentReportRequest([]crview.View{view}, releases, req, allJobVariants, 12*time.Hour, 4*time.Hour)
			require.NoError(t, err)

			wantStart, wantEnd := HistoricalSampleWindow(tt.lastFailure)
			assert.Equal(t, "5.0", opts.SampleRelease.Name)
			assert.Equal(t, wantStart, opts.SampleRelease.Start)
			assert.Equal(t, wantEnd, opts.SampleRelease.End)
			assert.Equal(t, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), opts.BaseRelease.Start)
			assert.Equal(t, time.Date(2026, 6, 9, 23, 59, 59, 0, time.UTC), opts.BaseRelease.End)
			require.Len(t, opts.TestIDOptions, 1)
			assert.Equal(t, r.TestID, opts.TestIDOptions[0].TestID)
			assert.Len(t, opts.TestIDOptions[0].RequestedVariants, len(r.Variants))
		})
	}
}
