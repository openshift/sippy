package prowloader

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/apis/prow"
)

func (pl *ProwLoader) fetchProwJobsFromOpenShiftBigQuery() ([]prow.ProwJob, []error) {
	errs := []error{}

	// Figure out our last imported job timestamp:
	var lastProwJobRun time.Time
	if pl.loadSince != nil {
		lastProwJobRun = *pl.loadSince
		log.Infof("Using manually specified load-since time: %s", lastProwJobRun.UTC().Format(time.RFC3339))
	} else {
		row := pl.dbc.DB.Table("prow_job_runs").Select("max(timestamp)").Row()
		err := row.Scan(&lastProwJobRun)
		if err != nil || lastProwJobRun.IsZero() {
			log.WithError(err).Warnf("no last prow job run found (new database?), importing previous %d days", DefaultLookbackDays)
			lastProwJobRun = time.Now().AddDate(0, 0, -DefaultLookbackDays)
		} else {
			// adjust the last job run time, we're querying all jobs that have completed since our last recorded
			// job START time, but we need to subtract our max job runtime in-case a job ended early and was our last
			// imported start time, while others that started before it hadn't completed yet.
			// 12 hours should safely cover our max timeout.
			lastProwJobRun = lastProwJobRun.Add(-12 * time.Hour)
		}

		// we need to know how far back we are looking for partitioning
		pl.loadSince = &lastProwJobRun
	}
	log.Infof("Loading prow jobs from bigquery completed since: %s", lastProwJobRun.UTC().Format(time.RFC3339))

	// NOTE: casting a couple datetime columns to timestamps, it does appear they go in as UTC, and thus come out
	// as the default UTC correctly.
	// Annotations and labels can be queried here if we need them.
	query := pl.bigQueryClient.Query(pl.ctx, bqlabel.ProwLoaderProwJobs, `
        SELECT
			prowjob_job_name,
			prowjob_state,
			prowjob_build_id,
			prowjob_type,
			prowjob_cluster,
			prowjob_url,
			prowjob_annotations,
			pr_sha,
			pr_author,
			pr_number,
			org,
			repo,
			gcs_bucket,
			TIMESTAMP(prowjob_start) AS prowjob_start_ts,
			TIMESTAMP(prowjob_completion) AS prowjob_completion_ts `+
		"FROM `ci_analysis_us.jobs` "+
		`WHERE TIMESTAMP(prowjob_completion) > @queryFrom
	       AND prowjob_url IS NOT NULL
	       AND prowjob_state NOT IN ('pending', 'triggered')
	       ORDER BY prowjob_start_ts`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "queryFrom",
			Value: lastProwJobRun,
		},
	}
	it, err := query.Read(pl.ctx)
	if err != nil {
		errs = append(errs, err)
		log.WithError(err).Error("error querying jobs from bigquery")
		return []prow.ProwJob{}, errs
	}

	// Using a set since sometimes bigquery has multiple copies of the same prow job
	prowJobs := map[string]prow.ProwJob{}
	count := 0
	for {
		bqjr := bigqueryProwJobRun{}
		err := it.Next(&bqjr)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing prowjob from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing prowjob from bigquery"))
			continue
		}

		var refs *prow.Refs
		if bqjr.PRNumber.StringVal != "" {
			prNumber, err := strconv.Atoi(bqjr.PRNumber.StringVal)
			if err != nil {
				log.WithError(err).Errorf("Invalid pull request number from big query fetch prow jobs")
			} else {
				refs = &prow.Refs{Org: bqjr.PROrg.StringVal, Repo: bqjr.PRRepo.StringVal}
				pull := prow.Pull{Number: prNumber, SHA: bqjr.PRSha.StringVal, Author: bqjr.PRAuthor.StringVal}
				refs.Pulls = []prow.Pull{pull}
			}
		} else if bqjr.Type == "presubmit" {
			log.Warningf("Presubmit job found without matching PR data for: %s", bqjr.JobName)
		}
		// Convert to a prow.ProwJob:
		// If we read in an invalid StartTime, skip this job but put out an error.
		if !bqjr.StartTime.Valid {
			log.WithField("job", bqjr.JobName).Error("invalid start time for prowjob")
			// Do not return an error as that will cause the job to fail.
			continue
		}
		// Filter out annotations with excluded prefixes
		filteredAnnotations := make(map[string]string)
		for _, a := range bqjr.Annotations {
			parts := strings.SplitN(a, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			value := parts[1]
			if strings.HasPrefix(key, "prow.k8s.io") || strings.HasPrefix(key, "ci.openshift.io") {
				continue
			}
			filteredAnnotations[key] = value
		}

		jobName := bqjr.JobName
		jobType := bqjr.Type

		// Detect /payload sub-jobs: they have a releaseJobName annotation and PR data,
		// and are run as periodic jobs. Transform them into presubmit-style records so
		// they appear on presubmit UI pages. Skip aggregator jobs (they just orchestrate
		// sub-jobs and don't have test results themselves).
		if releaseJobName, ok := filteredAnnotations["releaseJobName"]; ok &&
			refs != nil &&
			!strings.HasPrefix(bqjr.JobName, "aggregator-") {

			// Normalize releaseJobName: the BQ query generator splits on comma
			// to strip suffixes, so we must do the same here.
			releaseJobName = strings.SplitN(releaseJobName, ",", 2)[0]
			filteredAnnotations["releaseJobName"] = releaseJobName

			prNumber := bqjr.PRNumber.StringVal
			org := bqjr.PROrg.StringVal
			repo := bqjr.PRRepo.StringVal

			// Strip the PR number from the sub-job name to create a stable name
			// shared across all PRs running the same canonical job.
			// e.g. openshift-origin-31301-ci-5.0-... -> openshift-origin-ci-5.0-...
			prPrefix := fmt.Sprintf("%s-%s-%s-", org, repo, prNumber)
			stablePrefix := fmt.Sprintf("%s-%s-", org, repo)
			stableName := strings.Replace(bqjr.JobName, prPrefix, stablePrefix, 1)

			if stableName == bqjr.JobName {
				// PR number not found in job name pattern, use releaseJobName-based fallback
				stableName = "payload-pr-" + releaseJobName
				log.WithField("job", bqjr.JobName).
					WithField("releaseJobName", releaseJobName).
					Warningf("could not strip PR number from /payload sub-job name, using fallback")
			}

			jobName = stableName
			jobType = "presubmit"

			log.WithField("originalName", bqjr.JobName).
				WithField("stableName", stableName).
				WithField("releaseJobName", releaseJobName).
				WithField("prNumber", prNumber).
				Debugf("transformed /payload sub-job for presubmit ingestion")
		}

		prowJobs[bqjr.BuildID] = prow.ProwJob{
			Spec: prow.ProwJobSpec{
				Type:    jobType,
				Cluster: bqjr.Cluster,
				Job:     jobName,
				Refs:    refs,
				DecorationConfig: prow.DecorationConfig{
					GCSConfiguration: prow.GCSConfiguration{
						Bucket: bqjr.GCSBucket.String(),
					},
				},
			},
			Status: prow.ProwJobStatus{
				StartTime:      bqjr.StartTime.Timestamp,
				CompletionTime: &bqjr.CompletionTime,
				State:          prow.ProwJobState(bqjr.State),
				URL:            bqjr.URL,
				BuildID:        bqjr.BuildID,
			},
			Annotations: filteredAnnotations,
		}
		count++
	}

	var prowJobsList []prow.ProwJob
	for _, job := range prowJobs {
		prowJobsList = append(prowJobsList, job)
	}

	log.Infof("found %d jobs (%d dupes) in bigquery since last import (roughly)", len(prowJobs), count-len(prowJobs))
	return prowJobsList, errs
}

// bigqueryProwJobRun is a transient struct for processing results from the bigquery jobs table.
// Ultimately just used to convert to a prow.ProwJob.
type bigqueryProwJobRun struct {
	JobName        string                 `bigquery:"prowjob_job_name"`
	State          string                 `bigquery:"prowjob_state"`
	BuildID        string                 `bigquery:"prowjob_build_id"`
	Type           string                 `bigquery:"prowjob_type"`
	Cluster        string                 `bigquery:"prowjob_cluster"`
	StartTime      bigquery.NullTimestamp `bigquery:"prowjob_start_ts"`
	CompletionTime time.Time              `bigquery:"prowjob_completion_ts"`
	URL            string                 `bigquery:"prowjob_url"`
	PRSha          bigquery.NullString    `bigquery:"pr_sha"`
	PRAuthor       bigquery.NullString    `bigquery:"pr_author"`
	PRNumber       bigquery.NullString    `bigquery:"pr_number"`
	PROrg          bigquery.NullString    `bigquery:"org"`
	PRRepo         bigquery.NullString    `bigquery:"repo"`
	GCSBucket      bigquery.NullString    `bigquery:"gcs_bucket"`
	Annotations    []string               `bigquery:"prowjob_annotations"`
}
