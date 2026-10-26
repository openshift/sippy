package symptomre

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

// StatusQuerier queries the current status of a symptom re-evaluation batch
// by joining batch items with River job state.
type StatusQuerier struct {
	gormDB *gorm.DB
}

// NewStatusQuerier creates a StatusQuerier.
func NewStatusQuerier(gormDB *gorm.DB) *StatusQuerier {
	return &StatusQuerier{gormDB: gormDB}
}

// GetUpdated loads a batch and its items, joining with river_job to get current
// states. It performs lazy completion detection: when all items have reached
// a terminal state, the batch is marked complete (or failed if all items
// failed) and completed_at is set. This is idempotent.
func (q *StatusQuerier) GetUpdated(ctx context.Context, batchID uuid.UUID) (*BatchStatusResponse, error) {
	db := q.gormDB.WithContext(ctx)

	var batch Batch
	if err := db.Take(&batch, "id = ?", batchID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading batch %s: %w", batchID, err)
	}

	itemStatus, resp, err := queryItemStatus(db, batch)
	if err != nil {
		return resp, err
	}

	// Lazy completion: if batch is running/processing and all items are terminal,
	// update the batch status.
	if batch.Status == workqueue.BatchStatusRunning || batch.Status == workqueue.BatchStatusProcessing {
		if workqueue.IsTerminalBatchStatus(itemStatus) {
			now := time.Now()
			if res := db.Where(&Batch{ID: batchID, Status: batch.Status}).
				Updates(&Batch{Status: itemStatus, CompletedAt: &now}); res.Error != nil {
				// updating the status in the DB is optional hygiene, really
				log.WithError(res.Error).Warnf("failed updating batch %s completion", batchID)
			} else if res.RowsAffected == 0 {
				// lost an update race; reload and report that status instead
				resp = nil
				return q.GetUpdated(ctx, batchID)
			}
			resp.Status = itemStatus
		}
	}

	return resp, nil
}

func queryItemStatus(db *gorm.DB, batch Batch) (workqueue.BatchStatus, *BatchStatusResponse, error) {
	var items []ItemStatus
	if err := db.Raw(`
		SELECT bi.item_key, rj.metadata->'output' AS result,
		       CASE WHEN bi.river_job_id IS NULL THEN @notEnqueued
		            WHEN rj.id IS NULL THEN @orphaned
		            ELSE rj.state::text END AS state
		FROM workqueue_symptom_re_batch_items bi
		LEFT JOIN river_job rj ON rj.id = bi.river_job_id
		WHERE bi.batch_id = @batchID`,
		map[string]interface{}{
			"notEnqueued": ItemStateNotEnqueued,
			"orphaned":    ItemStateOrphaned,
			"batchID":     batch.ID,
		}).Scan(&items).Error; err != nil {
		return "", nil, fmt.Errorf("querying batch items for %s: %w", batch.ID, err)
	}

	counts := classifyItemStates(items)
	itemStatus := workqueue.OverallStatus(counts)

	resp := &BatchStatusResponse{
		BatchID: batch.ID,
		Status:  batch.Status,
		BatchStatusCounts: BatchStatusCounts{
			Requested: batch.RequestedCount,
			Enqueued:  batch.EnqueuedCount,
			Deduped:   batch.DedupedCount,
			Completed: counts.Completed,
			Failed:    counts.Failed,
			Running:   counts.Running,
			Pending:   counts.Pending,
		},
		Items: items,
	}
	return itemStatus, resp, nil
}

// classifyItemStates aggregates a slice of ItemStatus into counts by
// category: completed, failed (discarded/cancelled/orphaned), running,
// and pending (everything else including not-enqueued).
func classifyItemStates(items []ItemStatus) workqueue.ItemStateCounts {
	counts := workqueue.ItemStateCounts{Total: len(items)}
	for _, item := range items {
		switch item.State {
		case ItemStateCompleted:
			counts.Completed++
		case ItemStateDiscarded, ItemStateCancelled, ItemStateOrphaned:
			counts.Failed++
		case ItemStateRunning:
			counts.Running++
		default:
			// ItemStateNotEnqueued, ItemStateAvailable, ItemStateScheduled,
			// ItemStateRetryable, ItemStatePending
			counts.Pending++
		}
	}
	return counts
}
