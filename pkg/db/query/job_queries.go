package query

import (
	"database/sql"
	"time"

	log "github.com/sirupsen/logrus"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/filter"
)

func JobRunTestCount(dbc *db.DB, jobRunID int64) (int, error) {
	var prowJobRunTestCount int
	var tests []models.ProwJobRunTest

	res := dbc.DB.Find(&tests, "prow_job_run_id = ?", jobRunID)

	if res.Error != nil {
		return -1, res.Error
	}

	prowJobRunTestCount = len(tests)

	return prowJobRunTestCount, nil
}

func ProwJobSimilarName(dbc *db.DB, rootName, release string) ([]models.ProwJob, error) {

	// pull-ci-openshift-origin-master-e2e-vsphere-ovn-etcd-scaling
	// periodic-ci-openshift-release-master-nightly-4.14-e2e-vsphere-ovn-etcd-scaling
	// can we split on - and strip out pieces until we get a 'like' / 'contains' match
	// the compare versions / variants to match up, all in search of is this a 'never-stable' job
	// and other edge cases
	jobs := make([]models.ProwJob, 0)
	q := dbc.DB.Raw(`SELECT * FROM prow_jobs WHERE name LIKE ? AND release = ?`, "%"+rootName, release)
	if q.Error != nil {
		return nil, q.Error
	}
	q.Scan(&jobs)

	return jobs, nil
}

func ProwJobRunIDs(dbc *db.DB, prowJobID uint) ([]uint, error) {
	jobIDs := make([]uint, 0)
	q := dbc.DB.Raw(`SELECT id 
	FROM prow_job_runs WHERE prow_job_id = ?`, prowJobID)
	if q.Error != nil {
		return nil, q.Error
	}
	q.Scan(&jobIDs)

	return jobIDs, nil
}

func ProwJobHistoricalTestCounts(dbc *db.DB, prowJobID uint) (int, error) {

	var historicalProwJobRunTestCount float64
	q := dbc.DB.Raw(`SELECT avg(count) 
	FROM (SELECT count(*) 
	FROM prow_job_run_tests INNER JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id 
	WHERE prow_job_runs.prow_job_id = ? 
	AND prow_job_runs.timestamp >= CURRENT_DATE - interval '14' day  
	GROUP BY prow_job_run_id) t`, prowJobID)

	if q.Error != nil {
		return 0, q.Error
	}

	q.First(&historicalProwJobRunTestCount)

	return int(historicalProwJobRunTestCount), nil
}

func JobReports(dbc *db.DB, filterOpts *filter.FilterOptions, release string, start, boundary, end time.Time) ([]apitype.Job, error) {
	now := time.Now()
	jobReports := make([]apitype.Job, 0)

	table := dbc.DB.Table("job_results(?, ?, ?, ?)", release, start, boundary, end)
	if table.Error != nil {
		return jobReports, table.Error
	}

	q, err := filter.FilterableDBResult(table, filterOpts, apitype.Job{})
	if err != nil {
		return jobReports, err
	}

	q.Scan(&jobReports)
	elapsed := time.Since(now)
	log.Infof("JobReports completed in %s with %d results from db", elapsed, len(jobReports))

	return jobReports, nil
}

func VariantReports(dbc *db.DB, release string, start, boundary, end time.Time) ([]apitype.Variant, error) {
	variantResults := make([]apitype.Variant, 0)
	q := dbc.DB.Raw(`
WITH results AS (
        select unnest(prow_jobs.variants) as variant,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN @start AND @boundary then 1 end), 0) as previous_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN @start AND @boundary then 1 end), 0) as previous_fails,
                coalesce(count(case when timestamp BETWEEN @start AND @boundary then 1 end), 0) as previous_runs,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_fails,        
                coalesce(count(case when timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_runs
        FROM prow_job_runs 
        JOIN prow_jobs 
                ON prow_jobs.id = prow_job_runs.prow_job_id                 
                                AND prow_jobs.release = @release
                AND timestamp BETWEEN @start AND @end 
        group by variant
)
SELECT variant as name,
	current_passes,
	current_fails,
	current_passes + current_fails AS current_runs,
    current_passes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
    current_fails * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
    previous_passes,
    previous_fails,
  	previous_passes + previous_fails AS previous_runs,
    previous_passes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
    previous_fails * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
    (current_passes * 100.0 / NULLIF(current_runs, 0)) - (previous_passes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
FROM results
ORDER BY current_pass_percentage ASC;
`, sql.Named("release", release), sql.Named("start", start), sql.Named("boundary", boundary), sql.Named("end", end))
	if q.Error != nil {
		return nil, q.Error
	}
	q.Scan(&variantResults)
	return variantResults, nil
}

func ListFilteredJobIDs(dbc *db.DB, release string, fil *filter.Filter, start, boundary, end time.Time, limit int, sortField string, sort apitype.Sort) ([]int, error) {
	table := dbc.DB.Table("job_results(?, ?, ?, ?)", release, start, boundary, end)

	q, err := filter.ApplyFilters(fil, sortField, sort, limit, table, apitype.Job{})
	if err != nil {
		return nil, err
	}

	jobs := make([]int, 0)
	q.Pluck("id", &jobs)
	log.WithField("jobIDs", jobs).Debug("found job IDs after filtering")
	return jobs, nil
}

// LoadBugsForJobs returns all bugs in the database for the given jobs, across all releases.
// See ListFilteredJobIDs for obtaining the list of job IDs.
func LoadBugsForJobs(dbc *db.DB,
	jobIDs []int, filterClosed bool) ([]models.Bug, error) {
	results := []models.Bug{}

	job := models.ProwJob{}
	q := dbc.DB.Where("id IN ?", jobIDs)
	if filterClosed {
		q = q.Preload("Bugs", "UPPER(status) != 'CLOSED' and UPPER(status) != 'VERIFIED'")
	} else {
		q = q.Preload("Bugs")
	}
	res := q.First(&job)
	if res.Error != nil {
		return results, res.Error
	}
	log.Infof("found %d bugs for job", len(job.Bugs))
	return job.Bugs, nil
}
