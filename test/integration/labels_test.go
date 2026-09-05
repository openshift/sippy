package integration

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api/labels"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/infrafailure"
	"github.com/openshift/sippy/pkg/db/models"
	intutil "github.com/openshift/sippy/test/integration/util"
)

func TestApplyLabelPartitionOutcomes(t *testing.T) {
	timestamp := time.Date(2026, 8, 24, 11, 30, 0, 0, time.UTC)
	requestFor := func(runID uint) labels.ApplyRequest {
		return labels.ApplyRequest{
			RunID:        uintString(runID),
			Label:        "DNSTimeout",
			ProwJobStart: timestamp,
			Release:      "4.18",
		}
	}

	t.Run("new keyed update succeeds without id map row", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
		run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", timestamp, true, v1.JobSucceeded)
		require.NoError(t, dbc.DB.Where("id = ?", run.ID).Delete(&models.ProwJobRunIDMap{}).Error)

		result, outcome := labels.NewApplier(dbc).Apply(context.Background(), requestFor(run.ID))

		assert.Equal(t, labels.ApplyOutcomeRecorded, outcome)
		assert.Equal(t, "label recorded", result.Message)
		assertRunLabels(t, dbc, run.ID, timestamp, "DNSTimeout")
	})

	t.Run("zero-row duplicate with no id map returns missing run", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
		run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", timestamp, true, v1.JobSucceeded, intutil.WithLabels("DNSTimeout"))
		require.NoError(t, dbc.DB.Where("id = ?", run.ID).Delete(&models.ProwJobRunIDMap{}).Error)

		result, outcome := labels.NewApplier(dbc).Apply(context.Background(), requestFor(run.ID))

		assert.Equal(t, labels.ApplyOutcomeRunNotFound, outcome)
		assert.Equal(t, "job run not found", result.Message)
		assertRunLabels(t, dbc, run.ID, timestamp, "DNSTimeout")
	})

	t.Run("release mismatch", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
		run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", timestamp, true, v1.JobSucceeded)
		request := requestFor(run.ID)
		request.Release = "4.19"

		result, outcome := labels.NewApplier(dbc).Apply(context.Background(), request)

		assert.Equal(t, labels.ApplyOutcomePartitionKeyMismatch, outcome)
		assert.Equal(t, "job run partition keys do not match request", result.Message)
		assertRunLabels(t, dbc, run.ID, timestamp)
	})

	t.Run("timestamp mismatch", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
		run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", timestamp, true, v1.JobSucceeded)
		request := requestFor(run.ID)
		request.ProwJobStart = timestamp.Add(time.Second)

		result, outcome := labels.NewApplier(dbc).Apply(context.Background(), request)

		assert.Equal(t, labels.ApplyOutcomePartitionKeyMismatch, outcome)
		assert.Equal(t, "job run partition keys do not match request", result.Message)
		assertRunLabels(t, dbc, run.ID, timestamp)
	})

	t.Run("matching keys append generic label once", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
		run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", timestamp, true, v1.JobSucceeded)
		request := requestFor(run.ID)

		result, outcome := labels.NewApplier(dbc).Apply(context.Background(), request)
		assert.Equal(t, labels.ApplyOutcomeRecorded, outcome)
		assert.Equal(t, "label recorded", result.Message)
		assertRunLabels(t, dbc, run.ID, timestamp, "DNSTimeout")

		result, outcome = labels.NewApplier(dbc).Apply(context.Background(), request)
		assert.Equal(t, labels.ApplyOutcomeAlreadyLabeled, outcome)
		assert.Equal(t, "label already present", result.Message)
		assertRunLabels(t, dbc, run.ID, timestamp, "DNSTimeout")
	})

	t.Run("mapped target row missing is an application error", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		const runID = 99001
		require.NoError(t, dbc.DB.Create(&models.ProwJobRunIDMap{
			ID:             runID,
			ProwJobRelease: "4.18",
			Timestamp:      timestamp,
		}).Error)

		result, outcome := labels.NewApplier(dbc).Apply(context.Background(), requestFor(runID))

		assert.Equal(t, labels.ApplyOutcomeError, outcome)
		assert.Contains(t, result.Error, "present in the id map but its target row is missing")
	})
}

func TestApplyInfraFailureSubtractsOnce(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 10, 0, 0, 0, time.UTC)
	const runID = 49001

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp},
		Tests: []pgwriter.TestRow{{
			ProwJobRunID: runID, ProwJobID: jobID, ProwJobRunTimestamp: timestamp,
			ProwJobRunRelease: "4.18", TestName: "label-api-infra-test", SuiteName: "junit_e2e",
			Status: statusSuccess, Duration: 1.0,
		}},
	}})
	request := labels.ApplyRequest{
		RunID:        "49001",
		Label:        infrafailure.LabelInfraFailure,
		ProwJobStart: timestamp,
		Release:      "4.18",
	}

	result, outcome := labels.NewApplier(dbc).Apply(context.Background(), request)
	require.Equal(t, labels.ApplyOutcomeRecorded, outcome, result.Error)
	assertRunLabels(t, dbc, runID, timestamp, infrafailure.LabelInfraFailure)
	assertDailyRuns(t, dbc, jobID, "label-api-infra-test", 0)

	result, outcome = labels.NewApplier(dbc).Apply(context.Background(), request)
	require.Equal(t, labels.ApplyOutcomeAlreadyLabeled, outcome, result.Error)
	assertRunLabels(t, dbc, runID, timestamp, infrafailure.LabelInfraFailure)
	assertDailyRuns(t, dbc, jobID, "label-api-infra-test", 0)
}

func TestApplyInfraFailureRollsBackLabelWhenSubtractionFails(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	jobID := seedProwJob(t, dbc, "periodic-e2e-aws", "4.18")
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 10, 0, 0, 0, time.UTC)
	const runID = 49002

	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp},
		Tests: []pgwriter.TestRow{{
			ProwJobRunID: runID, ProwJobID: jobID, ProwJobRunTimestamp: timestamp,
			ProwJobRunRelease: "4.18", TestName: "label-api-rollback-test", SuiteName: "junit_e2e",
			Status: statusSuccess, Duration: 1.0,
		}},
	}})
	require.NoError(t, dbc.DB.Exec("DROP TABLE test_daily_totals").Error)

	result, outcome := labels.NewApplier(dbc).Apply(context.Background(), labels.ApplyRequest{
		RunID:        "49002",
		Label:        infrafailure.LabelInfraFailure,
		ProwJobStart: timestamp,
		Release:      "4.18",
	})

	assert.Equal(t, labels.ApplyOutcomeError, outcome)
	assert.True(t, strings.Contains(result.Error, "subtracting daily totals"), result.Error)
	assertRunLabels(t, dbc, runID, timestamp)
}

func uintString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}

func assertRunLabels(t *testing.T, dbc *db.DB, runID uint, timestamp time.Time, expected ...string) {
	t.Helper()
	var run models.ProwJobRun
	require.NoError(t, dbc.DB.Where("id = ? AND prow_job_release = ? AND timestamp = ?", runID, "4.18", timestamp).Take(&run).Error)
	assert.ElementsMatch(t, expected, []string(run.Labels))
}

func assertDailyRuns(t *testing.T, dbc *db.DB, jobID uint, testName string, expected int32) {
	t.Helper()
	var test models.Test
	require.NoError(t, dbc.DB.Where("name = ?", testName).Take(&test).Error)
	var daily models.TestDailyTotal
	require.NoError(t, dbc.DB.Where("test_id = ? AND prow_job_id = ? AND release = ? AND date = ?", test.ID, jobID, "4.18", testDate).Take(&daily).Error)
	assert.Equal(t, expected, daily.Runs)
}
