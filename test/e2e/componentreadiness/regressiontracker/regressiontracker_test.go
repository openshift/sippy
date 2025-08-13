package regressiontracker

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanupAllRegressions(dbc *db.DB) {
	// Delete all test regressions in the e2e postgres db.
	res := dbc.DB.Where("1 = 1").Delete(&models.TestRegression{})
	if res.Error != nil {
		log.Errorf("error deleting test regressions: %v", res.Error)
	}
}

func Test_RegressionTracker(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)
	newRegression := componentreport.ReportTestSummary{
		Identification: crtest.Identification{
			RowIdentification: crtest.RowIdentification{
				Component:  "comp",
				Capability: "cap",
				TestName:   "fake test",
				TestSuite:  "fakesuite",
				TestID:     "faketestid",
			},
			ColumnIdentification: crtest.ColumnIdentification{
				Variants: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
		},
	}
	view := crview.View{
		Name: "4.19-main",
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.19",
			},
		},
	}

	t.Run("open a new regression", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		tr, err := tracker.OpenRegression(view, newRegression)
		require.NoError(t, err)
		assert.Equal(t, "4.19", tr.Release)
		assert.Equal(t, "4.19-main", tr.View)
		assert.ElementsMatch(t, pq.StringArray([]string{"a:b", "c:d"}), tr.Variants)
		assert.True(t, tr.ID > 0)
	})

	t.Run("close and reopen a regression", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		tr, err := tracker.OpenRegression(view, newRegression)
		require.NoError(t, err)

		// look it up just to be sure:
		lookup := &models.TestRegression{
			ID: tr.ID,
		}
		dbc.DB.First(&lookup)
		assert.Equal(t, tr.ID, lookup.ID)
		assert.Equal(t, tr.TestName, lookup.TestName)

		// Now close it:
		assert.False(t, lookup.Closed.Valid)
		lookup.Closed = sql.NullTime{Valid: true, Time: time.Now()}
		err = tracker.UpdateRegression(lookup)
		assert.NoError(t, err)
		// look it up again because we're being ridiculous:
		dbc.DB.First(&lookup)

		assert.True(t, lookup.Closed.Valid)

		lookup.Closed = sql.NullTime{Valid: false}
		err = tracker.UpdateRegression(lookup)
		assert.NoError(t, err)
		dbc.DB.First(&lookup)
		assert.False(t, lookup.Closed.Valid)
	})

	t.Run("list current regressions for release", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		var err error
		open419, err := rawCreateRegression(dbc, "4.19-main", "4.19",
			"test1ID", "test 1",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Time{})
		require.NoError(t, err)
		recentlyClosed419, err := rawCreateRegression(dbc, "4.19-main", "4.19",
			"test2ID", "test 2",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Now().Add(-2*24*time.Hour))
		require.NoError(t, err)
		_, err = rawCreateRegression(dbc, "4.19-main", "4.19",
			"test3ID", "test 3",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Now().Add(-70*24*time.Hour))
		require.NoError(t, err)
		_, err = rawCreateRegression(dbc, "4.18-main", "4.18",
			"test1ID", "test 1",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Time{})
		require.NoError(t, err)

		// List all regressions for 4.19, should exclude 4.18, include recently closed regressions,
		// and exclude older closed regressions.
		relRegressions, err := tracker.ListCurrentRegressionsForRelease("4.19")
		require.NoError(t, err)
		assert.Equal(t, 2, len(relRegressions))
		for _, rel := range relRegressions {
			assert.True(t, rel.ID == open419.ID || rel.ID == recentlyClosed419.ID,
				"unexpected regression was returned: %+v", *rel)
		}
	})

	t.Run("closing a regression should resolve associated triages that have no other active regressions", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

		regressionToClose, err := tracker.OpenRegression(view, newRegression)
		require.NoError(t, err)

		// Create a second regression that will remain open
		secondRegression := componentreport.ReportTestSummary{
			Identification: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "comp2",
					Capability: "cap2",
					TestName:   "second test",
					TestSuite:  "fakesuite",
					TestID:     "secondtestid",
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
			},
		}
		regressionToRemainOpened, err := tracker.OpenRegression(view, secondRegression)
		require.NoError(t, err)

		// Create first triage associated only with the first regression
		triage := models.Triage{
			URL:         "https://issues.redhat.com/browse/TEST-123",
			Description: "Test triage for auto-resolution",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				*regressionToClose,
			},
		}
		dbWithContext := dbc.DB.WithContext(context.WithValue(context.Background(), models.CurrentUserKey, "e2e-test"))
		res := dbWithContext.Create(&triage)
		require.NoError(t, res.Error)

		// Create second triage associated with both regressions
		triage2 := models.Triage{
			URL:         "https://issues.redhat.com/browse/TEST-456",
			Description: "Test triage with multiple regressions",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				*regressionToClose,
				*regressionToRemainOpened,
			},
		}
		res = dbWithContext.Create(&triage2)
		require.NoError(t, res.Error)

		// Close the regression with a time of NOW, this should not result in resolved triage
		regressionToClose.Closed = sql.NullTime{Valid: true, Time: time.Now()}
		err = tracker.UpdateRegression(regressionToClose)
		require.NoError(t, err)

		// Verify the regression is closed
		var checkRegression models.TestRegression
		_ = dbc.DB.First(&checkRegression, regressionToClose.ID)
		assert.True(t, checkRegression.Closed.Valid, "Regression should be closed by SyncRegressionsForReport")

		err = tracker.ResolveTriages()
		require.NoError(t, err)

		// Verify the triage is NOT resolved
		checkTriage := models.Triage{}
		res = dbc.DB.First(&checkTriage, triage.ID)
		require.NoError(t, res.Error)
		assert.False(t, checkTriage.Resolved.Valid, "Triage should NOT be automatically resolved when its regression has been closed less than 5 days ago")

		// Close the regression with a time > 5 days in the past, this should result in resolved triage
		sixDaysAgo := time.Now().Add(-6 * 24 * time.Hour)
		regressionToClose.Closed = sql.NullTime{Valid: true, Time: sixDaysAgo}
		err = tracker.UpdateRegression(regressionToClose)
		require.NoError(t, err)

		// Verify the regression is closed at the proper time
		_ = dbc.DB.First(&checkRegression, regressionToClose.ID)
		assert.True(t, checkRegression.Closed.Valid, "Regression should be closed by SyncRegressionsForReport")
		assert.WithinDuration(t, sixDaysAgo, checkRegression.Closed.Time, time.Second, "regression closing time should be six days ago")

		err = tracker.ResolveTriages()
		require.NoError(t, err)

		// Verify the triage is now resolved
		res = dbc.DB.First(&checkTriage, triage.ID)
		require.NoError(t, res.Error)
		assert.True(t, checkTriage.Resolved.Valid, "Triage should be automatically resolved when its only regression is closed")
		assert.WithinDuration(t, checkRegression.Closed.Time, checkTriage.Resolved.Time, time.Second, "Triage resolution time should match regression closing time")

		// Verify triage2 is NOT resolved because it still has an open regression
		checkTriage2 := models.Triage{}
		res = dbc.DB.First(&checkTriage2, triage2.ID)
		require.NoError(t, res.Error)
		assert.False(t, checkTriage2.Resolved.Valid, "Triage2 should not be resolved because it still has an open regression")

		// Verify an audit log entry was created correctly for the triage resolution
		var auditLog models.AuditLog
		res = dbc.DB.Where("table_name = ?", "triage").
			Where("row_id = ?", triage.ID).
			Where("operation = ?", models.Update).
			Order("created_at DESC").
			First(&auditLog)
		require.NoError(t, res.Error)
		assert.Equal(t, "regression-tracker", auditLog.User, "Audit log should show regression-tracker as the user for auto-resolution")
	})

}

func rawCreateRegression(
	dbc *db.DB,
	view string,
	release string,
	testID string,
	testName string,
	variants []string,
	opened, closed time.Time) (*models.TestRegression, error) {
	newRegression := &models.TestRegression{
		View:     view,
		Release:  release,
		TestID:   testID,
		TestName: testName,
		Opened:   opened,
		Variants: variants,
	}
	if closed.IsZero() {
		newRegression.Closed = sql.NullTime{
			Valid: false,
		}
	} else {
		newRegression.Closed = sql.NullTime{
			Valid: true,
			Time:  closed,
		}
	}
	res := dbc.DB.Create(newRegression)
	return newRegression, res.Error
}
