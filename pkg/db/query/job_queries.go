package query

import (
	"database/sql"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
	log "github.com/sirupsen/logrus"
)

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

	// FIXME(stbenjam): There's a UI bug where the jobs page won't load if either bugs filled is "null"
	// instead of empty array. Quick hack to make this work.
	for i, j := range jobReports {
		if len(j.Bugs) == 0 {
			jobReports[i].Bugs = make([]bugsv1.Bug, 0)
		}

		if len(j.AssociatedBugs) == 0 {
			jobReports[i].AssociatedBugs = make([]bugsv1.Bug, 0)
		}
	}

	return jobReports, nil
}

func VariantReports(dbc *db.DB, release string, start, boundary, end time.Time) ([]apitype.Variant, error) {
	var variantResults []apitype.Variant
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
