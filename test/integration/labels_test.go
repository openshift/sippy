package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/api/labels"
	"github.com/openshift/sippy/pkg/componentreadiness/jobrunannotator"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/infrafailure"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/db/query"
	intutil "github.com/openshift/sippy/test/integration/util"
)

const labelRunPath = "logs/periodic-label-job/"

func writeLabelFile(t *testing.T, bucket, jobPath, labelID, title, explanation string) string {
	t.Helper()
	return writeLabelObject(t, bucket, fmt.Sprintf("%s/artifacts/job_labels/%s.json", jobPath, labelID), labelID, title, explanation)
}

func writeLabelObject(t *testing.T, bucket, name, labelID, title, explanation string) string {
	t.Helper()
	writer := gcsContainer.Client.Bucket(bucket).Object(name).NewWriter(context.Background())
	data, err := json.Marshal(jobrunannotator.JobRunBucketLabelContainer{
		V1: &jobrunannotator.JobRunBucketLabel{
			Label: jobrunscan.LabelContent{ID: labelID, LabelTitle: title, Explanation: explanation},
		},
	})
	require.NoError(t, err)
	_, err = writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return name
}

func deleteLabelFile(t *testing.T, bucket, name string) {
	t.Helper()
	require.NoError(t, gcsContainer.Client.Bucket(bucket).Object(name).Delete(context.Background()))
}

func reconcileLabels(t *testing.T, dbc *db.DB, runID string) labels.ReconcileResult {
	t.Helper()
	result, outcome := labels.NewReconciler(dbc, gcsContainer.Client).Reconcile(context.Background(), labels.ReconcileRequest{RunID: runID})
	require.NotEqual(t, labels.ReconcileOutcomeError, outcome, "%s", result.Error)
	return result
}

func loadRunLabels(t *testing.T, dbc *db.DB, runID int64) pq.StringArray {
	t.Helper()
	keys, err := query.LookupProwJobRunPartitionKeys(dbc.DB, runID)
	require.NoError(t, err)
	var run models.ProwJobRun
	require.NoError(t, dbc.DB.Where("id = ? AND prow_job_release = ? AND timestamp = ?", runID, keys.ProwJobRelease, keys.Timestamp).Take(&run).Error)
	return run.Labels
}

func TestLabelReconciliationUsesCompleteGCSSet(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	bucket := intutil.NewTestBucket(t, gcsContainer)
	jobID := seedProwJob(t, dbc, "periodic-label-job", "4.18")
	const runID = 49001
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 10, 0, 0, 0, time.UTC)
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{Run: pgwriter.RunRow{
		ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp,
		URL:       "https://prow.example/view/gs/" + bucket + "/" + labelRunPath,
		GCSBucket: bucket,
	}}})

	first := writeLabelFile(t, bucket, labelRunPath, "FirstLabel", "First title", "First explanation")
	writeLabelFile(t, bucket, labelRunPath, "SecondLabel", "Second title", "Second explanation")
	result := reconcileLabels(t, dbc, fmt.Sprint(runID))
	assert.Equal(t, 2, result.Added)
	assert.ElementsMatch(t, []string{"FirstLabel", "SecondLabel"}, []string(loadRunLabels(t, dbc, runID)))

	writeLabelFile(t, bucket, labelRunPath, "FirstLabel", "Updated title", "Updated explanation")
	deleteLabelFile(t, bucket, fmt.Sprintf("%s/artifacts/job_labels/SecondLabel.json", labelRunPath))
	writeLabelFile(t, bucket, labelRunPath, "ThirdLabel", "Third title", "Third explanation")
	result = reconcileLabels(t, dbc, fmt.Sprint(runID))
	assert.Equal(t, 1, result.Added)
	assert.Equal(t, 1, result.Updated)
	assert.Equal(t, 1, result.Removed)
	assert.ElementsMatch(t, []string{"FirstLabel", "ThirdLabel"}, []string(loadRunLabels(t, dbc, runID)))

	var firstDefinition jobrunscan.Label
	require.NoError(t, dbc.DB.Where("id = ?", "FirstLabel").Take(&firstDefinition).Error)
	assert.Equal(t, "Updated title", firstDefinition.LabelTitle)
	assert.Equal(t, "Updated explanation", firstDefinition.Explanation)

	deleteLabelFile(t, bucket, first)
	deleteLabelFile(t, bucket, fmt.Sprintf("%s/artifacts/job_labels/ThirdLabel.json", labelRunPath))
	result = reconcileLabels(t, dbc, fmt.Sprint(runID))
	assert.Equal(t, 2, result.Removed)
	assert.Empty(t, loadRunLabels(t, dbc, runID))
}

func TestLabelReconciliationDoesNotDeleteOnMalformedGCSFile(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	bucket := intutil.NewTestBucket(t, gcsContainer)
	jobID := seedProwJob(t, dbc, "periodic-malformed-label-job", "4.18")
	const runID = 49002
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 11, 0, 0, 0, time.UTC)
	jobPath := "logs/periodic-malformed-label-job/"
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{Run: pgwriter.RunRow{
		ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp,
		URL:       "https://prow.example/view/gs/" + bucket + "/" + jobPath,
		GCSBucket: bucket,
	}}})

	name := writeLabelFile(t, bucket, jobPath, "RetainedLabel", "Retained title", "Retained explanation")
	reconcileLabels(t, dbc, fmt.Sprint(runID))
	badName := jobPath + "artifacts/job_labels/malformed.json"
	writer := gcsContainer.Client.Bucket(bucket).Object(badName).NewWriter(context.Background())
	_, err := writer.Write([]byte("{"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	result, outcome := labels.NewReconciler(dbc, gcsContainer.Client).Reconcile(context.Background(), labels.ReconcileRequest{RunID: fmt.Sprint(runID)})
	assert.Equal(t, labels.ReconcileOutcomeError, outcome)
	assert.NotEmpty(t, result.Error)
	assert.Equal(t, pq.StringArray{"RetainedLabel"}, loadRunLabels(t, dbc, runID))
	deleteLabelFile(t, bucket, name)
}

func TestLabelReconciliationIgnoresMissingRun(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	result, outcome := labels.NewReconciler(dbc, gcsContainer.Client).Reconcile(context.Background(), labels.ReconcileRequest{RunID: "49005"})
	assert.Equal(t, labels.ReconcileOutcomeRunNotFound, outcome)
	assert.Equal(t, "49005", result.RunID)
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Message, "labels will be read from GCS when the run is loaded")
}

func TestLabelReconciliationAcceptsInitiallyEmptyGCSState(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	bucket := intutil.NewTestBucket(t, gcsContainer)
	jobID := seedProwJob(t, dbc, "periodic-empty-label-job", "4.18")
	const runID = 49006
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 13, 0, 0, 0, time.UTC)
	jobPath := "logs/periodic-empty-label-job/"
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{Run: pgwriter.RunRow{
		ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp,
		URL: "https://prow.example/view/gs/" + bucket + "/" + jobPath, GCSBucket: bucket,
	}}})

	result, outcome := labels.NewReconciler(dbc, gcsContainer.Client).Reconcile(context.Background(), labels.ReconcileRequest{RunID: fmt.Sprint(runID)})
	assert.Equal(t, labels.ReconcileOutcomeNoOp, outcome)
	assert.Empty(t, result.Error)
	assert.Empty(t, loadRunLabels(t, dbc, runID))
}

func TestLabelReconciliationDoesNotDeleteOnGCSReadFailure(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	bucket := intutil.NewTestBucket(t, gcsContainer)
	jobID := seedProwJob(t, dbc, "periodic-gcs-error-label-job", "4.18")
	const runID = 49007
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 14, 0, 0, 0, time.UTC)
	jobPath := "logs/periodic-gcs-error-label-job/"
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{Run: pgwriter.RunRow{
		ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp,
		URL: "https://prow.example/view/gs/" + bucket + "/" + jobPath, GCSBucket: bucket,
	}}})

	writeLabelFile(t, bucket, jobPath, "RetainedLabel", "Retained title", "Retained explanation")
	reconcileLabels(t, dbc, fmt.Sprint(runID))
	keys, err := query.LookupProwJobRunPartitionKeys(dbc.DB, runID)
	require.NoError(t, err)
	missingBucket := bucket + "-missing"
	require.NoError(t, dbc.DB.Model(&models.ProwJobRun{}).
		Where("id = ? AND prow_job_release = ? AND timestamp = ?", runID, keys.ProwJobRelease, keys.Timestamp).
		Update("gcs_bucket", missingBucket).Error)

	result, outcome := labels.NewReconciler(dbc, gcsContainer.Client).Reconcile(context.Background(), labels.ReconcileRequest{RunID: fmt.Sprint(runID)})
	assert.Equal(t, labels.ReconcileOutcomeError, outcome)
	assert.NotEmpty(t, result.Error)
	assert.Equal(t, pq.StringArray{"RetainedLabel"}, loadRunLabels(t, dbc, runID))
}

func TestLabelReconciliationRejectsConflictingLabelMetadata(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	bucket := intutil.NewTestBucket(t, gcsContainer)
	jobID := seedProwJob(t, dbc, "periodic-conflicting-label-job", "4.18")
	const runID = 49008
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 15, 0, 0, 0, time.UTC)
	jobPath := "logs/periodic-conflicting-label-job/"
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{Run: pgwriter.RunRow{
		ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp,
		URL: "https://prow.example/view/gs/" + bucket + "/" + jobPath, GCSBucket: bucket,
	}}})

	writeLabelObject(t, bucket, jobPath+"artifacts/job_labels/first.json", "SameLabel", "First title", "First explanation")
	writeLabelObject(t, bucket, jobPath+"artifacts/job_labels/second.json", "SameLabel", "Second title", "Second explanation")
	result, outcome := labels.NewReconciler(dbc, gcsContainer.Client).Reconcile(context.Background(), labels.ReconcileRequest{RunID: fmt.Sprint(runID)})
	assert.Equal(t, labels.ReconcileOutcomeError, outcome)
	assert.Contains(t, result.Error, "conflicting metadata")
	assert.Empty(t, loadRunLabels(t, dbc, runID))
}

func TestLabelReconciliationPreservesInfraFailureSummaryInvariant(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	bucket := intutil.NewTestBucket(t, gcsContainer)
	jobID := seedProwJob(t, dbc, "periodic-infra-label-job", "4.18")
	const runID = 49004
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 12, 0, 0, 0, time.UTC)
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{
			ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp,
			URL:       "https://prow.example/view/gs/" + bucket + "/" + labelRunPath,
			GCSBucket: bucket,
		},
		Tests: []pgwriter.TestRow{{
			ProwJobRunID: runID, ProwJobID: jobID, ProwJobRunTimestamp: timestamp,
			ProwJobRunRelease: "4.18", TestName: "reconcile-infra-test", SuiteName: "junit_e2e",
			Status: statusSuccess, Duration: 1,
		}},
	}})

	assertDailyRuns(t, dbc, jobID, "reconcile-infra-test", 1)
	infraFile := writeLabelFile(t, bucket, labelRunPath, infrafailure.LabelInfraFailure, "Infrastructure failure", "Infrastructure failure explanation")
	reconcileLabels(t, dbc, fmt.Sprint(runID))
	assertDailyRuns(t, dbc, jobID, "reconcile-infra-test", 0)

	// A duplicate notification must not subtract the run a second time.
	reconcileLabels(t, dbc, fmt.Sprint(runID))
	assertDailyRuns(t, dbc, jobID, "reconcile-infra-test", 0)

	deleteLabelFile(t, bucket, infraFile)
	reconcileLabels(t, dbc, fmt.Sprint(runID))
	assertDailyRuns(t, dbc, jobID, "reconcile-infra-test", 1)

	// A duplicate delete notification must not restore the run twice.
	result, outcome := labels.NewReconciler(dbc, gcsContainer.Client).Reconcile(context.Background(), labels.ReconcileRequest{RunID: fmt.Sprint(runID)})
	require.Equal(t, labels.ReconcileOutcomeNoOp, outcome)
	assert.Equal(t, 0, result.Added+result.Updated+result.Removed)
	assertDailyRuns(t, dbc, jobID, "reconcile-infra-test", 1)
}

func TestLabelReconciliationConcurrentInfraFailureNotificationsAreIdempotent(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	bucket := intutil.NewTestBucket(t, gcsContainer)
	jobID := seedProwJob(t, dbc, "periodic-concurrent-infra-label-job", "4.18")
	const runID = 49009
	timestamp := time.Date(testDate.Year, testDate.Month, testDate.Day, 16, 0, 0, 0, time.UTC)
	jobPath := "logs/periodic-concurrent-infra-label-job/"
	writeBatch(t, dbc, testDate, []pgwriter.JobRunResult{{
		Run: pgwriter.RunRow{
			ID: runID, ProwJobID: jobID, ProwJobRelease: "4.18", Timestamp: timestamp,
			URL: "https://prow.example/view/gs/" + bucket + "/" + jobPath, GCSBucket: bucket,
		},
		Tests: []pgwriter.TestRow{{
			ProwJobRunID: runID, ProwJobID: jobID, ProwJobRunTimestamp: timestamp,
			ProwJobRunRelease: "4.18", TestName: "concurrent-reconcile-infra-test", SuiteName: "junit_e2e",
			Status: statusSuccess, Duration: 1,
		}},
	}})
	writeLabelFile(t, bucket, jobPath, infrafailure.LabelInfraFailure, "Infrastructure failure", "Infrastructure failure explanation")

	type reconciliation struct {
		result  labels.ReconcileResult
		outcome labels.ReconcileOutcome
	}
	results := make(chan reconciliation, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, outcome := labels.NewReconciler(dbc, gcsContainer.Client).Reconcile(context.Background(), labels.ReconcileRequest{RunID: fmt.Sprint(runID)})
			results <- reconciliation{result: result, outcome: outcome}
		}()
	}
	wg.Wait()
	close(results)

	var applied, noOp int
	for result := range results {
		assert.Empty(t, result.result.Error)
		switch result.outcome {
		case labels.ReconcileOutcomeApplied:
			applied++
		case labels.ReconcileOutcomeNoOp:
			noOp++
		default:
			t.Errorf("unexpected reconciliation outcome %v", result.outcome)
		}
	}
	assert.Equal(t, 1, applied)
	assert.Equal(t, 1, noOp)
	assertDailyRuns(t, dbc, jobID, "concurrent-reconcile-infra-test", 0)
}

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
