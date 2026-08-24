package symptomre

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

// SubmitResult is returned by Submitter.Submit with the batch ID and
// requested count so the caller can return it to the user.
type SubmitResult struct {
	BatchID   uuid.UUID `json:"batch_id"`
	Requested int       `json:"requested"`
}

// Submitter creates batch specifications for symptom re-evaluation. It writes
// the batch and item rows via GORM and enqueues a ProcessBatchArgs River job
// so the daemon discovers the new batch.
type Submitter struct {
	gormDB      *gorm.DB
	riverClient *river.Client[pgx.Tx]
}

// NewSubmitter creates a Submitter with the given GORM DB and River client.
// The River client may be insert-only (API server) since Submitter only
// inserts jobs, never processes them.
func NewSubmitter(gormDB *gorm.DB, riverClient *river.Client[pgx.Tx]) *Submitter {
	return &Submitter{
		gormDB:      gormDB,
		riverClient: riverClient,
	}
}

// Submit creates a batch specification and enqueues a River job for daemon
// processing. The batch and item rows are written via GORM (pgx/v4); the
// River job is inserted via the River client (pgx/v5). These are separate
// transactions: if the GORM write succeeds but the River insert fails, the
// batch remains in "pending" status until resubmitted or cleaned up.
func (s *Submitter) Submit(ctx context.Context, prowJobBuildIDs []string) (*SubmitResult, error) {
	batchID := uuid.New()
	batch := Batch{
		ID:             batchID,
		RequestedCount: len(prowJobBuildIDs),
		Status:         workqueue.BatchStatusPending,
	}

	items := make([]BatchItem, len(prowJobBuildIDs))
	for i, id := range prowJobBuildIDs {
		items[i] = BatchItem{
			BatchID: batchID,
			ItemKey: id,
		}
	}

	if err := s.gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&batch).Error; err != nil {
			return fmt.Errorf("creating batch: %w", err)
		}
		if err := tx.Create(&items).Error; err != nil {
			return fmt.Errorf("creating batch items: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("submitting batch: %w", err)
	}

	if _, err := s.riverClient.Insert(ctx, ProcessBatchArgs{BatchID: batchID}, nil); err != nil {
		return nil, fmt.Errorf("enqueuing process-batch job for batch %s: %w", batchID, err)
	}

	return &SubmitResult{
		BatchID:   batchID,
		Requested: len(prowJobBuildIDs),
	}, nil
}
