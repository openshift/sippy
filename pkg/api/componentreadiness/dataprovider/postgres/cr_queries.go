package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
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

// drilldownFilters holds optional SQL WHERE fragments for TestID and
// Capability filtering when drilling down to a specific test + environment.
type drilldownFilters struct {
	// innerClause filters on test_id via subquery (for the inner aggregation)
	innerClause string
	innerArgs   []any
	// outerClause filters on tow.unique_id and tow.capabilities (for outerQuery and placeholder)
	outerClause string
	outerArgs   []any
}

// buildDrilldownFilters returns SQL filter fragments for TestID and Capability
// from reqOptions.TestIDOptions[0], matching the BQ provider's behavior in
// BuildComponentReportQuery. The tableAlias is the alias of the table that
// has test_id in the inner aggregation query ("e" for prefix-sum, "raw" for GA).
func buildDrilldownFilters(reqOptions reqopts.RequestOptions) drilldownFilters {
	if len(reqOptions.TestIDOptions) != 1 {
		return drilldownFilters{}
	}
	tid := reqOptions.TestIDOptions[0]
	var f drilldownFilters

	if tid.TestID != "" {
		f.innerClause = " AND e.test_id IN (SELECT test_id FROM test_ownerships WHERE unique_id = ?)"
		f.innerArgs = []any{tid.TestID}
		f.outerClause += " AND tow.unique_id = ?"
		f.outerArgs = append(f.outerArgs, tid.TestID)
	}

	if tid.Capability != "" {
		f.outerClause += " AND ? = ANY(tow.capabilities)"
		f.outerArgs = append(f.outerArgs, tid.Capability)
	}

	return f
}

// testStatusSpec parameterizes the shared query structure used by both the
// prefix-sum and GA query paths. The two paths differ only in their source
// table, aggregation expressions, and date/window filter.
type testStatusSpec struct {
	fromTemplate    string // FROM clause template with two %s for variantSubquery and groupMapping
	preJoinArgs     []any  // args bound in the FROM clause before filterArgs (e.g. lookupStart)
	totalExpr       string // SQL expression for total runs
	successExpr     string // SQL expression for successes
	flakeExpr       string // SQL expression for flakes
	lastFailureExpr string // SQL expression for last failure timestamp (e.g. "MAX(e.prefix_max_last_failure)" or "NULL::timestamptz")
	whereFilter     string // WHERE fragment like "e.release = ? AND e.date = ?"
	whereArgs       []any  // args for whereFilter
}

// queryTestStatus builds and executes the failure + placeholder query pair
// that both queryTestStatusPrefixSum and queryBaseTestStatusGA share.
//
// Two queries run in parallel:
//   - Failure query: tests with >= MinimumFailure failures (regression candidates)
//   - Placeholder query: (component, col_group_id) pairs with any runs (for grid gating)
//
// Grid placeholders are only injected for cells where data confirms
// tests actually ran, so that cells without data on one side correctly show
// MissingSample / MissingBasis instead of NotSignificant.
func (p *PostgresProvider) queryTestStatus(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
	includeVariants map[string][]string,
	spec testStatusSpec,
) (map[string]crstatus.TestStatus, []error) {

	includeVariants = mergeRequestedVariants(includeVariants, reqOptions)
	filters := buildDrilldownFilters(reqOptions)

	setup, err := prepareVariantQuery(ctx, p.dbc, includeVariants, reqOptions.VariantOption.DBGroupBy, reqOptions.AdvancedOption.MinimumFailure)
	if err != nil {
		return nil, []error{err}
	}
	if setup == nil {
		return map[string]crstatus.TestStatus{}, nil
	}

	fromClause := fmt.Sprintf(spec.fromTemplate, setup.variantSubquery, setup.groupMapping.valuesClause)

	joinArgs := make([]any, 0, len(spec.preJoinArgs)+len(setup.filterArgs)+len(spec.whereArgs))
	joinArgs = append(joinArgs, spec.preJoinArgs...)
	joinArgs = append(joinArgs, setup.filterArgs...)
	joinArgs = append(joinArgs, spec.whereArgs...)

	failureInner := fmt.Sprintf(`
        SELECT
            e.test_id, e.suite_id, vg.group_id AS variant_group_id,
            SUM(%s) AS total_count,
            SUM(%s) AS success_count,
            SUM(%s) AS flake_count,
            %s AS last_failure
        %s
        WHERE %s`+filters.innerClause+`
        GROUP BY e.test_id, e.suite_id, vg.group_id
        HAVING SUM(%s) > 0
            AND SUM(%s) - SUM(%s) - SUM(%s) >= ?`,
		spec.totalExpr, spec.successExpr, spec.flakeExpr,
		spec.lastFailureExpr,
		fromClause,
		spec.whereFilter,
		spec.totalExpr, spec.totalExpr, spec.successExpr, spec.flakeExpr)

	failureArgs := make([]any, len(joinArgs))
	copy(failureArgs, joinArgs)
	failureArgs = append(failureArgs, filters.innerArgs...)
	failureArgs = append(failureArgs, setup.minimumFailure)

	colMapping := buildColumnGroupMapping(setup.groupMapping.groupToVariants, reqOptions.VariantOption.ColumnGroupBy)

	placeholderQuery := fmt.Sprintf(`
        SELECT
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
        %s
        JOIN test_ownerships tow ON tow.test_id = e.test_id
            AND (tow.suite_id = e.suite_id OR (tow.suite_id IS NULL AND e.suite_id = 0))
            AND tow.staff_approved_obsolete = false
        JOIN (%s) AS cm(group_id, col_group_id) ON cm.group_id = vg.group_id
        WHERE %s`+filters.outerClause+`
        GROUP BY tow.component, tow.capabilities, cm.col_group_id
        HAVING SUM(%s) > 0`,
		fromClause, colMapping.valuesClause, spec.whereFilter, spec.totalExpr)

	placeholderArgs := make([]any, len(joinArgs))
	copy(placeholderArgs, joinArgs)
	placeholderArgs = append(placeholderArgs, filters.outerArgs...)

	return p.runFailureAndPlaceholder(ctx, failureInner, failureArgs, placeholderQuery, placeholderArgs,
		setup.groupMapping, filters)
}

// queryTestStatusPrefixSum queries test_cumulative_summaries using a
// 2-way self-join on prefix sums to compute aggregated counts for a date range.
func (p *PostgresProvider) queryTestStatusPrefixSum(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
	release string,
	includeVariants map[string][]string,
	dateRange query.DateRange,
) (map[string]crstatus.TestStatus, []error) {

	if err := query.ResolveDateRanges(p.dbc, release, &dateRange); err != nil {
		return nil, []error{err}
	}
	lookupEnd := dateRange.End.AddDays(-1)
	lookupStart := dateRange.Start.AddDays(-1)

	return p.queryTestStatus(ctx, reqOptions, includeVariants, testStatusSpec{
		fromTemplate: `
        FROM test_cumulative_summaries e
        LEFT JOIN test_cumulative_summaries s
            ON s.release = e.release AND s.test_id = e.test_id
            AND s.prow_job_id = e.prow_job_id AND s.suite_id = e.suite_id
            AND s.date = ?
        JOIN prow_jobs pj ON pj.id = e.prow_job_id AND pj.deleted_at IS NULL
            AND pj.variant_combination_id IN (%s)
        JOIN (%s) AS vg(vcid, group_id) ON vg.vcid = pj.variant_combination_id`,
		preJoinArgs:     []any{lookupStart},
		totalExpr:       "e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)",
		successExpr:     "e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0)",
		flakeExpr:       "e.prefix_sum_flakes - COALESCE(s.prefix_sum_flakes, 0)",
		lastFailureExpr: "MAX(e.prefix_max_last_failure)",
		whereFilter:     "e.release = ? AND e.date = ?",
		whereArgs:       []any{release, lookupEnd},
	})
}

// queryBaseTestStatusGA queries prow_ga_raw_test_data to compute aggregated
// base test status for GA releases.
func (p *PostgresProvider) queryBaseTestStatusGA(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
	baseRange query.DateRange,
) (map[string]crstatus.TestStatus, []error) {

	release := reqOptions.BaseRelease.Name
	windowDays := baseRange.End.AddDays(-1).DaysSince(baseRange.Start)

	return p.queryTestStatus(ctx, reqOptions, reqOptions.VariantOption.IncludeVariants, testStatusSpec{
		fromTemplate: `
        FROM prow_ga_raw_test_data e
        JOIN prow_jobs pj ON pj.id = e.prow_job_id AND pj.deleted_at IS NULL
            AND pj.variant_combination_id IN (%s)
        JOIN (%s) AS vg(vcid, group_id) ON vg.vcid = pj.variant_combination_id`,
		totalExpr:       "e.runs",
		successExpr:     "e.passes",
		flakeExpr:       "e.flakes",
		lastFailureExpr: "NULL::timestamptz",
		whereFilter:     "e.release = ? AND e.window_days = ?",
		whereArgs:       []any{release, windowDays},
	})
}

// runFailureAndPlaceholder runs the failure query and a placeholder query in
// parallel. The placeholder query groups by (component, col_group_id) to
// identify grid cells that have data, returning rows in the same 10-column
// format as the failure query. After both complete, placeholder entries are
// merged into the failure map for cells that have data but no failures.
func (p *PostgresProvider) runFailureAndPlaceholder(
	ctx context.Context,
	failureInner string,
	failureArgs []any,
	placeholderQuery string,
	placeholderArgs []any,
	groupMapping variantGroupMapping,
	filters drilldownFilters,
) (map[string]crstatus.TestStatus, []error) {

	var failureResult map[string]crstatus.TestStatus
	var failureErrs []error
	var placeholderResult map[string]crstatus.TestStatus
	var placeholderErrs []error

	var wg sync.WaitGroup
	wg.Go(func() {
		failureResult, failureErrs = p.queryAndScan(ctx, failureInner, failureArgs, groupMapping, filters)
	})
	wg.Go(func() {
		placeholderResult, placeholderErrs = p.scanGroupedResults(ctx, placeholderQuery, placeholderArgs, groupMapping)
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
// appends any drill-down filter clauses, and executes with parallel worker hints.
func (p *PostgresProvider) queryAndScan(
	ctx context.Context,
	innerQuery string,
	innerArgs []any,
	groupMapping variantGroupMapping,
	filters drilldownFilters,
) (map[string]crstatus.TestStatus, []error) {
	fullQuery := fmt.Sprintf(outerQuery, innerQuery) + filters.outerClause
	allArgs := make([]any, 0, len(innerArgs)+len(filters.outerArgs))
	allArgs = append(allArgs, innerArgs...)
	allArgs = append(allArgs, filters.outerArgs...)
	return p.scanWithParallelHints(ctx, fullQuery, allArgs, groupMapping)
}

// scanGroupedResults executes a query that is already grouped by
// variant_group_id (not variant_combination_id), mapping each group ID back
// to dimension values via the group mapping.
func (p *PostgresProvider) scanGroupedResults(
	ctx context.Context,
	sqlQuery string,
	args []any,
	groupMapping variantGroupMapping,
) (map[string]crstatus.TestStatus, []error) {
	return p.scanWithParallelHints(ctx, sqlQuery, args, groupMapping)
}

// scanWithParallelHints runs the query inside a transaction that enables
// parallel workers, then scans the results into a test status map.
func (p *PostgresProvider) scanWithParallelHints(
	ctx context.Context,
	sqlQuery string,
	args []any,
	groupMapping variantGroupMapping,
) (map[string]crstatus.TestStatus, []error) {
	var result map[string]crstatus.TestStatus
	txErr := p.dbc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL max_parallel_workers_per_gather = 4; SET LOCAL parallel_setup_cost = 0; SET LOCAL parallel_tuple_cost = 0").Error; err != nil {
			return fmt.Errorf("setting parallel query hints: %w", err)
		}
		var err error
		result, err = scanRows(tx, sqlQuery, args, groupMapping)
		return err
	})
	if txErr != nil {
		return nil, []error{txErr}
	}
	return result, nil
}

func scanRows(
	gormDB *gorm.DB,
	sqlQuery string,
	args []any,
	groupMapping variantGroupMapping,
) (map[string]crstatus.TestStatus, error) {

	rows, err := gormDB.Raw(sqlQuery, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("querying test status: %w", err)
	}
	defer rows.Close()

	result := make(map[string]crstatus.TestStatus)

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
			return nil, fmt.Errorf("scanning row: %w", err)
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

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
