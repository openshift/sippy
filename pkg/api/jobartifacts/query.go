package jobartifacts

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const maxJobFilesToScan = 12 // limit the number of files inspected under each job
const maxFileMatches = 12    // limit the number of content matches returned for each file
const artifactUrlFmt = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/%s/%s"

type JobArtifactQuery struct {
	GcsBucket        *storage.BucketHandle
	DbClient         *db.DB
	JobRunIDs        []int64
	PathMatch        regexp.Regexp // (TODO) A regex to match files in the artifact bucket for each queried run
	PathGlob         string        // A simple glob to match files in the artifact bucket for each queried run
	ArtifactContains string        // A string to match in the content of the files
	// TODO: regex and jq support for matching content
}

func (q *JobArtifactQuery) Query(logger *log.Entry) (res QueryResponse) {
	for _, jobRunID := range q.JobRunIDs {
		jobRun, err := q.queryJobArtifacts(jobRunID, logger)
		if err != nil {
			res.Errors = append(res.Errors, JobRun{ID: jobRunID, Error: err.Error()})
			continue
		}
		res.JobRuns = append(res.JobRuns, jobRun)
	}
	return
}

// will be more complicated than this
func (q *JobArtifactQuery) queryJobArtifacts(jobRunID int64, logger *log.Entry) (JobRun, error) {
	jobRunResponse := JobRun{ID: jobRunID}
	logger = logger.WithField("func", "queryJobArtifacts").WithField("job_run_id", jobRunID)

	jobRunPath, jobName, err := q.getJobRunPath(jobRunID)
	if err != nil {
		logger.WithError(err).Error("could not query job's bucket path")
		return jobRunResponse, err
	}
	jobRunResponse.JobName = jobName

	filePaths, truncated, err := q.getJobRunFiles(jobRunPath)
	if err != nil {
		logger.WithError(err).Error("could not find job artifact files")
		return jobRunResponse, err
	}
	jobRunResponse.ArtifactListTruncated = truncated

	for _, filePath := range filePaths {
		contentMatches, truncated, err := q.getFileContentMatches(filePath)
		if err != nil {
			logger.WithError(err).Errorf("could not scan job artifact content at %q", filePath)
			return jobRunResponse, err
		}
		jobRunResponse.Artifacts = append(jobRunResponse.Artifacts, JobRunArtifact{
			JobRunID:         jobRunID,
			ArtifactURL:      fmt.Sprintf(artifactUrlFmt, util.GcsBucketRoot, filePath),
			MatchesTruncated: truncated,
			MatchedContent:   contentMatches,
		})
	}
	return jobRunResponse, nil
}

func (q *JobArtifactQuery) getJobRunPath(jobRunID int64) (string, string, error) {
	jobRun := new(models.ProwJobRun)
	res := q.DbClient.DB.Preload("ProwJob").First(jobRun, jobRunID)
	if res.Error != nil {
		return "", "", res.Error
	}

	url := jobRun.URL
	if url == "" {
		return "", "", fmt.Errorf("DB entry for job run %d has no URL", jobRunID)
	}

	const marker = "/" + util.GcsBucketRoot + "/"
	pathStart := strings.Index(url, marker)
	if pathStart == -1 {
		return "", "", fmt.Errorf("job run %d URL %s does not include bucket root %q", jobRunID, url, util.GcsBucketRoot)
	}

	jobRunPath := url[pathStart+len(marker):]
	if !strings.HasSuffix(jobRunPath, "/") {
		jobRunPath += "/" // ensure the path looks like a bucket "object prefix"
	}
	return jobRunPath, jobRun.ProwJob.Name, nil
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

func (q *JobArtifactQuery) getFileContentMatches(filePath string) ([]string, bool, error) {
	matches := []string{}
	truncated := false
	if q.ArtifactContains == "" { // no snippets requested
		return matches, false, nil
	}

	gcsReader, err := q.GcsBucket.Object(filePath).NewReader(context.Background())
	if err != nil {
		return nil, false, err
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
			return matches, false, err
		}

		if q.lineMatchesQuery(line) {
			if len(matches) >= maxFileMatches {
				truncated = true
				break
			}
			matches = append(matches, line)
		}
	}

	return matches, truncated, nil
}

func (q *JobArtifactQuery) lineMatchesQuery(content string) bool {
	// this can become more interesting when we want.
	return strings.Contains(content, q.ArtifactContains)
}
