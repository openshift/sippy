package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	intutil "github.com/openshift/sippy/test/integration/util"
	"github.com/stretchr/testify/require"
)

func TestTriageArchival(t *testing.T) {
	database := intutil.NewTestDB(t, pgContainer)
	// PostgreSQL's transaction timestamp makes the strict 90-day boundary deterministic.
	tx := database.DB.WithContext(context.WithValue(context.Background(), models.CurrentUserKey, "archival-test")).Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	dbc := &db.DB{DB: tx}
	var now time.Time
	require.NoError(t, tx.Raw("SELECT CURRENT_TIMESTAMP").Scan(&now).Error)
	today := civil.DateOf(now.UTC())
	future := today.AddDays(1)
	for _, release := range []models.ReleaseDefinition{
		{Release: "ga", GADate: &today},
		{Release: "other-ga", GADate: &today},
		{Release: "future", GADate: &future},
		{Release: "unknown"},
		{Release: "deleted", GADate: &today},
	} {
		require.NoError(t, tx.Create(&release).Error)
	}
	require.NoError(t, tx.Where("release = ?", "deleted").Delete(&models.ReleaseDefinition{}).Error)
	for i, tc := range []struct {
		name       string
		age        time.Duration
		unresolved bool
		releases   []string
		visible    bool
	}{
		{name: "unresolved", unresolved: true, releases: []string{"ga"}, visible: true},
		{name: "recent", age: 89 * 24 * time.Hour, releases: []string{"ga"}, visible: true},
		{name: "just before boundary", age: 90*24*time.Hour - time.Microsecond, releases: []string{"ga"}, visible: true},
		{name: "exact boundary", age: 90 * 24 * time.Hour, releases: []string{"ga"}, visible: true},
		{name: "beyond boundary", age: 90*24*time.Hour + time.Microsecond, releases: []string{"ga"}},
		{name: "all GA open regressions", age: 100 * 24 * time.Hour, releases: []string{"ga", "other-ga"}},
		{name: "mixed releases", age: 100 * 24 * time.Hour, releases: []string{"ga", "future"}, visible: true},
		{name: "missing GA date", age: 100 * 24 * time.Hour, releases: []string{"ga", "unknown"}, visible: true},
		{name: "deleted metadata", age: 100 * 24 * time.Hour, releases: []string{"ga", "deleted"}, visible: true},
		{name: "missing metadata", age: 100 * 24 * time.Hour, releases: []string{"ga", "missing"}, visible: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var regs []*models.TestRegression
			for j, release := range tc.releases {
				reg := intutil.CreateTestRegression(t, dbc, fmt.Sprintf("archival-%d-%d", i, j), release)
				regs = append(regs, reg)
			}
			triage := intutil.CreateTriage(t, dbc, fmt.Sprintf("https://example.com/%d", i), intutil.WithRegressions(regs...))
			if !tc.unresolved {
				require.NoError(t, tx.Model(&models.Triage{}).Where("id = ?", triage.ID).UpdateColumn("resolved", now.Add(-tc.age)).Error)
			}
			// Only the first regression is in this view: other releases still protect the triage.
			view := fmt.Sprintf("archival-%d", i)
			require.NoError(t, tx.Create(&models.RegressionView{TestRegressionID: regs[0].ID, ViewName: view, Active: true}).Error)
			for _, filter := range []string{"", view} {
				listed, err := query.ListTriages(dbc, filter, false)
				require.NoError(t, err)
				found := false
				for _, item := range listed {
					if item.ID == triage.ID {
						found = true
						require.Len(t, item.Regressions, len(regs))
					}
				}
				require.Equal(t, tc.visible, found)
				historical, err := query.ListTriages(dbc, view, true)
				require.NoError(t, err)
				require.Len(t, historical, 1)
				require.Len(t, historical[0].Regressions, len(regs))
			}
			wrongView, err := query.ListTriages(dbc, "nonexistent", true)
			require.NoError(t, err)
			require.Empty(t, wrongView)
			// Query-time archival preserves both the triage and its associations.
			var stored models.Triage
			require.NoError(t, tx.Preload("Regressions").First(&stored, triage.ID).Error)
			require.Len(t, stored.Regressions, len(regs))
			if !tc.visible {
				require.NoError(t, tx.Model(&models.Triage{}).Where("id = ?", triage.ID).UpdateColumn("resolved", nil).Error)
				listed, err := query.ListTriages(dbc, view, false)
				require.NoError(t, err)
				require.Len(t, listed, 1, "reopening reactivates")
				require.NoError(t, tx.Model(&models.Triage{}).Where("id = ?", triage.ID).UpdateColumn("resolved", now.Add(-tc.age)).Error)
				nonGA := intutil.CreateTestRegression(t, dbc, fmt.Sprintf("non-ga-%d", i), "future")
				require.NoError(t, tx.Model(&triage).Association("Regressions").Append(nonGA))
				listed, err = query.ListTriages(dbc, view, false)
				require.NoError(t, err)
				require.Len(t, listed, 1, "adding non-GA association reactivates across releases")
			}
		})
	}
}
