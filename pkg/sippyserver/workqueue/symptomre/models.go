// Package symptomre implements the asynchronous symptom re-evaluation
// workflow. It defines domain-specific database models, batch submission,
// status querying, and River job types for re-evaluating job run symptoms
// via a work queue.
package symptomre

import (
	"time"

	"github.com/google/uuid"

	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

// Batch represents a user-initiated batch of symptom re-evaluation work.
// The API creates batches; the daemon processes them via River jobs.
type Batch struct {
	ID             uuid.UUID             `gorm:"type:uuid;primaryKey"          json:"id"`
	RequestedCount int                   `gorm:"not null"                      json:"requested_count"`
	EnqueuedCount  int                   `gorm:"not null"                      json:"enqueued_count"`
	DedupedCount   int                   `gorm:"not null"                      json:"deduped_count"`
	Status         workqueue.BatchStatus `gorm:"not null;default:'pending'"    json:"status"`
	CreatedAt      time.Time             `gorm:"autoCreateTime"                json:"created_at"`
	CompletedAt    *time.Time            `                                     json:"completed_at,omitempty"`
}

// TableName returns the PostgreSQL table name for Batch.
func (Batch) TableName() string { return "workqueue_symptom_re_batches" }

// BatchItem associates a batch with an individual job run to re-evaluate.
// Before the daemon processes the batch, RiverJobID is nil (the item is a
// specification). After processing, RiverJobID is populated with the River
// job that performs the work.
type BatchItem struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"                            json:"id"`
	BatchID    uuid.UUID `gorm:"type:uuid;not null;index:idx_symptom_re_batch_items" json:"batch_id"`
	RiverJobID *int64    `gorm:"index:idx_symptom_re_batch_items"                    json:"river_job_id"`
	ItemKey    string    `gorm:"not null"                                            json:"item_key"`
}

// TableName returns the PostgreSQL table name for BatchItem.
func (BatchItem) TableName() string { return "workqueue_symptom_re_batch_items" }
