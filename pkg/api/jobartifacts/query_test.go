package jobartifacts

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestFunctional_JobRunPathFound(t *testing.T) {
	dbClient := util.GetDbHandle(t)
	query := JobArtifactQuery{DbClient: dbClient}
	path, jobRun, err := query.getJobRun(1898704060324777984)
	assert.NoError(t, err)
	assert.Equal(t, "logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/", path)
	assert.Equal(t, "periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn", jobRun.JobName)
}

func TestFunctional_ListFiles(t *testing.T) {
	gcsClient := util.GetGcsBucket(t)
	query := JobArtifactQuery{GcsBucket: gcsClient}
	files, truncated, err := query.getJobRunFiles("logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/")
	assert.NoError(t, err)
	assert.True(t, truncated, "expected a lot of files under the job")
	assert.Equal(t, maxJobFilesToScan, len(files), "expected to receive the max number of files")
}

func TestFunctional_FilterFiles(t *testing.T) {
	gcsClient := util.GetGcsBucket(t)
	query := JobArtifactQuery{GcsBucket: gcsClient, PathGlob: "artifacts/*e2e*/gather-extra/build-log.txt"}
	files, truncated, err := query.getJobRunFiles("logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/")
	assert.NoError(t, err)
	assert.False(t, truncated, "expected no need for truncating the file list")
	assert.Equal(t, 1, len(files), "expected glob to match one file")
}

func TestFunctional_FilterContent(t *testing.T) {
	gcsClient := util.GetGcsBucket(t)
	const filePath = "logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/artifacts/e2e-azure-ovn/gather-extra/build-log.txt"

	query := JobArtifactQuery{GcsBucket: gcsClient, ContentMatcher: NewStringMatcher("ClusterVersion:", 0, 0, maxFileMatches)}
	artifact := query.getFileContentMatches(1898704060324777984, &storage.ObjectAttrs{Name: filePath})
	assert.Empty(t, artifact.Error)
	assert.False(t, artifact.MatchedContent.(ContentLineMatches).Truncated, "expected no need for truncating the content matches")
	assert.Equal(t, 2, len(artifact.MatchedContent.(ContentLineMatches).Matches), "expected content to match with two lines")

	query.ContentMatcher = NewStringMatcher("error:", 0, 0, maxFileMatches)
	artifact = query.getFileContentMatches(1898704060324777984, &storage.ObjectAttrs{Name: filePath})
	assert.Empty(t, artifact.Error)
	assert.True(t, artifact.MatchedContent.(ContentLineMatches).Truncated, "expected to truncate content matches")
	assert.Equal(t, maxFileMatches, len(artifact.MatchedContent.(ContentLineMatches).Matches), "expected content to match with many lines")
}

func TestFunctional_GzipContent(t *testing.T) {
	gcsClient := util.GetGcsBucket(t)
	const filePath = "logs/periodic-ci-openshift-release-master-ci-4.19-e2e-aws-ovn-techpreview/1909930323508989952/artifacts/e2e-aws-ovn-techpreview/gather-extra/artifacts/nodes/ip-10-0-59-177.us-east-2.compute.internal/journal"

	query := JobArtifactQuery{GcsBucket: gcsClient, ContentMatcher: NewStringMatcher("error", 0, 0, maxFileMatches)}
	artifact := query.getFileContentMatches(1909930323508989952, &storage.ObjectAttrs{Name: filePath, ContentType: "application/gzip"})
	assert.Empty(t, artifact.Error)
	assert.True(t, artifact.MatchedContent.(ContentLineMatches).Truncated, "expected a lot of matches")
	assert.Contains(t,
		artifact.MatchedContent.(ContentLineMatches).Matches[0].Match,
		"localhost kernel: GPT: Use GNU Parted to correct GPT errors.",
		"expected to scan uncompressed text")
}

func TestFunctional_QueryJobArtifacts(t *testing.T) {
	mgr := NewManager(context.Background())
	query := JobArtifactQuery{
		DbClient:       util.GetDbHandle(t),
		GcsBucket:      util.GetGcsBucket(t),
		PathGlob:       "artifacts/*e2e*/gather-extra/build-log.txt",
		ContentMatcher: NewStringMatcher("ClusterVersion:", 0, 0, maxFileMatches),
	}
	jobRun, err := query.queryJobArtifacts(context.Background(), 1898704060324777984, mgr, log.WithField("test", "queryJobArtifacts"))
	assert.NoError(t, err)
	assert.NotEmpty(t, jobRun.Artifacts, "expected to find some artifacts")
	mgr.Close()
}

func TestFunctional_Query(t *testing.T) {
	mgr := NewManager(context.Background())
	query := JobArtifactQuery{
		DbClient:       util.GetDbHandle(t),
		GcsBucket:      util.GetGcsBucket(t),
		JobRunIDs:      []int64{1898704060324777984, 42},
		PathGlob:       "artifacts/*e2e*/gather-extra/build-log.txt",
		ContentMatcher: NewStringMatcher("ClusterVersion:", 0, 0, maxFileMatches),
	}
	res := mgr.Query(context.Background(), &query)
	assert.NotEmpty(t, res.Errors)
	assert.Equal(t, "42", res.Errors[0].ID, "expected not to find the answer to everything")
	assert.NotEmpty(t, res.JobRuns, "expected to find the job")
	assert.NotEmpty(t, res.JobRuns[0].Artifacts, "expected to find some artifacts")
	mgr.Close()
}
