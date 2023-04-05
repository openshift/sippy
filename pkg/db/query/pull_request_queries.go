package query

import (
	"time"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

func PullRequestReport(dbc *db.DB, filterOpts *filter.FilterOptions, release string) ([]api.PullRequest, error) {
	firstCIPayload := dbc.DB.Table("release_pull_requests pr").
		Joins("join release_tag_pull_requests rtpr on rtpr.release_pull_request_id = pr.id").
		Joins("inner join release_tags rt on rt.id = rtpr.release_tag_id").Select("release, release_tag, url, phase").
		Where("stream = ?", "ci").Where("architecture = ?", "amd64")

	firstNightlyPayload := dbc.DB.Table("release_pull_requests pr").
		Joins("join release_tag_pull_requests rtpr on rtpr.release_pull_request_id = pr.id").
		Joins("inner join release_tags rt on rt.id = rtpr.release_tag_id").Select("release, release_tag, url, phase").
		Where("stream = ?", "nightly").Where("architecture = ?", "amd64")

	prs := dbc.DB.Table("prow_pull_requests").
		Joins("LEFT JOIN (?) ci_payload ON ci_payload.url = prow_pull_requests.link", firstCIPayload).
		Joins("LEFT JOIN (?) nightly_payload ON nightly_payload.url = prow_pull_requests.link", firstNightlyPayload).
		Joins("INNER JOIN prow_job_run_prow_pull_requests ON prow_job_run_prow_pull_requests.prow_pull_request_id = prow_pull_requests.id").
		Joins("INNER JOIN prow_job_runs on prow_job_run_prow_pull_requests.prow_job_run_id = prow_job_runs.id").
		Joins("INNER JOIN prow_jobs on prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_jobs.release = ?", release).
		Select("DISTINCT ON(prow_pull_requests.link) prow_pull_requests.*, ci_payload.release_tag AS first_ci_payload, ci_payload.phase AS first_ci_payload_phase, ci_payload.release as first_ci_payload_release, nightly_payload.release_tag as first_nightly_payload, nightly_payload.phase as first_nightly_payload_phase, nightly_payload.release as first_nightly_payload_release")

	results := make([]api.PullRequest, 0)
	q, err := filter.FilterableDBResult(dbc.DB.Table("(?) as prs", prs), filterOpts, api.PullRequest{})
	if err != nil {
		return results, err
	}
	q.Scan(&results)
	return results, nil
}

func PullRequestAveragePremergeFailures(dbc *db.DB, start, end *time.Time) *gorm.DB {
	premergeFailures := dbc.DB.Table("prow_job_runs").
		Select("prow_jobs.id as prow_job_id, prow_jobs.name as prow_job_name, prow_pull_requests.org, prow_pull_requests.repo, prow_pull_requests.link, COUNT(*) as total_runs").
		Joins("INNER JOIN prow_job_run_prow_pull_requests on prow_job_run_prow_pull_requests.prow_job_run_id = prow_job_runs.id").
		Joins("INNER JOIN prow_pull_requests on prow_pull_requests.id = prow_job_run_prow_pull_requests.prow_pull_request_id").
		Joins("INNER JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_job_runs.overall_result != 'S'").
		Where("prow_job_runs.overall_result != 'A'").
		Where("prow_pull_requests.merged_at IS NOT NULL").
		Group("prow_jobs.id, prow_jobs.name, prow_pull_requests.org, prow_pull_requests.repo, prow_pull_requests.id, prow_pull_requests.link")

	if start != nil {
		premergeFailures = premergeFailures.Where("prow_pull_requests.merged_at >= ?", start)
	}

	if end != nil {
		premergeFailures = premergeFailures.Where("prow_pull_requests.merged_at <= ?", end)
	}

	return dbc.DB.Table("(?) as premerge_failures", premergeFailures).
		Select("org, repo, prow_job_id, prow_job_name, AVG(total_runs) as average_premerge_job_failures").
		Group("prow_job_id, prow_job_name, org, repo")
}
