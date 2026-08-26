package symptomre

import (
	"context"
	"fmt"
	"time"

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

// BatchCleanupProcess periodically removes old completed batches and stale
// non-terminal batches from the database. It implements the DaemonProcess
// interface so the daemon server can manage its lifecycle.
type BatchCleanupProcess struct {
	db                 *gorm.DB
	completedRetention time.Duration
	staleTimeout       time.Duration
}

// NewBatchCleanupProcess creates a cleanup process with the default retention
// periods for completed and stale batches.
func NewBatchCleanupProcess(db *gorm.DB) *BatchCleanupProcess {
	return &BatchCleanupProcess{
		db:                 db,
		completedRetention: CompletedBatchRetention,
		staleTimeout:       StaleBatchTimeout,
	}
}

// Run executes the periodic cleanup loop, deleting old batches every hour
// until the context is cancelled.
func (p *BatchCleanupProcess) Run(ctx context.Context) {
	log.Info("batch cleanup: starting periodic cleanup process")

	// Run once immediately at startup, then on a ticker.
	p.runCleanup()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("batch cleanup: shutting down")
			return
		case <-ticker.C:
			p.runCleanup()
		}
	}
}

// runCleanup performs a single cleanup pass, removing completed batches older
// than the retention period and stale non-terminal batches.
func (p *BatchCleanupProcess) runCleanup() {
	deleted, err := p.deleteCompletedBatches()
	if err != nil {
		log.WithError(err).Error("batch cleanup: failed to delete completed batches")
	} else if deleted > 0 {
		log.WithField("count", deleted).Info("batch cleanup: deleted old completed batches")
	}

	deleted, err = p.deleteStaleBatches()
	if err != nil {
		log.WithError(err).Error("batch cleanup: failed to delete stale batches")
	} else if deleted > 0 {
		log.WithField("count", deleted).Info("batch cleanup: deleted stale non-terminal batches")
	}
}

// deleteCompletedBatches removes batches that have a non-null completed_at
// timestamp older than the configured retention period.
func (p *BatchCleanupProcess) deleteCompletedBatches() (int64, error) {
	cutoff := time.Now().UTC().Add(-p.completedRetention)
	result := p.db.
		Where("completed_at IS NOT NULL AND completed_at < ?", cutoff).
		Delete(&Batch{})
	if result.Error != nil {
		return 0, fmt.Errorf("deleting completed batches older than %v: %w", p.completedRetention, result.Error)
	}
	return result.RowsAffected, nil
}

// deleteStaleBatches removes batches stuck in non-terminal statuses
// (pending, processing, running) that were created more than the stale
// timeout ago, indicating they are likely abandoned.
func (p *BatchCleanupProcess) deleteStaleBatches() (int64, error) {
	cutoff := time.Now().UTC().Add(-p.staleTimeout)
	staleStatuses := []workqueue.BatchStatus{
		workqueue.BatchStatusPending,
		workqueue.BatchStatusProcessing,
		workqueue.BatchStatusRunning,
	}
	result := p.db.
		Where("status IN ? AND created_at < ?", staleStatuses, cutoff).
		Delete(&Batch{})
	if result.Error != nil {
		return 0, fmt.Errorf("deleting stale batches older than %v: %w", p.staleTimeout, result.Error)
	}
	return result.RowsAffected, nil
}
