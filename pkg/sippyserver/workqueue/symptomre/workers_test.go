package symptomre

import (
	"context"
	"fmt"
	"testing"

	apijobrunscan "github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReevaluateWorker(t *testing.T) {
	t.Run("delegates to ReevaluateFunc with correct args", func(t *testing.T) {
		var calledID string
		var calledDryRun bool
		worker := &ReevaluateWorker{
			Reevaluate: func(_ context.Context, prowJobBuildID string, dryRun bool) (*apijobrunscan.ReEvaluationResult, error) {
				calledID = prowJobBuildID
				calledDryRun = dryRun
				return nil, nil
			},
		}

		job := &river.Job[ReevaluateJobRunArgs]{
			Args: ReevaluateJobRunArgs{
				ProwJobBuildID: "test-build-42",
				DryRun:         true,
			},
		}
		err := worker.Work(context.Background(), job)
		require.NoError(t, err, "Work should succeed when ReevaluateFunc returns nil")
		assert.Equal(t, "test-build-42", calledID,
			"prowJobBuildID should be forwarded to ReevaluateFunc")
		assert.True(t, calledDryRun,
			"dryRun flag should be forwarded to ReevaluateFunc")
	})

	t.Run("propagates errors from ReevaluateFunc", func(t *testing.T) {
		worker := &ReevaluateWorker{
			Reevaluate: func(_ context.Context, _ string, _ bool) (*apijobrunscan.ReEvaluationResult, error) {
				return nil, fmt.Errorf("simulated evaluation failure")
			},
		}

		job := &river.Job[ReevaluateJobRunArgs]{
			Args: ReevaluateJobRunArgs{ProwJobBuildID: "fail-build"},
		}
		err := worker.Work(context.Background(), job)
		assert.Error(t, err, "Work should propagate ReevaluateFunc errors for River retry")
		assert.Contains(t, err.Error(), "simulated evaluation failure",
			"error message should come from ReevaluateFunc")
	})
}
