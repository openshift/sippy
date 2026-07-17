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
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"

	"github.com/openshift/sippy/test/e2e/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

func cleanupAllRegressions(dbc *db.DB) {
	// Delete regression views first to avoid FK constraint violations
	resView := dbc.DB.Where("1 = 1").Delete(&models.RegressionView{})
	if resView.Error != nil {
		log.Errorf("error deleting regression views: %v", resView.Error)
	}
	res := dbc.DB.Where("1 = 1").Delete(&models.TestRegression{})
	if res.Error != nil {
		log.Errorf("error deleting test regressions: %v", res.Error)
	}
}

func Test_RegressionTracker(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	// Pass nil jiraClient as we don't want to comment on jiras in the e2e test
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)
	newRegression := componentreport.ReportTestSummary{
		TestComparison: testdetails.TestComparison{
			BaseStats: &testdetails.ReleaseStats{
				Release: "4.18",
			},
		},
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
		assert.Equal(t, "4.18", tr.BaseRelease, "BaseRelease should be set from BaseStats.Release")
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
		open419, err := rawCreateRegression(dbc, "4.19",
			"test1ID", "test 1",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Time{})
		require.NoError(t, err)
		recentlyClosed419, err := rawCreateRegression(dbc, "4.19",
			"test2ID", "test 2",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Now().Add(-2*24*time.Hour))
		require.NoError(t, err)
		_, err = rawCreateRegression(dbc, "4.19",
			"test3ID", "test 3",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Now().Add(-70*24*time.Hour))
		require.NoError(t, err)
		_, err = rawCreateRegression(dbc, "4.18",
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

	t.Run("list returns regressions with BaseRelease set", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		tr, err := rawCreateRegressionWithBase(dbc, "4.19", "4.18", "baseTestID", "base test",
			[]string{"a:b"}, time.Now().Add(-1*24*time.Hour), time.Time{})
		require.NoError(t, err)
		relRegressions, err := tracker.ListCurrentRegressionsForRelease("4.19")
		require.NoError(t, err)
		require.Len(t, relRegressions, 1)
		assert.Equal(t, tr.ID, relRegressions[0].ID)
		assert.Equal(t, "4.18", relRegressions[0].BaseRelease, "ListCurrentRegressionsForRelease should return BaseRelease")
	})

	t.Run("closing a regression should resolve associated triages that have no other active regressions", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

		regressionToClose, err := tracker.OpenRegression(view, newRegression)
		require.NoError(t, err)

		// Create a second regression that will remain open
		secondRegression := componentreport.ReportTestSummary{
			TestComparison: testdetails.TestComparison{
				BaseStats: &testdetails.ReleaseStats{
					Release: "4.18",
				},
			},
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
			URL:         "https://redhat.atlassian.net/browse/TEST-123",
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
			URL:         "https://redhat.atlassian.net/browse/TEST-456",
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

func cleanupJobRuns(dbc *db.DB) {
	res := dbc.DB.Where("1 = 1").Delete(&models.RegressionJobRun{})
	if res.Error != nil {
		log.Errorf("error deleting regression job runs: %v", res.Error)
	}
}

func Test_RegressionJobRuns(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)
	view := crview.View{
		Name: "4.19-main",
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.19",
			},
		},
	}

	t.Run("merge job runs for a regression", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupJobRuns(dbc)

		reg, err := tracker.OpenRegression(view, componentreport.ReportTestSummary{
			TestComparison: testdetails.TestComparison{
				BaseStats: &testdetails.ReleaseStats{Release: "4.18"},
			},
			Identification: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "comp",
					Capability: "cap",
					TestName:   "job run test",
					TestID:     "jobruntestid",
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{"a": "b"},
				},
			},
		})
		require.NoError(t, err)

		jobRuns := []models.RegressionJobRun{
			{
				ProwJobRunID: "run-1",
				ProwJobName:  "periodic-ci-job-1",
				ProwJobURL:   "https://prow.ci/run-1",
				StartTime:    time.Now().Add(-24 * time.Hour),
				TestFailed:   true,
				TestFailures: 15,
				JobLabels:    []string{"InfraFailure"},
			},
			{
				ProwJobRunID: "run-2",
				ProwJobName:  "periodic-ci-job-1",
				ProwJobURL:   "https://prow.ci/run-2",
				StartTime:    time.Now().Add(-12 * time.Hour),
				TestFailed:   false,
				TestFailures: 0,
			},
		}

		err = tracker.MergeJobRuns(reg.ID, jobRuns)
		require.NoError(t, err)

		// Verify job runs were stored
		var stored []models.RegressionJobRun
		res := dbc.DB.Where("regression_id = ?", reg.ID).Order("prow_job_run_id").Find(&stored)
		require.NoError(t, res.Error)
		assert.Len(t, stored, 2)

		assert.Equal(t, "run-1", stored[0].ProwJobRunID)
		assert.Equal(t, "periodic-ci-job-1", stored[0].ProwJobName)
		assert.Equal(t, "https://prow.ci/run-1", stored[0].ProwJobURL)
		assert.True(t, stored[0].TestFailed)
		assert.Equal(t, 15, stored[0].TestFailures)
		assert.Equal(t, []string{"InfraFailure"}, []string(stored[0].JobLabels))

		assert.Equal(t, "run-2", stored[1].ProwJobRunID)
		assert.False(t, stored[1].TestFailed)
	})

	t.Run("merge deduplicates by prowjob_run_id", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupJobRuns(dbc)

		reg, err := tracker.OpenRegression(view, componentreport.ReportTestSummary{
			TestComparison: testdetails.TestComparison{
				BaseStats: &testdetails.ReleaseStats{Release: "4.18"},
			},
			Identification: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "comp",
					Capability: "cap",
					TestName:   "dedup test",
					TestID:     "deduptestid",
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{"a": "b"},
				},
			},
		})
		require.NoError(t, err)

		// First merge
		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-1", ProwJobName: "job-1", TestFailed: true, TestFailures: 10},
			{ProwJobRunID: "run-2", ProwJobName: "job-1", TestFailed: false, TestFailures: 0},
		})
		require.NoError(t, err)

		// Second merge with overlapping + new runs
		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-2", ProwJobName: "job-1", TestFailed: false, TestFailures: 0},
			{ProwJobRunID: "run-3", ProwJobName: "job-1", TestFailed: true, TestFailures: 5},
		})
		require.NoError(t, err)

		// Should have 3 unique runs, not 4
		var stored []models.RegressionJobRun
		res := dbc.DB.Where("regression_id = ?", reg.ID).Find(&stored)
		require.NoError(t, res.Error)
		assert.Len(t, stored, 3, "should have 3 unique job runs after dedup")
	})

	t.Run("new job run with symptoms", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupJobRuns(dbc)

		reg, err := tracker.OpenRegression(view, componentreport.ReportTestSummary{
			TestComparison: testdetails.TestComparison{
				BaseStats: &testdetails.ReleaseStats{Release: "4.18"},
			},
			Identification: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "comp",
					Capability: "cap",
					TestName:   "symptom test",
					TestID:     "symptomtestid",
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{"a": "b"},
				},
			},
		})
		require.NoError(t, err)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{
				ProwJobRunID: "run-sym-1",
				ProwJobName:  "job-1",
				TestFailed:   true,
				JobSymptoms:  pq.StringArray{"SymA"},
			},
		})
		require.NoError(t, err)

		var stored []models.RegressionJobRun
		res := dbc.DB.Where("regression_id = ? AND prow_job_run_id = ?", reg.ID, "run-sym-1").Find(&stored)
		require.NoError(t, res.Error)
		require.Len(t, stored, 1)
		assert.Equal(t, []string{"SymA"}, []string(stored[0].JobSymptoms))
	})

	t.Run("existing job run gains symptoms on re-merge", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupJobRuns(dbc)

		reg, err := tracker.OpenRegression(view, componentreport.ReportTestSummary{
			TestComparison: testdetails.TestComparison{
				BaseStats: &testdetails.ReleaseStats{Release: "4.18"},
			},
			Identification: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "comp",
					Capability: "cap",
					TestName:   "symptom update test",
					TestID:     "symptomuptestid",
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{"a": "b"},
				},
			},
		})
		require.NoError(t, err)

		// First merge without symptoms
		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-update-1", ProwJobName: "job-1", TestFailed: true},
		})
		require.NoError(t, err)

		var stored models.RegressionJobRun
		dbc.DB.Where("regression_id = ? AND prow_job_run_id = ?", reg.ID, "run-update-1").First(&stored)
		assert.Nil(t, stored.JobSymptoms)

		// Second merge with symptoms
		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-update-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA"}},
		})
		require.NoError(t, err)

		dbc.DB.Where("regression_id = ? AND prow_job_run_id = ?", reg.ID, "run-update-1").First(&stored)
		assert.Equal(t, []string{"SymA"}, []string(stored.JobSymptoms))
	})

	t.Run("job runs deleted when regression is deleted", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupJobRuns(dbc)

		reg, err := tracker.OpenRegression(view, componentreport.ReportTestSummary{
			TestComparison: testdetails.TestComparison{
				BaseStats: &testdetails.ReleaseStats{Release: "4.18"},
			},
			Identification: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "comp",
					Capability: "cap",
					TestName:   "cascade test",
					TestID:     "cascadetestid",
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{"a": "b"},
				},
			},
		})
		require.NoError(t, err)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-1", ProwJobName: "job-1"},
		})
		require.NoError(t, err)

		// Delete the regression
		res := dbc.DB.Delete(&models.TestRegression{}, reg.ID)
		require.NoError(t, res.Error)

		// Job runs should be cascade deleted
		var count int64
		dbc.DB.Model(&models.RegressionJobRun{}).Where("regression_id = ?", reg.ID).Count(&count)
		assert.Equal(t, int64(0), count, "job runs should be cascade deleted with regression")
	})
}

func cleanupTriages(dbc *db.DB) {
	dbc.DB.Exec("DELETE FROM triage_regressions WHERE 1=1")
	dbc.DB.Where("1 = 1").Delete(&models.Triage{})
}

func Test_SyncTriageSymptoms(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)
	view := crview.View{
		Name: "4.19-main",
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{Name: "4.19"},
		},
	}

	newRegSummary := func(testID string) componentreport.ReportTestSummary {
		return componentreport.ReportTestSummary{
			TestComparison: testdetails.TestComparison{
				BaseStats: &testdetails.ReleaseStats{Release: "4.18"},
			},
			Identification: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "comp",
					Capability: "cap",
					TestName:   "sync symptom test " + testID,
					TestID:     testID,
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{"a": "b"},
				},
			},
		}
	}

	util.SeedSymptom(t, dbc, "SymA", "Symptom A")
	util.SeedSymptom(t, dbc, "SymB", "Symptom B")
	defer util.CleanupSymptoms(dbc, "SymA", "SymB")

	cleanup := func() {
		util.CleanupTriageSymptoms(dbc)
		cleanupJobRuns(dbc)
		cleanupTriages(dbc)
		cleanupAllRegressions(dbc)
	}

	dbCtx := dbc.DB.WithContext(context.WithValue(context.Background(), models.CurrentUserKey, "e2e-test"))

	t.Run("links symptoms to triage", func(t *testing.T) {
		defer cleanup()

		reg, err := tracker.OpenRegression(view, newRegSummary("link-sym-1"))
		require.NoError(t, err)

		triage := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/TEST-SYM-1",
			Description: "symptom link test",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{*reg},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA", "SymB"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var rows []models.TriageSymptom
		dbc.DB.Where("triage_id = ?", triage.ID).Find(&rows)
		assert.Len(t, rows, 2, "should have 2 symptom rows")
		symptomIDs := sets.New[string]()
		for _, row := range rows {
			symptomIDs.Insert(row.SymptomID)
			assert.Equal(t, reg.ID, row.RegressionID)
			assert.Equal(t, 1, row.JobRunCount)
		}
		assert.True(t, symptomIDs.Has("SymA"))
		assert.True(t, symptomIDs.Has("SymB"))
	})

	t.Run("idempotent", func(t *testing.T) {
		defer cleanup()

		reg, err := tracker.OpenRegression(view, newRegSummary("idempotent-1"))
		require.NoError(t, err)

		triage := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/TEST-SYM-2",
			Description: "idempotent test",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{*reg},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var count1 int64
		dbc.DB.Model(&models.TriageSymptom{}).Where("triage_id = ?", triage.ID).Count(&count1)

		// Second sync
		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var count2 int64
		dbc.DB.Model(&models.TriageSymptom{}).Where("triage_id = ?", triage.ID).Count(&count2)
		assert.Equal(t, count1, count2, "row count should not change on re-sync")

		var row models.TriageSymptom
		dbc.DB.Where("triage_id = ? AND symptom_id = ?", triage.ID, "SymA").First(&row)
		assert.Equal(t, 1, row.JobRunCount, "job_run_count should remain the same")
	})

	t.Run("count accuracy", func(t *testing.T) {
		defer cleanup()

		reg, err := tracker.OpenRegression(view, newRegSummary("count-acc-1"))
		require.NoError(t, err)

		triage := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/TEST-SYM-3",
			Description: "count accuracy test",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{*reg},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA"}},
			{ProwJobRunID: "run-2", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA"}},
			{ProwJobRunID: "run-3", ProwJobName: "job-1", TestFailed: true},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var row models.TriageSymptom
		res := dbc.DB.Where("triage_id = ? AND symptom_id = ?", triage.ID, "SymA").First(&row)
		require.NoError(t, res.Error)
		assert.Equal(t, 2, row.JobRunCount, "job_run_count should be 2 (only runs with SymA)")
	})

	t.Run("count grows with new job runs", func(t *testing.T) {
		defer cleanup()

		reg, err := tracker.OpenRegression(view, newRegSummary("count-grow-1"))
		require.NoError(t, err)

		triage := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/TEST-SYM-4",
			Description: "count grows test",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{*reg},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var row models.TriageSymptom
		dbc.DB.Where("triage_id = ? AND symptom_id = ?", triage.ID, "SymA").First(&row)
		assert.Equal(t, 1, row.JobRunCount)

		// Add another job run with the same symptom
		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-2", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		dbc.DB.Where("triage_id = ? AND symptom_id = ?", triage.ID, "SymA").First(&row)
		assert.Equal(t, 2, row.JobRunCount, "job_run_count should increment after new run")
	})

	t.Run("regression without triage is skipped", func(t *testing.T) {
		defer cleanup()

		reg, err := tracker.OpenRegression(view, newRegSummary("no-triage-1"))
		require.NoError(t, err)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var count int64
		dbc.DB.Model(&models.TriageSymptom{}).Where("regression_id = ?", reg.ID).Count(&count)
		assert.Equal(t, int64(0), count, "no triage_symptoms rows should exist for untriaged regression")
	})

	t.Run("multiple symptoms per run", func(t *testing.T) {
		defer cleanup()

		reg, err := tracker.OpenRegression(view, newRegSummary("multi-sym-1"))
		require.NoError(t, err)

		triage := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/TEST-SYM-6",
			Description: "multi symptom test",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{*reg},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA", "SymB"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var rows []models.TriageSymptom
		dbc.DB.Where("triage_id = ?", triage.ID).Find(&rows)
		assert.Len(t, rows, 2, "both symptoms should get junction rows")
		for _, row := range rows {
			assert.Equal(t, 1, row.JobRunCount, "each symptom seen once")
		}
	})

	t.Run("deduplicates symptoms within a job run", func(t *testing.T) {
		defer cleanup()

		reg, err := tracker.OpenRegression(view, newRegSummary("dedup-1"))
		require.NoError(t, err)

		triage := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/TEST-DEDUP-1",
			Description: "dedup test",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{*reg},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "dedup-run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA", "SymA", "SymA"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var rows []models.TriageSymptom
		dbc.DB.Where("triage_id = ?", triage.ID).Find(&rows)
		require.Len(t, rows, 1, "duplicate symptoms should produce one row")
		assert.Equal(t, "SymA", rows[0].SymptomID)
		assert.Equal(t, 1, rows[0].JobRunCount, "job_run_count should be 1 despite duplicates in same run")
	})

	t.Run("syncs to multiple triages", func(t *testing.T) {
		defer cleanup()

		reg, err := tracker.OpenRegression(view, newRegSummary("multi-triage-1"))
		require.NoError(t, err)

		triage1 := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/TEST-MT-1",
			Description: "multi triage test 1",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{*reg},
		}
		require.NoError(t, dbCtx.Create(&triage1).Error)

		triage2 := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/TEST-MT-2",
			Description: "multi triage test 2",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{*reg},
		}
		require.NoError(t, dbCtx.Create(&triage2).Error)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "mt-run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"SymA"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var rows1 []models.TriageSymptom
		dbc.DB.Where("triage_id = ?", triage1.ID).Find(&rows1)
		require.Len(t, rows1, 1, "first triage should have symptom row")
		assert.Equal(t, "SymA", rows1[0].SymptomID)
		assert.Equal(t, 1, rows1[0].JobRunCount)

		var rows2 []models.TriageSymptom
		dbc.DB.Where("triage_id = ?", triage2.ID).Find(&rows2)
		require.Len(t, rows2, 1, "second triage should also have symptom row")
		assert.Equal(t, "SymA", rows2[0].SymptomID)
		assert.Equal(t, 1, rows2[0].JobRunCount)
	})
}

func cleanupRegressionViews(dbc *db.DB) {
	res := dbc.DB.Where("1 = 1").Delete(&models.RegressionView{})
	if res.Error != nil {
		log.Errorf("error deleting regression views: %v", res.Error)
	}
}

func Test_RegressionViews(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)
	view := crview.View{
		Name: "4.19-main",
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.19",
			},
		},
	}

	newRegSummary := func(testID string) componentreport.ReportTestSummary {
		return componentreport.ReportTestSummary{
			TestComparison: testdetails.TestComparison{
				BaseStats: &testdetails.ReleaseStats{Release: "4.18"},
			},
			Identification: crtest.Identification{
				RowIdentification: crtest.RowIdentification{
					Component:  "comp",
					Capability: "cap",
					TestName:   "view test " + testID,
					TestID:     testID,
				},
				ColumnIdentification: crtest.ColumnIdentification{
					Variants: map[string]string{"a": "b"},
				},
			},
		}
	}

	t.Run("upsert creates a new regression view", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		var beforeCreate time.Time
		require.NoError(t, dbc.DB.Raw("SELECT NOW()").Scan(&beforeCreate).Error)
		reg, err := tracker.OpenRegression(view, newRegSummary("upsert-create"))
		require.NoError(t, err)

		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)

		var rv models.RegressionView
		res := dbc.DB.Where("test_regression_id = ? AND view_name = ?", reg.ID, "4.19-main").First(&rv)
		require.NoError(t, res.Error)
		assert.True(t, rv.Active, "newly upserted view should be active")
		assert.False(t, rv.OpenedAt.IsZero(), "opened_at should be set")
		assert.True(t, rv.OpenedAt.After(beforeCreate) || rv.OpenedAt.Equal(beforeCreate), "opened_at should be >= test start time")
		assert.False(t, rv.ClosedAt.Valid, "closed_at should be null for a new view")
	})

	t.Run("upsert reactivates an inactive view", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		reg, err := tracker.OpenRegression(view, newRegSummary("upsert-reactivate"))
		require.NoError(t, err)

		// Create and then deactivate
		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)
		dbc.DB.Model(&models.RegressionView{}).
			Where("test_regression_id = ? AND view_name = ?", reg.ID, "4.19-main").
			Update("active", false)

		// Verify it's inactive
		var rv models.RegressionView
		dbc.DB.Where("test_regression_id = ? AND view_name = ?", reg.ID, "4.19-main").First(&rv)
		assert.False(t, rv.Active, "view should be inactive before re-upsert")

		// Upsert again should reactivate
		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)

		dbc.DB.Where("test_regression_id = ? AND view_name = ?", reg.ID, "4.19-main").First(&rv)
		assert.True(t, rv.Active, "view should be reactivated after upsert")
	})

	t.Run("upsert is idempotent for active views", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		reg, err := tracker.OpenRegression(view, newRegSummary("upsert-idempotent"))
		require.NoError(t, err)

		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)
		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)

		var count int64
		dbc.DB.Model(&models.RegressionView{}).
			Where("test_regression_id = ? AND view_name = ?", reg.ID, "4.19-main").
			Count(&count)
		assert.Equal(t, int64(1), count, "should have exactly one row, not duplicates")
	})

	t.Run("multiple views for one regression", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		reg, err := tracker.OpenRegression(view, newRegSummary("multi-view"))
		require.NoError(t, err)

		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)
		err = tracker.UpsertRegressionView(reg.ID, "4.19-arm64")
		require.NoError(t, err)

		var views []models.RegressionView
		res := dbc.DB.Where("test_regression_id = ?", reg.ID).Find(&views)
		require.NoError(t, res.Error)
		assert.Len(t, views, 2, "regression should be associated with two views")
	})

	t.Run("deactivate rolled-off views removes views not in active map", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		reg, err := tracker.OpenRegression(view, newRegSummary("deactivate-rolled-off"))
		require.NoError(t, err)

		// Associate with three views
		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)
		err = tracker.UpsertRegressionView(reg.ID, "4.19-arm64")
		require.NoError(t, err)
		err = tracker.UpsertRegressionView(reg.ID, "4.19-ppc64le")
		require.NoError(t, err)

		beforeDeactivate := time.Now().Truncate(time.Microsecond)

		// Only 4.19-main is still active
		activeViewMap := map[uint][]string{
			reg.ID: {"4.19-main"},
		}
		err = tracker.DeactivateRolledOffViews([]uint{reg.ID}, activeViewMap)
		require.NoError(t, err)

		var views []models.RegressionView
		dbc.DB.Where("test_regression_id = ?", reg.ID).Order("view_name").Find(&views)
		require.Len(t, views, 3, "all three view rows should still exist")

		for _, v := range views {
			if v.ViewName == "4.19-main" {
				assert.True(t, v.Active, "4.19-main should remain active")
				assert.False(t, v.ClosedAt.Valid, "4.19-main closed_at should remain null")
			} else {
				assert.False(t, v.Active, "%s should be deactivated", v.ViewName)
				assert.True(t, v.ClosedAt.Valid, "%s closed_at should be set", v.ViewName)
				assert.True(t, v.ClosedAt.Time.After(beforeDeactivate) || v.ClosedAt.Time.Equal(beforeDeactivate),
					"%s closed_at should be >= deactivation time", v.ViewName)
			}
		}
	})

	t.Run("deactivate with empty active views deactivates all", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		reg, err := tracker.OpenRegression(view, newRegSummary("deactivate-all"))
		require.NoError(t, err)

		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)
		err = tracker.UpsertRegressionView(reg.ID, "4.19-arm64")
		require.NoError(t, err)

		// No active views for this regression
		activeViewMap := map[uint][]string{}
		err = tracker.DeactivateRolledOffViews([]uint{reg.ID}, activeViewMap)
		require.NoError(t, err)

		var activeCount int64
		dbc.DB.Model(&models.RegressionView{}).
			Where("test_regression_id = ? AND active = true", reg.ID).
			Count(&activeCount)
		assert.Equal(t, int64(0), activeCount, "all views should be deactivated")
	})

	t.Run("deactivate with empty regression IDs is a no-op", func(t *testing.T) {
		err := tracker.DeactivateRolledOffViews([]uint{}, map[uint][]string{})
		require.NoError(t, err)
	})

	t.Run("views are preloaded on regression queries", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		reg, err := tracker.OpenRegression(view, newRegSummary("preload-test"))
		require.NoError(t, err)

		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)
		err = tracker.UpsertRegressionView(reg.ID, "4.19-arm64")
		require.NoError(t, err)

		// Query the regression with Views preloaded
		var loaded models.TestRegression
		res := dbc.DB.Preload("Views").First(&loaded, reg.ID)
		require.NoError(t, res.Error)
		assert.Len(t, loaded.Views, 2, "views should be preloaded")

		viewNames := make(map[string]bool)
		for _, v := range loaded.Views {
			viewNames[v.ViewName] = true
		}
		assert.True(t, viewNames["4.19-main"])
		assert.True(t, viewNames["4.19-arm64"])
	})

	t.Run("regression view lifecycle: upsert, deactivate, re-upsert", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		reg, err := tracker.OpenRegression(view, newRegSummary("lifecycle"))
		require.NoError(t, err)

		// Cycle 1: regression appears in both views
		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)
		err = tracker.UpsertRegressionView(reg.ID, "4.19-arm64")
		require.NoError(t, err)

		var rv models.RegressionView
		dbc.DB.Where("test_regression_id = ? AND view_name = ?", reg.ID, "4.19-arm64").First(&rv)
		originalOpenedAt := rv.OpenedAt

		// Cycle 2: regression rolls off 4.19-arm64
		activeViewMap := map[uint][]string{
			reg.ID: {"4.19-main"},
		}
		err = tracker.DeactivateRolledOffViews([]uint{reg.ID}, activeViewMap)
		require.NoError(t, err)

		dbc.DB.Where("test_regression_id = ? AND view_name = ?", reg.ID, "4.19-arm64").First(&rv)
		assert.False(t, rv.Active, "arm64 should be inactive after deactivation")
		assert.True(t, rv.ClosedAt.Valid, "arm64 closed_at should be set after deactivation")

		// Cycle 3: regression reappears in 4.19-arm64
		err = tracker.UpsertRegressionView(reg.ID, "4.19-arm64")
		require.NoError(t, err)

		dbc.DB.Where("test_regression_id = ? AND view_name = ?", reg.ID, "4.19-arm64").First(&rv)
		assert.True(t, rv.Active, "arm64 should be reactivated after upsert")
		assert.False(t, rv.ClosedAt.Valid, "arm64 closed_at should be cleared after re-upsert")
		assert.True(t, rv.OpenedAt.After(originalOpenedAt),
			"opened_at should be updated when regression reappears on a view")
	})

	t.Run("regression views cascade delete with regression", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		defer cleanupRegressionViews(dbc)

		reg, err := tracker.OpenRegression(view, newRegSummary("cascade-delete"))
		require.NoError(t, err)

		err = tracker.UpsertRegressionView(reg.ID, "4.19-main")
		require.NoError(t, err)

		// Delete the regression
		res := dbc.DB.Delete(&models.TestRegression{}, reg.ID)
		require.NoError(t, res.Error)

		// Views should be cascade deleted
		var count int64
		dbc.DB.Model(&models.RegressionView{}).Where("test_regression_id = ?", reg.ID).Count(&count)
		assert.Equal(t, int64(0), count, "regression views should be cascade deleted with regression")
	})
}

func Test_CrossCompareIsolation(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)
	rLog := log.WithField("test", "cross-compare-isolation")

	standardView := crview.View{
		Name: "4.19-main",
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{Name: "4.19"},
		},
		BaseRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{Name: "4.18"},
		},
	}

	crossCompareView := crview.View{
		Name: "4.19-ha-vs-two-node",
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{Name: "4.19"},
		},
		BaseRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{Name: "4.19"},
		},
		VariantOptions: reqopts.Variants{
			VariantCrossCompare: []string{"Topology"},
		},
	}

	testSummary := componentreport.ReportTestSummary{
		TestComparison: testdetails.TestComparison{
			BaseStats:   &testdetails.ReleaseStats{Release: "4.18"},
			SampleStats: testdetails.ReleaseStats{Stats: crtest.NewTestStats(10, 5, 0, false)},
		},
		Identification: crtest.Identification{
			RowIdentification: crtest.RowIdentification{
				Component:  "networking",
				Capability: "connectivity",
				TestName:   "cross compare test",
				TestID:     "cross-compare-test-id",
			},
			ColumnIdentification: crtest.ColumnIdentification{
				Variants: map[string]string{"arch": "amd64", "network": "OVNKubernetes"},
			},
		},
	}

	makeReport := func(tests ...componentreport.ReportTestSummary) *componentreport.ComponentReport {
		return &componentreport.ComponentReport{
			Rows: []componentreport.ReportRow{
				{
					Columns: []componentreport.ReportColumn{
						{RegressedTests: tests},
					},
				},
			},
		}
	}

	t.Run("OpenRegression sets CrossCompare from view config", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

		stdReg, err := tracker.OpenRegression(standardView, testSummary)
		require.NoError(t, err)
		assert.False(t, stdReg.CrossCompare, "standard view regression should have CrossCompare=false")

		ccReg, err := tracker.OpenRegression(crossCompareView, testSummary)
		require.NoError(t, err)
		assert.True(t, ccReg.CrossCompare, "cross-compare view regression should have CrossCompare=true")
	})

	t.Run("SyncRegressionsForReport creates isolated regressions for same test", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

		report := makeReport(testSummary)

		stdRegs, err := componentreadiness.SyncRegressionsForReport(tracker, standardView, rLog, report)
		require.NoError(t, err)
		require.Len(t, stdRegs, 1, "should open one regression for standard view")
		assert.False(t, stdRegs[0].CrossCompare)

		ccRegs, err := componentreadiness.SyncRegressionsForReport(tracker, crossCompareView, rLog, report)
		require.NoError(t, err)
		require.Len(t, ccRegs, 1, "should open one regression for cross-compare view")
		assert.True(t, ccRegs[0].CrossCompare)

		assert.NotEqual(t, stdRegs[0].ID, ccRegs[0].ID,
			"standard and cross-compare regressions should be separate records")

		// Verify both exist in the database
		var allRegs []models.TestRegression
		res := dbc.DB.Where("release = ? AND test_id = ?", "4.19", "cross-compare-test-id").Find(&allRegs)
		require.NoError(t, res.Error)
		assert.Len(t, allRegs, 2, "should have two separate regression records in the database")
	})

	t.Run("standard sync does not match cross-compare regressions", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

		// Pre-create a cross-compare regression
		ccReg, err := tracker.OpenRegression(crossCompareView, testSummary)
		require.NoError(t, err)
		assert.True(t, ccReg.CrossCompare)

		report := makeReport(testSummary)

		// Sync with standard view — should NOT match the cross-compare regression
		stdRegs, err := componentreadiness.SyncRegressionsForReport(tracker, standardView, rLog, report)
		require.NoError(t, err)
		require.Len(t, stdRegs, 1)
		assert.NotEqual(t, ccReg.ID, stdRegs[0].ID,
			"standard sync should have opened a new regression, not matched the cross-compare one")
		assert.False(t, stdRegs[0].CrossCompare)
	})

	t.Run("cross-compare sync does not match standard regressions", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

		// Pre-create a standard regression
		stdReg, err := tracker.OpenRegression(standardView, testSummary)
		require.NoError(t, err)
		assert.False(t, stdReg.CrossCompare)

		report := makeReport(testSummary)

		// Sync with cross-compare view — should NOT match the standard regression
		ccRegs, err := componentreadiness.SyncRegressionsForReport(tracker, crossCompareView, rLog, report)
		require.NoError(t, err)
		require.Len(t, ccRegs, 1)
		assert.NotEqual(t, stdReg.ID, ccRegs[0].ID,
			"cross-compare sync should have opened a new regression, not matched the standard one")
		assert.True(t, ccRegs[0].CrossCompare)
	})

	t.Run("re-sync reuses existing regression within same pool", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

		report := makeReport(testSummary)

		// First sync for standard view
		stdRegs1, err := componentreadiness.SyncRegressionsForReport(tracker, standardView, rLog, report)
		require.NoError(t, err)
		require.Len(t, stdRegs1, 1)

		// Second sync for standard view should reuse the same regression
		stdRegs2, err := componentreadiness.SyncRegressionsForReport(tracker, standardView, rLog, report)
		require.NoError(t, err)
		require.Len(t, stdRegs2, 1)
		assert.Equal(t, stdRegs1[0].ID, stdRegs2[0].ID, "standard re-sync should reuse existing regression")

		// First sync for cross-compare view
		ccRegs1, err := componentreadiness.SyncRegressionsForReport(tracker, crossCompareView, rLog, report)
		require.NoError(t, err)
		require.Len(t, ccRegs1, 1)

		// Second sync for cross-compare view should reuse the same regression
		ccRegs2, err := componentreadiness.SyncRegressionsForReport(tracker, crossCompareView, rLog, report)
		require.NoError(t, err)
		require.Len(t, ccRegs2, 1)
		assert.Equal(t, ccRegs1[0].ID, ccRegs2[0].ID, "cross-compare re-sync should reuse existing regression")
	})
}

func rawCreateRegression(
	dbc *db.DB,
	release string,
	testID string,
	testName string,
	variants []string,
	opened, closed time.Time) (*models.TestRegression, error) {
	return rawCreateRegressionWithBase(dbc, release, "", testID, testName, variants, opened, closed)
}

func rawCreateRegressionWithBase(
	dbc *db.DB,
	release, baseRelease string,
	testID string,
	testName string,
	variants []string,
	opened, closed time.Time) (*models.TestRegression, error) {
	newRegression := &models.TestRegression{
		Release:     release,
		BaseRelease: baseRelease,
		TestID:      testID,
		TestName:    testName,
		Opened:      opened,
		Variants:    variants,
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
