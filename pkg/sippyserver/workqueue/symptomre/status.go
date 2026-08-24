package symptomre

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

// BatchStatusCounts holds the numeric counters for a batch status response.
type BatchStatusCounts struct {
	Requested int `json:"requested"`
	Enqueued  int `json:"enqueued"`
	Deduped   int `json:"deduped"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Running   int `json:"running"`
	Pending   int `json:"pending"`
}

// BatchStatusResponse is the API response for querying a batch's status.
type BatchStatusResponse struct {
	BatchID uuid.UUID             `json:"batch_id"`
	Status  workqueue.BatchStatus `json:"status"`
	BatchStatusCounts
	Items []ItemStatus `json:"items"`
}

// ItemStatus reports the state of a single item within a batch.
type ItemStatus struct {
	ItemKey string `json:"item_key"`
	State   string `json:"state"`
}

// StatusQuerier queries the current status of a symptom re-evaluation batch
// by joining batch items with River job state.
type StatusQuerier struct {
	gormDB *gorm.DB
}

// NewStatusQuerier creates a StatusQuerier.
func NewStatusQuerier(gormDB *gorm.DB) *StatusQuerier {
	return &StatusQuerier{gormDB: gormDB}
}

// itemRow is the result of the batch-item/river-job join query.
type itemRow struct {
	ItemKey string `gorm:"column:item_key"`
	State   string `gorm:"column:state"`
}

// Query loads a batch and its items, joining with river_job to get current
// states. It performs lazy completion detection: when all items have reached
// a terminal state, the batch is marked complete (or failed if all items
// failed) and completed_at is set. This is idempotent.
func (q *StatusQuerier) Query(batchID uuid.UUID) (*BatchStatusResponse, error) {
	var batch Batch
	if err := q.gormDB.Take(&batch, "id = ?", batchID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading batch %s: %w", batchID, err)
	}

	var rows []itemRow
	if err := q.gormDB.Raw(`
		SELECT bi.item_key,
		       CASE WHEN bi.river_job_id IS NULL THEN 'pending'
		            ELSE rj.state END AS state
		FROM workqueue_symptom_re_batch_items bi
		LEFT JOIN river_job rj ON rj.id = bi.river_job_id
		WHERE bi.batch_id = ?`, batchID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying batch items for %s: %w", batchID, err)
	}

	counts := workqueue.ItemStateCounts{Total: len(rows)}
	items := make([]ItemStatus, len(rows))
	for i, r := range rows {
		items[i] = ItemStatus{ItemKey: r.ItemKey, State: r.State}
		switch r.State {
		case "completed":
			counts.Completed++
		case "discarded", "cancelled":
			counts.Failed++
		case "running":
			counts.Running++
		default:
			// pending, available, scheduled, retryable
			counts.Pending++
		}
	}

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
			if err := q.gormDB.Model(&Batch{}).Where("id = ?", batchID).
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
