package prowloader

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/openshift/sippy/pkg/apis/prow"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/bqjunit"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/testconversion"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/types"
	"github.com/openshift/sippy/pkg/testidentification"
	log "github.com/sirupsen/logrus"
)

// HistoricalJob identifies a Prow run and the metadata needed to build its
// complete raw database representation before persistence.
type HistoricalJob struct {
	ProwJob prow.ProwJob
	Run     pgwriter.RunRow
}

// BigQueryClient exposes the configured read client to the one-off backfill
// command so it can reuse the bounded JUnit query implementation.
func (pl *ProwLoader) BigQueryClient() *bqcachedclient.Client { return pl.bigQueryClient }

// EnsureHistoricalPartitions creates all partitions needed for an explicit
// historical range before raw rows are written.
func (pl *ProwLoader) EnsureHistoricalPartitions(start, end time.Time) error {
	if !start.Before(end) {
		return fmt.Errorf("historical partition start must be before end")
	}
	if len(pl.releases) == 0 {
		return fmt.Errorf("no configured releases available for historical partitions")
	}
	count, err := pl.dbc.EnsurePartitions(pl.releases, start, end, false)
	if err != nil {
		return fmt.Errorf("ensuring historical partitions: %w", err)
	}
	log.WithFields(log.Fields{"releases": len(pl.releases), "start": start, "end": end, "partitions": count}).Info("historical partitions ensured")
	return nil
}

// HistoricalJobCandidates returns new Prow runs in an explicit historical
// window after applying the database-backed build-ID idempotency check. The
// returned rows have complete metadata but are not written yet.
func (pl *ProwLoader) HistoricalJobCandidates(ctx context.Context, start, end time.Time, descending bool) ([]HistoricalJob, error) {
	if !start.Before(end) {
		return nil, fmt.Errorf("historical job start must be before end")
	}
	pl.historicalStart = &start
	pl.historicalEnd = &end
	jobs, errs := pl.fetchProwJobsFromOpenShiftBigQuery()
	if len(errs) > 0 {
		return nil, fmt.Errorf("querying historical jobs: %w", errors.Join(errs...))
	}
	if descending {
		sortProwJobsDescending(jobs)
	}
	entries, err := pl.preprocessProwJobs(ctx, jobs)
	if err != nil {
		return nil, err
	}
	newJobs := make([]prow.ProwJob, 0, len(entries))
	for _, pj := range entries {
		newJobs = append(newJobs, *pj)
	}
	if err := pl.prefetchHistoricalLabels(newJobs); err != nil {
		return nil, err
	}
	results := make([]HistoricalJob, 0, len(entries))
	for _, pj := range entries {
		result, err := pl.historicalJobResult(pj)
		if err != nil {
			return nil, err
		}
		results = append(results, HistoricalJob{ProwJob: *pj, Run: result.Run})
	}
	return results, nil
}

// PersistHistoricalRaw writes complete job and test rows atomically while
// deferring summary maintenance to the explicit summary commands.
func (pl *ProwLoader) PersistHistoricalRaw(ctx context.Context, results []pgwriter.JobRunResult) error {
	if err := pgwriter.WriteRaw(ctx, pl.dbc, results); err != nil {
		return fmt.Errorf("writing historical batch: %w", err)
	}
	return nil
}

// BuildHistoricalGCSResult uses the live XML conversion path and reports
// whether the build prefix exists for the historical fallback decision.
func (pl *ProwLoader) BuildHistoricalGCSResult(ctx context.Context, job HistoricalJob) (*pgwriter.JobRunResult, bool, error) {
	if pl.gcsClient == nil {
		return nil, false, fmt.Errorf("GCS client is not configured")
	}
	path, err := GetGCSPathForProwJobURL(log.WithField("buildID", job.ProwJob.Status.BuildID), job.ProwJob.Status.URL)
	if err != nil {
		return nil, false, err
	}
	bucket := pl.gcsClient.Bucket(job.ProwJob.Spec.DecorationConfig.GCSConfiguration.Bucket)
	gcsJobRun := gcs.NewGCSJobRun(bucket, path)
	matches, err := gcsJobRun.FindAllMatches(ctx, gcs.GlobJunitXML)
	if err != nil {
		return nil, false, err
	}
	buildPrefixExists, err := gcsJobRun.HasObjects(ctx)
	if err != nil {
		return nil, false, err
	}
	if !buildPrefixExists {
		return nil, false, nil
	}
	dbProwJob, ok := pl.prowJobCache[job.ProwJob.Spec.Job]
	if !ok {
		return nil, true, fmt.Errorf("ProwJob %q is missing from PostgreSQL", job.ProwJob.Spec.Job)
	}
	result, err := pl.buildJobRunResult(ctx, &job.ProwJob, uint64(job.Run.ID), path, matches, dbProwJob)
	return result, true, err
}

// BuildHistoricalBigQueryResult converts ordered JUnit rows using the loader's
// duplicate, skip, lifecycle, and synthetic-test rules.
func (pl *ProwLoader) BuildHistoricalBigQueryResult(job HistoricalJob, rows []bqjunit.TestRow) (*pgwriter.JobRunResult, error) {
	tests, failures, flakes, overall := pl.historicalBQTests(&job.ProwJob, job.Run, rows)
	result := job.Run
	result.TestFailures = failures
	result.TestFlakes = flakes
	result.OverallResult = overall
	result.Succeeded = overall == sippyprocessingv1.JobSucceeded
	annotations := make([]pgwriter.AnnotationRow, 0, len(job.ProwJob.Annotations))
	for key, value := range job.ProwJob.Annotations {
		annotations = append(annotations, pgwriter.AnnotationRow{
			ProwJobRunID:        result.ID,
			Key:                 key,
			Value:               value,
			ProwJobRunRelease:   result.ProwJobRelease,
			ProwJobRunTimestamp: result.Timestamp,
		})
	}
	return &pgwriter.JobRunResult{Run: result, Annotations: annotations, Tests: tests}, nil
}

// historicalJobResult builds the parent row with the loader's synthetic
// overall result before either source supplies real test rows.
func (pl *ProwLoader) historicalJobResult(pj *prow.ProwJob) (*pgwriter.JobRunResult, error) {
	dbProwJob, ok := pl.prowJobCache[pj.Spec.Job]
	if !ok {
		return nil, fmt.Errorf("ProwJob %q missing after preprocessing", pj.Spec.Job)
	}
	id, err := strconv.ParseUint(pj.Status.BuildID, 10, 63)
	if err != nil {
		return nil, fmt.Errorf("parsing build ID %q: %w", pj.Status.BuildID, err)
	}
	var duration time.Duration
	if pj.Status.CompletionTime != nil {
		duration = pj.Status.CompletionTime.Sub(pj.Status.StartTime)
	}
	_, overallResult := testconversion.ConvertProwJobRunToSyntheticTests(*pj, nil, pl.syntheticTestManager)
	return &pgwriter.JobRunResult{Run: pgwriter.RunRow{
		ID: uint(id), Cluster: pj.Spec.Cluster, Duration: duration, ProwJobID: dbProwJob.ID,
		ProwJobRelease: dbProwJob.Release, URL: pj.Status.URL,
		GCSBucket: pj.Spec.DecorationConfig.GCSConfiguration.Bucket, Timestamp: pj.Status.StartTime,
		OverallResult: overallResult, Succeeded: overallResult == sippyprocessingv1.JobSucceeded, Labels: []string(pl.labelsCache[pj.Status.BuildID]),
	}}, nil
}

// prefetchHistoricalLabels reuses the live loader's bulk label lookup for
// candidate jobs only, avoiding one BigQuery request per run.
func (pl *ProwLoader) prefetchHistoricalLabels(jobs []prow.ProwJob) error {
	if pl.bigQueryClient == nil || len(jobs) == 0 {
		return nil
	}
	labels, err := pl.prefetchLabels(jobs)
	if err != nil {
		return fmt.Errorf("prefetching historical labels: %w", err)
	}
	pl.labelsCache = labels
	return nil
}

// historicalBQTests applies the loader's duplicate, skipped-test, lifecycle,
// and synthetic-test rules to BQ rows before producing PostgreSQL test rows.
func (pl *ProwLoader) historicalBQTests(pj *prow.ProwJob, run pgwriter.RunRow, rows []bqjunit.TestRow) ([]pgwriter.TestRow, int, int, sippyprocessingv1.JobOverallResult) {
	testCases := make(map[testCaseKey]*types.TestCaseEntry)
	for _, row := range rows {
		if row.BuildID != pj.Status.BuildID || row.TestName == "" || row.Skipped || testidentification.IsIgnoredTest(row.TestName) {
			continue
		}
		status := int(sippyprocessingv1.TestStatusFailure)
		if row.FlakeCount > 0 {
			status = int(sippyprocessingv1.TestStatusFlake)
		} else if row.Success != 0 {
			status = int(sippyprocessingv1.TestStatusSuccess)
		}
		var output *string
		if row.FailureContent != "" && status != int(sippyprocessingv1.TestStatusSuccess) {
			output = &row.FailureContent
		}
		candidate := &types.TestCaseEntry{TestName: row.TestName, SuiteName: row.Suite, Status: status, Duration: row.Duration, Output: output, Lifecycle: normalizeLifecycle(row.Lifecycle)}
		key := testCaseKey{SuiteName: row.Suite, TestName: row.TestName}
		if current, ok := testCases[key]; !ok {
			testCases[key] = candidate
		} else {
			if current.Duration == 0 && candidate.Duration != 0 {
				current.Duration = candidate.Duration
			}
			if current.Output == nil && candidate.Output != nil {
				current.Output = candidate.Output
			}
			if current.Status != status {
				current.Status = int(sippyprocessingv1.TestStatusFlake)
			}
		}
	}
	old := make([]*types.TestCaseEntry, 0, len(testCases))
	for _, test := range testCases {
		old = append(old, test)
	}
	syntheticSuite, overall := testconversion.ConvertProwJobRunToSyntheticTests(*pj, old, pl.syntheticTestManager)
	extractTestCases(syntheticSuite, testCases)
	result := make([]pgwriter.TestRow, 0, len(testCases))
	failures, flakes := 0, 0
	for _, tc := range testCases {
		if testidentification.IsIgnoredTest(tc.TestName) {
			continue
		}
		result = append(result, pgwriter.TestRow{ProwJobRunID: run.ID, ProwJobID: run.ProwJobID, ProwJobRunTimestamp: run.Timestamp, ProwJobRunRelease: run.ProwJobRelease, TestName: tc.TestName, SuiteName: tc.SuiteName, Status: tc.Status, Duration: tc.Duration, Output: tc.Output, Lifecycle: tc.Lifecycle})
		switch tc.Status {
		case int(sippyprocessingv1.TestStatusFailure):
			failures++
		case int(sippyprocessingv1.TestStatusFlake):
			flakes++
		}
	}
	return result, failures, flakes, overall
}

// sortProwJobsDescending makes a restart resume from the newest missing run.
func sortProwJobsDescending(jobs []prow.ProwJob) {
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Status.StartTime.After(jobs[j].Status.StartTime) })
}
