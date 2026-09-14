package symptomre

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

const (
	// CompletedBatchRetention is how long completed (or failed) batches are
	// kept before periodic cleanup deletes them. The ON DELETE CASCADE
	// foreign key on batch items handles child-row removal automatically.
	CompletedBatchRetention = 7 * 24 * time.Hour

	// StaleBatchTimeout is the maximum age for a batch in a non-terminal
	// status (pending, processing, running) before it is considered stuck
	// and removed by the cleanup process.
	StaleBatchTimeout = 24 * time.Hour

	// cleanupInterval is how often the cleanup loop runs.
	cleanupInterval = 1 * time.Hour
)

// staleBatchCancelFunc cancels a single stale batch by ID, including any
// in-flight River jobs.
type staleBatchCancelFunc func(ctx context.Context, batchID uuid.UUID) error

// BatchCleanupProcess periodically removes old completed batches and
// cancels stale non-terminal batches. It implements the DaemonProcess
// interface so the daemon server can manage its lifecycle.
type BatchCleanupProcess struct {
	db                 *gorm.DB
	completedRetention time.Duration
	staleTimeout       time.Duration
	cancelStale        staleBatchCancelFunc
}

// NewBatchCleanupProcess creates a cleanup process with the default retention
// periods for completed and stale batches. The canceller is used to cancel
// stale batches (including their in-flight River jobs).
func NewBatchCleanupProcess(db *gorm.DB, canceller *BatchCanceller) *BatchCleanupProcess {
	p := &BatchCleanupProcess{
		db:                 db,
		completedRetention: CompletedBatchRetention,
		staleTimeout:       StaleBatchTimeout,
	}
	if canceller != nil {
		p.cancelStale = func(ctx context.Context, batchID uuid.UUID) error {
			_, err := canceller.Cancel(ctx, batchID)
			return err
		}
	}
	return p
}

// Run executes the periodic cleanup loop, deleting old batches every hour
// until the context is canceled.
func (p *BatchCleanupProcess) Run(ctx context.Context) {
	log.Info("batch cleanup: starting periodic cleanup process")

	// Run once immediately at startup, then on a ticker.
	p.runCleanup(ctx)

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("batch cleanup: shutting down")
			return
		case <-ticker.C:
			p.runCleanup(ctx)
		}
	}
}

// runCleanup performs a single cleanup pass, removing completed batches older
// than the retention period and cancelling stale non-terminal batches.
func (p *BatchCleanupProcess) runCleanup(ctx context.Context) {
	deleted, err := p.deleteCompletedBatches(ctx)
	if err != nil {
		log.WithError(err).Error("batch cleanup: failed to delete completed batches")
	} else if deleted > 0 {
		log.WithField("count", deleted).Info("batch cleanup: deleted old completed batches")
	}

	cancelled, err := p.cancelStaleBatches(ctx)
	if err != nil {
		log.WithError(err).Error("batch cleanup: failed to cancel stale batches")
	} else if cancelled > 0 {
		log.WithField("count", cancelled).Info("batch cleanup: cancelled stale non-terminal batches")
	}
}

// deleteCompletedBatches removes batches that have a non-null completed_at
// timestamp older than the configured retention period.
func (p *BatchCleanupProcess) deleteCompletedBatches(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-p.completedRetention)
	result := p.db.WithContext(ctx).
		Where("completed_at IS NOT NULL AND completed_at < ?", cutoff).
		Delete(&Batch{})
	if result.Error != nil {
		return 0, fmt.Errorf("deleting completed batches older than %v: %w", p.completedRetention, result.Error)
	}
	return result.RowsAffected, nil
}

// cancelStaleBatches finds batches stuck in non-terminal statuses (pending,
// processing, running) past the stale timeout and cancels them, including
// any in-flight River jobs. This preserves history for the frontend (which
// would otherwise see a 404) and lets deleteCompletedBatches remove them
// after the normal retention period.
func (p *BatchCleanupProcess) cancelStaleBatches(ctx context.Context) (int, error) {
	cutoff := time.Now().UTC().Add(-p.staleTimeout)
	staleStatuses := []workqueue.BatchStatus{
		workqueue.BatchStatusPending,
		workqueue.BatchStatusProcessing,
		workqueue.BatchStatusRunning,
	}

	if p.cancelStale == nil {
		return 0, fmt.Errorf("no batch canceller configured; cannot cancel stale batches")
	}

	var batches []Batch
	if err := p.db.WithContext(ctx).Where("status IN ? AND created_at < ?", staleStatuses, cutoff).
		Find(&batches).Error; err != nil {
		return 0, fmt.Errorf("querying stale batches older than %v: %w", p.staleTimeout, err)
	}

	cancelled := 0
	for _, batch := range batches {
		if ctx.Err() != nil {
			return cancelled, fmt.Errorf("stale batch cleanup interrupted: %w", ctx.Err())
		}
		if err := p.cancelStale(ctx, batch.ID); err != nil {
			log.WithError(err).WithField("batchID", batch.ID).
				Error("batch cleanup: failed to cancel stale batch")
			continue
		}
		cancelled++
	}
	return cancelled, nil
}
