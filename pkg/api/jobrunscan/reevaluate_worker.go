package jobrunscan

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

const (
	ReevaluateJobKind     = "reevaluate_job_run"
	ReevaluateQueue       = "reevaluate"
	ReevaluateDedupPeriod = 90 * time.Minute
	ReevaluateMaxAttempts = 3
	MaxJobRunsPerBatch    = 10000
)

// ReevaluateJobRunArgs are the arguments for a single re-evaluation River job.
// Both ProwJobBuildID and SymptomHash participate in uniqueness, so the same
// job run will be re-evaluated if the symptom set changes.
type ReevaluateJobRunArgs struct {
	ProwJobBuildID string `json:"prow_job_build_id" river:"unique"`
	SymptomHash    string `json:"symptom_hash"      river:"unique"`
}

func (ReevaluateJobRunArgs) Kind() string { return ReevaluateJobKind }

func (ReevaluateJobRunArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       ReevaluateQueue,
		MaxAttempts: ReevaluateMaxAttempts,
		UniqueOpts: river.UniqueOpts{
			ByArgs:   true,
			ByPeriod: ReevaluateDedupPeriod,
		},
	}
}

// ReevaluateWorker processes a single re-evaluation job using the cached
// symptoms from the ReEvaluator.
type ReevaluateWorker struct {
	river.WorkerDefaults[ReevaluateJobRunArgs]
	evaluator *ReEvaluator
}

// NewReevaluateWorker creates a worker that delegates to the given ReEvaluator.
func NewReevaluateWorker(evaluator *ReEvaluator) *ReevaluateWorker {
	return &ReevaluateWorker{evaluator: evaluator}
}

func (w *ReevaluateWorker) Work(ctx context.Context, job *river.Job[ReevaluateJobRunArgs]) error {
	symptoms := w.evaluator.CachedSymptoms()
	if symptoms == nil {
		return fmt.Errorf("symptom cache not initialized")
	}

	result := w.evaluator.reEvaluateOne(ctx, job.Args.ProwJobBuildID, symptoms)
	if result.Status != ReEvalSuccess {
		return fmt.Errorf("re-evaluation failed for %s: %s", job.Args.ProwJobBuildID, result.Error)
	}
	return nil
}

// BuildInsertParams creates River insert parameters for a batch of build IDs.
func BuildInsertParams(buildIDs []string, symptomHash string) ([]river.InsertManyParams, []string) {
	params := make([]river.InsertManyParams, len(buildIDs))
	keys := make([]string, len(buildIDs))
	for i, id := range buildIDs {
		params[i] = river.InsertManyParams{
			Args: ReevaluateJobRunArgs{
				ProwJobBuildID: id,
				SymptomHash:    symptomHash,
			},
		}
		keys[i] = id
	}
	return params, keys
}

// SymptomHash computes a stable hash of symptom definitions for dedup purposes.
func SymptomHash(symptoms []jobrunscan.Symptom) string {
	return computeSymptomHash(symptoms)
}
