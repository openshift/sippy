package query

import (
	"database/sql"
	"time"

	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
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

	// Separate query for open bug counts, add to the models before we return.
	// This was getting too difficult to jam into the job_results function.
	openBugsQuery := `SELECT prow_jobs.name AS name,
		COUNT(DISTINCT bug_jobs.bug_id) AS open_bugs
		FROM prow_jobs
		JOIN bug_jobs ON prow_jobs.id = bug_jobs.prow_job_id
		WHERE prow_jobs.release = ?
		GROUP BY prow_jobs.name`

	type jobOpenBugs struct {
		Name     string
		OpenBugs int
	}
	allOpenBugs := make([]*jobOpenBugs, 0)
	res := dbc.DB.Raw(openBugsQuery, release).Scan(&allOpenBugs)
	if res.Error != nil {
		log.WithError(res.Error).Error("error loading all bugs to populate open_bugs for jobs")
		return jobReports, res.Error
	}
	for i := range jobReports {
		for _, ob := range allOpenBugs {
			log.Infof("%s = %s?", ob.Name, jobReports[i].Name)
			if ob.Name == jobReports[i].Name {
				log.WithField("name", jobReports[i].Name).WithField("openBugs", ob.OpenBugs).Debug("job has open bugs")
				jobReports[i].OpenBugs = ob.OpenBugs
			}
		}
	}

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
	jobIDs []int) ([]models.Bug, error) {
	results := []models.Bug{}

	job := models.ProwJob{}
	res := dbc.DB.Where("id IN ?", jobIDs).Preload("Bugs").First(&job)
	if res.Error != nil {
		return results, res.Error
	}
	log.Infof("found %d bugs for job", len(job.Bugs))
	return job.Bugs, nil
}
