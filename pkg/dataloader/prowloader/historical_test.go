package prowloader

import (
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/prow"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/bqjunit"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/stretchr/testify/require"
)

func TestSortProwJobsDescending(t *testing.T) {
	start := time.Date(2026, time.May, 22, 0, 0, 0, 0, time.UTC)
	jobs := []prow.ProwJob{
		{Status: prow.ProwJobStatus{BuildID: "middle", StartTime: start.Add(time.Hour)}},
		{Status: prow.ProwJobStatus{BuildID: "oldest", StartTime: start}},
		{Status: prow.ProwJobStatus{BuildID: "newest", StartTime: start.Add(2 * time.Hour)}},
	}
	sortProwJobsDescending(jobs)
	require.Equal(t, []string{"newest", "middle", "oldest"}, []string{jobs[0].Status.BuildID, jobs[1].Status.BuildID, jobs[2].Status.BuildID})
}

func TestEnsureHistoricalPartitionsRejectsInvalidInputsBeforeDatabaseAccess(t *testing.T) {
	start := time.Date(2026, time.May, 22, 0, 0, 0, 0, time.UTC)
	loader := &ProwLoader{}
	require.ErrorContains(t, loader.EnsureHistoricalPartitions(start, start), "start must be before end")
	require.ErrorContains(t, loader.EnsureHistoricalPartitions(start, start.AddDate(0, 0, 1)), "no configured releases")
}

func TestBuildHistoricalBigQueryResultPreservesJobAnnotations(t *testing.T) {
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

	result, err := loader.BuildHistoricalBigQueryResult(job, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, run.ID, result.Run.ID)
	require.ElementsMatch(t, []pgwriter.AnnotationRow{
		{ProwJobRunID: 123, Key: "releaseJobName", Value: "canonical-job", ProwJobRunRelease: "4.22", ProwJobRunTimestamp: timestamp},
		{ProwJobRunID: 123, Key: "reason", Value: "historical", ProwJobRunRelease: "4.22", ProwJobRunTimestamp: timestamp},
	}, result.Annotations)
}

func TestBuildHistoricalBigQueryResultPreservesJUnitOutput(t *testing.T) {
	run := pgwriter.RunRow{ID: 123, ProwJobID: 7, ProwJobRelease: "4.22", Timestamp: time.Date(2026, time.May, 22, 14, 30, 0, 0, time.UTC)}
	job := HistoricalJob{
		ProwJob: prow.ProwJob{Status: prow.ProwJobStatus{BuildID: "123", State: prow.FailureState}},
		Run:     run,
	}
	loader := &ProwLoader{syntheticTestManager: synthetictests.NewEmptySyntheticTestManager()}

	result, err := loader.BuildHistoricalBigQueryResult(job, []bqjunit.TestRow{{
		BuildID: "123", TestName: "historical test", Suite: "suite", FailureContent: "failure details", Duration: 1.5,
	}})
	require.NoError(t, err)
	for _, test := range result.Tests {
		if test.TestName != "historical test" {
			continue
		}
		require.Equal(t, "suite", test.SuiteName)
		require.Equal(t, 1.5, test.Duration)
		require.NotNil(t, test.Output)
		require.Equal(t, "failure details", *test.Output)
		return
	}
	t.Fatal("historical JUnit row was not converted")
}

func TestHistoricalBQTestsMergesDuplicateFlakyRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  bqjunit.TestRow
		second bqjunit.TestRow
	}{
		{name: "success then explicit flake", first: bqjunit.TestRow{Success: 1}, second: bqjunit.TestRow{FlakeCount: 1}},
		{name: "failure then explicit flake", first: bqjunit.TestRow{}, second: bqjunit.TestRow{FlakeCount: 1}},
		{name: "explicit flake then success", first: bqjunit.TestRow{FlakeCount: 1}, second: bqjunit.TestRow{Success: 1}},
		{name: "explicit flake then failure", first: bqjunit.TestRow{FlakeCount: 1}, second: bqjunit.TestRow{}},
		{name: "success then failure", first: bqjunit.TestRow{Success: 1}, second: bqjunit.TestRow{}},
		{name: "failure then success", first: bqjunit.TestRow{}, second: bqjunit.TestRow{Success: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := &prow.ProwJob{Status: prow.ProwJobStatus{BuildID: "123", State: prow.SuccessState}}
			loader := &ProwLoader{syntheticTestManager: synthetictests.NewEmptySyntheticTestManager()}
			first, second := tc.first, tc.second
			first.BuildID, second.BuildID = "123", "123"
			first.TestName, second.TestName = "duplicate test", "duplicate test"
			first.Suite, second.Suite = "suite", "suite"

			tests, failures, flakes, _ := loader.historicalBQTests(job, pgwriter.RunRow{}, []bqjunit.TestRow{first, second})
			require.Zero(t, failures)
			require.Equal(t, 1, flakes)
			var matches []pgwriter.TestRow
			for _, test := range tests {
				if test.TestName == "duplicate test" && test.SuiteName == "suite" {
					matches = append(matches, test)
				}
			}
			require.Len(t, matches, 1)
			require.Equal(t, int(sippyprocessingv1.TestStatusFlake), matches[0].Status)
		})
	}
}
