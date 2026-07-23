package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db"
)

// variantQuerySetup holds the shared prologue results used by both the
// prefix-sum and GA query paths.
type variantQuerySetup struct {
	groupMapping    variantGroupMapping
	filterArgs      []any
	variantSubquery string
	minimumFailure  int
}

func prepareVariantQuery(
	ctx context.Context,
	dbc *db.DB,
	includeVariants map[string][]string,
	dbGroupBy sets.Set[string],
	minimumFailure int,
) (*variantQuerySetup, error) {
	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}

	variantLookup, err := lookupVariantValues(ctx, dbc, includeVariants, dbGroupBy)
	if err != nil {
		return nil, err
	}
	if len(variantLookup) == 0 {
		return nil, nil
	}

	groupMapping := buildVariantGroupMapping(variantLookup)

	filterClause, filterArgs := buildVariantFilterClause(includeVariants)

	variantSubquery := "SELECT vc.id FROM variant_combinations vc"
	if filterClause != "" {
		variantSubquery += " WHERE " + filterClause
	}

	return &variantQuerySetup{
		groupMapping:    groupMapping,
		filterArgs:      filterArgs,
		variantSubquery: variantSubquery,
		minimumFailure:  minimumFailure,
	}, nil
}

// lastFailureLateral wraps a failure aggregation subquery with a LATERAL join
// that finds the most recent failure timestamp for each (test_id, suite_id).
// Parameters after the inner query args: release, rangeStart, rangeEnd.
const lastFailureLateral = `
        SELECT fi.test_id, fi.suite_id, fi.variant_group_id,
               fi.total_count, fi.success_count, fi.flake_count,
               lf.last_failure
        FROM (%s) fi
        LEFT JOIN LATERAL (
            SELECT MAX(pjrt.prow_job_run_timestamp) AS last_failure
            FROM prow_job_run_tests pjrt
            WHERE pjrt.test_id = fi.test_id
              AND (pjrt.suite_id = fi.suite_id OR (fi.suite_id = 0 AND pjrt.suite_id IS NULL))
              AND pjrt.prow_job_run_release = ?
              AND pjrt.prow_job_run_timestamp BETWEEN ? AND ?
              AND pjrt.status = 12
        ) lf ON true`

// querySampleTestStatusPrefixSum queries test_cumulative_summaries using a
// 2-way self-join on prefix sums to compute aggregated counts for a date range.
//
// Two queries run in parallel:
//   - Failure query: tests with >= MinimumFailure failures (regression candidates)
//   - Existence query: DISTINCT (component, group_id) with any runs (for grid gating)
//
// Grid placeholders are only injected for cells where existence data confirms
// tests actually ran, so that cells without data on one side correctly show
// MissingSample / MissingBasis instead of NotSignificant.
func (p *PostgresProvider) queryTestStatusPrefixSum(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
	release string,
	includeVariants map[string][]string,
	start, end time.Time,
) (map[string]crstatus.TestStatus, []error) {

	setup, err := prepareVariantQuery(ctx, p.dbc, includeVariants, reqOptions.VariantOption.DBGroupBy, reqOptions.AdvancedOption.MinimumFailure)
	if err != nil {
		return nil, []error{err}
	}
	if setup == nil {
		return map[string]crstatus.TestStatus{}, nil
	}

	endDate := civil.DateOf(end.AddDate(0, 0, -1))
	startDate := civil.DateOf(start.AddDate(0, 0, -1))

	endDate, startDate, err = p.clampToAvailableDates(ctx, release, endDate, startDate)
	if err != nil {
		return nil, []error{err}
	}

	// Shared join fragment for both failure and existence queries.
	prefixSumJoin := fmt.Sprintf(`
        FROM test_cumulative_summaries e
        LEFT JOIN test_cumulative_summaries s
            ON s.release = e.release AND s.test_id = e.test_id
            AND s.prow_job_id = e.prow_job_id AND s.suite_id = e.suite_id
            AND s.date = ?
        JOIN prow_jobs pj ON pj.id = e.prow_job_id AND pj.deleted_at IS NULL
            AND pj.variant_combination_id IN (%s)
        JOIN (%s) AS vg(vcid, group_id) ON vg.vcid = pj.variant_combination_id
        WHERE e.release = ? AND e.date = ?
        GROUP BY e.test_id, e.suite_id, vg.group_id`,
		setup.variantSubquery, setup.groupMapping.valuesClause)

	joinArgs := []any{startDate}
	joinArgs = append(joinArgs, setup.filterArgs...)
	joinArgs = append(joinArgs, release, endDate)

	failureAgg := fmt.Sprintf(`
        SELECT
            e.test_id, e.suite_id, vg.group_id AS variant_group_id,
            SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)) AS total_count,
            SUM(e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0)) AS success_count,
            SUM(e.prefix_sum_flakes - COALESCE(s.prefix_sum_flakes, 0)) AS flake_count
        %s
        HAVING SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)) > 0
            AND SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0))
              - SUM(e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0)) >= ?`,
		prefixSumJoin)

	failureInner := fmt.Sprintf(lastFailureLateral, failureAgg)

	failureArgs := make([]any, len(joinArgs))
	copy(failureArgs, joinArgs)
	failureArgs = append(failureArgs, setup.minimumFailure, release, start, end)

	existenceInner := fmt.Sprintf(`
        SELECT e.test_id, e.suite_id, vg.group_id AS variant_group_id
        %s
        HAVING SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)) > 0`,
		prefixSumJoin)

	existenceArgs := make([]any, len(joinArgs))
	copy(existenceArgs, joinArgs)

	return p.runFailureAndExistence(ctx, failureInner, failureArgs, existenceInner, existenceArgs,
		setup.groupMapping, reqOptions.VariantOption.ColumnGroupBy)
}

// queryBaseTestStatusGA queries prow_ga_raw_test_data to compute aggregated
// base test status for GA releases. Like the sample query, it runs failure and
// existence queries in parallel to support Missing* status detection.
func (p *PostgresProvider) queryBaseTestStatusGA(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
) (map[string]crstatus.TestStatus, []error) {

	setup, err := prepareVariantQuery(ctx, p.dbc, reqOptions.VariantOption.IncludeVariants, reqOptions.VariantOption.DBGroupBy, reqOptions.AdvancedOption.MinimumFailure)
	if err != nil {
		return nil, []error{err}
	}
	if setup == nil {
		return map[string]crstatus.TestStatus{}, nil
	}

	release := reqOptions.BaseRelease.Name
	windowDays := int(reqOptions.BaseRelease.End.Sub(reqOptions.BaseRelease.Start).Hours() / 24)

	gaJoin := fmt.Sprintf(`
        FROM prow_ga_raw_test_data raw
        JOIN prow_jobs pj ON pj.id = raw.prow_job_id AND pj.deleted_at IS NULL
            AND pj.variant_combination_id IN (%s)
        JOIN (%s) AS vg(vcid, group_id) ON vg.vcid = pj.variant_combination_id
        WHERE raw.release = ? AND raw.window_days = ?
        GROUP BY raw.test_id, raw.suite_id, vg.group_id`,
		setup.variantSubquery, setup.groupMapping.valuesClause)

	joinArgs := make([]any, 0, len(setup.filterArgs)+2)
	joinArgs = append(joinArgs, setup.filterArgs...)
	joinArgs = append(joinArgs, release, windowDays)

	failureAgg := fmt.Sprintf(`
        SELECT
            raw.test_id, raw.suite_id, vg.group_id AS variant_group_id,
            SUM(raw.runs) AS total_count,
            SUM(raw.passes) AS success_count,
            SUM(raw.flakes) AS flake_count
        %s
        HAVING SUM(raw.runs) > 0
            AND SUM(raw.runs) - SUM(raw.passes) >= ?`,
		gaJoin)

	failureInner := fmt.Sprintf(lastFailureLateral, failureAgg)

	baseStart := reqOptions.BaseRelease.Start
	baseEnd := reqOptions.BaseRelease.End
	failureArgs := make([]any, len(joinArgs))
	copy(failureArgs, joinArgs)
	failureArgs = append(failureArgs, setup.minimumFailure, release, baseStart, baseEnd)

	existenceInner := fmt.Sprintf(`
        SELECT raw.test_id, raw.suite_id, vg.group_id AS variant_group_id
        %s
        HAVING SUM(raw.runs) > 0`,
		gaJoin)

	existenceArgs := make([]any, len(joinArgs))
	copy(existenceArgs, joinArgs)

	return p.runFailureAndExistence(ctx, failureInner, failureArgs, existenceInner, existenceArgs,
		setup.groupMapping, reqOptions.VariantOption.ColumnGroupBy)
}

// placeholderOuterQuery wraps the inner existence subquery to produce
// placeholder rows in the same 9-column format as outerQuery. Each row
// represents a (component, column) cell with data, using synthetic test IDs
// and placeholder counts (1 total, 1 success) so they evaluate as NotSignificant.
// The column group mapping (cm) maps real group_ids to column-level col_group_ids.
const placeholderOuterQuery = `SELECT DISTINCT
    'grid:' || tow.component AS test_id,
    '' AS test_name,
    '' AS test_suite,
    tow.component,
    tow.capabilities,
    cm.col_group_id AS variant_group_id,
    1 AS total_count,
    1 AS success_count,
    0 AS flake_count,
    NULL::timestamptz AS last_failure
FROM (%s) pa
JOIN test_ownerships tow ON tow.test_id = pa.test_id
    AND (tow.suite_id = pa.suite_id OR (tow.suite_id IS NULL AND pa.suite_id = 0))
JOIN (%s) AS cm(group_id, col_group_id) ON cm.group_id = pa.variant_group_id
WHERE tow.staff_approved_obsolete = false`

// runFailureAndExistence runs the failure query and a placeholder query in
// parallel. The placeholder query reshapes existence data into the same
// 9-column format as the failure query, using column-level variant groups and
// synthetic test IDs. After both complete, placeholder entries are merged into
// the failure map for cells that have data but no failures.
func (p *PostgresProvider) runFailureAndExistence(
	ctx context.Context,
	failureInner string,
	failureArgs []any,
	existenceInner string,
	existenceArgs []any,
	groupMapping variantGroupMapping,
	columnGroupBy sets.Set[string],
) (map[string]crstatus.TestStatus, []error) {

	colMapping := buildColumnGroupMapping(groupMapping.groupToVariants, columnGroupBy)
	placeholderQuery := fmt.Sprintf(placeholderOuterQuery, existenceInner, colMapping.valuesClause)

	var failureResult map[string]crstatus.TestStatus
	var failureErrs []error
	var placeholderResult map[string]crstatus.TestStatus
	var placeholderErrs []error

	var wg sync.WaitGroup
	wg.Go(func() {
		failureResult, failureErrs = p.queryAndScan(ctx, failureInner, failureArgs, groupMapping)
	})
	wg.Go(func() {
		placeholderResult, placeholderErrs = p.scanGroupedResults(ctx, placeholderQuery, existenceArgs, groupMapping)
	})
	wg.Wait()

	if len(failureErrs) > 0 || len(placeholderErrs) > 0 {
		var errs []error
		errs = append(errs, failureErrs...)
		errs = append(errs, placeholderErrs...)
		return nil, errs
	}

	merged := 0
	for k, v := range placeholderResult {
		if _, exists := failureResult[k]; !exists {
			failureResult[k] = v
			merged++
		}
	}
	log.WithField("placeholders", len(placeholderResult)).
		WithField("merged", merged).
		WithField("failures", len(failureResult)-merged).
		WithField("total", len(failureResult)).
		Info("placeholder query complete")
	return failureResult, nil
}

// outerQuery wraps an inner aggregation subquery with the shared outer SELECT
// that joins tests, test_ownerships, and suites to produce the final result
// columns. Both sample and base queries use the same outer structure.
const outerQuery = `SELECT
    tow.unique_id AS test_id,
    t.name AS test_name,
    COALESCE(su.name, '') AS test_suite,
    tow.component,
    tow.capabilities,
    pa.variant_group_id,
    pa.total_count,
    pa.success_count,
    pa.flake_count,
    pa.last_failure
FROM (%s) pa
JOIN tests t ON t.id = pa.test_id
JOIN test_ownerships tow ON tow.test_id = pa.test_id
    AND (tow.suite_id = pa.suite_id OR (tow.suite_id IS NULL AND pa.suite_id = 0))
LEFT JOIN suites su ON su.id = pa.suite_id
WHERE tow.staff_approved_obsolete = false`

// queryAndScan wraps an inner aggregation subquery with the shared outer query,
// executes it within a transaction with optimizer hints, and scans the results
// into a test status map keyed by variant group.
func (p *PostgresProvider) queryAndScan(
	ctx context.Context,
	innerQuery string,
	innerArgs []any,
	groupMapping variantGroupMapping,
) (map[string]crstatus.TestStatus, []error) {

	query := fmt.Sprintf(outerQuery, innerQuery)
	return p.scanGroupedResults(ctx, query, innerArgs, groupMapping)
}

// scanGroupedResults executes a query within a transaction that disables
// nested loops (to force parallel hash joins), scans rows that are already
// grouped by variant_group_id (not variant_combination_id), and maps each
// group ID back to dimension values via the group mapping.
//
// Because the SQL already aggregates by group ID, each (test_id, group_id) pair
// appears exactly once. No Go-side accumulation is needed.
func (p *PostgresProvider) scanGroupedResults(
	ctx context.Context,
	query string,
	args []any,
	groupMapping variantGroupMapping,
) (map[string]crstatus.TestStatus, []error) {

	var result map[string]crstatus.TestStatus

	txErr := p.dbc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stmt := range []string{
			"SET LOCAL enable_nestloop = off",
			"SET LOCAL max_parallel_workers_per_gather = 4",
		} {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("executing %q: %w", stmt, err)
			}
		}

		rows, err := tx.Raw(query, args...).Rows()
		if err != nil {
			return fmt.Errorf("querying test status: %w", err)
		}
		defer rows.Close()

		result = make(map[string]crstatus.TestStatus)

		for rows.Next() {
			var testID, testName, testSuite, component string
			var capabilities pq.StringArray
			var variantGroupID int
			var totalCount, successCount, flakeCount int
			var lastFailure sql.NullTime

			if err := rows.Scan(
				&testID, &testName, &testSuite, &component, &capabilities,
				&variantGroupID, &totalCount, &successCount, &flakeCount,
				&lastFailure,
			); err != nil {
				return fmt.Errorf("scanning row: %w", err)
			}

			variantMap := groupMapping.groupToVariants[variantGroupID]

			key := crtest.KeyWithVariants{
				TestID:   testID,
				Variants: variantMap,
			}
			keyStr := key.Encode()

			ts := crstatus.TestStatus{
				TestID:       testID,
				TestName:     testName,
				TestSuite:    testSuite,
				Component:    component,
				Capabilities: capabilities,
				Variants:     variantMap,
				Count: crtest.Count{
					TotalCount:   totalCount,
					SuccessCount: successCount,
					FlakeCount:   flakeCount,
				},
			}
			if lastFailure.Valid {
				ts.LastFailure = lastFailure.Time
			}
			result[keyStr] = ts
		}

		return rows.Err()
	})

	if txErr != nil {
		return nil, []error{txErr}
	}
	return result, nil
}

// clampToAvailableDates adjusts endDate and startDate so they do not exceed the
// latest date with data in test_cumulative_summaries for the given release. When
// data ingestion lags behind the requested time window (e.g. "now-7d to now"
// resolves to a date not yet backfilled), the prefix-sum query would return zero
// rows without this clamping.
func (p *PostgresProvider) clampToAvailableDates(ctx context.Context, release string, endDate, startDate civil.Date) (civil.Date, civil.Date, error) {
	var maxDate *civil.Date
	err := p.dbc.DB.WithContext(ctx).
		Table("test_cumulative_summaries").
		Select("MAX(date)").
		Where("release = ?", release).
		Row().Scan(&maxDate)
	if err != nil {
		return endDate, startDate, fmt.Errorf("resolving max date for release %s: %w", release, err)
	}
	if maxDate == nil {
		return endDate, startDate, nil
	}
	if endDate.After(*maxDate) {
		delta := endDate.DaysSince(*maxDate)
		endDate = *maxDate
		startDate = startDate.AddDays(-delta)
	}
	return endDate, startDate, nil
}
