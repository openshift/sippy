package query

import (
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

func RepositoryReport(dbc *db.DB, filterOpts *filter.FilterOptions, release string) ([]api.Repository, error) {
	start := time.Now().Add(-14 * 24 * time.Hour)
	end := time.Now()
	averageByJob := PullRequestAveragePremergeFailures(dbc, &start, &end)

	repos := dbc.DB.Table("prow_pull_requests").
		Joins("INNER JOIN prow_job_run_prow_pull_requests ON prow_job_run_prow_pull_requests.prow_pull_request_id = prow_pull_requests.id").
		Joins("INNER JOIN prow_job_runs on prow_job_run_prow_pull_requests.prow_job_run_id = prow_job_runs.id").
		Joins("INNER JOIN prow_jobs on prow_job_runs.prow_job_id = prow_jobs.id").
		Joins("LEFT JOIN (?) premerge_failures ON premerge_failures.prow_job_ID = prow_jobs.id", averageByJob).
		Where("prow_jobs.release = ?", release).
		Group("prow_pull_requests.org, prow_pull_requests.repo").
		Select("ROW_NUMBER() OVER() as id, prow_pull_requests.org, prow_pull_requests.repo, coalesce(max(average_premerge_job_failures), 0) as worst_premerge_job_failures, count(distinct(prow_jobs.id)) as job_count")

	results := make([]api.Repository, 0)
	q, err := filter.FilterableDBResult(dbc.DB.Table("(?) as repos", repos), filterOpts, api.Repository{})
	if err != nil {
		return results, err
	}
	q.Scan(&results)
	return results, nil
}
