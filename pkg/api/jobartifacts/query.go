package jobartifacts

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const artifactURLFmt = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/%s/%s"

type JobArtifactQuery struct {
	GcsBucket        *storage.BucketHandle
	DbClient         *db.DB
	JobRunIDs        []int64
	PathGlob         string // A simple glob to match files in the artifact bucket for each queried run
	ArtifactContains string // A string to match in the content of the files
	// TODO: regex, jq, xpath support for matching content; requesting match context lines; etc.
}

func (q *JobArtifactQuery) queryJobArtifacts(ctx context.Context, jobRunID int64, mgr *Manager, logger *log.Entry) (JobRun, error) {
	logger = logger.WithField("func", "queryJobArtifacts").WithField("job_run_id", jobRunID)

	jobRunPath, jobRunResponse, err := q.getJobRun(jobRunID)
	if err != nil {
		logger.WithError(err).Error("could not query job's bucket path")
		return jobRunResponse, err
	}

	filePaths, truncated, err := q.getJobRunFiles(jobRunPath)
	if err != nil {
		logger.WithError(err).Error("could not find job artifact files")
		return jobRunResponse, err
	}
	jobRunResponse.ArtifactListTruncated = truncated
	jobRunResponse.Artifacts = mgr.QueryJobRunArtifacts(ctx, q, jobRunID, filePaths)
	return jobRunResponse, nil
}

func (q *JobArtifactQuery) getJobRun(jobRunID int64) (string, JobRun, error) {
	jobRunResponse := JobRun{ID: strconv.FormatInt(jobRunID, 10)}
	jobRunModel := new(models.ProwJobRun)
	res := q.DbClient.DB.Preload("ProwJob").First(jobRunModel, jobRunID)
	if res.Error != nil {
		return "", jobRunResponse, res.Error
	}
	jobRunResponse.JobName = jobRunModel.ProwJob.Name

	url := jobRunModel.URL
	if url == "" {
		return "", jobRunResponse, fmt.Errorf("DB entry for job run %d has no URL", jobRunID)
	}
	jobRunResponse.URL = url

	const marker = "/" + util.GcsBucketRoot + "/"
	pathStart := strings.Index(url, marker)
	if pathStart == -1 {
		return "", jobRunResponse, fmt.Errorf("job run %d URL %s does not include bucket root %q", jobRunID, url, util.GcsBucketRoot)
	}

	jobRunPath := url[pathStart+len(marker):]
	if !strings.HasSuffix(jobRunPath, "/") {
		jobRunPath += "/" // ensure the path looks like a bucket "object prefix"
	}
	return jobRunPath, jobRunResponse, nil
}

func (q *JobArtifactQuery) getJobRunFiles(jobRunPath string) ([]string, bool, error) {
	files := []string{}
	truncated := false
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	query := &storage.Query{
		Prefix:     jobRunPath,
		Projection: storage.ProjectionNoACL,
	}
	if q.PathGlob != "" {
		query.MatchGlob = jobRunPath + q.PathGlob
	}
	iter := q.GcsBucket.Objects(ctx, query)
	for {
		attrs, err := iter.Next()
		if err != nil {
			if errors.Is(err, iterator.Done) {
				break
			}
			return nil, false, err
		}
		if len(files) >= maxJobFilesToScan {
			truncated = true
			break
		}
		files = append(files, attrs.Name)
	}

	return files, truncated, nil
}

func (q *JobArtifactQuery) getFileContentMatches(jobRunID int64, filePath string) (artifact JobRunArtifact) {
	artifact.JobRunID = strconv.FormatInt(jobRunID, 10)
	artifact.ArtifactURL = fmt.Sprintf(artifactURLFmt, util.GcsBucketRoot, filePath)
	if q.ArtifactContains == "" { // no snippets requested
		return
	}

	gcsReader, err := q.GcsBucket.Object(filePath).NewReader(context.Background())
	if err != nil {
		artifact.Error = err.Error()
		return
	}
	defer gcsReader.Close()

	reader := bufio.NewReader(gcsReader)
	for {
		// scan the file line by line; however, if lines are too long, ReadString
		// breaks them up instead of failing, so these may not be complete lines.
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			artifact.Error = err.Error()
			return
		}

		if q.lineMatchesQuery(line) {
			if len(artifact.MatchedContent) >= maxFileMatches {
				artifact.MatchesTruncated = true
				break
			}
			artifact.MatchedContent = append(artifact.MatchedContent, line)
		}
	}

	return
}

func (q *JobArtifactQuery) lineMatchesQuery(content string) bool {
	// this can become more interesting when we want.
	return strings.Contains(content, q.ArtifactContains)
}
