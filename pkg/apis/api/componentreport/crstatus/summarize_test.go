package crstatus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
)

func TestSummarizeTestJobRuns_LifecyclePromotion(t *testing.T) {
	tests := []struct {
		name              string
		lifecycles        []string
		expectedLifecycle string
	}{
		{
			name:              "all blocking stays blocking",
			lifecycles:        []string{"blocking", "blocking", "blocking"},
			expectedLifecycle: "blocking",
		},
		{
			name:              "blocking then informing promotes to informing",
			lifecycles:        []string{"blocking", "informing", "blocking"},
			expectedLifecycle: "informing",
		},
		{
			name:              "informing first stays informing",
			lifecycles:        []string{"informing", "blocking", "blocking"},
			expectedLifecycle: "informing",
		},
		{
			name:              "single informing promotes to informing",
			lifecycles:        []string{"blocking", "blocking", "informing"},
			expectedLifecycle: "informing",
		},
		{
			name:              "all informing stays informing",
			lifecycles:        []string{"informing", "informing"},
			expectedLifecycle: "informing",
		},
		{
			name:              "empty lifecycle ignored when non-empty exists",
			lifecycles:        []string{"", "blocking", ""},
			expectedLifecycle: "blocking",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rows []TestJobRunRows
			for _, lc := range tc.lifecycles {
				rows = append(rows, TestJobRunRows{
					TestIdentification:   TestIdentification{TestKeyStr: "test-key"},
					JobRunIdentification: JobRunIdentification{ProwJob: "job-name"},
					TestJobRunCounts:     TestJobRunCounts{Count: crtest.Count{TotalCount: 1, SuccessCount: 1}},
					TestJobRunMetadata:   TestJobRunMetadata{Lifecycle: lc},
				})
			}

			result := SummarizeTestJobRuns(map[string][]TestJobRunRows{
				"job-name": rows,
			})

			require.Len(t, result["job-name"], 1)
			assert.Equal(t, tc.expectedLifecycle, result["job-name"][0].Lifecycle)
		})
	}
}

func TestSummarizeTestJobRuns_UsesExclusiveStatsForProjectedFlakes(t *testing.T) {
	statsCount := crtest.Count{TotalCount: 1, FlakeCount: 1}
	result := SummarizeTestJobRuns(map[string][]TestJobRunRows{
		"job-name": {
			{
				TestIdentification: TestIdentification{TestKeyStr: "test-key"},
				JobRunIdentification: JobRunIdentification{
					ProwJob:      "job-name",
					ProwJobRunID: "run-1",
				},
				TestJobRunCounts: TestJobRunCounts{
					Count:      crtest.Count{TotalCount: 1, SuccessCount: 1, FlakeCount: 1},
					StatsCount: &statsCount,
				},
			},
		},
	})

	require.Len(t, result["job-name"], 1)
	summary := result["job-name"][0]
	assert.Equal(t, crtest.NewTestStats(0, 0, 1, false), summary.Stats)
	require.Len(t, summary.JobRuns, 1)
	assert.Equal(t, crtest.Count{TotalCount: 1, SuccessCount: 1, FlakeCount: 1}, summary.JobRuns[0].Count)
}

func TestSummarizeTestJobRuns_KeepsStoredJobIdentityWithinNormalizedBucket(t *testing.T) {
	result := SummarizeTestJobRuns(map[string][]TestJobRunRows{
		"periodic-ga-frequency-aws": {
			{
				TestIdentification: TestIdentification{TestKeyStr: "test-key"},
				JobRunIdentification: JobRunIdentification{
					ProwJob:      "periodic-ga-frequency-f24-aws",
					ProwJobRunID: "f24-run",
				},
				TestJobRunCounts: TestJobRunCounts{Count: crtest.Count{TotalCount: 1, SuccessCount: 1}},
			},
			{
				TestIdentification: TestIdentification{TestKeyStr: "test-key"},
				JobRunIdentification: JobRunIdentification{
					ProwJob:      "periodic-ga-frequency-f48-aws",
					ProwJobRunID: "f48-run",
				},
				TestJobRunCounts: TestJobRunCounts{Count: crtest.Count{TotalCount: 1, SuccessCount: 1}},
			},
		},
	})

	summaries := result["periodic-ga-frequency-aws"]
	require.Len(t, summaries, 2)
	assert.Equal(t, "periodic-ga-frequency-f24-aws", summaries[0].ProwJob)
	assert.Equal(t, "f24-run", summaries[0].JobRuns[0].ProwJobRunID)
	assert.Equal(t, "periodic-ga-frequency-f48-aws", summaries[1].ProwJob)
	assert.Equal(t, "f48-run", summaries[1].JobRuns[0].ProwJobRunID)
}
