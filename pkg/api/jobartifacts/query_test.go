package jobartifacts

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// convenience test setup method, glob and matcher are optional
func baseTestingJAQ(t *testing.T, pathGlob string, matcher ContentMatcher) *JobArtifactQuery {
	return &JobArtifactQuery{
		GcsBucket:      util.GetGcsBucket(t),
		DbClient:       util.GetDbHandle(t),
		Cache:          &util.PseudoCache{Cache: map[string][]byte{}},
		PathGlob:       pathGlob,
		ContentMatcher: matcher,
	}
}

func TestFunctional_JobRunPathFound(t *testing.T) {
	query := baseTestingJAQ(t, "", nil)
	jobRun, err := query.getJobRun(1898704060324777984)
	assert.NoError(t, err)
	assert.Equal(t, "logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/", jobRun.BucketPath)
	assert.Equal(t, "periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn", jobRun.JobName)
}

func TestFunctional_ListFiles(t *testing.T) {
	query := baseTestingJAQ(t, "", nil)
	files, truncated, err := query.getJobRunFiles("logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/")
	assert.NoError(t, err)
	assert.True(t, truncated, "expected a lot of files under the job")
	assert.Equal(t, maxJobFilesToScan, len(files), "expected to receive the max number of files")
}

func TestFunctional_FilterFiles(t *testing.T) {
	query := baseTestingJAQ(t, "artifacts/*e2e*/gather-extra/build-log.txt", nil)
	files, truncated, err := query.getJobRunFiles("logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/")
	assert.NoError(t, err)
	assert.False(t, truncated, "expected no need for truncating the file list")
	assert.Equal(t, 1, len(files), "expected glob to match one file")
}

func TestFunctional_FilterContent(t *testing.T) {
	const filePath = "logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/artifacts/e2e-azure-ovn/gather-extra/build-log.txt"

	query := baseTestingJAQ(t, "", NewStringMatcher("ClusterVersion:", 0, 0, maxFileMatches))
	artifact := query.getFileContentMatches(1898704060324777984, &storage.ObjectAttrs{Name: filePath})
	assert.Empty(t, artifact.Error)
	assert.False(t, artifact.MatchedContent.ContentLineMatches.Truncated, "expected no need for truncating the content matches")
	assert.Equal(t, 2, len(artifact.MatchedContent.ContentLineMatches.Matches), "expected content to match with two lines")

	query.ContentMatcher = NewStringMatcher("error:", 0, 0, maxFileMatches)
	artifact = query.getFileContentMatches(1898704060324777984, &storage.ObjectAttrs{Name: filePath})
	assert.Empty(t, artifact.Error)
	assert.True(t, artifact.MatchedContent.ContentLineMatches.Truncated, "expected to truncate content matches")
	assert.Equal(t, maxFileMatches, len(artifact.MatchedContent.ContentLineMatches.Matches), "expected content to match with many lines")
}

func TestFunctional_GzipContent(t *testing.T) {
	const filePath = "logs/periodic-ci-openshift-release-master-ci-4.19-e2e-aws-ovn-techpreview/1909930323508989952/artifacts/e2e-aws-ovn-techpreview/gather-extra/artifacts/nodes/ip-10-0-59-177.us-east-2.compute.internal/journal"

	query := baseTestingJAQ(t, "", NewStringMatcher("error", 0, 0, maxFileMatches))
	artifact := query.getFileContentMatches(1909930323508989952, &storage.ObjectAttrs{Name: filePath, ContentType: "application/gzip"})
	assert.Empty(t, artifact.Error)
	assert.True(t, artifact.MatchedContent.ContentLineMatches.Truncated, "expected a lot of matches")
	assert.Contains(t,
		artifact.MatchedContent.ContentLineMatches.Matches[0].Match,
		"localhost kernel: GPT: Use GNU Parted to correct GPT errors.",
		"expected to scan uncompressed text")
}

func TestFunctional_RequeryCachedTimedOutJobArtifacts(t *testing.T) {
	ctx := context.Background()
	jobRunID := int64(1898704060324777984)
	mgr := NewManager(ctx)
	query := baseTestingJAQ(t, "artifacts/*e2e*/gather-extra/build-log.txt",
		NewStringMatcher("ClusterVersion:", 0, 0, maxFileMatches))
	jobRun, err := query.queryJobArtifacts(context.Background(), jobRunID, mgr, log.WithField("test", "queryJobArtifacts"))
	assert.NoError(t, err)
	assert.NotEmpty(t, jobRun.Artifacts, "expected to find some artifacts")

	// now modify the cache so that at least one artifact is marked as timed out
	cachedJobRun, err := query.GetCachedJobRun(ctx, jobRunID)
	assert.NoError(t, err, "expected no error when getting cache")
	cachedJobRun.Artifacts[0].TimedOut = true
	cachedJobRun.Artifacts[0].Error = "timed out"
	cachedJobRun.Artifacts[0].MatchedContent = MatchedContent{}
	cachedJobRun.IsFinal = false
	err = query.SetJobRunCache(ctx, jobRunID, cachedJobRun)
	assert.NoError(t, err, "expected no error when updating cache")

	// verify that the cache returns the timed out artifact
	cachedJobRun, err = query.GetCachedJobRun(ctx, jobRunID)
	assert.NoError(t, err, "expected no error when getting cache")
	assert.True(t, cachedJobRun.Artifacts[0].TimedOut, "expected cached artifact to be marked as timed out")
	assert.False(t, cachedJobRun.IsFinal, "expected job run to be marked as incomplete")

	// requery the job run
	jobRun, err = query.queryJobArtifacts(context.Background(), jobRunID, mgr, log.WithField("test", "requeryJobArtifacts"))
	assert.NoError(t, err)
	assert.NotEmpty(t, jobRun.Artifacts, "expected to find some artifacts")
	for _, artifact := range jobRun.Artifacts {
		assert.False(t, artifact.TimedOut, "expected all artifacts to be retrieved")
	}
	mgr.Close()
}

func TestFunctional_QueryJobArtifacts(t *testing.T) {
	mgr := NewManager(context.Background())
	query := baseTestingJAQ(t, "artifacts/*e2e*/gather-extra/build-log.txt",
		NewStringMatcher("ClusterVersion:", 0, 0, maxFileMatches))
	jobRun, err := query.queryJobArtifacts(context.Background(), 1898704060324777984, mgr, log.WithField("test", "queryJobArtifacts"))
	assert.NoError(t, err)
	assert.NotEmpty(t, jobRun.Artifacts, "expected to find some artifacts")
	mgr.Close()
}

func TestFunctional_Query(t *testing.T) {
	mgr := NewManager(context.Background())
	query := baseTestingJAQ(t, "artifacts/*e2e*/gather-extra/build-log.txt",
		NewStringMatcher("ClusterVersion:", 0, 0, maxFileMatches))
	query.JobRunIDs = []int64{1898704060324777984, 42}

	res := mgr.Query(context.Background(), query)
	assert.NotEmpty(t, res.Errors)
	assert.Equal(t, "42", res.Errors[0].ID, "expected not to find the answer to everything")
	assert.NotEmpty(t, res.JobRuns, "expected to find the job")
	assert.NotEmpty(t, res.JobRuns[0].Artifacts, "expected to find some artifacts")
	mgr.Close()
}

func TestCacheKeyForJobRun(t *testing.T) {
	query := &JobArtifactQuery{
		PathGlob: "artifacts/*e2e*/gather-extra/build-log.txt",
	}

	jobRunID := int64(123456789)

	expectedKey := `{"id":"123456789","pathGlob":"artifacts/*e2e*/gather-extra/build-log.txt","type":"JAQJobRun~v1"}`
	cacheKey := query.CacheKeyForJobRun(jobRunID)
	assert.Equal(t, expectedKey, cacheKey, "CacheKeyForJobRun did not return the expected key")

	query.ContentMatcher = NewStringMatcher("ClusterVersion:", 0, 0, maxFileMatches)
	expectedKey = `{"contentMatcher":"stringLineMatcher: ClusterVersion:","id":"123456789","pathGlob":"artifacts/*e2e*/gather-extra/build-log.txt","type":"JAQJobRun~v1"}`
	cacheKey = query.CacheKeyForJobRun(jobRunID)
	assert.Equal(t, expectedKey, cacheKey, "CacheKeyForJobRun did not return the expected key")
}

func TestJobRunCaching(t *testing.T) {
	ctx := context.Background()
	jobRunID := int64(123456789)

	cache := &util.PseudoCache{Cache: make(map[string][]byte)}
	query := &JobArtifactQuery{
		Cache:          cache,
		ContentMatcher: NewStringMatcher("", 0, 0, 0),
	}

	// Attempt to retrieve non-existent cache entry
	cachedResponse, err := query.GetCachedJobRun(ctx, jobRunID)
	assert.Empty(t, cachedResponse, "expected empty response for missing cache entry")
	assert.Error(t, err, "expected no error for missing cache entry")

	// try to cache the thing that was just not found
	response := JobRun{
		ID:      "123456789",
		JobName: "test-job",
		URL:     "http://example.com",
		Artifacts: []JobRunArtifact{
			{
				MatchedContent: MatchedContent{
					&ContentLineMatches{
						Matches: []ContentLineMatch{{Match: "stuff"}},
					},
				},
			},
		},
	}
	err = query.SetJobRunCache(ctx, jobRunID, response)
	assert.NoError(t, err, "expected no error when setting cache")

	cacheKey := query.CacheKeyForJobRun(jobRunID)
	cachedData, exists := cache.Cache[cacheKey]
	assert.True(t, exists, "expected cache to contain the key")
	assert.NotEmpty(t, cachedData, "expected cached data to be non-empty")

	// and then try to get it again
	cachedResponse, err = query.GetCachedJobRun(ctx, jobRunID)
	assert.NoError(t, err, "expected no error when getting cache")
	assert.Equal(t, response, cachedResponse, "expected cached response to match original")
}

func TestSeparateCompletedAndRequeries(t *testing.T) {
	cachedArtifacts := []JobRunArtifact{
		{ArtifactPath: "path1", TimedOut: false},
		{ArtifactPath: "path2", TimedOut: true},
		{ArtifactPath: "path3", TimedOut: false},
	}

	fileAttrs := []*storage.ObjectAttrs{
		{Name: "path1"},
		{Name: "path2"},
		{Name: "path3"},
		{Name: "path4"}, // this would be unusual but in theory a new file could show up and be included *shrug*
	}

	completedArtifacts, requeryAttrs := separateCompletedAndRequeries(cachedArtifacts, fileAttrs)

	// Verify completed artifacts
	assert.Equal(t, 2, len(completedArtifacts), "expected 2 completed artifacts")
	assert.Equal(t, "path1", completedArtifacts[0].ArtifactPath)
	assert.Equal(t, "path3", completedArtifacts[1].ArtifactPath)

	// Verify requery attributes
	assert.Equal(t, 2, len(requeryAttrs), "expected 2 requery attributes")
	assert.Equal(t, "path2", requeryAttrs[0].Name)
	assert.Equal(t, "path4", requeryAttrs[1].Name)
}
