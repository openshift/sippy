// Package symptomre implements the asynchronous symptom re-evaluation
// workflow. It defines domain-specific database models, batch submission,
// status querying, and River job types for re-evaluating job run symptoms
// via a work queue.
package symptomre

import (
	"time"

	"github.com/google/uuid"
	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
	"github.com/riverqueue/river/rivertype"
)

// Batch represents a user-initiated batch of symptom re-evaluation work.
// The API creates batches; the daemon processes them via River jobs.
type Batch struct {
	ID             uuid.UUID             `gorm:"type:uuid;primaryKey"          json:"id"`
	RequestedCount int                   `gorm:"not null"                      json:"requested_count"`
	EnqueuedCount  int                   `gorm:"not null"                      json:"enqueued_count"`
	DedupedCount   int                   `gorm:"not null"                      json:"deduped_count"`
	DryRun         bool                  `gorm:"not null;default:false"        json:"dry_run"`
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
	ItemKey string `gorm:"column:item_key" json:"item_key"`
	State   string `gorm:"column:state"    json:"state"`
}

// ItemState represents the resolved state of a batch item. Most values come
// directly from River's job state enum; the two synthetic states cover items
// that have not yet been enqueued or whose River job row is missing.
type ItemState = string

const (
	// Synthetic states (not in River's enum):

	// ItemStateNotEnqueued means the batch item has no river_job_id yet
	// (the batch fan-out hasn't reached it).
	ItemStateNotEnqueued ItemState = "not_enqueued"
	// ItemStateOrphaned means the batch item references a river_job_id that
	// no longer exists (e.g. cleaned up by River's job cleaner).
	ItemStateOrphaned ItemState = "orphaned"

	// River-native states, re-exported for use in the status switch so
	// callers don't need to import rivertype directly:

	ItemStateAvailable ItemState = string(rivertype.JobStateAvailable)
	ItemStateCancelled ItemState = string(rivertype.JobStateCancelled)
	ItemStateCompleted ItemState = string(rivertype.JobStateCompleted)
	ItemStateDiscarded ItemState = string(rivertype.JobStateDiscarded)
	ItemStatePending   ItemState = string(rivertype.JobStatePending)
	ItemStateRetryable ItemState = string(rivertype.JobStateRetryable)
	ItemStateRunning   ItemState = string(rivertype.JobStateRunning)
	ItemStateScheduled ItemState = string(rivertype.JobStateScheduled)
)
