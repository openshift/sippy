package jobrunannotator

import (
	"cloud.google.com/go/civil"
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/exp/maps"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/api/jobartifacts"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const (
	jobAnnotationTable   = "job_annotations"
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
	dryRun           bool
	Release          string
	allVariants      crtype.JobVariants
	IncludedVariants []crtype.Variant
	Label            string
	BuildCluster     string
	StartTime        time.Time
	Duration         time.Duration
	MinimumFailure   int
	FlakeAsFailure   bool
	TextContains     string
	TextRegex        string
	PathGlob         string
	JobRunIDs        []int64
	remark           string
}

func NewJobRunAnnotator(
	bqClient *bqclient.Client,
	cacheOptions cache.RequestOptions,
	gcsClient *storage.Client,
	dbClient *db.DB,
	cache cache.Cache,
	dryRun bool,
	release string,
	allVariants crtype.JobVariants,
	Variants []crtype.Variant,
	Label string,
	BuildCluster string,
	StartTime time.Time,
	Duration time.Duration,
	MinimumFailure int,
	flakeAsFailure bool,
	textContains string,
	textRegex string,
	pathGlob string,
	jobRunIDs []int64,
	remarkPrefix string,
) (JobRunAnnotator, error) {

	j := JobRunAnnotator{
		bqClient:         bqClient,
		cacheOptions:     cacheOptions,
		gcsClient:        gcsClient,
		dbClient:         dbClient,
		cache:            cache,
		dryRun:           dryRun,
		Release:          release,
		allVariants:      allVariants,
		IncludedVariants: Variants,
		Label:            Label,
		BuildCluster:     BuildCluster,
		StartTime:        StartTime,
		Duration:         Duration,
		MinimumFailure:   MinimumFailure,
		FlakeAsFailure:   flakeAsFailure,
		TextContains:     textContains,
		TextRegex:        textRegex,
		PathGlob:         pathGlob,
		JobRunIDs:        jobRunIDs,
		remark:           remarkPrefix,
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
		log.Infof("Limit to %d job runs based on artifact search", len(jobRuns))
	}

	if len(jobRunIDs) != 0 {
		log.Infof("Going to annotate %d job runs", len(jobRuns))
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
}

type jobRunAnnotation struct {
	jobRun
	Label  string `bigquery:"label"`
	Remark string `bigquery:"remark"`
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

	if len(j.BuildCluster) != 0 {
		jobRunWhereStr += fmt.Sprintf(" AND prowjob_cluster='%s'\n", j.BuildCluster)
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
	if j.MinimumFailure != 0 {
		if j.FlakeAsFailure {
			minimumFailureStr += fmt.Sprintf("WHERE %d < total_count - success_count\n", j.MinimumFailure)
		} else {
			minimumFailureStr += fmt.Sprintf("WHERE %d < total_count - success_count - flake_count\n", j.MinimumFailure)
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
		FROM filtered_job_runs_with_filtered_variants
	),
	candidate_job_run_stats AS (
		SELECT
			prowjob_build_id,
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
			if a.MatchedContent.ContentLineMatches != nil {
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
func (j JobRunAnnotator) bulkInsertJobRunAnnotations(ctx context.Context, inserts []jobRunAnnotation) error {
	var batchSize = 500

	if j.dryRun {
		log.Infof("\n===========================================================\nDry run mode enabled\nBulk inserting\n%+v", inserts)
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
		log.Infof("added %d new job annotation rows", end-i)
	}

	return nil
}

func (j JobRunAnnotator) generateRemark() string {
	remark := j.remark
	annotatorRemark, err := json.MarshalIndent(j, "", "    ")
	if err == nil {
		remark += fmt.Sprintf("\nAnnotator\n%s", annotatorRemark)
	}
	return remark
}

func (j JobRunAnnotator) annotateJobRuns(ctx context.Context, jobRunIDs []int64, jobRuns map[int64]jobRun) error {
	jobRunAnnotations := make([]jobRunAnnotation, 0, len(jobRunIDs))
	for _, jobRunID := range jobRunIDs {
		if jobRun, ok := jobRuns[jobRunID]; ok {
			jobRunAnnotations = append(jobRunAnnotations, jobRunAnnotation{jobRun, j.Label, j.generateRemark()})
		}
	}
	return j.bulkInsertJobRunAnnotations(ctx, jobRunAnnotations)
}
