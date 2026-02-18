package api

import (
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

// GetRecentTestFailures returns tests that failed within the given period, optionally
// excluding tests that also failed in a previous period (to surface only new regressions).
func GetRecentTestFailures(
	dbc *db.DB,
	release string,
	period time.Duration,
	previousPeriod *time.Duration,
	includeOutputs bool,
	filterOpts *filter.FilterOptions,
	pagination *apitype.Pagination,
	reportEnd time.Time,
) (*apitype.PaginationResult, error) {
	periodStart := reportEnd.Add(-period)

	q := dbc.DB.Table("prow_job_run_tests").
		Joins("JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id").
		Joins("JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id").
		Joins("JOIN tests ON tests.id = prow_job_run_tests.test_id").
		Joins("LEFT JOIN suites ON suites.id = prow_job_run_tests.suite_id").
		Joins("LEFT JOIN test_ownerships ON test_ownerships.test_id = tests.id").
		Where("prow_job_runs.timestamp >= ? AND prow_job_runs.timestamp < ?", periodStart, reportEnd).
		Where("prow_job_run_tests.status = ?", int(sippyprocessingv1.TestStatusFailure)).
		Where("prow_jobs.release = ?", release).
		Where("prow_job_run_tests.deleted_at IS NULL").
		Where("prow_job_runs.deleted_at IS NULL").
		Where("prow_jobs.deleted_at IS NULL").
		Where("tests.deleted_at IS NULL").
		Group("tests.id, tests.name, suites.name, test_ownerships.jira_component").
		Select(`
			tests.id AS test_id,
			tests.name AS test_name,
			suites.name AS suite_name,
			test_ownerships.jira_component AS jira_component,
			COUNT(*) AS failure_count,
			MIN(prow_job_runs.timestamp) AS first_failure,
			MAX(prow_job_runs.timestamp) AS last_failure`)

	if previousPeriod != nil {
		prevStart := periodStart.Add(-*previousPeriod)
		q = q.Where(`NOT EXISTS (
			SELECT 1
			FROM prow_job_run_tests prev_t
			JOIN prow_job_runs prev_r ON prev_r.id = prev_t.prow_job_run_id
			JOIN prow_jobs prev_j ON prev_j.id = prev_r.prow_job_id
			WHERE prev_t.test_id = tests.id
			  AND prev_t.status = ?
			  AND prev_r.timestamp >= ? AND prev_r.timestamp < ?
			  AND prev_j.release = ?
			  AND prev_t.deleted_at IS NULL
			  AND prev_r.deleted_at IS NULL
			  AND prev_j.deleted_at IS NULL
		)`, int(sippyprocessingv1.TestStatusFailure), prevStart, periodStart, release)
	}

	// Wrap the aggregated query as a subquery so we can apply filters/sort/pagination
	subquery := dbc.DB.Table("(?) AS recent_failures", q)

	filteredQuery, err := filter.FilterableDBResult(subquery, filterOpts, apitype.RecentTestFailure{})
	if err != nil {
		return nil, err
	}

	var rowCount int64
	if err := filteredQuery.Count(&rowCount).Error; err != nil {
		return nil, err
	}

	if pagination == nil {
		pagination = &apitype.Pagination{
			PerPage: int(rowCount),
			Page:    0,
		}
	} else {
		filteredQuery = filteredQuery.Limit(pagination.PerPage).Offset(pagination.Page * pagination.PerPage)
	}

	results := make([]apitype.RecentTestFailure, 0)
	res := filteredQuery.Scan(&results)
	if res.Error != nil {
		return nil, res.Error
	}

	if len(results) > 0 {
		testIDs := make([]uint, len(results))
		for i, r := range results {
			testIDs[i] = r.TestID
		}

		// Bound the last_pass lookback to the total analysis window (period + previousPeriod)
		// to avoid scanning the entire history for large releases.
		lastPassLookback := periodStart
		if previousPeriod != nil {
			lastPassLookback = periodStart.Add(-*previousPeriod)
		}
		var lastPasses []struct {
			TestID   uint       `gorm:"column:test_id"`
			LastPass *time.Time `gorm:"column:last_pass"`
		}
		if err := dbc.DB.Table("prow_job_run_tests").
			Joins("JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id").
			Joins("JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id").
			Where("prow_job_run_tests.test_id IN ?", testIDs).
			Where("prow_job_run_tests.status = ?", int(sippyprocessingv1.TestStatusSuccess)).
			Where("prow_job_runs.timestamp >= ?", lastPassLookback).
			Where("prow_jobs.release = ?", release).
			Where("prow_job_run_tests.deleted_at IS NULL").
			Where("prow_job_runs.deleted_at IS NULL").
			Where("prow_jobs.deleted_at IS NULL").
			Group("prow_job_run_tests.test_id").
			Select("prow_job_run_tests.test_id AS test_id, MAX(prow_job_runs.timestamp) AS last_pass").
			Scan(&lastPasses).Error; err != nil {
			return nil, err
		}

		lastPassByTestID := make(map[uint]*time.Time)
		for _, lp := range lastPasses {
			lastPassByTestID[lp.TestID] = lp.LastPass
		}
		for i := range results {
			results[i].LastPass = lastPassByTestID[results[i].TestID]
		}

		if includeOutputs {
			var outputs []struct {
				TestID       uint      `gorm:"column:test_id"`
				ProwJobRunID uint      `gorm:"column:prow_job_run_id"`
				ProwJobName  string    `gorm:"column:prow_job_name"`
				ProwJobURL   string    `gorm:"column:prow_job_url"`
				FailedAt     time.Time `gorm:"column:failed_at"`
				Output       string    `gorm:"column:output"`
			}

			if err := dbc.DB.Table("prow_job_run_tests").
				Joins("JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id").
				Joins("JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id").
				Joins("LEFT JOIN prow_job_run_test_outputs ON prow_job_run_test_outputs.prow_job_run_test_id = prow_job_run_tests.id").
				Where("prow_job_run_tests.test_id IN ?", testIDs).
				Where("prow_job_run_tests.status = ?", int(sippyprocessingv1.TestStatusFailure)).
				Where("prow_job_runs.timestamp >= ? AND prow_job_runs.timestamp < ?", periodStart, reportEnd).
				Where("prow_jobs.release = ?", release).
				Where("prow_job_run_tests.deleted_at IS NULL").
				Where("prow_job_runs.deleted_at IS NULL").
				Where("prow_jobs.deleted_at IS NULL").
				Select(`
					prow_job_run_tests.test_id AS test_id,
					prow_job_runs.id AS prow_job_run_id,
					prow_jobs.name AS prow_job_name,
					prow_job_runs.url AS prow_job_url,
					prow_job_runs.timestamp AS failed_at,
					COALESCE(prow_job_run_test_outputs.output, '') AS output`).
				Scan(&outputs).Error; err != nil {
				return nil, err
			}

			outputsByTestID := make(map[uint][]apitype.RecentTestFailureOutput)
			for _, o := range outputs {
				outputsByTestID[o.TestID] = append(outputsByTestID[o.TestID], apitype.RecentTestFailureOutput{
					ProwJobRunID: o.ProwJobRunID,
					ProwJobName:  o.ProwJobName,
					ProwJobURL:   o.ProwJobURL,
					FailedAt:     o.FailedAt,
					Output:       o.Output,
				})
			}

			for i := range results {
				results[i].Outputs = outputsByTestID[results[i].TestID]
			}
		}
	}

	return &apitype.PaginationResult{
		Rows:      results,
		TotalRows: rowCount,
		PageSize:  pagination.PerPage,
		Page:      pagination.Page,
	}, nil
}
