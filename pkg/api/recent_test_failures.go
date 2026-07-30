package api

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
	"k8s.io/apimachinery/pkg/util/sets"
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

	q := buildRecentFailuresQuery(dbc, release, periodStart, reportEnd, previousPeriod)

	subquery := dbc.DB.Table("(?) AS recent_failures", q)

	filteredQuery, err := filter.FilterableDBResult(subquery, filterOpts, apitype.RecentTestFailure{})
	if err != nil {
		return nil, err
	}

	var rowCount int64
	results := make([]apitype.RecentTestFailure, 0)

	scanQuery := filteredQuery.Session(&gorm.Session{NewDB: false})
	if pagination != nil {
		scanQuery = scanQuery.Limit(pagination.PerPage).Offset(pagination.Page * pagination.PerPage)
	}

	var g errgroup.Group
	g.Go(func() error {
		return scanQuery.Scan(&results).Error
	})
	if pagination != nil {
		countQuery := filteredQuery.Session(&gorm.Session{NewDB: false})
		g.Go(func() error {
			return countQuery.Count(&rowCount).Error
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	if pagination == nil {
		rowCount = int64(len(results))
		pagination = &apitype.Pagination{
			PerPage: len(results),
			Page:    0,
		}
	}

	if len(results) > 0 {
		pageKeys := make([]testSuiteKey, 0, len(results))
		for _, r := range results {
			key := testSuiteKey{testID: r.TestID}
			if r.SuiteID != nil {
				key.suiteID = *r.SuiteID
			}
			pageKeys = append(pageKeys, key)
		}

		var lastPassMap map[testSuiteKey]*time.Time
		var outputMap map[testSuiteKey][]apitype.RecentTestFailureOutput

		var followUp errgroup.Group
		followUp.Go(func() error {
			var err error
			lastPassMap, err = findLastPass(dbc, pageKeys, release, reportEnd)
			return err
		})
		if includeOutputs {
			followUp.Go(func() error {
				var err error
				outputMap, err = fetchOutputs(dbc, pageKeys, release, periodStart, reportEnd)
				return err
			})
		}
		if err := followUp.Wait(); err != nil {
			return nil, err
		}

		for i := range results {
			key := pageKeys[i]
			results[i].LastPass = lastPassMap[key]
			if outputMap != nil {
				results[i].Outputs = outputMap[key]
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

// buildRecentFailuresQuery builds the main aggregation query using a reversed join
// order (prow_job_run_tests first, filtered by the status index) and deferred
// dimension lookups (tests/suites/test_ownerships joined after aggregation).
func buildRecentFailuresQuery(
	dbc *db.DB,
	release string,
	periodStart, reportEnd time.Time,
	previousPeriod *time.Duration,
) *gorm.DB {
	statusFailure := int(sippyprocessingv1.TestStatusFailure)

	innerSQL := `SELECT pjrt.test_id, pjrt.suite_id,
			COUNT(*) AS failure_count,
			MIN(pjrt.prow_job_run_timestamp) AS first_failure,
			MAX(pjrt.prow_job_run_timestamp) AS last_failure
		FROM prow_job_run_tests pjrt
		WHERE pjrt.status = ?
			AND pjrt.prow_job_run_release = ?
			AND pjrt.prow_job_run_timestamp >= ? AND pjrt.prow_job_run_timestamp < ?
			AND pjrt.deleted_at IS NULL`

	params := []interface{}{
		statusFailure, release, periodStart, reportEnd,
	}

	if previousPeriod != nil {
		prevStart := periodStart.Add(-*previousPeriod)
		innerSQL += `
			AND NOT EXISTS (
				SELECT 1 FROM prow_job_run_tests prev_t
				WHERE prev_t.test_id = pjrt.test_id
					AND prev_t.suite_id IS NOT DISTINCT FROM pjrt.suite_id
					AND prev_t.status = ?
					AND prev_t.prow_job_run_release = ?
					AND prev_t.prow_job_run_timestamp >= ? AND prev_t.prow_job_run_timestamp < ?
					AND prev_t.deleted_at IS NULL
			)`
		params = append(params,
			statusFailure, release, prevStart, periodStart,
		)
	}

	innerSQL += `
		GROUP BY pjrt.test_id, pjrt.suite_id`

	outerSQL := fmt.Sprintf(`SELECT agg.test_id, agg.suite_id, t.name AS test_name, s.name AS suite_name,
			tow.jira_component AS jira_component,
			agg.failure_count, agg.first_failure, agg.last_failure
		FROM (%s) agg
		JOIN tests t ON t.id = agg.test_id AND t.deleted_at IS NULL
		LEFT JOIN suites s ON s.id = agg.suite_id
		LEFT JOIN test_ownerships tow ON tow.test_id = agg.test_id AND tow.suite_id IS NOT DISTINCT FROM agg.suite_id AND tow.deleted_at IS NULL`, innerSQL)

	return dbc.DB.Raw(outerSQL, params...)
}

type testSuiteKey struct {
	testID  uint
	suiteID uint // 0 represents NULL suite_id
}

// findLastPass finds the most recent successful run for each (test_id, suite_id)
// pair by sliding backwards one day at a time from reportEnd. This approach
// ensures each query hits a single partition for fast execution while
// guaranteeing the actual last pass is found (up to 90 days back).
func findLastPass(
	dbc *db.DB,
	keys []testSuiteKey,
	release string,
	reportEnd time.Time,
) (map[testSuiteKey]*time.Time, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	remaining := sets.New(keys...)
	result := make(map[testSuiteKey]*time.Time, len(keys))

	endDate := civil.DateOf(reportEnd.UTC())
	limitDate := endDate.AddDays(-90)

	for date := endDate; remaining.Len() > 0 && !date.Before(limitDate); date = date.AddDays(-1) {
		testIDs := testIDsFromKeys(remaining)

		var passes []struct {
			TestID   uint      `gorm:"column:test_id"`
			SuiteID  *uint     `gorm:"column:suite_id"`
			LastPass time.Time `gorm:"column:last_pass"`
		}

		if err := dbc.DB.Table("prow_job_run_tests pjrt").
			Where("pjrt.test_id IN ?", testIDs).
			Where("pjrt.status = ?", int(sippyprocessingv1.TestStatusSuccess)).
			Where("pjrt.prow_job_run_release = ?", release).
			Where("pjrt.prow_job_run_timestamp >= ? AND pjrt.prow_job_run_timestamp < ?", date, date.AddDays(1)).
			Where("pjrt.deleted_at IS NULL").
			Group("pjrt.test_id, pjrt.suite_id").
			Select("pjrt.test_id AS test_id, pjrt.suite_id AS suite_id, MAX(pjrt.prow_job_run_timestamp) AS last_pass").
			Scan(&passes).Error; err != nil {
			return nil, err
		}

		for i := range passes {
			key := testSuiteKey{testID: passes[i].TestID}
			if passes[i].SuiteID != nil {
				key.suiteID = *passes[i].SuiteID
			}
			if remaining.Has(key) {
				t := passes[i].LastPass
				result[key] = &t
				remaining.Delete(key)
			}
		}
	}

	return result, nil
}

func testIDsFromKeys(keys sets.Set[testSuiteKey]) []uint {
	ids := sets.New[uint]()
	for key := range keys {
		ids.Insert(key.testID)
	}
	return ids.UnsortedList()
}

// fetchOutputs retrieves individual failure outputs for the given test/suite
// pairs, grouped by (test_id, suite_id). Operates only on the current page.
func fetchOutputs(
	dbc *db.DB,
	keys []testSuiteKey,
	release string,
	periodStart, reportEnd time.Time,
) (map[testSuiteKey][]apitype.RecentTestFailureOutput, error) {
	testIDs := testIDsFromKeys(sets.New(keys...))

	var outputs []struct {
		TestID       uint      `gorm:"column:test_id"`
		SuiteID      *uint     `gorm:"column:suite_id"`
		ProwJobRunID uint      `gorm:"column:prow_job_run_id"`
		ProwJobName  string    `gorm:"column:prow_job_name"`
		ProwJobURL   string    `gorm:"column:prow_job_url"`
		FailedAt     time.Time `gorm:"column:failed_at"`
		Output       string    `gorm:"column:output"`
	}

	if err := dbc.DB.Table("prow_job_run_tests pjrt").
		Joins("JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id").
		Joins("JOIN prow_jobs pj ON pj.id = pjr.prow_job_id").
		Joins(`LEFT JOIN prow_job_run_test_outputs pjrto ON pjrto.prow_job_run_test_id = pjrt.id
			AND pjrto.prow_job_run_test_release = pjrt.prow_job_run_release
			AND pjrto.prow_job_run_test_timestamp = pjrt.prow_job_run_timestamp`).
		Where("pjrt.test_id IN ?", testIDs).
		Where("pjrt.status = ?", int(sippyprocessingv1.TestStatusFailure)).
		Where("pjrt.prow_job_run_release = ?", release).
		Where("pjrt.prow_job_run_timestamp >= ? AND pjrt.prow_job_run_timestamp < ?", periodStart, reportEnd).
		Where("pjrt.deleted_at IS NULL").
		Where("pjr.prow_job_release = ?", release).
		Where("pjr.timestamp >= ? AND pjr.timestamp < ?", periodStart, reportEnd).
		Where("pjr.deleted_at IS NULL").
		Where("pj.deleted_at IS NULL").
		Select(`pjrt.test_id AS test_id,
			pjrt.suite_id AS suite_id,
			pjr.id AS prow_job_run_id,
			pj.name AS prow_job_name,
			pjr.url AS prow_job_url,
			pjr.timestamp AS failed_at,
			COALESCE(pjrto.output, '') AS output`).
		Scan(&outputs).Error; err != nil {
		return nil, err
	}

	result := make(map[testSuiteKey][]apitype.RecentTestFailureOutput)
	for _, o := range outputs {
		key := testSuiteKey{testID: o.TestID}
		if o.SuiteID != nil {
			key.suiteID = *o.SuiteID
		}
		result[key] = append(result[key], apitype.RecentTestFailureOutput{
			ProwJobRunID: o.ProwJobRunID,
			ProwJobName:  o.ProwJobName,
			ProwJobURL:   o.ProwJobURL,
			FailedAt:     o.FailedAt,
			Output:       o.Output,
		})
	}

	return result, nil
}
