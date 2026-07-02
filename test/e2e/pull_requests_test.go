package e2e

import (
	"testing"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prTestResultsPath(extra string) string {
	return "/api/pull_requests/test_results?org=openshift&repo=origin&pr_number=99001" + extra
}

func TestPRTestResultsDefaultFailuresOnly(t *testing.T) {
	var results []api.PRTestResult
	err := util.SippyGet(prTestResultsPath(""), &results)
	require.NoError(t, err, "error making http request")
	require.Greater(t, len(results), 0, "expected at least one result")

	for _, r := range results {
		assert.Equal(t, "failure", r.Status, "default query should only return failures, got %s for test %s", r.Status, r.TestName)
	}
}

func TestPRTestResultsIncludeSuccesses(t *testing.T) {
	var results []api.PRTestResult
	err := util.SippyGet(prTestResultsPath("&include_successes=install"), &results)
	require.NoError(t, err, "error making http request")
	require.Greater(t, len(results), 0, "expected at least one result")

	statuses := map[string]int{}
	for _, r := range results {
		statuses[r.Status]++
	}

	assert.Greater(t, statuses["failure"], 0, "should still have failures")
	// include_successes=install should match "install should succeed: overall" and return
	// success and flake statuses for it alongside the failures
	assert.Greater(t, statuses["success"]+statuses["flake"], 0, "should have successes or flakes for matching tests")
}

func TestPRTestResultsMultiplePRs(t *testing.T) {
	var results []api.PRTestResult
	err := util.SippyGet("/api/pull_requests/test_results?org=openshift&repo=origin&pr_number=99002", &results)
	require.NoError(t, err, "error making http request")
	require.Greater(t, len(results), 0, "expected results for PR 99002")

	for _, r := range results {
		assert.Contains(t, r.ProwJobName, "gcp", "PR 99002 should be linked to GCP job, got %s", r.ProwJobName)
	}
}

func TestPRTestResultsSHAFilter(t *testing.T) {
	// SHA "abc123def456" is the SHA for PR 99001 in seed data
	var results []api.PRTestResult
	err := util.SippyGet(prTestResultsPath("&sha=abc123def456"), &results)
	require.NoError(t, err, "error making http request")
	require.Greater(t, len(results), 0, "expected results for matching SHA")

	for _, r := range results {
		assert.Equal(t, "abc123def456", r.PRSha, "all results should have the filtered SHA")
	}

	// Non-existent SHA should return empty
	var empty []api.PRTestResult
	err = util.SippyGet(prTestResultsPath("&sha=nonexistent"), &empty)
	require.NoError(t, err, "error making http request")
	assert.Empty(t, empty, "expected empty results for non-matching SHA")
}

func TestPRTestResultsDefaultDateRange(t *testing.T) {
	// Query without start_date/end_date should work (defaults to last 14 days)
	var results []api.PRTestResult
	err := util.SippyGet(prTestResultsPath(""), &results)
	require.NoError(t, err, "error making http request without date params")
	assert.Greater(t, len(results), 0, "expected results with default date range")
}

func TestPRTestResultsMissingParams(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"missing repo", "/api/pull_requests/test_results?org=openshift&pr_number=1"},
		{"missing pr_number", "/api/pull_requests/test_results?org=openshift&repo=origin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result any
			err := util.SippyGet(tt.path, &result)
			assert.Error(t, err, "expected error for %s", tt.name)
			assert.Contains(t, err.Error(), "400", "expected 400 status code")
		})
	}
}

func TestPRTestResultsEmptyResults(t *testing.T) {
	var results []api.PRTestResult
	err := util.SippyGet("/api/pull_requests/test_results?org=openshift&repo=origin&pr_number=99999", &results)
	require.NoError(t, err, "error making http request")
	assert.Empty(t, results, "expected empty results for non-existent PR")
}
