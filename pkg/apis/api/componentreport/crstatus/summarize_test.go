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
					TestKeyStr: "test-key",
					ProwJob:    "job-name",
					Count:      crtest.Count{TotalCount: 1, SuccessCount: 1},
					Lifecycle:  lc,
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
