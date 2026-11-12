package crstatus

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTestJobRunRowsJSONCompatibility protects the detail-row API payload while
// its internal fields are grouped into embedded types.
func TestTestJobRunRowsJSONCompatibility(t *testing.T) {
	row := TestJobRunRows{
		TestIdentification: TestIdentification{
			TestKey: crtest.KeyWithVariants{
				TestID:   "test-id",
				Variants: map[string]string{"platform": "aws"},
			},
			TestKeyStr: "test-id\x00platform:aws",
			TestName:   "test name",
		},
		JobRunIdentification: JobRunIdentification{
			ProwJob:      "periodic-job",
			ProwJobRunID: "run-123",
			ProwJobURL:   "https://prow.example.test/view/gs/run-123",
			StartTime:    time.Date(2026, time.November, 25, 15, 4, 5, 0, time.UTC),
		},
		TestJobRunCounts: TestJobRunCounts{
			Count:      crtest.Count{TotalCount: 1, SuccessCount: 1},
			StatsCount: &crtest.Count{TotalCount: 1, FlakeCount: 1},
		},
		TestJobRunMetadata: TestJobRunMetadata{
			JiraComponent:   "Cluster Version Operator",
			JiraComponentID: big.NewRat(9, 2),
			JobLabels:       []string{"periodic", "aws"},
			TestFailures:    4,
			Lifecycle:       "blocking",
		},
	}

	actual, err := json.Marshal(row)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"test_key":{"test_id":"test-id","variants":{"platform":"aws"}},
		"TestName":"test name",
		"ProwJob":"periodic-job",
		"ProwJobRunID":"run-123",
		"ProwJobURL":"https://prow.example.test/view/gs/run-123",
		"StartTime":"2026-11-25T15:04:05Z",
		"total_count":1,
		"success_count":1,
		"flake_count":0,
		"JiraComponent":"Cluster Version Operator",
		"JiraComponentID":"9/2",
		"job_labels":["periodic","aws"],
		"test_failures":4,
		"Lifecycle":"blocking"
	}`, string(actual))
}
