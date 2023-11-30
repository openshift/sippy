package query

import (
	"time"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

func RepositoryReport(dbc *db.DB, filterOpts *filter.FilterOptions, release string, reportEnd time.Time) ([]api.Repository, error) {
	end := reportEnd

	premergeFailureStart := reportEnd.Add(-14 * 24 * time.Hour)
	averageByJob := PullRequestAveragePremergeFailures(dbc, &premergeFailureStart, &end)

	revertCountStart := reportEnd.Add(-90 * 24 * time.Hour)
	revertCount := RepositoryRevertCount(dbc, &revertCountStart, &end)

	repos := dbc.DB.Table("prow_pull_requests").
		Joins("INNER JOIN prow_job_run_prow_pull_requests ON prow_job_run_prow_pull_requests.prow_pull_request_id = prow_pull_requests.id").
		Joins("INNER JOIN prow_job_runs on prow_job_run_prow_pull_requests.prow_job_run_id = prow_job_runs.id").
		Joins("INNER JOIN prow_jobs on prow_job_runs.prow_job_id = prow_jobs.id").
		Joins("LEFT JOIN (?) revert_count ON revert_count.org = prow_pull_requests.org AND revert_count.repo = prow_pull_requests.repo", revertCount).
		Joins("LEFT JOIN (?) premerge_failures ON premerge_failures.prow_job_ID = prow_jobs.id", averageByJob).
		Where("prow_jobs.release = ?", release).
		Group("prow_pull_requests.org, prow_pull_requests.repo").
		Select("ROW_NUMBER() OVER() as id, prow_pull_requests.org, prow_pull_requests.repo, max(revert_count) as revert_count, coalesce(max(average_premerge_job_failures), 0) as worst_premerge_job_failures, count(distinct(prow_jobs.id)) as job_count")

	results := make([]api.Repository, 0)
	q, err := filter.FilterableDBResult(dbc.DB.Table("(?) as repos", repos), filterOpts, api.Repository{})
	if err != nil {
		return results, err
	}
	q.Scan(&results)
	return results, nil
}

func RepositoryRevertCount(dbc *db.DB, start, end *time.Time) *gorm.DB {
	query := dbc.DB.Table("prow_pull_requests").
		Where("title ILIKE '%Revert%'").
		Where("title NOT ILIKE '%Unrevert%'").
		Group("org, repo").
		Select("org, repo, COUNT(DISTINCT link) AS revert_count")

	if start != nil {
		query = query.Where("prow_pull_requests.merged_at >= ?", start)
	}

	if end != nil {
		query = query.Where("prow_pull_requests.merged_at <= ?", end)
	}

	return query
}
