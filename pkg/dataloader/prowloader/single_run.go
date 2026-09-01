package prowloader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgconn"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/synthetictests"
)

const (
	SingleRunMaximumAge = 14 * 24 * time.Hour
	finishedFilename    = "finished.json"
	prowJobFilename     = "prowjob.json"

	SingleRunStatusImported        = "imported"
	SingleRunStatusAlreadyImported = "already_imported"
	SingleRunStatusIgnored         = "ignored"
	SingleRunReasonNotTracked      = "prow_job_not_tracked"
)

type SingleRunImportErrorKind string

const (
	SingleRunInvalidRequest     SingleRunImportErrorKind = "invalid_request"
	SingleRunUnauthenticated    SingleRunImportErrorKind = "unauthenticated"
	SingleRunNotFound           SingleRunImportErrorKind = "not_found"
	SingleRunInvalidProwJob     SingleRunImportErrorKind = "invalid_prow_job"
	SingleRunArtifactFailure    SingleRunImportErrorKind = "artifact_failure"
	SingleRunDependencyFailure  SingleRunImportErrorKind = "dependency_unavailable"
	SingleRunPersistenceFailure SingleRunImportErrorKind = "persistence_failure"
)

type SingleRunImportError struct {
	Kind SingleRunImportErrorKind
	Err  error
}

func (e *SingleRunImportError) Error() string { return e.Err.Error() }
func (e *SingleRunImportError) Unwrap() error { return e.Err }

func importError(kind SingleRunImportErrorKind, format string, args ...any) error {
	return &SingleRunImportError{Kind: kind, Err: fmt.Errorf(format, args...)}
}

type SingleRunImportRequest struct {
	ProwJobRunID string `json:"prow_job_run_id"`
	Bucket       string `json:"bucket"`
	JobPrefix    string `json:"job_prefix"`
}

type SingleRunImportResult struct {
	ProwJobRunID string            `json:"prow_job_run_id"`
	Status       string            `json:"status"`
	Reason       string            `json:"reason,omitempty"`
	ProwJobName  string            `json:"prow_job_name,omitempty"`
	Release      string            `json:"release,omitempty"`
	Bucket       string            `json:"bucket"`
	JobPrefix    string            `json:"job_prefix"`
	GCSLocation  string            `json:"gcs_location"`
	JUnitFiles   int               `json:"junit_files"`
	Tests        int               `json:"tests"`
	Links        map[string]string `json:"links,omitempty"`
}

type singleRunExistsFunc func(context.Context, uint) (bool, error)
type singleRunReadArtifactFunc func(context.Context, string, string) ([]byte, error)
type singleRunLoadJUnitFunc func(context.Context, string, string) (*junit.TestSuites, []string, error)
type singleRunDefinitionFunc func(context.Context, string) (*models.ProwJob, error)
type singleRunLabelsFunc func(context.Context, string, civil.Date) ([]string, error)
type singleRunWriteFunc func(context.Context, *db.DB, civil.Date, pgwriter.JobRunResult) error

type SingleRunImporter struct {
	dbc              *db.DB
	gcsClient        *storage.Client
	bqClient         *bqcachedclient.Client
	configuredBucket string
	syntheticManager synthetictests.SyntheticTestManager

	now              func() time.Time
	exists           singleRunExistsFunc
	readArtifact     singleRunReadArtifactFunc
	loadJUnit        singleRunLoadJUnitFunc
	lookupDefinition singleRunDefinitionFunc
	loadLabels       singleRunLabelsFunc
	write            singleRunWriteFunc
}

func NewSingleRunImporter(dbc *db.DB, gcsClient *storage.Client, bqClient *bqcachedclient.Client, configuredBucket string, syntheticManager synthetictests.SyntheticTestManager) *SingleRunImporter {
	if syntheticManager == nil {
		syntheticManager = synthetictests.NewEmptySyntheticTestManager()
	}
	i := &SingleRunImporter{
		dbc: dbc, gcsClient: gcsClient, bqClient: bqClient,
		configuredBucket: configuredBucket, syntheticManager: syntheticManager,
		now: time.Now, write: pgwriter.WriteSingleIdempotent,
	}
	i.exists = func(ctx context.Context, id uint) (bool, error) {
		return ProwJobRunExists(ctx, i.dbc, id)
	}
	i.readArtifact = i.readGCSArtifact
	i.loadJUnit = i.loadGCSJUnit
	i.lookupDefinition = i.findDefinition
	i.loadLabels = func(ctx context.Context, buildID string, startDate civil.Date) ([]string, error) {
		return GatherLabelsForRunFromBQ(ctx, i.bqClient, buildID, startDate)
	}
	return i
}

func (i *SingleRunImporter) Import(ctx context.Context, request SingleRunImportRequest) (*SingleRunImportResult, error) {
	runID, prefix, err := validateSingleRunRequest(request, i.configuredBucket)
	if err != nil {
		return nil, err
	}
	result := &SingleRunImportResult{
		ProwJobRunID: request.ProwJobRunID, Bucket: request.Bucket, JobPrefix: prefix,
		GCSLocation: fmt.Sprintf("gs://%s/%s", request.Bucket, prefix),
	}

	exists, err := i.exists(ctx, uint(runID))
	if err != nil {
		return nil, classifyImportError(SingleRunPersistenceFailure, "checking existing Prow job run", err)
	}
	if exists {
		result.Status = SingleRunStatusAlreadyImported
		return result, nil
	}

	prowBytes, err := i.readArtifact(ctx, request.Bucket, path.Join(prefix, prowJobFilename))
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, classifyImportError(SingleRunNotFound, "reading prowjob.json", err)
		}
		return nil, classifyImportError(SingleRunArtifactFailure, "reading prowjob.json", err)
	}
	var pj prow.ProwJob
	if err := json.Unmarshal(prowBytes, &pj); err != nil {
		return nil, importError(SingleRunInvalidProwJob, "decoding prowjob.json: %v", err)
	}

	now := i.now().UTC()
	if err := validateSingleRunProwJob(&pj, request, prefix, now); err != nil {
		return nil, err
	}

	duration, err := i.resolveDuration(ctx, &pj, request.Bucket, prefix)
	if err != nil {
		return nil, err
	}

	suites, junitPaths, err := i.loadJUnit(ctx, request.Bucket, prefix)
	if err != nil {
		return nil, classifyImportError(SingleRunArtifactFailure, "loading JUnit artifacts", err)
	}

	definition, err := i.lookupDefinition(ctx, pj.Spec.Job)
	if err != nil {
		return nil, classifyImportError(SingleRunPersistenceFailure, "looking up ProwJob definition", err)
	}
	if definition == nil {
		result.ProwJobName = pj.Spec.Job
		result.Status = SingleRunStatusIgnored
		result.Reason = SingleRunReasonNotTracked
		return result, nil
	}
	if definition.Release == "" {
		return nil, importError(SingleRunNotFound, "ProwJob definition %q has no release", pj.Spec.Job)
	}

	labels, err := i.loadLabels(ctx, request.ProwJobRunID, civil.DateOf(pj.Status.StartTime.UTC()))
	if err != nil {
		return nil, classifyImportError(SingleRunArtifactFailure, "loading authoritative job labels", err)
	}

	pl := &ProwLoader{syntheticTestManager: i.syntheticManager}
	tests, failures, flakes, overall, err := pl.prowJobRunTestsFromSuites(&pj, uint(runID), definition.ID, definition.Release, suites)
	if err != nil {
		return nil, importError(SingleRunInvalidProwJob, "converting JUnit results: %v", err)
	}
	pulls := pullRequestDataFromProw(pj.Spec.Refs)
	prepared := assembleJobRunResult(&pj, runID, definition, tests, failures, flakes, overall, pulls, labels, duration)
	prepared.Run.GCSBucket = request.Bucket

	writeErr := i.write(ctx, i.dbc, civil.DateOf(now), *prepared)
	if writeErr != nil && !errors.Is(writeErr, pgwriter.ErrProwJobRunAlreadyExists) {
		return nil, classifyImportError(SingleRunPersistenceFailure, "writing Prow job run", writeErr)
	}
	result.ProwJobName = pj.Spec.Job
	result.Release = definition.Release
	result.JUnitFiles = len(junitPaths)
	result.Tests = len(tests)
	result.Status = SingleRunStatusImported
	if errors.Is(writeErr, pgwriter.ErrProwJobRunAlreadyExists) {
		result.Status = SingleRunStatusAlreadyImported
	}
	if pj.Status.URL != "" {
		result.Links = map[string]string{"prow_job": pj.Status.URL}
	}
	return result, nil
}

func (i *SingleRunImporter) resolveDuration(ctx context.Context, pj *prow.ProwJob, bucket, prefix string) (time.Duration, error) {
	completion := pj.Status.CompletionTime
	if completion == nil {
		markerBytes, err := i.readArtifact(ctx, bucket, path.Join(prefix, finishedFilename))
		if err != nil {
			return 0, classifyImportError(SingleRunArtifactFailure, "reading top-level finished.json", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(markerBytes))
		decoder.UseNumber()
		var marker struct {
			Timestamp json.Number `json:"timestamp"`
		}
		if err := decoder.Decode(&marker); err != nil {
			return 0, importError(SingleRunInvalidProwJob, "decoding finished.json: %v", err)
		}
		if err := decoder.Decode(&struct{}{}); err != nil && err != io.EOF {
			return 0, importError(SingleRunInvalidProwJob, "finished.json contains trailing data: %v", err)
		} else if err == nil {
			return 0, importError(SingleRunInvalidProwJob, "finished.json contains multiple JSON values")
		}
		seconds, err := strconv.ParseInt(string(marker.Timestamp), 10, 64)
		if err != nil || seconds <= 0 {
			return 0, importError(SingleRunInvalidProwJob, "finished.json timestamp must be positive Unix seconds")
		}
		markerTime := time.Unix(seconds, 0).UTC()
		completion = &markerTime
	}
	if !completion.After(pj.Status.StartTime) {
		return 0, importError(SingleRunInvalidProwJob, "completion time must be after status.startTime")
	}
	return completion.Sub(pj.Status.StartTime), nil
}

var gcsBucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$`)

func validateSingleRunRequest(request SingleRunImportRequest, configuredBucket string) (uint64, string, error) {
	id, err := strconv.ParseUint(request.ProwJobRunID, 10, 63)
	if err != nil || id == 0 {
		return 0, "", importError(SingleRunInvalidRequest, "prow_job_run_id must be a positive 63-bit integer")
	}
	if !validGCSBucket(request.Bucket) {
		return 0, "", importError(SingleRunInvalidRequest, "bucket is not a valid Google Cloud Storage bucket name")
	}
	if configuredBucket == "" {
		return 0, "", importError(SingleRunDependencyFailure, "the Sippy Google storage bucket is not configured")
	}
	if request.Bucket != configuredBucket {
		return 0, "", importError(SingleRunInvalidRequest, "bucket %q is not the configured Sippy bucket", request.Bucket)
	}
	prefix, err := canonicalJobPrefix(request.JobPrefix, request.ProwJobRunID)
	if err != nil {
		return 0, "", importError(SingleRunInvalidRequest, "%v", err)
	}
	return id, prefix, nil
}

func validGCSBucket(bucket string) bool {
	return len(bucket) >= 3 && len(bucket) <= 63 && gcsBucketName.MatchString(bucket) &&
		!strings.Contains(bucket, "..") && net.ParseIP(bucket) == nil
}

func canonicalJobPrefix(raw, runID string) (string, error) {
	if raw == "" || strings.HasPrefix(raw, "/") || strings.Contains(raw, "//") || strings.Contains(raw, `\`) {
		return "", fmt.Errorf("job_prefix must be a clean relative top-level Prow job path")
	}
	prefix := strings.TrimSuffix(raw, "/")
	if prefix == "" || path.Clean(prefix) != prefix {
		return "", fmt.Errorf("job_prefix must be canonical")
	}
	parts := strings.Split(prefix, "/")
	valid := len(parts) == 3 && parts[0] == "logs" && parts[1] != "" ||
		len(parts) == 5 && parts[0] == "pr-logs" && parts[1] == "pull" && isPositiveDecimal(parts[2]) && parts[3] != "" ||
		len(parts) == 6 && parts[0] == "pr-logs" && parts[1] == "pull" && parts[2] != "" && isPositiveDecimal(parts[3]) && parts[4] != ""
	if !valid || parts[len(parts)-1] != runID {
		return "", fmt.Errorf("job_prefix must be a supported top-level Prow path rooted at run %s", runID)
	}
	return prefix, nil
}

func isPositiveDecimal(value string) bool {
	n, err := strconv.ParseUint(value, 10, 63)
	return err == nil && n > 0
}

func validateSingleRunProwJob(pj *prow.ProwJob, request SingleRunImportRequest, prefix string, now time.Time) error {
	if pj.Spec.Job == "" || pj.Status.BuildID != request.ProwJobRunID || pj.Status.StartTime.IsZero() {
		return importError(SingleRunInvalidProwJob, "prowjob.json is missing a job, matching build ID, or status.startTime")
	}
	if pj.Spec.DecorationConfig.GCSConfiguration.Bucket != request.Bucket {
		return importError(SingleRunInvalidProwJob, "prowjob.json bucket does not match the requested bucket")
	}
	switch pj.Status.State {
	case prow.SuccessState, prow.FailureState, prow.AbortedState, prow.ErrorState:
	default:
		return importError(SingleRunInvalidProwJob, "ProwJob state %q is not terminal", pj.Status.State)
	}
	start := pj.Status.StartTime.UTC()
	if start.After(now) {
		return importError(SingleRunInvalidProwJob, "status.startTime is in the future")
	}
	if start.Before(now.Add(-SingleRunMaximumAge)) {
		return importError(SingleRunInvalidProwJob, "status.startTime is older than the 14-day import limit")
	}
	if pj.Status.URL != "" {
		bucket, urlPrefix, err := parseProwJobURLLocation(pj.Status.URL, request.ProwJobRunID)
		if err != nil {
			return importError(SingleRunInvalidProwJob, "invalid ProwJob status.url: %v", err)
		}
		if bucket != request.Bucket || urlPrefix != prefix {
			return importError(SingleRunInvalidProwJob, "ProwJob status.url does not match requested bucket and job_prefix")
		}
	}
	return nil
}

func parseProwJobURLLocation(rawURL, runID string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("URL is not absolute")
	}
	const marker = "/view/gs/"
	idx := strings.Index(u.EscapedPath(), marker)
	if idx < 0 {
		return "", "", fmt.Errorf("URL has no /view/gs/ location")
	}
	remainder, err := url.PathUnescape(u.EscapedPath()[idx+len(marker):])
	if err != nil {
		return "", "", fmt.Errorf("invalid escaped path: %w", err)
	}
	bucket, objectPath, found := strings.Cut(remainder, "/")
	if !found || bucket == "" {
		return "", "", fmt.Errorf("URL has no bucket or object path")
	}
	prefix, err := canonicalJobPrefix(objectPath, runID)
	if err != nil {
		return "", "", err
	}
	return bucket, prefix, nil
}

// ProwJobRunExists reports whether id resolves through the partition-key map
// to a real, committed Prow job run and its job definition. An orphan mapping
// alone is deliberately not treated as an imported run.
func ProwJobRunExists(ctx context.Context, dbc *db.DB, id uint) (bool, error) {
	if dbc == nil {
		return false, fmt.Errorf("PostgreSQL is not configured")
	}
	var exists bool
	err := dbc.DB.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1
			FROM prow_job_run_id_map m
			JOIN prow_job_runs r ON r.id = m.id
				AND r.prow_job_release = m.prow_job_release
				AND r.timestamp = m.timestamp
			JOIN prow_jobs j ON j.id = r.prow_job_id
			WHERE m.id = ?
		)`, id).Scan(&exists).Error
	return exists, err
}

func (i *SingleRunImporter) readGCSArtifact(ctx context.Context, bucket, object string) ([]byte, error) {
	if i.gcsClient == nil {
		return nil, fmt.Errorf("storage client is not configured")
	}
	return gcs.NewGCSJobRun(i.gcsClient.Bucket(bucket), "").GetContent(ctx, object)
}

func (i *SingleRunImporter) loadGCSJUnit(ctx context.Context, bucket, prefix string) (*junit.TestSuites, []string, error) {
	if i.gcsClient == nil {
		return nil, nil, fmt.Errorf("storage client is not configured")
	}
	jobRun := gcs.NewGCSJobRun(i.gcsClient.Bucket(bucket), prefix+"/")
	paths, err := jobRun.FindAllMatches(ctx, gcs.GlobJunitXML)
	if err != nil {
		return nil, nil, err
	}
	jobRun.SetGCSJunitPaths(paths)
	suites, err := jobRun.GetCombinedJUnitTestSuites(ctx)
	return suites, paths, err
}

func (i *SingleRunImporter) findDefinition(ctx context.Context, jobName string) (*models.ProwJob, error) {
	if i.dbc == nil {
		return nil, fmt.Errorf("PostgreSQL is not configured")
	}
	var definition models.ProwJob
	query := i.dbc.DB.WithContext(ctx).Where("name = ?", jobName).Limit(1).Find(&definition)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected == 0 {
		return nil, nil
	}
	return &definition, nil
}

func classifyImportError(fallback SingleRunImportErrorKind, operation string, err error) error {
	kind := fallback
	if isDependencyUnavailable(err) {
		kind = SingleRunDependencyFailure
	}
	return &SingleRunImportError{Kind: kind, Err: fmt.Errorf("%s: %w", operation, err)}
}

func isDependencyUnavailable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 401 || apiErr.Code == 403 || apiErr.Code == 408 || apiErr.Code == 429 || apiErr.Code >= 500
	}
	switch status.Code(err) {
	case codes.Unauthenticated, codes.PermissionDenied, codes.ResourceExhausted, codes.DeadlineExceeded, codes.Unavailable:
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "08") || strings.HasPrefix(pgErr.Code, "53") || pgErr.Code == "57P01"
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not configured") || strings.Contains(message, "connection refused")
}
