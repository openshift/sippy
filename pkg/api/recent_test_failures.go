package api

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
)

const maxTestIDsPerQuery = 500

// GetRecentTestFailures returns tests that failed within the given period, optionally
// excluding tests that also failed in a previous period (to surface only new regressions).
func GetRecentTestFailures(
	dbc *db.DB,
	release string,
	periodDays int,
	previousPeriodDays *int,
	includeOutputs bool,
	filterOpts *filter.FilterOptions,
	pagination *apitype.Pagination,
	reportEnd time.Time,
) (*apitype.PaginationResult, error) {
	// Align to full calendar days for partition pruning. AddDays(1) ensures the
	// entire day containing reportEnd is included; endDate is exclusive.
	endDate := civil.DateOf(reportEnd.UTC()).AddDays(1)
	startDate := endDate.AddDays(-periodDays)

	var q *gorm.DB
	if previousPeriodDays != nil {
		var err error
		q, err = buildRecentFailuresWithPrevPeriodFilter(dbc, release, startDate, endDate, *previousPeriodDays)
		if err != nil {
			return nil, err
		}
	} else {
		q = buildRecentFailuresQuery(dbc, release, startDate, endDate)
	}

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
		lastPassLookbackDate := startDate
		if previousPeriodDays != nil {
			lastPassLookbackDate = startDate.AddDays(-*previousPeriodDays)
		}

		lastPassByTestID, err := findLastPassIterative(dbc, testIDs, release, endDate, lastPassLookbackDate)
		if err != nil {
			return nil, err
		}
		for i := range results {
			results[i].LastPass = lastPassByTestID[results[i].TestID]
		}

		if includeOutputs {
			if err := attachOutputs(dbc, results, testIDs, release, startDate, endDate); err != nil {
				return nil, err
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

// recentFailuresMainSQL returns the core aggregation SQL and its parameters.
// Both the simple and previous-period-filtered paths share this fragment.
func recentFailuresMainSQL(release string, startDate, endDate civil.Date) (string, []interface{}) {
	sql := `SELECT tests.id AS test_id,
			tests.name AS test_name,
			suites.name AS suite_name,
			test_ownerships.jira_component AS jira_component,
			COUNT(*) AS failure_count,
			MIN(prow_job_runs.timestamp) AS first_failure,
			MAX(prow_job_runs.timestamp) AS last_failure
		FROM prow_job_run_tests
		JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id
		JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id
		JOIN tests ON tests.id = prow_job_run_tests.test_id
		LEFT JOIN suites ON suites.id = prow_job_run_tests.suite_id
		LEFT JOIN test_ownerships ON test_ownerships.test_id = tests.id AND test_ownerships.deleted_at IS NULL
		WHERE prow_job_runs.timestamp >= ? AND prow_job_runs.timestamp < ?
			AND prow_job_runs.prow_job_release = ?
			AND prow_job_run_tests.prow_job_run_timestamp >= ? AND prow_job_run_tests.prow_job_run_timestamp < ?
			AND prow_job_run_tests.status = ?
			AND prow_jobs.release = ?
			AND prow_job_run_tests.prow_job_run_release = ?
			AND prow_job_run_tests.deleted_at IS NULL
			AND prow_job_runs.deleted_at IS NULL
			AND prow_jobs.deleted_at IS NULL
			AND tests.deleted_at IS NULL
		GROUP BY tests.id, tests.name, suites.name, test_ownerships.jira_component`

	params := []interface{}{
		startDate, endDate, release,
		startDate, endDate,
		int(sippyprocessingv1.TestStatusFailure),
		release, release,
	}
	return sql, params
}

// buildRecentFailuresQuery builds the main aggregation query without any previous-period filtering.
func buildRecentFailuresQuery(dbc *db.DB, release string, startDate, endDate civil.Date) *gorm.DB {
	sql, params := recentFailuresMainSQL(release, startDate, endDate)
	return dbc.DB.Raw("("+sql+")", params...)
}

// buildRecentFailuresWithPrevPeriodFilter wraps the main aggregation as a CTE,
// then anti-joins against test_cumulative_summaries to exclude tests that also
// failed in the previous period. The cumulative summaries are pre-aggregated
// per test_id (summing across all prow_job_id/suite_id combos) before comparing.
func buildRecentFailuresWithPrevPeriodFilter(
	dbc *db.DB,
	release string,
	startDate, endDate civil.Date,
	previousPeriodDays int,
) (*gorm.DB, error) {
	mainSQL, mainParams := recentFailuresMainSQL(release, startDate, endDate)

	prevPeriod := query.DateRange{
		Start: startDate.AddDays(-previousPeriodDays),
		End:   startDate,
	}
	if err := query.ResolveDateRanges(dbc, release, &prevPeriod); err != nil {
		return nil, err
	}
	cumEndDate := prevPeriod.End.AddDays(-1).String()
	cumStartDate := prevPeriod.Start.AddDays(-1).String()

	rawSQL := fmt.Sprintf(`(
		WITH main_agg AS (%s),
		prev_failures AS (
			SELECT cum_end_agg.test_id
			FROM (
				SELECT test_id, SUM(prefix_sum_failures) AS total_failures
				FROM test_cumulative_summaries
				WHERE release = ? AND date = ?
					AND test_id IN (SELECT test_id FROM main_agg)
				GROUP BY test_id
			) cum_end_agg
			LEFT JOIN (
				SELECT test_id, SUM(prefix_sum_failures) AS total_failures
				FROM test_cumulative_summaries
				WHERE release = ? AND date = ?
					AND test_id IN (SELECT test_id FROM main_agg)
				GROUP BY test_id
			) cum_start_agg ON cum_start_agg.test_id = cum_end_agg.test_id
			WHERE (cum_end_agg.total_failures - COALESCE(cum_start_agg.total_failures, 0)) > 0
		)
		SELECT m.*
		FROM main_agg m
		WHERE m.test_id NOT IN (SELECT test_id FROM prev_failures)
	)`, mainSQL)

	params := append(mainParams, release, cumEndDate, release, cumStartDate)
	return dbc.DB.Raw(rawSQL, params...), nil
}

// findLastPassIterative scans backwards one day at a time from endDate, looking
// for the most recent successful run for each test_id. Each iteration queries only
// the remaining unfound IDs, and scans a single daily partition.
func findLastPassIterative(
	dbc *db.DB,
	testIDs []uint,
	release string,
	endDate, lookbackDate civil.Date,
) (map[uint]*time.Time, error) {
	lastPassByTestID := make(map[uint]*time.Time, len(testIDs))

	toSearch := make([]uint, len(testIDs))
	copy(toSearch, testIDs)
	notFound := make([]uint, 0, len(testIDs))

	windowEnd := endDate
	statusSuccess := int(sippyprocessingv1.TestStatusSuccess)

	for len(toSearch) > 0 {
		windowStart := windowEnd.AddDays(-1)
		if windowStart.Before(lookbackDate) {
			windowStart = lookbackDate
		}
		if !windowStart.Before(windowEnd) {
			break
		}

		for batchStart := 0; batchStart < len(toSearch); batchStart += maxTestIDsPerQuery {
			batchEnd := batchStart + maxTestIDsPerQuery
			if batchEnd > len(toSearch) {
				batchEnd = len(toSearch)
			}
			batch := toSearch[batchStart:batchEnd]

			var passes []struct {
				TestID   uint      `gorm:"column:test_id"`
				LastPass time.Time `gorm:"column:last_pass"`
			}

			if err := dbc.DB.Table("prow_job_run_tests").
				Joins("JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id").
				Where("prow_job_run_tests.test_id IN ?", batch).
				Where("prow_job_run_tests.status = ?", statusSuccess).
				Where("prow_job_run_tests.prow_job_run_timestamp >= ? AND prow_job_run_tests.prow_job_run_timestamp < ?", windowStart, windowEnd).
				Where("prow_job_run_tests.prow_job_run_release = ?", release).
				Where("prow_job_runs.prow_job_release = ?", release).
				Where("prow_job_run_tests.deleted_at IS NULL").
				Where("prow_job_runs.deleted_at IS NULL").
				Group("prow_job_run_tests.test_id").
				Select("prow_job_run_tests.test_id AS test_id, MAX(prow_job_runs.timestamp) AS last_pass").
				Scan(&passes).Error; err != nil {
				return nil, err
			}

			for i := range passes {
				t := passes[i].LastPass
				lastPassByTestID[passes[i].TestID] = &t
			}
		}

		for _, id := range toSearch {
			if _, ok := lastPassByTestID[id]; !ok {
				notFound = append(notFound, id)
			}
		}

		log.WithFields(log.Fields{
			"windowStart": windowStart.String(),
			"windowEnd":   windowEnd.String(),
			"found":       len(toSearch) - len(notFound),
			"remaining":   len(notFound),
		}).Debug("last_pass iterative scan progress")

		toSearch, notFound = notFound, toSearch[:0]
		windowEnd = windowStart
	}

	return lastPassByTestID, nil
}

// attachOutputs fetches individual failure outputs and attaches them to the results.
func attachOutputs(
	dbc *db.DB,
	results []apitype.RecentTestFailure,
	testIDs []uint,
	release string,
	startDate, endDate civil.Date,
) error {
	outputsByTestID := make(map[uint][]apitype.RecentTestFailureOutput)

	for batchStart := 0; batchStart < len(testIDs); batchStart += maxTestIDsPerQuery {
		batchEnd := batchStart + maxTestIDsPerQuery
		if batchEnd > len(testIDs) {
			batchEnd = len(testIDs)
		}
		batch := testIDs[batchStart:batchEnd]

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
			Joins(`LEFT JOIN prow_job_run_test_outputs ON prow_job_run_test_outputs.prow_job_run_test_id = prow_job_run_tests.id
				AND prow_job_run_test_outputs.prow_job_run_test_release = prow_job_run_tests.prow_job_run_release
				AND prow_job_run_test_outputs.prow_job_run_test_timestamp >= ? AND prow_job_run_test_outputs.prow_job_run_test_timestamp < ?`, startDate, endDate).
			Where("prow_job_run_tests.test_id IN ?", batch).
			Where("prow_job_run_tests.status = ?", int(sippyprocessingv1.TestStatusFailure)).
			Where("prow_job_runs.timestamp >= ? AND prow_job_runs.timestamp < ?", startDate, endDate).
			Where("prow_job_runs.prow_job_release = ?", release).
			Where("prow_job_run_tests.prow_job_run_timestamp >= ? AND prow_job_run_tests.prow_job_run_timestamp < ?", startDate, endDate).
			Where("prow_jobs.release = ?", release).
			Where("prow_job_run_tests.prow_job_run_release = ?", release).
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
			return err
		}

		for _, o := range outputs {
			outputsByTestID[o.TestID] = append(outputsByTestID[o.TestID], apitype.RecentTestFailureOutput{
				ProwJobRunID: o.ProwJobRunID,
				ProwJobName:  o.ProwJobName,
				ProwJobURL:   o.ProwJobURL,
				FailedAt:     o.FailedAt,
				Output:       o.Output,
			})
		}
	}

	for i := range results {
		results[i].Outputs = outputsByTestID[results[i].TestID]
	}

	return nil
}
