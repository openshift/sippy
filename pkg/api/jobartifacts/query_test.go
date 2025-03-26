package jobartifacts

import (
	"testing"

	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestFunctional_JobRunPathFound(t *testing.T) {
	dbClient := util.GetDbHandle(t)
	query := JobArtifactQuery{DbClient: dbClient}
	path, jobName, err := query.getJobRunPath(1898704060324777984)
	assert.NoError(t, err)
	assert.Equal(t, "logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/", path)
	assert.Equal(t, "periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn", jobName)
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

	query := JobArtifactQuery{GcsBucket: gcsClient, ArtifactContains: "ClusterVersion:"}
	lines, truncated, err := query.getFileContentMatches(filePath)
	assert.NoError(t, err)
	assert.False(t, truncated, "expected no need for truncating the file list")
	assert.Equal(t, 2, len(lines), "expected content to match with two lines")

	query.ArtifactContains = "error:"
	lines, truncated, err = query.getFileContentMatches(filePath)
	assert.NoError(t, err)
	assert.True(t, truncated, "expected to truncate content matches")
	assert.Equal(t, maxFileMatches, len(lines), "expected content to match with many lines")
}

func TestFunctional_QueryJobArtifacts(t *testing.T) {
	query := JobArtifactQuery{
		DbClient:         util.GetDbHandle(t),
		GcsBucket:        util.GetGcsBucket(t),
		PathGlob:         "artifacts/*e2e*/gather-extra/build-log.txt",
		ArtifactContains: "ClusterVersion:",
	}
	jobRun, err := query.QueryJobArtifacts(1898704060324777984, log.WithField("test", "QueryJobArtifacts"))
	assert.NoError(t, err)
	assert.NotEmpty(t, jobRun.Artifacts, "expected to find some artifacts")
}

func TestFunctional_Query(t *testing.T) {
	query := JobArtifactQuery{
		DbClient:         util.GetDbHandle(t),
		GcsBucket:        util.GetGcsBucket(t),
		JobRunIDs:        []int64{1898704060324777984, 42},
		PathGlob:         "artifacts/*e2e*/gather-extra/build-log.txt",
		ArtifactContains: "ClusterVersion:",
	}
	jobRuns, errs := query.Query(log.WithField("test", "Query"))
	assert.NotEmpty(t, errs)
	assert.Equal(t, int64(42), errs[0].ID, "expected not to find the answer to everything")
	assert.NotEmpty(t, jobRuns, "expected to find the job")
	assert.NotEmpty(t, jobRuns[0].Artifacts, "expected to find some artifacts")
}
