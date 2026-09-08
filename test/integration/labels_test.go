package integration

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/infrafailure"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	intutil "github.com/openshift/sippy/test/integration/util"
)

func TestInfraFailureReconcileRemoveRestoresSummaries(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 10, 0, 0, 0, time.UTC)
	const runID = 49003

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp},
		Tests: []pgwriter.TestRow{{
			ProwJobRunID: runID, ProwJobID: jobID, ProwJobRunTimestamp: timestamp,
			ProwJobRunRelease: "4.18", TestName: "label-reconcile-restore-test", SuiteName: "junit_e2e",
			Status: statusSuccess, Duration: 1.0,
		}},
	}})
	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", "label-reconcile-restore-test").First(&test).Error)

	keys, err := query.LookupProwJobRunPartitionKeys(dbc.DB, runID)
	require.NoError(t, err)
	require.NoError(t, dbc.DB.Transaction(func(tx *gorm.DB) error {
		if err := infrafailure.SubtractInfraFailureFromSummaries(tx, runID, keys); err != nil {
			return err
		}
		return tx.Model(&models.ProwJobRun{}).Where("id = ? AND prow_job_release = ? AND timestamp = ?", runID, keys.ProwJobRelease, keys.Timestamp).Update("labels", pq.StringArray{infrafailure.LabelInfraFailure}).Error
	}))
	assertDailyRuns(t, dbc, jobID, "label-reconcile-restore-test", 0)

	require.NoError(t, dbc.DB.Transaction(func(tx *gorm.DB) error {
		if err := infrafailure.RestoreInfraFailureToSummaries(tx, runID, keys); err != nil {
			return err
		}
		return tx.Model(&models.ProwJobRun{}).Where("id = ? AND prow_job_release = ? AND timestamp = ?", runID, keys.ProwJobRelease, keys.Timestamp).Update("labels", pq.StringArray{}).Error
	}))
	assertDailyRuns(t, dbc, jobID, "label-reconcile-restore-test", 1)
}

func assertDailyRuns(t *testing.T, dbc *db.DB, jobID uint, testName string, expected int32) {
	t.Helper()
	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", testName).Take(&test).Error)
	var daily models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", testDate).Take(&daily).Error)
	assert.Equal(t, expected, daily.Runs)
}
