// Package workqueue provides generic infrastructure for River-based
// asynchronous work queues. Domain-specific workloads live in subpackages
// (e.g. symptomre); this package holds shared types and helpers.
package workqueue

// BatchStatus represents the lifecycle state of a batch of work items.
type BatchStatus string

const (
	BatchStatusPending    BatchStatus = "pending"
	BatchStatusProcessing BatchStatus = "processing"
	BatchStatusRunning    BatchStatus = "running"
	BatchStatusComplete   BatchStatus = "complete"
	BatchStatusFailed     BatchStatus = "failed"
	BatchStatusCancelled  BatchStatus = "cancelled"
)
