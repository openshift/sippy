package workqueue

import (
	"time"

	"github.com/google/uuid"
)

// BatchStatus represents the lifecycle state of a batch.
type BatchStatus string

const (
	BatchStatusPending  BatchStatus = "pending"
	BatchStatusRunning  BatchStatus = "running"
	BatchStatusComplete BatchStatus = "complete"
	BatchStatusFailed   BatchStatus = "failed"
)

// Batch represents a user-initiated batch of work items.
// A batch groups related work items for status tracking and progress reporting.
type Batch struct {
	ID             uuid.UUID   `gorm:"type:uuid;primaryKey"       json:"id"`
	Kind           string      `gorm:"not null;index"             json:"kind"`
	RequestedCount int         `gorm:"not null"                   json:"requested_count"`
	EnqueuedCount  int         `gorm:"not null"                   json:"enqueued_count"`
	DedupedCount   int         `gorm:"not null"                   json:"deduped_count"`
	Status         BatchStatus `gorm:"not null;default:'pending'" json:"status"`
	CreatedAt      time.Time   `gorm:"autoCreateTime"             json:"created_at"`
	CompletedAt    *time.Time  `                                  json:"completed_at,omitempty"`
}

func (Batch) TableName() string {
	return "workqueue_batches"
}

// BatchItem associates a batch with a River job for many-to-many status tracking.
// Multiple batches can reference the same River job (when deduplication occurs).
type BatchItem struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"                 json:"id"`
	BatchID    uuid.UUID `gorm:"type:uuid;not null;index:idx_batch_items" json:"batch_id"`
	RiverJobID int64     `gorm:"not null;index:idx_batch_items"           json:"river_job_id"`
	ItemKey    string    `gorm:"not null"                                 json:"item_key"`
}

func (BatchItem) TableName() string {
	return "workqueue_batch_items"
}
