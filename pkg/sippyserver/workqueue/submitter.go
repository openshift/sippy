package workqueue

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// SubmitResult contains the outcome of a batch submission.
type SubmitResult struct {
	BatchID   uuid.UUID `json:"batch_id"`
	Requested int       `json:"requested"`
	Enqueued  int       `json:"enqueued"`
	Deduped   int       `json:"deduped"`
}

// Submitter creates batches and enqueues River jobs for async processing.
type Submitter struct {
	db          *gorm.DB
	riverClient *river.Client[*pgxpool.Pool]
}

// NewSubmitter creates a Submitter.
func NewSubmitter(db *gorm.DB, riverClient *river.Client[*pgxpool.Pool]) *Submitter {
	return &Submitter{
		db:          db,
		riverClient: riverClient,
	}
}

// Submit creates a batch, enqueues River jobs for each item, and records
// batch-item associations. Uses two separate transactions (gorm for batch
// tracking, pgx/v5 for River jobs) because the two driver versions cannot
// share a transaction.
func (s *Submitter) Submit(ctx context.Context, kind string, items []river.InsertManyParams, itemKeys []string) (*SubmitResult, error) {
	if len(items) != len(itemKeys) {
		return nil, fmt.Errorf("items and itemKeys must have the same length")
	}

	batchID := uuid.New()
	batch := Batch{
		ID:             batchID,
		Kind:           kind,
		RequestedCount: len(items),
		Status:         BatchStatusPending,
	}

	// Insert River jobs first. If this succeeds but the batch tracking fails,
	// the jobs still run (harmless) and the user can submit another batch.
	results, err := s.riverClient.InsertMany(ctx, items)
	if err != nil {
		return nil, fmt.Errorf("inserting River jobs: %w", err)
	}

	enqueued, deduped := countInsertResults(results)

	// Now create the batch and batch-item rows in gorm.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		batch.EnqueuedCount = enqueued
		batch.DedupedCount = deduped
		batch.Status = BatchStatusRunning

		if err := tx.Create(&batch).Error; err != nil {
			return fmt.Errorf("creating batch: %w", err)
		}

		batchItems := buildBatchItems(batchID, results, itemKeys)
		if err := tx.CreateInBatches(batchItems, 500).Error; err != nil {
			return fmt.Errorf("creating batch items: %w", err)
		}

		return nil
	})
	if err != nil {
		log.WithError(err).WithField("batch_id", batchID).
			Warn("River jobs were enqueued but batch tracking failed")
		return nil, fmt.Errorf("recording batch: %w", err)
	}

	return &SubmitResult{
		BatchID:   batchID,
		Requested: len(items),
		Enqueued:  enqueued,
		Deduped:   deduped,
	}, nil
}

func countInsertResults(results []*rivertype.JobInsertResult) (enqueued, deduped int) {
	for _, r := range results {
		if r.UniqueSkippedAsDuplicate {
			deduped++
		} else {
			enqueued++
		}
	}
	return
}

func buildBatchItems(batchID uuid.UUID, results []*rivertype.JobInsertResult, itemKeys []string) []BatchItem {
	items := make([]BatchItem, len(results))
	for i, r := range results {
		items[i] = BatchItem{
			BatchID:    batchID,
			RiverJobID: r.Job.ID,
			ItemKey:    itemKeys[i],
		}
	}
	return items
}
