package symptomre

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apijobrunscan "github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/db"
	jobrunscanmodels "github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

// These functional tests require a real PostgreSQL instance. They are skipped
// unless the SIPPY_FUNCTIONAL_TEST_DSN environment variable is set. To run:
//
//	SIPPY_FUNCTIONAL_TEST_DSN=postgresql://postgres:password@localhost:5432/postgres \
//	go test -v -run TestFunctional ./pkg/sippyserver/workqueue/symptomre/
//
// The tests create and clean up their own tables (workqueue_symptom_re_batches,
// workqueue_symptom_re_batch_items) and run River migrations for the river_job
// table that StatusQuerier joins against.
//
// ReevaluateWorker is tested via its function-field seam (reEvalFunc)
// without requiring GCS or BigQuery credentials.

// functionalTestSetup creates a pgx/v5 pool, runs River migrations, creates a
// GORM DB, and auto-migrates the Batch/BatchItem tables. It returns the GORM
// DB, a cleanup function, and a context. The test is skipped if the DSN env
// var is not set.
func functionalTestSetup(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	dsn := os.Getenv("SIPPY_FUNCTIONAL_TEST_DSN")
	if dsn == "" {
		t.Skip("Set SIPPY_FUNCTIONAL_TEST_DSN to run functional tests")
	}

	ctx := context.Background()

	pool, err := workqueue.NewPgxV5Pool(ctx, dsn)
	require.NoError(t, err, "pgx/v5 pool creation should succeed")

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	require.NoError(t, err, "River migrator creation should succeed")
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	require.NoError(t, err, "River schema migration should succeed")

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "GORM connection should open")
	require.NoError(t, gormDB.AutoMigrate(&Batch{}, &BatchItem{}, &jobrunscanmodels.Symptom{}),
		"table auto-migration should succeed")

	cleanup := func() {
		gormDB.Exec("DELETE FROM workqueue_symptom_re_batch_items")
		gormDB.Exec("DELETE FROM workqueue_symptom_re_batches")
		pool.Close()
	}

	return gormDB, cleanup
}

func TestFunctionalSubmitter(t *testing.T) {
	gormDB, cleanup := functionalTestSetup(t)
	defer cleanup()

	ctx := context.Background()
	dsn := os.Getenv("SIPPY_FUNCTIONAL_TEST_DSN")
	pool, err := workqueue.NewPgxV5Pool(ctx, dsn)
	require.NoError(t, err, "pgx/v5 pool for River client should succeed")
	defer pool.Close()

	riverClient, err := workqueue.NewInsertOnlyClient(pool)
	require.NoError(t, err, "insert-only River client should be created")

	submitter := NewSubmitter(gormDB, riverClient)

	tests := []struct {
		name   string
		jobIDs []string
		dryRun bool
	}{
		{name: "submit three job IDs", jobIDs: []string{"build-1", "build-2", "build-3"}, dryRun: false},
		{name: "submit single job ID with dry run", jobIDs: []string{"build-dry-1"}, dryRun: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := submitter.Submit(ctx, tc.jobIDs, tc.dryRun)
			require.NoError(t, err, "Submit should succeed")
			assert.NotEqual(t, uuid.Nil, result.BatchID, "batch ID should be non-nil")
			assert.Equal(t, len(tc.jobIDs), result.Requested, "requested count should match input length")

			var batch Batch
			require.NoError(t, gormDB.Take(&batch, "id = ?", result.BatchID).Error, "batch row should exist in DB")
			assert.Equal(t, workqueue.BatchStatusPending, batch.Status, "new batch should be pending")
			assert.Equal(t, len(tc.jobIDs), batch.RequestedCount, "batch requested count should match")
			assert.Equal(t, tc.dryRun, batch.DryRun, "batch dry_run flag should propagate")

			var items []BatchItem
			require.NoError(t, gormDB.Where("batch_id = ?", result.BatchID).Find(&items).Error, "batch items should load")
			assert.Len(t, items, len(tc.jobIDs), "item count should match input length")
			for _, item := range items {
				assert.Nil(t, item.RiverJobID, "items should have nil RiverJobID before daemon processing")
			}
		})
	}
}

func TestFunctionalStatusQuerier(t *testing.T) {
	gormDB, cleanup := functionalTestSetup(t)
	defer cleanup()

	ctx := context.Background()
	dsn := os.Getenv("SIPPY_FUNCTIONAL_TEST_DSN")
	pool, err := workqueue.NewPgxV5Pool(ctx, dsn)
	require.NoError(t, err, "pgx/v5 pool for River client should succeed")
	defer pool.Close()

	riverClient, err := workqueue.NewInsertOnlyClient(pool)
	require.NoError(t, err, "insert-only River client should be created")

	submitter := NewSubmitter(gormDB, riverClient)
	querier := NewStatusQuerier(gormDB)

	t.Run("query existing batch shows pending items", func(t *testing.T) {
		result, err := submitter.Submit(ctx, []string{"status-1", "status-2"}, false)
		require.NoError(t, err, "Submit should succeed")

		resp, err := querier.Query(context.Background(), result.BatchID)
		require.NoError(t, err, "Query should succeed for existing batch")
		require.NotNil(t, resp, "response should be non-nil for existing batch")
		assert.Equal(t, result.BatchID, resp.BatchID, "response batch ID should match")
		assert.Equal(t, workqueue.BatchStatusPending, resp.Status, "batch should still be pending")
		assert.Equal(t, 2, resp.Requested, "requested count should reflect submitted items")
		assert.Equal(t, 2, resp.Pending, "all items should be pending before daemon processing")
		assert.Len(t, resp.Items, 2, "all items should appear in status response")
		for _, item := range resp.Items {
			assert.Equal(t, ItemStateNotEnqueued, item.State, "each item should be in not_enqueued state")
		}
	})

	t.Run("query non-existent batch returns nil", func(t *testing.T) {
		resp, err := querier.Query(context.Background(), uuid.New())
		require.NoError(t, err, "Query should not error for unknown batch ID")
		assert.Nil(t, resp, "response should be nil for non-existent batch")
	})
}

func TestFunctionalBatchCleanup(t *testing.T) {
	gormDB, cleanup := functionalTestSetup(t)
	defer cleanup()

	t.Run("deletes completed batches older than retention", func(t *testing.T) {
		batchID := uuid.New()
		eightDaysAgo := time.Now().UTC().Add(-8 * 24 * time.Hour)
		require.NoError(t, gormDB.Create(&Batch{
			ID: batchID, RequestedCount: 1,
			Status: workqueue.BatchStatusComplete, CompletedAt: &eightDaysAgo,
		}).Error, "test batch creation should succeed")
		require.NoError(t, gormDB.Create(&BatchItem{BatchID: batchID, ItemKey: "cleanup-1"}).Error,
			"test batch item creation should succeed")

		process := NewBatchCleanupProcess(gormDB)
		deleted, err := process.deleteCompletedBatches()
		require.NoError(t, err, "deleteCompletedBatches should succeed")
		assert.GreaterOrEqual(t, deleted, int64(1), "at least one old batch should be deleted")

		var count int64
		gormDB.Model(&Batch{}).Where("id = ?", batchID).Count(&count)
		assert.Zero(t, count, "completed batch older than retention should be removed")
	})

	t.Run("preserves recent completed batches", func(t *testing.T) {
		batchID := uuid.New()
		oneDayAgo := time.Now().UTC().Add(-24 * time.Hour)
		require.NoError(t, gormDB.Create(&Batch{
			ID: batchID, RequestedCount: 1,
			Status: workqueue.BatchStatusComplete, CompletedAt: &oneDayAgo,
		}).Error, "test batch creation should succeed")

		process := NewBatchCleanupProcess(gormDB)
		_, err := process.deleteCompletedBatches()
		require.NoError(t, err, "deleteCompletedBatches should succeed")

		var count int64
		gormDB.Model(&Batch{}).Where("id = ?", batchID).Count(&count)
		assert.Equal(t, int64(1), count, "recently completed batch should be preserved")
	})

	t.Run("marks stale non-terminal batches as failed", func(t *testing.T) {
		batchID := uuid.New()
		require.NoError(t, gormDB.Create(&Batch{
			ID: batchID, RequestedCount: 1, Status: workqueue.BatchStatusPending,
		}).Error, "test batch creation should succeed")
		twoDaysAgo := time.Now().UTC().Add(-2 * 24 * time.Hour)
		require.NoError(t, gormDB.Model(&Batch{}).Where("id = ?", batchID).
			Update("created_at", twoDaysAgo).Error, "backdating batch created_at should succeed")

		process := NewBatchCleanupProcess(gormDB)
		failed, err := process.failStaleBatches()
		require.NoError(t, err, "failStaleBatches should succeed")
		assert.GreaterOrEqual(t, failed, int64(1), "at least one stale batch should be marked failed")

		var batch Batch
		require.NoError(t, gormDB.Take(&batch, "id = ?", batchID).Error, "batch should still exist")
		assert.Equal(t, workqueue.BatchStatusFailed, batch.Status, "stale batch should be marked failed")
		assert.NotNil(t, batch.CompletedAt, "stale batch should have completed_at set")
	})
}

func TestFunctionalProcessBatchWorker(t *testing.T) {
	gormDB, cleanup := functionalTestSetup(t)
	defer cleanup()

	ctx := context.Background()
	dsn := os.Getenv("SIPPY_FUNCTIONAL_TEST_DSN")

	// Create a ReEvaluator with a real DB but nil cloud clients.
	// Only RefreshSymptomCache is called, which queries the symptoms table
	// (auto-migrated in setup, empty is fine).
	dbc := &db.DB{DB: gormDB}
	reEvaluator := apijobrunscan.NewReEvaluator(nil, nil, "", dbc, nil, nil)

	pool, err := workqueue.NewPgxV5Pool(ctx, dsn)
	require.NoError(t, err, "pgx/v5 pool for River client should succeed")
	defer pool.Close()

	riverClient, err := workqueue.NewInsertOnlyClient(pool)
	require.NoError(t, err, "River client creation should succeed")

	worker := NewProcessBatchWorker(reEvaluator, gormDB)
	worker.SetRiverClient(riverClient)

	submitter := NewSubmitter(gormDB, riverClient)
	suffix := uuid.New().String()[:8]
	result, err := submitter.Submit(ctx, []string{"batch-work-1-" + suffix, "batch-work-2-" + suffix}, false)
	require.NoError(t, err, "Submit should succeed")

	job := &river.Job[ProcessBatchArgs]{Args: ProcessBatchArgs{BatchID: result.BatchID}}
	require.NoError(t, worker.Work(ctx, job), "ProcessBatchWorker.Work should succeed")

	var items []BatchItem
	require.NoError(t, gormDB.Where("batch_id = ?", result.BatchID).Find(&items).Error,
		"batch items should load after processing")
	assert.Len(t, items, 2, "should have two batch items")
	for _, item := range items {
		assert.NotNil(t, item.RiverJobID, "item should have river_job_id populated after batch processing")
	}

	var batch Batch
	require.NoError(t, gormDB.Take(&batch, "id = ?", result.BatchID).Error,
		"batch should still exist after processing")
	assert.Equal(t, workqueue.BatchStatusRunning, batch.Status,
		"batch should transition to running after fan-out")
	assert.Equal(t, 2, batch.EnqueuedCount,
		"enqueued count should reflect items inserted into River")
}

// submitAndFanOut creates a batch with unique item keys, runs the
// ProcessBatchWorker to fan out River jobs, and returns the batch ID and
// the resulting batch items (with populated RiverJobIDs). The batch will
// be in BatchStatusRunning after this call.
func submitAndFanOut(ctx context.Context, t *testing.T, gormDB *gorm.DB, riverClient *river.Client[pgx.Tx], n int) (uuid.UUID, []BatchItem) {
	t.Helper()

	dbc := &db.DB{DB: gormDB}
	reEvaluator := apijobrunscan.NewReEvaluator(nil, nil, "", dbc, nil, nil)

	suffix := uuid.New().String()[:8]
	jobIDs := make([]string, n)
	for i := range jobIDs {
		jobIDs[i] = fmt.Sprintf("item-%d-%s", i, suffix)
	}

	submitter := NewSubmitter(gormDB, riverClient)
	result, err := submitter.Submit(ctx, jobIDs, false)
	require.NoError(t, err, "Submit should succeed")

	worker := NewProcessBatchWorker(reEvaluator, gormDB)
	worker.SetRiverClient(riverClient)
	job := &river.Job[ProcessBatchArgs]{Args: ProcessBatchArgs{BatchID: result.BatchID}}
	require.NoError(t, worker.Work(ctx, job), "ProcessBatchWorker.Work should succeed")

	var items []BatchItem
	require.NoError(t, gormDB.Where("batch_id = ?", result.BatchID).Find(&items).Error,
		"loading fan-out items should succeed")
	for _, item := range items {
		require.NotNil(t, item.RiverJobID, "all items should have river_job_id after fan-out")
	}
	return result.BatchID, items
}

// setRiverJobState directly updates a river_job row's state via raw SQL,
// bypassing River's state machine. Terminal states (completed, cancelled,
// discarded) also set finalized_at to satisfy River's check constraint.
func setRiverJobState(t *testing.T, gormDB *gorm.DB, riverJobID int64, state string) {
	t.Helper()
	var result *gorm.DB
	switch state {
	case ItemStateCompleted, ItemStateCancelled, ItemStateDiscarded:
		result = gormDB.Exec(
			"UPDATE river_job SET state = ?::river_job_state, finalized_at = NOW() WHERE id = ?",
			state, riverJobID)
	default:
		result = gormDB.Exec(
			"UPDATE river_job SET state = ?::river_job_state, finalized_at = NULL WHERE id = ?",
			state, riverJobID)
	}
	require.NoError(t, result.Error, "updating river_job state should succeed")
	require.Equal(t, int64(1), result.RowsAffected, "exactly one river_job row should be updated")
}

func TestFunctionalQueryLazyCompletion(t *testing.T) {
	gormDB, cleanup := functionalTestSetup(t)
	defer cleanup()

	ctx := context.Background()
	dsn := os.Getenv("SIPPY_FUNCTIONAL_TEST_DSN")
	pool, err := workqueue.NewPgxV5Pool(ctx, dsn)
	require.NoError(t, err, "pgx/v5 pool should succeed")
	defer pool.Close()

	riverClient, err := workqueue.NewInsertOnlyClient(pool)
	require.NoError(t, err, "River client should be created")
	querier := NewStatusQuerier(gormDB)

	t.Run("all completed triggers batch completion", func(t *testing.T) {
		batchID, items := submitAndFanOut(ctx, t, gormDB, riverClient, 3)

		for _, item := range items {
			setRiverJobState(t, gormDB, *item.RiverJobID, "completed")
		}

		resp, err := querier.Query(ctx, batchID)
		require.NoError(t, err, "Query should succeed")
		assert.Equal(t, workqueue.BatchStatusComplete, resp.Status,
			"batch should transition to complete when all items completed")
		assert.Equal(t, 3, resp.Completed, "all items should count as completed")
		assert.Zero(t, resp.Failed, "no items should be failed")
		assert.Zero(t, resp.Pending, "no items should be pending")

		var batch Batch
		require.NoError(t, gormDB.Take(&batch, "id = ?", batchID).Error)
		assert.NotNil(t, batch.CompletedAt, "completed_at should be set")
	})

	t.Run("all failed triggers batch failed", func(t *testing.T) {
		batchID, items := submitAndFanOut(ctx, t, gormDB, riverClient, 2)

		setRiverJobState(t, gormDB, *items[0].RiverJobID, "discarded")
		setRiverJobState(t, gormDB, *items[1].RiverJobID, "cancelled")

		resp, err := querier.Query(ctx, batchID)
		require.NoError(t, err, "Query should succeed")
		assert.Equal(t, workqueue.BatchStatusFailed, resp.Status,
			"batch should transition to failed when all items failed")
		assert.Equal(t, 2, resp.Failed, "all items should count as failed")
		assert.Zero(t, resp.Completed, "no items should be completed")
	})

	t.Run("mixed completed and failed triggers batch complete", func(t *testing.T) {
		batchID, items := submitAndFanOut(ctx, t, gormDB, riverClient, 3)

		setRiverJobState(t, gormDB, *items[0].RiverJobID, "completed")
		setRiverJobState(t, gormDB, *items[1].RiverJobID, "completed")
		setRiverJobState(t, gormDB, *items[2].RiverJobID, "discarded")

		resp, err := querier.Query(ctx, batchID)
		require.NoError(t, err, "Query should succeed")
		assert.Equal(t, workqueue.BatchStatusComplete, resp.Status,
			"batch with at least one success should be complete, not failed")
		assert.Equal(t, 2, resp.Completed)
		assert.Equal(t, 1, resp.Failed)
	})

	t.Run("non-terminal items prevent lazy completion", func(t *testing.T) {
		batchID, items := submitAndFanOut(ctx, t, gormDB, riverClient, 2)

		setRiverJobState(t, gormDB, *items[0].RiverJobID, "completed")
		// items[1] stays in its initial state (available/pending)

		resp, err := querier.Query(ctx, batchID)
		require.NoError(t, err, "Query should succeed")
		assert.Equal(t, workqueue.BatchStatusRunning, resp.Status,
			"batch should stay running while items are still pending")
		assert.Equal(t, 1, resp.Completed)
		assert.Equal(t, 1, resp.Pending)
	})

	t.Run("lazy completion is idempotent", func(t *testing.T) {
		batchID, items := submitAndFanOut(ctx, t, gormDB, riverClient, 1)

		setRiverJobState(t, gormDB, *items[0].RiverJobID, "completed")

		resp1, err := querier.Query(ctx, batchID)
		require.NoError(t, err, "first Query should succeed")
		assert.Equal(t, workqueue.BatchStatusComplete, resp1.Status)

		resp2, err := querier.Query(ctx, batchID)
		require.NoError(t, err, "second Query should succeed")
		assert.Equal(t, workqueue.BatchStatusComplete, resp2.Status,
			"repeated Query should return same status")
	})
}

func TestFunctionalBatchCanceller(t *testing.T) {
	gormDB, cleanup := functionalTestSetup(t)
	defer cleanup()

	ctx := context.Background()
	dsn := os.Getenv("SIPPY_FUNCTIONAL_TEST_DSN")
	pool, err := workqueue.NewPgxV5Pool(ctx, dsn)
	require.NoError(t, err, "pgx/v5 pool should succeed")
	defer pool.Close()

	riverClient, err := workqueue.NewInsertOnlyClient(pool)
	require.NoError(t, err, "River client should be created")
	canceller := NewBatchCanceller(gormDB, riverClient)

	t.Run("cancel running batch", func(t *testing.T) {
		batchID, _ := submitAndFanOut(ctx, t, gormDB, riverClient, 2)

		resp, err := canceller.Cancel(ctx, batchID)
		require.NoError(t, err, "Cancel should succeed")
		require.NotNil(t, resp, "response should be non-nil")
		assert.Equal(t, workqueue.BatchStatusCancelled, resp.Status,
			"batch should be cancelled")

		var batch Batch
		require.NoError(t, gormDB.Take(&batch, "id = ?", batchID).Error)
		assert.Equal(t, workqueue.BatchStatusCancelled, batch.Status)
		assert.NotNil(t, batch.CompletedAt, "completed_at should be set on cancellation")
	})

	t.Run("cancel already-cancelled batch returns ErrBatchTerminal", func(t *testing.T) {
		batchID, _ := submitAndFanOut(ctx, t, gormDB, riverClient, 1)

		_, err := canceller.Cancel(ctx, batchID)
		require.NoError(t, err, "first Cancel should succeed")

		_, err = canceller.Cancel(ctx, batchID)
		require.Error(t, err, "second Cancel should fail")
		assert.ErrorIs(t, err, ErrBatchTerminal,
			"cancelling an already-cancelled batch should return ErrBatchTerminal")
	})

	t.Run("cancel completed batch returns ErrBatchTerminal", func(t *testing.T) {
		batchID, items := submitAndFanOut(ctx, t, gormDB, riverClient, 1)
		setRiverJobState(t, gormDB, *items[0].RiverJobID, "completed")

		querier := NewStatusQuerier(gormDB)
		_, err := querier.Query(ctx, batchID)
		require.NoError(t, err, "Query to trigger lazy completion should succeed")

		_, err = canceller.Cancel(ctx, batchID)
		require.Error(t, err, "Cancel should fail for completed batch")
		assert.ErrorIs(t, err, ErrBatchTerminal)
	})

	t.Run("cancel non-existent batch returns nil", func(t *testing.T) {
		resp, err := canceller.Cancel(ctx, uuid.New())
		require.NoError(t, err, "Cancel should not error for unknown batch")
		assert.Nil(t, resp, "response should be nil for non-existent batch")
	})
}

func TestFunctionalReevaluateWorker(t *testing.T) {
	t.Run("delegates to reEvalFunc with correct args", func(t *testing.T) {
		var calledID string
		var calledDryRun bool
		worker := &ReevaluateWorker{
			reEval: func(_ context.Context, prowJobBuildID string, dryRun bool) error {
				calledID = prowJobBuildID
				calledDryRun = dryRun
				return nil
			},
		}

		job := &river.Job[ReevaluateJobRunArgs]{
			Args: ReevaluateJobRunArgs{
				ProwJobBuildID: "test-build-42",
				DryRun:         true,
			},
		}
		err := worker.Work(context.Background(), job)
		require.NoError(t, err, "Work should succeed when reEvalFunc returns nil")
		assert.Equal(t, "test-build-42", calledID,
			"prowJobBuildID should be forwarded to reEvalFunc")
		assert.True(t, calledDryRun,
			"dryRun flag should be forwarded to reEvalFunc")
	})

	t.Run("propagates errors from reEvalFunc", func(t *testing.T) {
		worker := &ReevaluateWorker{
			reEval: func(_ context.Context, _ string, _ bool) error {
				return fmt.Errorf("simulated evaluation failure")
			},
		}

		job := &river.Job[ReevaluateJobRunArgs]{
			Args: ReevaluateJobRunArgs{ProwJobBuildID: "fail-build"},
		}
		err := worker.Work(context.Background(), job)
		assert.Error(t, err, "Work should propagate reEvalFunc errors for River retry")
		assert.Contains(t, err.Error(), "simulated evaluation failure",
			"error message should come from reEvalFunc")
	})
}
