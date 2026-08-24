package symptomre

import (
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

const (
	// BatchQueue is the River queue for batch fan-out jobs. Using a separate
	// queue prevents a backlog of individual re-evaluation items from blocking
	// prompt processing of new batches.
	BatchQueue = "symptom_re_batch"

	// ItemQueue is the River queue for individual job run re-evaluation jobs.
	ItemQueue = "symptom_re_item"

	// DedupPeriod prevents re-evaluating the same job run within a two-hour
	// window, reducing the chance of BigQuery streaming buffer conflicts
	// (rows are not deletable within 90 minutes of insertion).
	DedupPeriod = 120 * time.Minute

	// MaxAttemptsPerItem is the number of times an individual re-evaluation
	// job is attempted before being discarded as a permanent failure.
	MaxAttemptsPerItem = 3

	// MaxJobRunsPerBatch is the maximum number of job runs that can be
	// submitted in a single batch request.
	MaxJobRunsPerBatch = 10000
)

// ProcessBatchArgs is a River job that the API enqueues when a new batch is
// created. The daemon picks this up, refreshes the symptom cache, and fans
// out into individual ReevaluateJobRunArgs jobs.
type ProcessBatchArgs struct {
	BatchID uuid.UUID `json:"batch_id"`
}

// Kind returns the River job kind identifier.
func (ProcessBatchArgs) Kind() string { return "symptom_re.process_batch" }

// InsertOpts configures River insertion behavior. Batch processing is not
// retried automatically; individual items have their own retry policy.
func (ProcessBatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       BatchQueue,
		MaxAttempts: 1,
	}
}

// ReevaluateJobRunArgs is a River job for re-evaluating a single job run's
// symptoms. Both ProwJobBuildID and SymptomHash participate in River's
// uniqueness check so that the same job run is re-evaluated if symptoms
// change, but duplicate requests within a DedupPeriod are skipped.
type ReevaluateJobRunArgs struct {
	ProwJobBuildID string `json:"prow_job_build_id" river:"unique"`
	SymptomHash    string `json:"symptom_hash"      river:"unique"`
}

// Kind returns the River job kind identifier.
func (ReevaluateJobRunArgs) Kind() string { return "symptom_re.reevaluate_job_run" }

// InsertOpts configures River insertion behavior with deduplication scoped to
// the combination of job run identity and symptom state hash.
func (ReevaluateJobRunArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       ItemQueue,
		MaxAttempts: MaxAttemptsPerItem,
		UniqueOpts: river.UniqueOpts{
			ByArgs:   true,
			ByPeriod: DedupPeriod,
		},
	}
}
