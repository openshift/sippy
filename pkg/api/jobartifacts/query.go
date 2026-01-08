package jobartifacts

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const artifactURLFmt = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/%s/%s"

type JobArtifactQuery struct {
	GcsBucket *storage.BucketHandle
	DbClient  *db.DB
	cache.Cache
	JobRunIDs      []int64
	PathGlob       string // A simple glob to match files in the artifact bucket for each queried run
	ContentMatcher        // An interface to match in the content of the files
	// TODO: jq, xpath support for matching content
}

func (q *JobArtifactQuery) queryJobArtifacts(ctx context.Context, jobRunID int64, mgr *Manager, logger *log.Entry) (JobRun, error) {
	logger = logger.WithField("func", "queryJobArtifacts").WithField("job_run_id", jobRunID)

	// check the cache first
	jobRunResponse, err := q.GetCachedJobRun(ctx, jobRunID)
	if err != nil { // cache miss, look up the job run from scratch
		if jobRunResponse, err = q.getJobRun(jobRunID); err != nil {
			logger.WithError(err).Error("could not query job's bucket path")
			return jobRunResponse, err
		}

		fileAttrs, truncated, err := q.getJobRunFiles(jobRunResponse.BucketPath)
		if err != nil {
			logger.WithError(err).Error("could not find job artifact files")
			return jobRunResponse, err
		}
		jobRunResponse.ArtifactListTruncated = truncated
		jobRunResponse.Artifacts, jobRunResponse.IsFinal = mgr.QueryJobRunArtifacts(ctx, q, jobRunID, fileAttrs)
	} else if !jobRunResponse.IsFinal { // cache hit but it's not final; re-query missing artifacts
		fileAttrs, truncated, err := q.getJobRunFiles(jobRunResponse.BucketPath)
		if err != nil {
			logger.WithError(err).Error("could not find job artifact files")
			return jobRunResponse, err
		}
		jobRunResponse.ArtifactListTruncated = truncated

		// now filter to just the ones needed to fill out the response
		completedArtifacts, requeryAttrs := separateCompletedAndRequeries(jobRunResponse.Artifacts, fileAttrs)
		var newArtifacts []JobRunArtifact
		newArtifacts, jobRunResponse.IsFinal = mgr.QueryJobRunArtifacts(ctx, q, jobRunID, requeryAttrs)
		jobRunResponse.Artifacts = append(completedArtifacts, newArtifacts...)
	}

	// cache the response and then post-process matches down to what was actually requested
	_ = q.SetJobRunCache(ctx, jobRunID, jobRunResponse)
	if q.ContentMatcher != nil {
		for i := range jobRunResponse.Artifacts {
			jobRunResponse.Artifacts[i] = q.ContentMatcher.PostProcessMatch(jobRunResponse.Artifacts[i])
		}
	}

	return jobRunResponse, nil
}

// filter to just the ones needed to fill out the response
func separateCompletedAndRequeries(cachedArtifacts []JobRunArtifact, fileAttrs []*storage.ObjectAttrs) ([]JobRunArtifact, []*storage.ObjectAttrs) {
	completedArtifacts := []JobRunArtifact{}
	completedPaths := sets.NewString()
	for _, artifact := range cachedArtifacts {
		if !artifact.TimedOut {
			completedArtifacts = append(completedArtifacts, artifact)
			completedPaths.Insert(artifact.ArtifactPath)
		}
	}
	requeryAttrs := []*storage.ObjectAttrs{}
	for _, file := range fileAttrs {
		if !completedPaths.Has(file.Name) {
			requeryAttrs = append(requeryAttrs, file)
		}
	}
	return completedArtifacts, requeryAttrs
}

func (q *JobArtifactQuery) getJobRun(jobRunID int64) (JobRun, error) {
	jobRunResponse := JobRun{
		ID:      strconv.FormatInt(jobRunID, 10),
		IsFinal: true, // even errors are final - only if we timed out getting answers might a retry get more
	}
	jobRunModel := new(models.ProwJobRun)
	res := q.DbClient.DB.Preload("ProwJob").First(jobRunModel, jobRunID)
	if res.Error != nil {
		return jobRunResponse, res.Error
	}
	jobRunResponse.JobName = jobRunModel.ProwJob.Name

	url := jobRunModel.URL
	if url == "" {
		return jobRunResponse, fmt.Errorf("DB entry for job run %d has no URL", jobRunID)
	}
	jobRunResponse.URL = url

	const marker = "/" + util.GcsBucketRoot + "/"
	pathStart := strings.Index(url, marker)
	if pathStart == -1 {
		return jobRunResponse, fmt.Errorf("job run %d URL %s does not include bucket root %q", jobRunID, url, util.GcsBucketRoot)
	}

	jobRunPath := url[pathStart+len(marker):]
	if !strings.HasSuffix(jobRunPath, "/") {
		jobRunPath += "/" // ensure the path looks like a bucket "object prefix"
	}
	jobRunResponse.BucketPath = jobRunPath
	return jobRunResponse, nil
}

func (q *JobArtifactQuery) getJobRunFiles(jobRunPath string) ([]*storage.ObjectAttrs, bool, error) {
	files := []*storage.ObjectAttrs{}
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
		files = append(files, attrs)
	}

	return files, truncated, nil
}

// path queries come back with full path after bucket name; reduce to the jobrun-specific part
func relativeArtifactPath(bucketPath, jobRunID string) string {
	marker := "/" + jobRunID + "/"
	start := strings.Index(bucketPath, marker)
	if start == -1 { // would be very weird, but not really something to choke on
		log.Errorf("artifact path %q somehow does not include jobRunID %q", bucketPath, jobRunID)
		return bucketPath
	}
	return bucketPath[start+len(marker):]
}

func (q *JobArtifactQuery) getFileContentMatches(ctx context.Context, jobRunID int64, attrs *storage.ObjectAttrs) (artifact JobRunArtifact) {
	artifact.JobRunID = strconv.FormatInt(jobRunID, 10)
	artifact.ArtifactPath = relativeArtifactPath(attrs.Name, artifact.JobRunID)
	artifact.ArtifactContentType = attrs.ContentType
	artifact.ArtifactURL = fmt.Sprintf(artifactURLFmt, util.GcsBucketRoot, attrs.Name)
	if q.ContentMatcher == nil { // no matching requested
		return
	}

	reader, closer, err := OpenArtifactReader(ctx, q.GcsBucket.Object(attrs.Name), attrs.ContentType)
	defer closer()
	if err != nil {
		artifact.Error = err.Error()
		return
	}

	matches, err := q.ContentMatcher.GetMatches(reader)
	if err != nil {
		artifact.Error = err.Error()
	}
	artifact.MatchedContent = matches // even if scanning hit an error, we may still want to see incomplete matches
	return
}

// OpenArtifactReader opens a reader on an artifact, transparently handling compressed archives.
// In addition to the reader, it returns a closer function which can and should be called in a defer -
// regardless of whether there was an error.
func OpenArtifactReader(ctx context.Context, file *storage.ObjectHandle, contentType string) (*bufio.Reader, func(), error) {
	var gcsReader *storage.Reader
	var gzipReader *gzip.Reader

	var reader *bufio.Reader
	closer := func() {
		if gzipReader != nil {
			_ = gzipReader.Close()
		}
		if gcsReader != nil {
			_ = gcsReader.Close()
		}
	}
	var err error

	gcsReader, err = file.NewReader(ctx)
	if err != nil {
		gcsReader = nil // will not need closing
		return nil, closer, err
	}

	if contentType == "application/gzip" {
		// if it's gzipped, decompress it in the stream
		gzipReader, err = gzip.NewReader(gcsReader)
		if err != nil {
			gzipReader = nil // will not need closing
			return nil, closer, err
		}
		reader = bufio.NewReader(gzipReader)
	} else { // otherwise read it as a normal text file
		reader = bufio.NewReader(gcsReader)
	}

	return reader, closer, nil
}

// ContentMatcher is a generic interface for matching content in artifact files
type ContentMatcher interface {
	// GetMatches reads lines from the provided reader and returns a content match object (possibly incomplete with an error)
	GetMatches(reader *bufio.Reader) (MatchedContent, error)
	// GetCacheKey uniquely identifies the content query so we can cache separate results of different queries against the same run
	GetCacheKey() string
	// GetContentTemplate returns an instance of the type returned in GetMatches (used to enable deserializing interface{} from cache)
	GetContentTemplate() interface{}
	// PostProcessMatch is called to trim (if needed) the full set of matches (which we cache) down to what was actually requested
	PostProcessMatch(fullArtifact JobRunArtifact) JobRunArtifact
}

/*
 Caching is implemented at the level of the job run, scoped to a specific file and content query.
 Content match and file match results have an indicator for whether they timed out, and job runs have an indicator for
 whether they are "final" meaning we have retrieved all the contents requested (possibly with errors, but not timeouts).

 When a JAQ comes in for a job run, we check the cache for an existing entry for the job run ID + JAQ. If there is one
 and it's complete, we can just return it. If an incomplete one exists, we can request just the parts that timed out,
 and then cache that result. In this way, by re-querying the same job run, we can fill out the results until they are
 completely cached.

 For content matching, we will scan a file for the widest possible match, and then post-filter the results down to
 what was requested. For example, if a string line matcher looks for "foo" with 2 lines of context and a limit of 2
 matches, we will scan the file and cache 12 matches with 12 lines of context; then just return the 2 matches with
 2 lines of context. This way, adjusting the matcher limits will not require re-scanning the file or caching results at
 different sizes.
*/

func (q *JobArtifactQuery) CacheKeyForJobRun(jobRunID int64) string {
	key := map[string]string{
		"type":     "JAQJobRun~v1",
		"id":       strconv.FormatInt(jobRunID, 10),
		"pathGlob": q.PathGlob,
	}
	if q.ContentMatcher != nil {
		key["contentMatcher"] = q.ContentMatcher.GetCacheKey()
	}

	jsonBytes, err := json.Marshal(key)
	if err != nil {
		log.Errorf("CacheKeyForJobRun should never fail to serialize the key: %v", err)
		panic(err) // an engineer causing this somehow should find out ASAP
	}
	return string(jsonBytes)
}

const cacheExpiration = 4 * time.Hour

func (q *JobArtifactQuery) SetJobRunCache(ctx context.Context, jobRunID int64, response JobRun) error {
	logger := log.WithField("func", "SetJobRunCache").WithField("job_run_id", jobRunID)

	if q.Cache == nil {
		return fmt.Errorf("job artifact query cache is disabled")
	}

	serialized, err := json.Marshal(response)
	if err != nil {
		logger.WithError(err).Fatal("Should never fail to serialize jobRunResponse")
	}

	// set the cache with the serialized response
	if err := q.Cache.Set(ctx, q.CacheKeyForJobRun(jobRunID), serialized, cacheExpiration); err != nil {
		logger.WithError(err).Error("failed to set job run cache")
		return err
	}
	return nil
}

func (q *JobArtifactQuery) GetCachedJobRun(ctx context.Context, jobRunID int64) (response JobRun, err error) {
	logger := log.WithField("func", "GetCachedJobRun").WithField("job_run_id", jobRunID)

	if q.Cache == nil {
		return JobRun{}, errors.New("cache not initialized")
	}

	// retrieve bytes from the cache if they exist
	jsonBytes, err := q.Cache.Get(ctx, q.CacheKeyForJobRun(jobRunID), cacheExpiration)
	if err != nil {
		logger.WithError(err).Debug("failed to get job run cache entry")
		return
	} else if len(jsonBytes) == 0 {
		err = fmt.Errorf("no cache entry found")
		logger.Debug(err)
		return
	}

	// unmarshal the bytes using zson so the interface{} type can be filled correctly
	if err = json.Unmarshal(jsonBytes, &response); err != nil {
		logger.WithError(err).Error("failed to unmarshal job run cache entry")
		return
	}

	logger.Debug("found job run cache")
	return
}
