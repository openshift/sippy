package symptomre

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

// ProcessBatchWorker handles ProcessBatchArgs River jobs. When the daemon picks
// up a new batch, this worker refreshes the symptom cache, fans out individual
// re-evaluation River jobs, and tracks which items were enqueued vs deduplicated.
type ProcessBatchWorker struct {
	river.WorkerDefaults[ProcessBatchArgs]
	reEvaluator *jobrunscan.ReEvaluator
	gormDB      *gorm.DB
	riverClient *river.Client[pgx.Tx]
}

// NewProcessBatchWorker creates a ProcessBatchWorker. The riverClient field
// must be set via SetRiverClient before the worker processes any jobs, because
// the River client cannot be created until after the worker is registered
// (circular dependency resolved by deferred wiring).
func NewProcessBatchWorker(reEvaluator *jobrunscan.ReEvaluator, gormDB *gorm.DB) *ProcessBatchWorker {
	return &ProcessBatchWorker{
		reEvaluator: reEvaluator,
		gormDB:      gormDB,
	}
}

// SetRiverClient wires the River client after construction. Must be called
// before the River client is started.
func (w *ProcessBatchWorker) SetRiverClient(client *river.Client[pgx.Tx]) {
	w.riverClient = client
}

// Work processes a batch: refreshes symptoms, fans out individual River jobs,
// and updates batch/item state. MaxAttempts is 1 (no automatic retry); on
// failure the batch stays in "processing" and must be resubmitted.
func (w *ProcessBatchWorker) Work(ctx context.Context, job *river.Job[ProcessBatchArgs]) error {
	batchID := job.Args.BatchID
	logger := log.WithField("batchID", batchID)
	logger.Info("symptom reEval: processing batch")

	var batch Batch
	if err := w.gormDB.Take(&batch, "id = ?", batchID).Error; err != nil {
		return fmt.Errorf("loading batch %s: %w", batchID, err)
	}

	var items []BatchItem
	if err := w.gormDB.Where("batch_id = ?", batchID).Find(&items).Error; err != nil {
		return fmt.Errorf("loading batch items for %s: %w", batchID, err)
	}

	if err := w.gormDB.Model(&Batch{}).Where("id = ?", batchID).
		Update("status", workqueue.BatchStatusProcessing).Error; err != nil {
		return fmt.Errorf("updating batch %s to processing: %w", batchID, err)
	}

	enqueued, deduped, err := w.fanOutItems(ctx, batchID, items, batch.DryRun)
	if err != nil {
		return err
	}

	if err := w.finalizeBatch(batchID, enqueued, deduped); err != nil {
		return err
	}

	logger.WithFields(log.Fields{
		"enqueued": enqueued,
		"deduped":  deduped,
		"total":    len(items),
	}).Info("symptom reEval: batch fan-out complete")
	return nil
}

// fanOutItems refreshes the symptom cache, inserts individual River jobs for
// each batch item, and links the resulting River job IDs back to the items.
// Returns the count of newly enqueued and deduplicated jobs.
func (w *ProcessBatchWorker) fanOutItems(ctx context.Context, batchID uuid.UUID, items []BatchItem, dryRun bool) (enqueued, deduped int, err error) {
	symptomHash, err := w.reEvaluator.RefreshSymptomCache(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("refreshing symptom cache for batch %s: %w", batchID, err)
	}

	params := make([]river.InsertManyParams, len(items))
	for i, item := range items {
		params[i] = river.InsertManyParams{
			Args: ReevaluateJobRunArgs{
				ProwJobBuildID: item.ItemKey,
				SymptomHash:    symptomHash,
				DryRun:         dryRun,
			},
		}
	}

	results, err := w.riverClient.InsertMany(ctx, params)
	if err != nil {
		return 0, 0, fmt.Errorf("inserting re-evaluation jobs for batch %s: %w", batchID, err)
	}

	for i, result := range results {
		riverJobID := result.Job.ID
		items[i].RiverJobID = &riverJobID
		if result.UniqueSkippedAsDuplicate {
			deduped++
		} else {
			enqueued++
		}
		if err := w.gormDB.Model(&BatchItem{}).Where("id = ?", items[i].ID).
			Update("river_job_id", riverJobID).Error; err != nil {
			return 0, 0, fmt.Errorf("updating batch item %d river_job_id: %w", items[i].ID, err)
		}
	}

	return enqueued, deduped, nil
}

// finalizeBatch updates the batch row with enqueued/deduped counts and sets
// status to running.
func (w *ProcessBatchWorker) finalizeBatch(batchID uuid.UUID, enqueued, deduped int) error {
	if err := w.gormDB.Model(&Batch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
		"enqueued_count": enqueued,
		"deduped_count":  deduped,
		"status":         workqueue.BatchStatusRunning,
	}).Error; err != nil {
		return fmt.Errorf("updating batch %s to running: %w", batchID, err)
	}
	return nil
}

// reEvalFunc is the function signature for re-evaluating a single job run's
// symptoms from the cached symptom definitions. Extracting this as a function
// type field lets tests substitute a stub without requiring a full ReEvaluator
// (which needs GCS and BigQuery credentials).
type reEvalFunc func(ctx context.Context, prowJobBuildID string, dryRun bool) error

// ReevaluateWorker handles individual ReevaluateJobRunArgs River jobs by
// delegating to the ReEvaluator's cached symptom evaluation.
type ReevaluateWorker struct {
	river.WorkerDefaults[ReevaluateJobRunArgs]
	reEval reEvalFunc
}

// NewReevaluateWorker creates a ReevaluateWorker that delegates to the
// ReEvaluator's cached evaluation method.
func NewReevaluateWorker(reEvaluator *jobrunscan.ReEvaluator) *ReevaluateWorker {
	return &ReevaluateWorker{reEval: reEvaluator.ReEvaluateOneFromCache}
}

// Work re-evaluates symptoms for a single job run. Transient errors trigger
// River's retry logic (up to MaxAttemptsPerItem attempts with exponential
// backoff). Permanent errors (e.g. missing job run) cancel the job immediately.
func (w *ReevaluateWorker) Work(ctx context.Context, job *river.Job[ReevaluateJobRunArgs]) error {
	err := w.reEval(ctx, job.Args.ProwJobBuildID, job.Args.DryRun)
	if err != nil && errors.Is(err, jobrunscan.ErrPermanent) {
		return river.JobCancel(err)
	}
	return err
}
