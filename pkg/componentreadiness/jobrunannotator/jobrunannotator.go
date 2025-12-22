package jobrunannotator

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/api/jobartifacts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"google.golang.org/api/iterator"
)

const (
	jobAnnotationTable   = "job_labels"
	dedupedJunitTableFmt = `
		WITH deduped_testcases AS (
			SELECT
				junit.*,
				ROW_NUMBER() OVER(PARTITION BY file_path, test_name, testsuite ORDER BY
					CASE
						WHEN flake_count > 0 THEN 0
						WHEN success_val > 0 THEN 1
						ELSE 2
					END) AS row_num,
				CASE
					WHEN flake_count > 0 THEN 0
					ELSE success_val
				END AS adjusted_success_val,
				CASE
					WHEN flake_count > 0 THEN 1
					ELSE 0
				END AS adjusted_flake_count
			FROM
				%s.junit AS junit
			WHERE modified_time >= DATETIME(TIMESTAMP('%s'))
			AND modified_time < DATETIME(TIMESTAMP('%s'))
			AND skipped = false
		)
		SELECT * FROM deduped_testcases WHERE row_num = 1`
)

type JobRunAnnotator struct {
	bqClient         *bqclient.Client
	cacheOptions     cache.RequestOptions
	gcsClient        *storage.Client
	dbClient         *db.DB
	cache            cache.Cache
	execute          bool
	allVariants      crtest.JobVariants
	Release          string        `json:"release"`
	IncludedVariants []bq.Variant  `json:"included_variants"`
	Label            string        `json:"label"`
	BuildClusters    []string      `json:"build_clusters"`
	StartTime        time.Time     `json:"start_time"`
	Duration         time.Duration `json:"duration"`
	MinFailures      int           `json:"minimum_failure"`
	FlakeAsFailure   bool          `json:"flake_as_failure"`
	TextContains     string        `json:"text_contains"`
	TextRegex        string        `json:"text_regex"`
	PathGlob         string        `json:"path_glob"`
	JobRunIDs        []int64       `json:"job_run_ids"`
	comment          string
	user             string
}

func NewJobRunAnnotator(
	bqClient *bqclient.Client,
	cacheOptions cache.RequestOptions,
	gcsClient *storage.Client,
	dbClient *db.DB,
	cacheClient cache.Cache,
	execute bool,
	release string,
	allVariants crtest.JobVariants,
	variants []bq.Variant,
	label string,
	buildClusters []string,
	startTime time.Time,
	duration time.Duration,
	minFailures int,
	flakeAsFailure bool,
	textContains string,
	textRegex string,
	pathGlob string,
	jobRunIDs []int64,
	commentPrefix string,
	user string,
) (JobRunAnnotator, error) {

	j := JobRunAnnotator{
		bqClient:         bqClient,
		cacheOptions:     cacheOptions,
		gcsClient:        gcsClient,
		dbClient:         dbClient,
		cache:            cacheClient,
		execute:          execute,
		Release:          release,
		allVariants:      allVariants,
		IncludedVariants: variants,
		Label:            label,
		BuildClusters:    buildClusters,
		StartTime:        startTime,
		Duration:         duration,
		MinFailures:      minFailures,
		FlakeAsFailure:   flakeAsFailure,
		TextContains:     textContains,
		TextRegex:        textRegex,
		PathGlob:         pathGlob,
		JobRunIDs:        jobRunIDs,
		comment:          commentPrefix,
		user:             user,
	}
	if bqClient == nil || bqClient.BQ == nil {
		return j, fmt.Errorf("we don't have a bigquery client for job run annotator")
	}

	if bqClient.Cache == nil {
		return j, fmt.Errorf("we don't have a cache configured for job run annotator")
	}

	return j, nil
}

func (j JobRunAnnotator) Run(ctx context.Context) error {
	var err error
	log.Infof("Start annotating job runs")

	jobRuns, err := j.getJobRunsFromBigQuery(ctx)
	if err != nil {
		return fmt.Errorf("could not get job run IDs from BigQuery: %v", err)
	}
	log.Infof("Found %d job runs from BigQuery", len(jobRuns))

	jobRunIDs := maps.Keys(jobRuns)
	if len(j.PathGlob) != 0 {
		jobRunIDs, err = j.filterJobRunByArtifact(ctx, jobRunIDs)
		if err != nil {
			return fmt.Errorf("could not perform artifact search: %v", err)
		}
		log.Infof("Limit to %d job runs based on artifact search", len(jobRunIDs))
	}

	if len(jobRunIDs) != 0 {
		log.Infof("Attempting to annotate %d job runs", len(jobRunIDs))
		err = j.annotateJobRuns(ctx, jobRunIDs, jobRuns)
		if err != nil {
			return fmt.Errorf("error annotating job runs: %v", err)
		}
	}

	log.Infof("Done annotating job runs")
	return nil
}

type jobRun struct {
	ID        string         `bigquery:"prowjob_build_id"`
	StartTime civil.DateTime `bigquery:"prowjob_start"`
	URL       string         `bigquery:"prowjob_url"`
}

func (j JobRunAnnotator) getJobRunsFromBigQuery(ctx context.Context) (map[int64]jobRun, error) { //lint:ignore
	now := time.Now()
	var jobRuns map[int64]jobRun

	jobRunWhereStr := fmt.Sprintf("WHERE prowjob_start BETWEEN DATETIME(TIMESTAMP('%s')) AND DATETIME(TIMESTAMP('%s'))\n", j.StartTime.UTC().Format(time.RFC3339), j.StartTime.Add(j.Duration).UTC().Format(time.RFC3339))
	if len(j.JobRunIDs) != 0 {
		strIDs := []string{}
		for _, id := range j.JobRunIDs {
			strIDs = append(strIDs, fmt.Sprintf("'%d'", id))
		}
		jobRunWhereStr += fmt.Sprintf(" AND prowjob_build_id IN UNNEST([%s])\n", strings.Join(strIDs, ", "))
	}

	if len(j.BuildClusters) != 0 {
		clusterStrs := []string{}
		for _, cluster := range j.BuildClusters {
			clusterStrs = append(clusterStrs, fmt.Sprintf(" prowjob_cluster='%s' ", cluster))
		}
		jobRunWhereStr += fmt.Sprintf(" AND (%s)\n", strings.Join(clusterStrs, " OR "))
	}
	joinVariantsStr := ""
	filterVariantsStr := ""
	selectVariants := ""
	for v := range j.allVariants.Variants {
		joinVariantsStr += fmt.Sprintf("LEFT JOIN %s.job_variants jv_%s ON prowjob_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			j.bqClient.Dataset, v, v, v, v)
		selectVariants += fmt.Sprintf("jv_%s.variant_value AS variant_%s,\n", v, v)
	}
	for i, v := range j.IncludedVariants {
		if i == 0 {
			filterVariantsStr += fmt.Sprintf("WHERE variant_%s = '%s'\n", v.Key, v.Value)
		} else {
			filterVariantsStr += fmt.Sprintf(" AND variant_%s = '%s'\n", v.Key, v.Value)
		}
	}

	minimumFailureStr := ""
	if j.MinFailures != 0 {
		if j.FlakeAsFailure {
			minimumFailureStr += fmt.Sprintf("WHERE %d < total_count - success_count\n", j.MinFailures)
		} else {
			minimumFailureStr += fmt.Sprintf("WHERE %d < total_count - success_count - flake_count\n", j.MinFailures)
		}
	}
	dedupedJunitTable := fmt.Sprintf(dedupedJunitTableFmt, j.bqClient.Dataset, j.StartTime.UTC().Format(time.RFC3339), j.StartTime.Add(j.Duration).UTC().Format(time.RFC3339))

	junitWhereStr := ""
	if len(j.Release) != 0 {
		junitWhereStr += fmt.Sprintf("WHERE branch='%s'\n", j.Release)
	}
	// The query is built with different level of filters:
	// 1. Filters that exist in jobs table like prowjob_cluster
	// 2. Filters in other tables to be joined like variants
	queryStr := fmt.Sprintf(`WITH filtered_job_runs AS (
		SELECT
			jobs.*
		FROM %s.jobs
		%s
	),
	filtered_job_runs_with_all_variants AS (
		SELECT
			%s
			filtered_job_runs.*
		FROM filtered_job_runs
		%s
	),
	filtered_job_runs_with_filtered_variants AS (
		SELECT
			*
		FROM filtered_job_runs_with_all_variants
		%s
	),
	candidate_job_runs AS (
		SELECT
			prowjob_build_id AS job_run_id,
			prowjob_start,
			prowjob_url,
		FROM filtered_job_runs_with_filtered_variants
	),
	candidate_job_run_stats AS (
		SELECT
			prowjob_build_id,
			ANY_VALUE(jobs.prowjob_url) as prowjob_url,
			ANY_VALUE(jobs.prowjob_start) as prowjob_start,
			COUNT(prowjob_build_id) AS total_count,
			SUM(adjusted_success_val) AS success_count,
			SUM(adjusted_flake_count) AS flake_count,
		FROM (%s)
		INNER JOIN candidate_job_runs jobs ON
			prowjob_build_id = jobs.job_run_id
		%s
		GROUP BY prowjob_build_id
	)
	SELECT
		prowjob_build_id,
		prowjob_start,
		prowjob_url,
	FROM candidate_job_run_stats
	%s
	`, j.bqClient.Dataset, jobRunWhereStr, selectVariants, joinVariantsStr, filterVariantsStr, dedupedJunitTable, junitWhereStr, minimumFailureStr)

	q := j.bqClient.BQ.Query(queryStr)
	jobRuns, errs := fetchJobRunsFromBQ(ctx, q)
	if len(errs) > 0 {
		return jobRuns, errs[0]
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{
		"elapsed": elapsed,
		"reports": len(jobRuns),
	}).Debug("getJobRunsFromBigQuery completed")

	return jobRuns, nil
}

func fetchJobRunsFromBQ(ctx context.Context, q *bigquery.Query) (map[int64]jobRun, []error) {
	errs := []error{}
	result := make(map[int64]jobRun)
	log.Infof("Fetching job runs with:\n%s\n", q.Q)

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying job runs from bigquery")
		errs = append(errs, err)
		return result, errs
	}

	for {
		row := jobRun{}
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing job run from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing job run from bigquery"))
			continue
		}
		id, err := strconv.ParseInt(row.ID, 10, 64)
		if err != nil {
			log.WithError(err).Errorf("error parsing job run ID %s from bigquery", row.ID)
			errs = append(errs, errors.Wrap(err, "error parsing job run IDs from bigquery"))
		} else {
			result[id] = row
		}
	}
	return result, errs
}

func (j JobRunAnnotator) getContentMatcher() (jobartifacts.ContentMatcher, error) {
	contextBefore := 0
	contextAfter := 0
	maxMatches := 1
	var contentMatcher jobartifacts.ContentMatcher
	if j.TextContains != "" {
		contentMatcher = jobartifacts.NewStringMatcher(j.TextContains, contextBefore, contextAfter, maxMatches)
	} else if j.TextRegex != "" {
		re, err := regexp.Compile(j.TextRegex)
		if err != nil {
			return nil, fmt.Errorf("error parsing regex: %q", j.TextRegex)
		}
		contentMatcher = jobartifacts.NewRegexMatcher(re, contextBefore, contextAfter, maxMatches)
	}

	return contentMatcher, nil // nil matcher means don't bother reading the artifacts, just return metadata without any matching
}

func (j JobRunAnnotator) filterJobRunByArtifact(ctx context.Context, jobRunIDs []int64) ([]int64, error) {
	resultIDs := []int64{}
	artifactMananger := jobartifacts.NewManager(ctx)
	contentMatcher, err := j.getContentMatcher()
	if err != nil {
		return jobRunIDs, err
	}

	q := &jobartifacts.JobArtifactQuery{
		GcsBucket:      j.gcsClient.Bucket(util.GcsBucketRoot),
		DbClient:       j.dbClient,
		Cache:          j.cache,
		JobRunIDs:      []int64{},
		ContentMatcher: contentMatcher,
		PathGlob:       j.PathGlob,
	}
	q.JobRunIDs = append(q.JobRunIDs, jobRunIDs...)

	// The query looks good to run
	result := artifactMananger.Query(ctx, q)
	// But there's one user input we can't validate without querying: PathGlob. Look for that error and treat it as a bad request.
	if len(result.JobRuns) == 0 && len(result.Errors) > 0 && strings.HasPrefix(result.Errors[0].Error, "googleapi: Error 400: Glob pattern") {
		// the pattern is built differently per run, but a single failure should be representative for all
		return resultIDs, fmt.Errorf("invalid pattern according to %s", result.Errors[0].Error)
	}
	for _, jobRun := range result.JobRuns {
		for _, a := range jobRun.Artifacts {
			if a.ContentLineMatches != nil {
				id, err := strconv.ParseInt(a.JobRunID, 10, 64)
				if err != nil {
					log.WithError(err).Errorf("error parsing job run ID %s", a.JobRunID)
				} else {
					resultIDs = append(resultIDs, id)
				}
			}
		}

	}
	return resultIDs, nil
}

// bulkInsertVariants inserts all new job variants in batches.
func (j JobRunAnnotator) bulkInsertJobRunAnnotations(ctx context.Context, inserts []models.JobRunLabel) error {
	var batchSize = 500

	if !j.execute {
		jobsStr := ""
		for _, jobRun := range inserts {
			jobsStr += fmt.Sprintf("StartTime: %v; URL: %s\n", jobRun.StartTime, jobRun.URL)
		}
		log.Infof("\n===========================================================\nDry run mode enabled\nBulk inserting\n%s\n\nTo write the label to DB, please use --execute argument\n", jobsStr)
		return nil
	}

	table := j.bqClient.BQ.Dataset(j.bqClient.Dataset).Table(jobAnnotationTable)
	inserter := table.Inserter()
	for i := 0; i < len(inserts); i += batchSize {
		end := i + batchSize
		if end > len(inserts) {
			end = len(inserts)
		}

		if err := inserter.Put(ctx, inserts[i:end]); err != nil {
			return err
		}
		log.Infof("added %d new job label rows", end-i)
	}

	return nil
}

func (j JobRunAnnotator) generateComment() string {
	comment := j.comment
	annotatorComment, err := json.MarshalIndent(j, "", "    ")
	if err == nil {
		comment += fmt.Sprintf("\nAnnotator\n%s", annotatorComment)
	}
	return comment
}

func (j JobRunAnnotator) getJobRunAnnotationsFromBigQuery(ctx context.Context) (map[int64]models.JobRunLabel, error) {
	now := time.Now()
	queryStr := fmt.Sprintf(`
		SELECT
			*
		FROM %s.%s
		WHERE prowjob_start BETWEEN DATETIME(TIMESTAMP('%s')) AND DATETIME(TIMESTAMP('%s'))
		`,
		j.bqClient.Dataset, jobAnnotationTable, j.StartTime.UTC().Format(time.RFC3339), j.StartTime.Add(j.Duration).UTC().Format(time.RFC3339))

	q := j.bqClient.BQ.Query(queryStr)

	errs := []error{}
	result := make(map[int64]models.JobRunLabel)
	log.Debugf("Fetching job run annotations with:\n%s\n", q.Q)

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying job run annotations from bigquery")
		return result, err
	}

	for {
		row := models.JobRunLabel{}
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing job run annotation from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing job run annotation from bigquery"))
			continue
		}
		id, err := strconv.ParseInt(row.ID, 10, 64)
		if err != nil {
			log.WithError(err).Errorf("error parsing job run ID %s from bigquery", row.ID)
			errs = append(errs, errors.Wrap(err, "error parsing job run IDs from bigquery"))
		} else {
			result[id] = row
		}
	}

	if len(errs) > 0 {
		return result, errs[0]
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{
		"elapsed": elapsed,
		"reports": len(result),
	}).Debug("getJobRunAnnotationsFromBigQuery completed")

	return result, nil
}

func (j JobRunAnnotator) annotateJobRuns(ctx context.Context, jobRunIDs []int64, jobRuns map[int64]jobRun) error {
	jobRunAnnotations := make([]models.JobRunLabel, 0, len(jobRunIDs))
	existingAnnotations, err := j.getJobRunAnnotationsFromBigQuery(ctx)
	if err != nil {
		return err
	}
	log.Infof("Found %d existing job run annotations.", len(existingAnnotations))
	now := civil.DateTimeOf(time.Now())
	for _, jobRunID := range jobRunIDs {
		if jobRun, ok := jobRuns[jobRunID]; ok {
			// Skip if the same label already exists
			if annotation, existing := existingAnnotations[jobRunID]; existing && annotation.Label == j.Label {
				continue
			}
			jobRunAnnotations = append(jobRunAnnotations, models.JobRunLabel{
				ID:         jobRun.ID,
				StartTime:  jobRun.StartTime,
				Label:      j.Label,
				Comment:    j.generateComment(),
				User:       j.user,
				CreatedAt:  now,
				UpdatedAt:  now,
				SourceTool: "sippy annotate-job-runs",
				SymptomID:  "", // Empty for manual annotations; will be populated when symptom detection is implemented
				URL:        jobRun.URL,
			})
		}
	}
	log.Infof("Going to annotate %d new job runs", len(jobRunAnnotations))
	return j.bulkInsertJobRunAnnotations(ctx, jobRunAnnotations)
}
