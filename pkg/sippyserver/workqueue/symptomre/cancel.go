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
// cannot be canceled. The handler maps this to 409 Conflict.
var ErrBatchTerminal = errors.New("batch is already in a terminal status")

// BatchCanceller cancels in-flight symptom re-evaluation batches by
// requesting cancellation of their River jobs and marking the batch as
// canceled. Jobs that have already completed are left alone.
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
// the batch as canceled. Returns the batch status response after cancellation.
// Returns nil if the batch does not exist.
func (c *BatchCanceller) Cancel(ctx context.Context, batchID uuid.UUID) (*BatchStatusResponse, error) {
	db := c.gormDB.WithContext(ctx) // enable canceling the context

	var batch Batch
	if err := db.Take(&batch, "id = ?", batchID).Error; err != nil {
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
	if err := db.Where("batch_id = ?", batchID).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("loading batch items for %s: %w", batchID, err)
	}

	canceled := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("cancellation interrupted: %w", ctx.Err())
		}
		if item.RiverJobID == nil {
			continue
		}
		if _, err := c.riverClient.JobCancel(ctx, *item.RiverJobID); err != nil {
			// cancellation is best-effort; if a job keeps running, it only wastes some resources. just log it.
			log.WithFields(log.Fields{
				"batchID":    batchID,
				"riverJobID": *item.RiverJobID,
			}).WithError(err).Warn("batch cancel: failed to cancel River job (may already be complete)")
			continue
		}
		canceled++
	}

	now := time.Now()
	if err := db.Model(&Batch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
		"status":       workqueue.BatchStatusCancelled,
		"completed_at": now,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating batch %s to canceled: %w", batchID, err)
	}

	log.WithFields(log.Fields{
		"batchID":        batchID,
		"riverCancelled": canceled,
		"totalItems":     len(items),
	}).Info("batch cancel: batch canceled")

	querier := NewStatusQuerier(db)
	return querier.GetUpdated(ctx, batchID)
}
