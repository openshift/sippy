package workqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river/rivertype"
	"gorm.io/gorm"
)

// ItemStatus represents a single work item's current state.
type ItemStatus struct {
	ItemKey string              `json:"item_key"`
	State   rivertype.JobState  `json:"state"`
	Errors  []map[string]string `json:"errors,omitempty"`
}

// BatchStatusResponse contains the full status of a batch.
type BatchStatusResponse struct {
	BatchID   uuid.UUID    `json:"batch_id"`
	Status    BatchStatus  `json:"status"`
	Total     int          `json:"total"`
	Completed int          `json:"completed"`
	Failed    int          `json:"failed"`
	Running   int          `json:"running"`
	Pending   int          `json:"pending"`
	Items     []ItemStatus `json:"items"`
}

// batchItemRow is the result of joining batch_items with river_job.
type batchItemRow struct {
	ItemKey  string
	JobState string
}

// StatusQuerier retrieves batch status by joining batch items with River jobs.
type StatusQuerier struct {
	db *gorm.DB
}

// NewStatusQuerier creates a StatusQuerier.
func NewStatusQuerier(db *gorm.DB) *StatusQuerier {
	return &StatusQuerier{db: db}
}

// Query loads a batch and its items' current River job states.
func (q *StatusQuerier) Query(ctx context.Context, batchID uuid.UUID) (*BatchStatusResponse, error) {
	var batch Batch
	if err := q.db.WithContext(ctx).First(&batch, "id = ?", batchID).Error; err != nil {
		return nil, fmt.Errorf("loading batch: %w", err)
	}

	var rows []batchItemRow
	err := q.db.WithContext(ctx).
		Table("workqueue_batch_items bi").
		Select("bi.item_key, rj.state as job_state").
		Joins("JOIN river_job rj ON rj.id = bi.river_job_id").
		Where("bi.batch_id = ?", batchID).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("querying batch items: %w", err)
	}

	resp := &BatchStatusResponse{
		BatchID: batchID,
		Total:   len(rows),
	}

	resp.Items = make([]ItemStatus, len(rows))
	for i, row := range rows {
		state := rivertype.JobState(row.JobState)
		resp.Items[i] = ItemStatus{
			ItemKey: row.ItemKey,
			State:   state,
		}

		switch categorizeState(state) {
		case stateCompleted:
			resp.Completed++
		case stateFailed:
			resp.Failed++
		case stateRunning:
			resp.Running++
		case statePending:
			resp.Pending++
		}
	}

	resp.Status = computeBatchStatus(&batch, resp)
	q.maybeFinalizeBatch(ctx, &batch, resp)

	return resp, nil
}

type stateCategory int

const (
	statePending stateCategory = iota
	stateRunning
	stateCompleted
	stateFailed
)

func categorizeState(s rivertype.JobState) stateCategory {
	switch s {
	case rivertype.JobStateCompleted:
		return stateCompleted
	case rivertype.JobStateDiscarded, rivertype.JobStateCancelled:
		return stateFailed
	case rivertype.JobStateRunning:
		return stateRunning
	default:
		return statePending
	}
}

func computeBatchStatus(batch *Batch, resp *BatchStatusResponse) BatchStatus {
	if batch.CompletedAt != nil {
		return batch.Status
	}
	if resp.Total == 0 {
		return BatchStatusComplete
	}
	terminal := resp.Completed + resp.Failed
	if terminal < resp.Total {
		return BatchStatusRunning
	}
	if resp.Completed == 0 {
		return BatchStatusFailed
	}
	return BatchStatusComplete
}

// maybeFinalizeBatch updates the batch row if all items have reached a terminal
// state. This is idempotent — repeated polls are harmless.
func (q *StatusQuerier) maybeFinalizeBatch(ctx context.Context, batch *Batch, resp *BatchStatusResponse) {
	if batch.CompletedAt != nil {
		return
	}
	terminal := resp.Completed + resp.Failed
	if resp.Total == 0 || terminal < resp.Total {
		return
	}

	now := time.Now()
	q.db.WithContext(ctx).Model(batch).Updates(map[string]interface{}{
		"status":       resp.Status,
		"completed_at": now,
	})
}
