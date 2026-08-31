package symptomre

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

// ErrBatchTerminal indicates the batch is already in a terminal status and
// cannot be cancelled. The handler maps this to 409 Conflict.
var ErrBatchTerminal = errors.New("batch is already in a terminal status")

// BatchCanceller cancels in-flight symptom re-evaluation batches by
// requesting cancellation of their River jobs and marking the batch as
// cancelled. Jobs that have already completed are left alone.
type BatchCanceller struct {
	gormDB      *gorm.DB
	riverClient *river.Client[pgx.Tx]
}

// NewBatchCanceller creates a BatchCanceller.
func NewBatchCanceller(gormDB *gorm.DB, riverClient *river.Client[pgx.Tx]) *BatchCanceller {
	return &BatchCanceller{
		gormDB:      gormDB,
		riverClient: riverClient,
	}
}

// Cancel attempts to cancel all non-completed River jobs in a batch and marks
// the batch as cancelled. Returns the batch status response after cancellation.
// Returns nil if the batch does not exist.
func (c *BatchCanceller) Cancel(ctx context.Context, batchID uuid.UUID) (*BatchStatusResponse, error) {
	var batch Batch
	if err := c.gormDB.Take(&batch, "id = ?", batchID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading batch %s: %w", batchID, err)
	}

	if batch.Status == workqueue.BatchStatusComplete ||
		batch.Status == workqueue.BatchStatusFailed ||
		batch.Status == workqueue.BatchStatusCancelled {
		return nil, fmt.Errorf("%w: batch %s has status %q", ErrBatchTerminal, batchID, batch.Status)
	}

	var items []BatchItem
	if err := c.gormDB.Where("batch_id = ?", batchID).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("loading batch items for %s: %w", batchID, err)
	}

	cancelled := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("cancellation interrupted: %w", ctx.Err())
		}
		if item.RiverJobID == nil {
			continue
		}
		if _, err := c.riverClient.JobCancel(ctx, *item.RiverJobID); err != nil {
			log.WithFields(log.Fields{
				"batchID":    batchID,
				"riverJobID": *item.RiverJobID,
			}).WithError(err).Warn("batch cancel: failed to cancel River job (may already be complete)")
			continue
		}
		cancelled++
	}

	now := time.Now()
	if err := c.gormDB.Model(&Batch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
		"status":       workqueue.BatchStatusCancelled,
		"completed_at": now,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating batch %s to cancelled: %w", batchID, err)
	}

	log.WithFields(log.Fields{
		"batchID":        batchID,
		"riverCancelled": cancelled,
		"totalItems":     len(items),
	}).Info("batch cancel: batch cancelled")

	querier := NewStatusQuerier(c.gormDB)
	return querier.Query(batchID)
}
