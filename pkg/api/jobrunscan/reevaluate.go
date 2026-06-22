package jobrunscan

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"k8s.io/apimachinery/pkg/util/sets"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/api/jobartifacts"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	jobrunannotator "github.com/openshift/sippy/pkg/componentreadiness/jobrunannotator"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"gorm.io/gorm"
)

const (
	reEvalSourceTool = "sippy-api-reevaluate"
	maxJobRunsPerReq = 50
)

// ReEvalStatus indicates the outcome of re-evaluating symptoms for a single job run.
type ReEvalStatus string

const (
	ReEvalSuccess      ReEvalStatus = "success"
	ReEvalMissingError ReEvalStatus = "missing_error"
	ReEvalEvalError    ReEvalStatus = "eval_error"
	ReEvalRewriteError ReEvalStatus = "rewrite_error"
)

// ReEvaluationResult reports what happened for a single job run.
type ReEvaluationResult struct {
	ProwJobBuildID      string            `json:"prow_job_build_id"`
	Status              ReEvalStatus      `json:"status"`
	SymptomsEvaluated   int               `json:"symptoms_evaluated,omitempty"`
	SymptomsMatched     []string          `json:"symptoms_matched,omitempty"`
	LabelsApplied       []string          `json:"labels_applied,omitempty"`
	BQEntriesWritten    int               `json:"bq_entries_written,omitempty"`
	GCSArtifactsWritten int               `json:"gcs_artifacts_written,omitempty"`
	PostgresUpdated     bool              `json:"postgres_updated,omitempty"`
	Error               string            `json:"error,omitempty"`
	Links               map[string]string `json:"links,omitempty"`
}

// ReEvaluationResponse wraps the results with HATEOAS links.
type ReEvaluationResponse struct {
	Results []ReEvaluationResult `json:"results"`
	Links   map[string]string    `json:"links"`
}

// InjectReEvalHATEOASLinks populates HATEOAS links on the response and each result.
func InjectReEvalHATEOASLinks(resp *ReEvaluationResponse, baseURL string) {
	resp.Links = map[string]string{
		"self": baseURL + "/api/jobs/runs/reevaluate",
	}
	for i := range resp.Results {
		r := &resp.Results[i]
		if r.Links == nil {
			continue
		}
		for k, v := range r.Links {
			if k == "job_run" {
				continue // already absolute URL
			}
			// symptom links need the base URL prefix
			if strings.HasPrefix(k, "symptom:") {
				r.Links[k] = baseURL + v
			}
		}
	}
}

// ReEvaluator re-runs symptom detection against job artifacts and updates BQ, GCS, and PostgreSQL.
type ReEvaluator struct {
	bqClient    *bqclient.Client
	gcsClient   *storage.Client
	gcsBucket   string
	db          *db.DB
	cache       cache.Cache
	artifactMgr *jobartifacts.Manager
	dryRun      bool
}

// NewReEvaluator creates a ReEvaluator with the given clients.
func NewReEvaluator(bqClient *bqclient.Client, gcsClient *storage.Client, gcsBucket string, dbc *db.DB, cacheClient cache.Cache, artifactMgr *jobartifacts.Manager, dryRun bool) *ReEvaluator {
	return &ReEvaluator{
		bqClient:    bqClient,
		gcsClient:   gcsClient,
		gcsBucket:   gcsBucket,
		db:          dbc,
		cache:       cacheClient,
		artifactMgr: artifactMgr,
		dryRun:      dryRun,
	}
}

// symptomMatch records that a symptom matched a file/text in a job run.
type symptomMatch struct {
	symptom   jobrunscan.Symptom
	fileMatch string // path relative to bucket root
	textMatch string // first matched line (empty for "none" matcher)
}

// ReEvaluateJobRuns re-evaluates all symptom matches for the specified job runs.
func (r *ReEvaluator) ReEvaluateJobRuns(ctx context.Context, prowJobBuildIDs []string) ([]ReEvaluationResult, error) {
	symptoms, err := r.loadActiveSymptoms()
	if err != nil {
		return nil, fmt.Errorf("loading symptoms: %w", err)
	}
	log.Debugf("symptom reEval: loaded %d active symptoms", len(symptoms))

	results := make([]ReEvaluationResult, 0, len(prowJobBuildIDs))
	for _, buildID := range prowJobBuildIDs {
		result := r.reEvaluateOne(ctx, buildID, symptoms)
		results = append(results, result)
	}
	return results, nil
}

// loadActiveSymptoms fetches all symptom definitions with implemented matcher types.
func (r *ReEvaluator) loadActiveSymptoms() ([]jobrunscan.Symptom, error) {
	var all []jobrunscan.Symptom
	res := r.db.DB.Order("id").Find(&all)
	if res.Error != nil {
		return nil, res.Error
	}
	return filterRelevantSymptoms(all), nil
}

// filterRelevantSymptoms returns only symptoms with labels and implemented matcher types (string, regex, none).
func filterRelevantSymptoms(symptoms []jobrunscan.Symptom) []jobrunscan.Symptom {
	var filtered []jobrunscan.Symptom
	for _, s := range symptoms {
		if len(s.LabelIDs) == 0 {
			continue // no value in matching symptoms that don't apply labels
		}
		switch s.MatcherType {
		case jobrunscan.MatcherTypeString, jobrunscan.MatcherTypeRegex, jobrunscan.MatcherTypeFile:
			filtered = append(filtered, s)
		case jobrunscan.MatcherTypeCEL:
			log.Warnf("symptom reEval: skipping symptom %q with unimplemented matcher_type %q (TRT-2466)", s.ID, s.MatcherType)
		default:
			log.Warnf("symptom reEval: skipping symptom %q with unknown matcher_type %q", s.ID, s.MatcherType)
		}
	}
	return filtered
}

// reEvaluateOne processes a single job run through all symptoms.
func (r *ReEvaluator) reEvaluateOne(ctx context.Context, buildID string, symptoms []jobrunscan.Symptom) ReEvaluationResult {
	result := ReEvaluationResult{
		ProwJobBuildID:    buildID,
		SymptomsEvaluated: len(symptoms),
	}

	jobRunID, err := strconv.ParseInt(buildID, 10, 64)
	if err != nil {
		result.Status = ReEvalEvalError
		result.Error = fmt.Sprintf("invalid build ID %q: %v", buildID, err)
		return result
	}

	// Look up the job run to get metadata
	jobRunModel := new(models.ProwJobRun)
	res := r.db.DB.First(jobRunModel, jobRunID)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			result.Status = ReEvalMissingError
			result.Error = fmt.Sprintf("job run %s not found in database", buildID)
		} else {
			result.Status = ReEvalEvalError
			result.Error = fmt.Sprintf("looking up job run %s: %v", buildID, res.Error)
		}
		return result
	}

	matches, err := r.evaluateSymptoms(ctx, jobRunID, symptoms)
	if err != nil {
		result.Status = ReEvalEvalError
		result.Error = err.Error()
		return result
	}
	result.SymptomsMatched = uniqueSymptomsMatched(matches)

	result.Links = map[string]string{"job_run": jobRunModel.URL}
	for _, m := range matches {
		result.Links["symptom:"+m.symptom.ID] = "/api/jobs/symptoms/" + m.symptom.ID
	}

	// Compute the label set from matches
	bqLabels, bucketLabels, err := r.buildOutputs(matches, buildID, jobRunModel)
	if err != nil {
		result.Status = ReEvalEvalError
		result.Error = err.Error()
		return result
	}
	result.LabelsApplied = uniqueLabels(bqLabels)

	if r.dryRun {
		log.Infof("symptom re-evaluation dry run for %s: %d symptoms matched, %d BQ labels, %d GCS artifacts",
			buildID, len(result.SymptomsMatched), len(bqLabels), len(bucketLabels))
		result.Status = ReEvalSuccess
		return result
	}

	// Clear existing symptom-originated data, then write new results
	if err := r.clearAndWrite(ctx, buildID, jobRunModel, bqLabels, bucketLabels); err != nil {
		result.Status = ReEvalRewriteError
		result.Error = err.Error()
		return result
	}

	result.BQEntriesWritten = len(bqLabels)
	result.GCSArtifactsWritten = len(bucketLabels)

	// Update PostgreSQL labels
	if err := r.updatePostgresLabels(ctx, buildID, jobRunModel, bqLabels); err != nil {
		result.Status = ReEvalRewriteError
		result.Error = fmt.Sprintf("postgres update failed: %v", err)
		return result
	}
	result.PostgresUpdated = true

	result.Status = ReEvalSuccess
	return result
}

// evaluateSymptoms runs one artifact query per symptom for the given job run.
func (r *ReEvaluator) evaluateSymptoms(ctx context.Context, jobRunID int64, symptoms []jobrunscan.Symptom) ([]symptomMatch, error) {
	var matches []symptomMatch
	mgr := r.artifactMgr

	for _, symptom := range symptoms {
		contentMatcher, err := ContentMatcherForSymptom(symptom.SymptomContent)
		if err != nil {
			log.WithError(err).Warnf("symptom reEval: skipping symptom %q due to matcher error", symptom.ID)
			continue
		}

		q := &jobartifacts.JobArtifactQuery{
			GcsBucket:      r.gcsClient.Bucket(util.GcsBucketRoot),
			DbClient:       r.db,
			Cache:          r.cache,
			JobRunIDs:      []int64{jobRunID},
			PathGlob:       symptom.FilePattern,
			ContentMatcher: contentMatcher,
		}

		queryResult := mgr.Query(ctx, q)
		for _, jr := range queryResult.JobRuns {
			for _, a := range jr.Artifacts {
				if a.Error != "" || a.TimedOut {
					continue
				}
				matched := false
				textMatch := ""
				if contentMatcher == nil {
					// "none" matcher: file existence is a match
					matched = true
				} else if text, ok := a.Matched(); ok {
					matched = true
					textMatch = strings.TrimRight(text, "\n")
				}
				if matched {
					matches = append(matches, symptomMatch{
						symptom:   symptom,
						fileMatch: a.ArtifactPath,
						textMatch: textMatch,
					})
				}
			}
		}

		for _, qErr := range queryResult.Errors {
			if qErr.TimedOut {
				return nil, fmt.Errorf("artifact scan timed out for symptom %q", symptom.ID)
			}
			return nil, fmt.Errorf("artifact scan error for symptom %q: %s", symptom.ID, qErr.Error)
		}
	}

	return matches, nil
}

// ContentMatcherForSymptom creates the appropriate ContentMatcher for a symptom definition.
// Returns nil for file-existence-only matcher types (no content matching needed).
func ContentMatcherForSymptom(symptom jobrunscan.SymptomContent) (jobartifacts.ContentMatcher, error) {
	switch symptom.MatcherType {
	case jobrunscan.MatcherTypeString:
		return jobartifacts.NewStringMatcher(symptom.MatchString, 0, 0, 1), nil
	case jobrunscan.MatcherTypeRegex:
		re, err := regexp.Compile(symptom.MatchString)
		if err != nil {
			return nil, fmt.Errorf("invalid regex for symptom %q: %w", symptom.ID, err)
		}
		return jobartifacts.NewRegexMatcher(re, 0, 0, 1), nil
	case jobrunscan.MatcherTypeFile:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported matcher type %q for symptom %q", symptom.MatcherType, symptom.ID)
	}
}

// buildOutputs creates the BQ labels and GCS bucket labels from symptom matches.
func (r *ReEvaluator) buildOutputs(matches []symptomMatch, buildID string, jobRun *models.ProwJobRun) ([]models.JobRunLabel, []jobrunannotator.JobRunBucketLabel, error) {
	now := civil.DateTimeOf(time.Now())
	startTime := civil.DateTimeOf(jobRun.Timestamp)
	jobRunPath := jobRunPathFromURL(jobRun.URL)

	var bqLabels []models.JobRunLabel
	var bucketLabels []jobrunannotator.JobRunBucketLabel

	labelDefs, err := r.loadLabelDefinitions(matches)
	if err != nil {
		return nil, nil, fmt.Errorf("loading label definitions: %w", err)
	}

	for _, m := range matches {
		for _, labelID := range m.symptom.LabelIDs {
			bqLabels = append(bqLabels, models.JobRunLabel{
				ID:         buildID,
				StartTime:  startTime,
				Label:      labelID,
				SourceTool: reEvalSourceTool,
				SymptomID:  m.symptom.ID,
				CreatedAt:  now,
				UpdatedAt:  now,
			})

			labelContent := jobrunscan.LabelContent{ID: labelID}
			if def, ok := labelDefs[labelID]; ok {
				labelContent = def
			}
			bucketLabels = append(bucketLabels, jobrunannotator.JobRunBucketLabel{
				Symptom:    m.symptom.SymptomContent,
				Label:      labelContent,
				FileMatch:  m.fileMatch,
				TextMatch:  m.textMatch,
				Bucket:     r.gcsBucket,
				JobRunPath: jobRunPath,
			})
		}
	}

	return bqLabels, bucketLabels, nil
}

// loadLabelDefinitions fetches label content for the labels referenced by matched symptoms.
func (r *ReEvaluator) loadLabelDefinitions(matches []symptomMatch) (map[string]jobrunscan.LabelContent, error) {
	ids := sets.NewString()
	for _, m := range matches {
		ids.Insert(m.symptom.LabelIDs...)
	}
	if ids.Len() == 0 || r.db == nil {
		return nil, nil
	}

	var labels []jobrunscan.Label
	if err := r.db.DB.Where("id IN ?", ids.List()).Find(&labels).Error; err != nil {
		return nil, fmt.Errorf("querying label definitions for %v: %w", ids.List(), err)
	}

	result := make(map[string]jobrunscan.LabelContent, len(labels))
	for _, l := range labels {
		result[l.ID] = l.LabelContent
	}
	return result, nil
}

// clearAndWrite clears existing symptom-originated data in BQ and GCS, then writes new results.
func (r *ReEvaluator) clearAndWrite(ctx context.Context, buildID string, jobRun *models.ProwJobRun,
	bqLabels []models.JobRunLabel, bucketLabels []jobrunannotator.JobRunBucketLabel) error {

	// Clear BQ
	if err := r.clearBQLabels(ctx, buildID, jobRun.Timestamp); err != nil {
		return fmt.Errorf("clearing BQ labels: %w", err)
	}

	// Clear GCS
	jobRunPath := jobRunPathFromURL(jobRun.URL)
	if jobRunPath != "" {
		if err := r.clearGCSLabels(ctx, jobRunPath); err != nil {
			return fmt.Errorf("clearing GCS labels: %w", err)
		}
	}

	// Write BQ
	if len(bqLabels) > 0 {
		if err := jobrunannotator.BulkInsertJobRunLabels(ctx, r.bqClient.BQ, r.bqClient.Dataset, "job_labels", bqLabels, 500); err != nil {
			return fmt.Errorf("writing BQ labels: %w", err)
		}
	}

	// Write GCS
	for _, bl := range bucketLabels {
		if err := bl.WriteJSONToBucket(ctx, r.gcsClient); err != nil {
			return fmt.Errorf("writing GCS label artifact: %w", err)
		}
	}

	// Write HTML summary
	if len(bucketLabels) > 0 && jobRunPath != "" {
		bucket := r.gcsClient.Bucket(r.gcsBucket)
		if _, err := jobrunannotator.WriteHTMLSummaryToBucket(ctx, bucket, jobRunPath); err != nil {
			return fmt.Errorf("writing GCS HTML summary: %w", err)
		}
	}

	return nil
}

// clearBQLabels deletes existing symptom-originated labels from BQ for the given build ID.
func (r *ReEvaluator) clearBQLabels(ctx context.Context, buildID string, startTime time.Time) error {
	table := fmt.Sprintf("`%s.job_labels`", r.bqClient.Dataset)
	queryStr := fmt.Sprintf(
		`DELETE FROM %s
		WHERE prowjob_build_id = @buildID
		AND symptom_id IS NOT NULL AND symptom_id != ''
		AND DATE(prowjob_start) = @startDate`,
		table)

	q := r.bqClient.Query(ctx, bqlabel.JobRunLabelsReEvaluateDelete, queryStr)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "buildID", Value: buildID},
		{Name: "startDate", Value: civil.DateOf(startTime.UTC())},
	}
	_, err := q.Read(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "streaming buffer") {
			log.Warnf("symptom reEval: BQ delete for %s hit streaming buffer, skipping: %v", buildID, err)
			return nil
		}
		return fmt.Errorf("BQ delete for %s: %w", buildID, err)
	}
	log.Debugf("symptom reEval: cleared BQ symptom labels for build %s", buildID)
	return nil
}

// clearGCSLabels removes all existing label JSON files and the HTML summary from GCS.
func (r *ReEvaluator) clearGCSLabels(ctx context.Context, jobRunPath string) error {
	bucket := r.gcsClient.Bucket(r.gcsBucket)
	labelDir := jobRunPath + jobrunannotator.BucketLabelsPrefix

	it := bucket.Objects(ctx, &storage.Query{
		Prefix: labelDir,
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("listing GCS label objects: %w", err)
		}
		if err := bucket.Object(attrs.Name).Delete(ctx); err != nil && err != storage.ErrObjectNotExist {
			return fmt.Errorf("deleting GCS object %s: %w", attrs.Name, err)
		}
	}
	log.Debugf("symptom reEval: cleared GCS label artifacts for %s", jobRunPath)
	return nil
}

// updatePostgresLabels updates the labels array on prow_job_runs (and release_job_runs if applicable).
func (r *ReEvaluator) updatePostgresLabels(ctx context.Context, buildID string, jobRun *models.ProwJobRun, newBQLabels []models.JobRunLabel) error {
	// Query BQ for existing non-symptom labels for this build ID
	manualLabels, err := r.queryNonSymptomLabels(ctx, buildID, jobRun.Timestamp)
	if err != nil {
		log.WithError(err).Warnf("symptom reEval: could not query non-symptom labels from BQ for %s, using existing PG labels as fallback", buildID)
		// Fallback: keep existing labels that aren't from symptoms
		// We can't distinguish these in PG alone, so keep all existing labels
		manualLabels = jobRun.Labels
	}

	// Merge manual labels with new symptom labels
	merged := pq.StringArray(mergeLabels(manualLabels, newBQLabels))

	// Update prow_job_runs
	if err := r.db.DB.Model(&models.ProwJobRun{}).Where("id = ?", jobRun.ID).
		Update("labels", merged).Error; err != nil {
		return fmt.Errorf("updating prow_job_runs.labels: %w", err)
	}

	// Check and update release_job_runs if this job run exists there
	var releaseJobRun models.ReleaseJobRun
	if err := r.db.DB.Where("prow_job_run_id = ?", jobRun.ID).First(&releaseJobRun).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("looking up release_job_runs for build %s: %w", buildID, err)
		}
	} else {
		if err := r.db.DB.Model(&releaseJobRun).Update("labels", merged).Error; err != nil {
			return fmt.Errorf("updating release_job_runs.labels: %w", err)
		}
	}

	log.Debugf("symptom reEval: updated PostgreSQL labels for build %s: %v", buildID, merged)
	return nil
}

// queryNonSymptomLabels queries BQ for labels that were NOT applied by symptom detection.
func (r *ReEvaluator) queryNonSymptomLabels(ctx context.Context, buildID string, startTime time.Time) ([]string, error) {
	table := fmt.Sprintf("`%s.job_labels`", r.bqClient.Dataset)
	queryStr := fmt.Sprintf(
		`SELECT DISTINCT label FROM %s
		WHERE prowjob_build_id = @buildID
		AND (symptom_id IS NULL OR symptom_id = '')
		AND DATE(prowjob_start) >= @startDate`,
		table)

	q := r.bqClient.Query(ctx, bqlabel.JobRunLabelsReEvaluate, queryStr)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "buildID", Value: buildID},
		{Name: "startDate", Value: civil.DateOf(startTime.UTC())},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}

	var labels []string
	for {
		var row struct {
			Label string `bigquery:"label"`
		}
		if err := it.Next(&row); err != nil {
			if err == iterator.Done {
				break
			}
			return labels, err
		}
		labels = append(labels, row.Label)
	}
	return labels, nil
}

// mergeLabels combines manual labels with newly applied symptom labels, deduplicating.
func mergeLabels(manualLabels []string, bqLabels []models.JobRunLabel) []string {
	merged := sets.NewString()
	for _, l := range manualLabels {
		merged.Insert(l)
	}
	for _, bl := range bqLabels {
		merged.Insert(bl.Label)
	}
	return merged.List()
}

// jobRunPathFromURL extracts the GCS path from a ProwJobRun URL.
func jobRunPathFromURL(url string) string {
	const marker = "/" + util.GcsBucketRoot + "/"
	pathStart := strings.Index(url, marker)
	if pathStart == -1 {
		return ""
	}
	path := url[pathStart+len(marker):]
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

// uniqueSymptomsMatched returns the sorted distinct symptom IDs from the matches.
func uniqueSymptomsMatched(matches []symptomMatch) []string {
	s := sets.NewString()
	for _, m := range matches {
		s.Insert(m.symptom.ID)
	}
	return s.List()
}

// uniqueLabels returns the unique label IDs from a set of BQ labels.
func uniqueLabels(labels []models.JobRunLabel) []string {
	s := sets.NewString()
	for _, l := range labels {
		s.Insert(l.Label)
	}
	return s.List()
}

// ValidateReEvalRequest validates the re-evaluation request parameters.
func ValidateReEvalRequest(prowJobBuildIDs []string) error {
	if len(prowJobBuildIDs) == 0 {
		return fmt.Errorf("prow_job_build_ids is required")
	}
	if len(prowJobBuildIDs) > maxJobRunsPerReq {
		return fmt.Errorf("maximum %d job runs per request", maxJobRunsPerReq)
	}
	for _, id := range prowJobBuildIDs {
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			return fmt.Errorf("invalid prow_job_build_id %q: must be a numeric string", id)
		}
	}
	return nil
}
