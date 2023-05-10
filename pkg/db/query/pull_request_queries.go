package query

import (
	"time"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

func PullRequestReport(dbc *db.DB, filterOpts *filter.FilterOptions, release string) ([]api.PullRequest, error) {
	// This finds each PR's first payload for each stream/arch combo, we use it below to join on so we can
	// find the first ci and nightly for each payload.
	firstPayloadsByStreamAndArch := dbc.DB.Table("release_pull_requests").
		Select("url, release_tags.stream, release_tags.architecture, MIN(release_tags.release_time) AS min_release_time, release_tags.release_tag, release_tags.phase, release_tags.release").
		Joins("JOIN release_tag_pull_requests ON release_tag_pull_requests.release_pull_request_id = release_pull_requests.id").
		Joins("INNER JOIN release_tags ON release_tags.id = release_tag_pull_requests.release_tag_id").
		Group("url, release_tags.stream, release_tags.architecture, release_tags.release_tag, release_tags.phase, release_tags.release")

	prs := dbc.DB.Table("prow_pull_requests").
		Joins("LEFT JOIN (?) ci ON ci.url = prow_pull_requests.link",
			dbc.DB.Table("(?) as ci", firstPayloadsByStreamAndArch).Where("ci.stream = 'ci' AND architecture = 'amd64'")).
		Joins("LEFT JOIN (?) nightly ON nightly.url = prow_pull_requests.link",
			dbc.DB.Table("(?) as nightly", firstPayloadsByStreamAndArch).Where("nightly.stream = 'nightly' AND nightly.architecture = 'amd64'")).
		Joins("INNER JOIN prow_job_run_prow_pull_requests ON prow_job_run_prow_pull_requests.prow_pull_request_id = prow_pull_requests.id").
		Joins("INNER JOIN prow_job_runs on prow_job_run_prow_pull_requests.prow_job_run_id = prow_job_runs.id").
		Joins("INNER JOIN prow_jobs on prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_jobs.release = ?", release).
		Select("DISTINCT ON(prow_pull_requests.link) prow_pull_requests.*, ci.release_tag AS first_ci_payload, ci.phase AS first_ci_payload_phase, ci.release as first_ci_payload_release, nightly.release_tag as first_nightly_payload, nightly.phase as first_nightly_payload_phase, nightly.release as first_nightly_payload_release")

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
