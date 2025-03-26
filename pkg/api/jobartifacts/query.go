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
	"google.golang.org/api/iterator"
)

const maxJobFilesToScan = 12 // limit the number of files inspected under each job
const maxFileMatches = 12    // limit the number of content matches returned for each file
const artifactUrlFmt = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/%s/%s"

type JobArtifactQuery struct {
	GcsBucket        *storage.BucketHandle
	DbClient         *db.DB
	JobName          string        // The name of the job to query
	JobRunIDs        []int64       // The runs of that job to query
	PathMatch        regexp.Regexp // A regex to match files in the artifact bucket for each queried run
	PathGlob         string        // A simple glob to match files in the artifact bucket for each queried run
	ArtifactContains string        // A string to match in the content of the files
}

func (q *JobArtifactQuery) getJobRunPath(jobRunID int64) (string, error) {
	jobRun := new(models.ProwJobRun)
	res := q.DbClient.DB.First(jobRun, jobRunID)
	if res.Error != nil {
		return "", res.Error
	}

	url := jobRun.URL
	if url == "" {
		return "", fmt.Errorf("DB entry for job run %d has no URL", jobRunID)
	}

	const marker = "/" + util.GcsBucketRoot + "/"
	pathStart := strings.Index(url, marker)
	if pathStart == -1 {
		return "", fmt.Errorf("job run %d URL %s does not include bucket root %q", jobRunID, url, util.GcsBucketRoot)
	}

	jobRunPath := url[pathStart+len(marker):]
	if !strings.HasSuffix(jobRunPath, "/") {
		jobRunPath += "/" // ensure the path looks like a bucket "object prefix"
	}
	return jobRunPath, nil
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
