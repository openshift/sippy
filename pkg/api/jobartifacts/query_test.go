package jobartifacts

import (
	"testing"

	"github.com/openshift/sippy/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestFunctional_JobRunPathFound(t *testing.T) {
	dbClient := util.GetDbHandle(t)
	query := JobArtifactQuery{DbClient: dbClient}
	path, err := query.getJobRunPath(1898704060324777984)
	assert.NoError(t, err)
	assert.Equal(t, "logs/periodic-ci-openshift-release-master-ci-4.19-e2e-azure-ovn/1898704060324777984/", path)
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
