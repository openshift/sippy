package prowloader

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/prow"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/stretchr/testify/require"
)

func TestBuildHistoricalTestResultPreservesBigQueryJobAnnotations(t *testing.T) {
	timestamp := time.Date(2026, time.May, 22, 14, 30, 0, 0, time.UTC)
	run := pgwriter.RunRow{ID: 123, ProwJobID: 7, ProwJobRelease: "4.22", Timestamp: timestamp}
	job := HistoricalJob{
		ProwJob: prow.ProwJob{
			Annotations: map[string]string{"releaseJobName": "canonical-job", "reason": "historical"},
			Status:      prow.ProwJobStatus{BuildID: "123", State: prow.SuccessState},
		},
		Run: run,
	}
	loader := &ProwLoader{syntheticTestManager: synthetictests.NewEmptySyntheticTestManager()}

	result, _, err := loader.BuildHistoricalTestResult(context.Background(), job, "bigquery", nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, run.ID, result.Run.ID)
	require.ElementsMatch(t, []pgwriter.AnnotationRow{
		{ProwJobRunID: 123, Key: "releaseJobName", Value: "canonical-job", ProwJobRunRelease: "4.22", ProwJobRunTimestamp: timestamp},
		{ProwJobRunID: 123, Key: "reason", Value: "historical", ProwJobRunRelease: "4.22", ProwJobRunTimestamp: timestamp},
	}, result.Annotations)
}
