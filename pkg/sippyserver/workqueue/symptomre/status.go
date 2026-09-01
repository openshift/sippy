package symptomre

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// Query loads a batch and its items, joining with river_job to get current
// states. It performs lazy completion detection: when all items have reached
// a terminal state, the batch is marked complete (or failed if all items
// failed) and completed_at is set. This is idempotent.
func (q *StatusQuerier) Query(ctx context.Context, batchID uuid.UUID) (*BatchStatusResponse, error) {
	db := q.gormDB.WithContext(ctx)

	var batch Batch
	if err := db.Take(&batch, "id = ?", batchID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading batch %s: %w", batchID, err)
	}

	var items []ItemStatus
	if err := db.Raw(`
		SELECT bi.item_key,
		       CASE WHEN bi.river_job_id IS NULL THEN @notEnqueued
		            WHEN rj.id IS NULL THEN @orphaned
		            ELSE rj.state::text END AS state
		FROM workqueue_symptom_re_batch_items bi
		LEFT JOIN river_job rj ON rj.id = bi.river_job_id
		WHERE bi.batch_id = @batchID`,
		map[string]interface{}{
			"notEnqueued": ItemStateNotEnqueued,
			"orphaned":    ItemStateOrphaned,
			"batchID":     batchID,
		}).Scan(&items).Error; err != nil {
		return nil, fmt.Errorf("querying batch items for %s: %w", batchID, err)
	}

	counts := classifyItemStates(items)

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

	// Lazy completion: if batch is running/processing and all items are terminal,
	// update the batch status.
	if batch.Status == workqueue.BatchStatusRunning || batch.Status == workqueue.BatchStatusProcessing {
		derived := workqueue.OverallStatus(counts)
		if derived == workqueue.BatchStatusComplete || derived == workqueue.BatchStatusFailed {
			now := time.Now()
			if err := db.Model(&Batch{}).Where("id = ? and status = ?", batchID, batch.Status).
				Updates(map[string]interface{}{
					"status":       derived,
					"completed_at": now,
				}).Error; err != nil {
				return nil, fmt.Errorf("updating batch %s completion: %w", batchID, err)
			}
			resp.Status = derived
		}
	}

	return resp, nil
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
