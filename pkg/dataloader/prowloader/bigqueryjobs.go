package prowloader

import (
	"context"
	"strconv"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/apis/prow"
)

func (pl *ProwLoader) fetchProwJobsFromOpenShiftBigQuery() ([]prow.ProwJob, []error) {
	errs := []error{}

	// Figure out our last imported job timestamp:
	var lastProwJobRun time.Time
	row := pl.dbc.DB.Table("prow_job_runs").Select("max(timestamp)").Row()
	err := row.Scan(&lastProwJobRun)
	if err != nil || lastProwJobRun.IsZero() {
		log.WithError(err).Warn("no last prow job run found (new database?), importing last two weeks")
		lastProwJobRun = time.Now().Add(-14 * 24 * time.Hour)
	} else {
		// adjust the last job run time, we're querying all jobs that have completed since our last recorded
		// job START time, but we need to subtract our max job runtime in-case a job ended early and was our last
		// imported start time, while others that started before it hadn't completed yet.
		// 12 hours should safely cover our max timeout.
		lastProwJobRun = lastProwJobRun.Add(-12 * time.Hour)
	}
	log.Infof("Loading prow jobs from bigquery completed since: %s", lastProwJobRun.UTC().Format(time.RFC3339))

	// NOTE: casting a couple datetime columns to timestamps, it does appear they go in as UTC, and thus come out
	// as the default UTC correctly.
	// Annotations and labels can be queried here if we need them.
	query := pl.bigQueryClient.BQ.Query(`SELECT
			prowjob_job_name,
			prowjob_state,
			prowjob_build_id,
			prowjob_type,
			prowjob_cluster,
			prowjob_url,
			pr_sha,
			pr_author,
			pr_number,
			org,
			repo,
			TIMESTAMP(prowjob_start) AS prowjob_start_ts,
			TIMESTAMP(prowjob_completion) AS prowjob_completion_ts ` +
		"FROM `ci_analysis_us.jobs` " +
		`WHERE TIMESTAMP(prowjob_completion) > @queryFrom
	       AND prowjob_url IS NOT NULL
	       ORDER BY prowjob_start_ts`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "queryFrom",
			Value: lastProwJobRun,
		},
	}
	it, err := query.Read(context.TODO())
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
				pulls := make([]prow.Pull, 0)
				pull := prow.Pull{Number: prNumber, SHA: bqjr.PRSha.StringVal, Author: bqjr.PRAuthor.StringVal}
				refs.Pulls = append(pulls, pull)
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
		prowJobs[bqjr.BuildID] = prow.ProwJob{
			Spec: prow.ProwJobSpec{
				Type:    bqjr.Type,
				Cluster: bqjr.Cluster,
				Job:     bqjr.JobName,
				Refs:    refs,
			},
			Status: prow.ProwJobStatus{
				StartTime:      bqjr.StartTime.Timestamp,
				CompletionTime: &bqjr.CompletionTime,
				State:          prow.ProwJobState(bqjr.State),
				URL:            bqjr.URL,
				BuildID:        bqjr.BuildID,
			},
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
}
