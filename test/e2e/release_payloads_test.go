package e2e

import (
	"testing"

	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleaseTagsAPI(t *testing.T) {
	var tags []models.ReleaseTag
	err := util.SippyGet("/api/releases/tags?release="+util.Release, &tags)
	require.NoError(t, err, "error fetching release tags")
	require.Greater(t, len(tags), 0, "no release tags returned")

	var hasAccepted, hasRejected bool
	for _, tag := range tags {
		assert.Equal(t, util.Release, tag.Release, "tag should be for the requested release")
		assert.NotEmpty(t, tag.ReleaseTag, "tag name should not be empty")
		assert.NotEmpty(t, tag.Stream, "stream should not be empty")
		assert.NotEmpty(t, tag.Architecture, "architecture should not be empty")
		if tag.Phase == "Accepted" {
			hasAccepted = true
		}
		if tag.Phase == "Rejected" {
			hasRejected = true
		}
	}
	assert.True(t, hasAccepted, "should have at least one Accepted tag")
	assert.True(t, hasRejected, "should have at least one Rejected tag")
}

func TestReleaseJobRunsAPI(t *testing.T) {
	var jobRuns []models.ReleaseJobRun
	err := util.SippyGet("/api/releases/job_runs?release="+util.Release, &jobRuns)
	require.NoError(t, err, "error fetching release job runs")
	require.Greater(t, len(jobRuns), 0, "no release job runs returned")

	for _, jr := range jobRuns {
		assert.NotEmpty(t, jr.JobName, "job name should not be empty")
		assert.NotEmpty(t, jr.State, "state should not be empty")
		assert.NotEmpty(t, jr.Kind, "kind should not be empty")
	}
}

func TestReleasePullRequestsAPI(t *testing.T) {
	var pullRequests []models.ReleasePullRequest
	err := util.SippyGet("/api/releases/pull_requests?release="+util.Release, &pullRequests)
	require.NoError(t, err, "error fetching release pull requests")
	require.Greater(t, len(pullRequests), 0, "no release pull requests returned")

	for _, pr := range pullRequests {
		assert.NotEmpty(t, pr.URL, "PR URL should not be empty")
		assert.NotEmpty(t, pr.Name, "PR name should not be empty")
	}
}
