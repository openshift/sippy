package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"strings"
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
	"github.com/openshift/sippy/pkg/db/query"
)

// variantQuerySetup holds the shared prologue results used by both the
// prefix-sum and GA query paths.
type variantQuerySetup struct {
	groupMapping    variantGroupMapping
	filterArgs      []any
	variantSubquery string
}

const queryPlannerHints = "SET LOCAL max_parallel_workers_per_gather = 4; SET LOCAL parallel_setup_cost = 0; SET LOCAL parallel_tuple_cost = 0; SET LOCAL enable_nestloop = off; SET LOCAL enable_sort = off"

func prepareVariantQuery(
	ctx context.Context,
	dbc *db.DB,
	includeVariants map[string][]string,
	dbGroupBy sets.Set[string],
) (*variantQuerySetup, error) {
	vf, err := resolveVariantFilter(ctx, dbc, includeVariants, dbGroupBy)
	if err != nil {
		return nil, err
	}
	if len(vf.lookup) == 0 {
		return nil, nil
	}

	return &variantQuerySetup{
		groupMapping:    buildVariantGroupMapping(vf.lookup),
		filterArgs:      vf.filterArgs,
		variantSubquery: vf.variantSubquery,
	}, nil
}

// drilldownFilters holds optional SQL WHERE fragments for TestID, Component,
// and Capability filtering when drilling down to a specific test + environment.
type drilldownFilters struct {
	// innerClause filters on test_id and optionally component via subquery (for the inner aggregation).
	// Component filtering here reduces join cost when drilling down to a specific component.
	innerClause string
	innerArgs   []any
	// outerClause filters on tow.unique_id, tow.component, and tow.capabilities (for outerQuery and placeholder).
	// Capability filtering happens at this level to respect the top-level Capabilities array.
	outerClause string
	outerArgs   []any
}

// buildDrilldownFilters returns SQL filter fragments for TestID, Component,
// Capability, and top-level capability filtering from reqOptions. For
// drilldown (single TestIDOption), it filters on test_id, component, and
// per-test capability. For top-level views, it applies the Capabilities
// array-overlap filter matching the BQ provider's behavior.
//
// Component filtering is pushed into innerClause (a subquery against the
// raw aggregation input, same as TestID) rather than only the outer
// test_ownerships join, so a component-scoped request narrows the
// expensive join through prow_jobs/variant combinations before it runs,
// not just the final result set. This matters when reqOptions.IncludeAllTests
// drops the MinimumFailure gate (see queryCombinedTestStatus): an unscoped
// includeAllTests request is allowed to be slow, but a component- or
// capability-scoped one should stay cheap.
func buildDrilldownFilters(reqOptions reqopts.RequestOptions) drilldownFilters {
	var f drilldownFilters

	if len(reqOptions.TestIDOptions) == 1 {
		tid := reqOptions.TestIDOptions[0]

		if tid.TestID != "" {
			f.innerClause += " AND e.test_id IN (SELECT test_id FROM test_ownerships WHERE unique_id = ?)"
			f.innerArgs = append(f.innerArgs, tid.TestID)
			f.outerClause += " AND tow.unique_id = ?"
			f.outerArgs = append(f.outerArgs, tid.TestID)
		}


		if tid.Capability != "" {
			f.innerClause += " AND e.test_id IN (SELECT test_id FROM test_ownerships WHERE ? = ANY(capabilities))"
			f.innerArgs = append(f.innerArgs, tid.Capability)
			f.outerClause += " AND ? = ANY(tow.capabilities)"
			f.outerArgs = append(f.outerArgs, tid.Capability)
		}
	}

	if len(reqOptions.Capabilities) > 0 {
		f.outerClause += " AND tow.capabilities && ?"
		f.outerArgs = append(f.outerArgs, pq.Array(reqOptions.Capabilities))
	}

	if reqOptions.AdvancedOption.IgnoreDisruption {
		f.outerClause += " AND NOT ('Disruption' = ANY(tow.capabilities))"
	}

	return f
}

// testStatusSpec parameterizes the inner aggregation query used by both the
// prefix-sum and GA query paths. Each spec produces an inner SELECT with
// columns: test_id, suite_id, variant_group_id, total_count, success_count,
// flake_count, last_failure. The outer CTE wrapper (test_ownerships join,
// column group mapping) is identical for all specs and handled by buildStatusCTE.
type testStatusSpec struct {
	fromTemplate string // FROM clause template with one %s for the formatted prowJobJoin
	fromArgs     []any  // args for FROM clause (before filterArgs)
	selectCols   string // the 4 aggregation column expressions (total_count through last_failure)
	selectArgs   []any  // args for placeholders within selectCols (e.g., lookupEnd/lookupStart for CASE WHEN)
	whereFilter  string // WHERE fragment like "e.release = ? AND e.date IN (?, ?)"
	whereArgs    []any  // args for whereFilter
	havingClause string // optional HAVING clause (e.g., "\nHAVING SUM(e.runs) > 0")
	lifecycles   []string
}

// buildInnerAggregation constructs the inner SELECT ... GROUP BY from a
// testStatusSpec and a pre-formatted prow job join clause. The result produces
// columns: test_id, suite_id, variant_group_id, total_count, success_count,
// flake_count, last_failure.
func buildInnerAggregation(spec testStatusSpec, prowJobJoin string, filterArgs []any, filters drilldownFilters) (string, []any) {
	fromClause := fmt.Sprintf(spec.fromTemplate, prowJobJoin)

	lifecycleClause := ""
	var lifecycleArgs []any
	if len(spec.lifecycles) > 0 {
		lifecycleClause = "\n                AND e.lifecycle = ANY(?)"
		lifecycleArgs = []any{pq.Array(spec.lifecycles)}
	}

	innerSQL := fmt.Sprintf(`
            SELECT e.test_id, e.suite_id, vg.group_id AS variant_group_id,
                %s
            %s
            WHERE %s%s%s
            GROUP BY e.test_id, e.suite_id, vg.group_id%s`,
		spec.selectCols,
		fromClause,
		spec.whereFilter, lifecycleClause, filters.innerClause,
		spec.havingClause)

	var args []any
	args = append(args, spec.selectArgs...)
	args = append(args, spec.fromArgs...)
	args = append(args, filterArgs...)
	args = append(args, spec.whereArgs...)
	args = append(args, lifecycleArgs...)
	args = append(args, filters.innerArgs...)
	return innerSQL, args
}

// buildStatusCTE wraps an inner aggregation query in a materialized CTE that
// joins test_ownerships (using COALESCE for suite_id NULL handling) and the
// column group mapping CTE. The resulting CTE has columns: test_id, suite_id,
// variant_group_id, total_count, success_count, flake_count, last_failure,
// col_group_id, unique_id, component, capabilities.
//
// colMappingCTE is the name of a CTE (defined earlier in the WITH clause)
// with columns (group_id, col_group_id).
func buildStatusCTE(
	cteName string,
	innerSQL string,
	innerArgs []any,
	colMappingCTE string,
	filters drilldownFilters,
) (string, []any) {
	cteSQL := fmt.Sprintf(`%s AS MATERIALIZED (
        SELECT agg.*, cm.col_group_id, tow.unique_id, tow.component, tow.capabilities FROM (
            %s
        ) agg
        JOIN %s cm ON cm.group_id = agg.variant_group_id
        JOIN test_ownerships tow ON tow.test_id = agg.test_id
            AND COALESCE(tow.suite_id, 0) = agg.suite_id
            AND tow.staff_approved_obsolete = false
        WHERE agg.total_count > 0%s
    )`, cteName, innerSQL, colMappingCTE, filters.outerClause)

	var args []any
	args = append(args, innerArgs...)
	args = append(args, filters.outerArgs...)
	return cteSQL, args
}

// testBranchTemplate is the UNION ALL branch that selects all tests with runs
// from a status CTE, regardless of failure count. Used by the standalone path
// and by the combined query when IncludeAllTests=true (TRT-2923). The standalone
// path has no "other side" to cross-reference (TRT-2883), so the MinimumFailure
// threshold is left entirely to the Go-side check. The combined query's
// IncludeAllTests mode uses this template for both sample and base sides,
// allowing tests below MinimumFailure to be surfaced in includeAllTests listings.
// Component and Capability filtering (via drilldownFilters) is still applied
// regardless of which template is chosen. The first %s is an optional column
// prefix, the second %s is the CTE name.
const testBranchTemplate = `SELECT
        %spa.unique_id AS test_id, t.name AS test_name,
        COALESCE(su.name, '') AS test_suite, pa.component, pa.capabilities,
        pa.variant_group_id, pa.total_count, pa.success_count, pa.flake_count, pa.last_failure
    FROM %s pa
    JOIN tests t ON t.id = pa.test_id
    LEFT JOIN suites su ON su.id = pa.suite_id`

// failureBranchTemplate is the UNION ALL branch that selects tests meeting the
// minimum failure threshold from a status CTE. The first %s is the column
// prefix (e.g. "'S'::text AS source, " for the combined query), the second %s
// is the CTE name.
const failureBranchTemplate = `SELECT
        %spa.unique_id AS test_id, t.name AS test_name,
        COALESCE(su.name, '') AS test_suite, pa.component, pa.capabilities,
        pa.variant_group_id, pa.total_count, pa.success_count, pa.flake_count, pa.last_failure
    FROM %s pa
    JOIN tests t ON t.id = pa.test_id
    LEFT JOIN suites su ON su.id = pa.suite_id
    WHERE pa.total_count - pa.success_count - pa.flake_count >= ?`

// keysCTETemplate projects a status CTE down to just the columns needed to
// answer "does this (unique_id, variant_group_id) exist on this side, and is
// it above the minimum failure threshold?" The status CTEs (sample_agg /
// base_agg) are MATERIALIZED and carry every output column (component,
// capabilities text[], last_failure, etc.) on every row; joining directly
// against them to answer a yes/no existence question forces Postgres to
// build a hash table over the full row width. Materializing this narrow
// projection once lets belowThresholdRescueBranchTemplate join against a
// 3-column table instead. The first %s is the CTE name to define, the
// second %s is the source status CTE.
const keysCTETemplate = `%s AS MATERIALIZED (
        SELECT unique_id, variant_group_id,
            total_count - success_count - flake_count AS fail_count
        FROM %s
    )`

// belowThresholdRescueBranchTemplate selects tests below the minimum failure
// threshold that should still be surfaced because the other side doesn't
// silently absorb them: either (a) the other side has a matching row that's
// >= threshold (a test crossing the threshold between sample and base,
// TRT-2883), or (b) the other side has no matching row at all (a test that
// only ran during this side's window, e.g. newly added or removed) — without
// this, such a test is silently dropped from both result maps even though
// BigQuery has no SQL threshold filter and would surface it as
// MissingBasis/MissingSample.
//
// Both conditions are expressed as a single LEFT JOIN against the other
// side's narrow keys CTE (see keysCTETemplate) rather than two separate
// correlated EXISTS/NOT EXISTS subqueries against the full status CTE, so
// the planner only needs to hash/join a small (key, key, fail_count) table
// once instead of hashing the full-width CTE twice. The first %s is the
// column prefix, the second %s is this side's CTE, the third %s is the
// other side's keys CTE.
const belowThresholdRescueBranchTemplate = `SELECT
        %spa.unique_id AS test_id, t.name AS test_name,
        COALESCE(su.name, '') AS test_suite, pa.component, pa.capabilities,
        pa.variant_group_id, pa.total_count, pa.success_count, pa.flake_count, pa.last_failure
    FROM %s pa
    JOIN tests t ON t.id = pa.test_id
    LEFT JOIN suites su ON su.id = pa.suite_id
    LEFT JOIN %s other
      ON other.unique_id = pa.unique_id
      AND other.variant_group_id = pa.variant_group_id
    WHERE pa.total_count - pa.success_count - pa.flake_count < ?
      AND (
        other.unique_id IS NULL
        OR other.fail_count >= ?
      )`

// placeholderBranchTemplate is the UNION ALL branch that produces grid
// placeholder entries (one per component + col_group_id) from a status CTE.
// The first %s is an optional column prefix, the second %s is the CTE name.
const placeholderBranchTemplate = `SELECT
        %s'grid:' || pa.component AS test_id, '' AS test_name, '' AS test_suite,
        pa.component, pa.capabilities,
        pa.col_group_id AS variant_group_id,
        1 AS total_count, 1 AS success_count, 0 AS flake_count, NULL::timestamptz AS last_failure
    FROM %s pa
    GROUP BY pa.component, pa.capabilities, pa.col_group_id`

// queryTestStatusCTE builds and executes a single CTE-based query that produces
// test results and grid placeholder entries (component-level cells confirming
// data exists).
//
// Grid placeholders ensure that cells without any test data on one side
// correctly show MissingSample / MissingBasis instead of NotSignificant.
func (p *PostgresProvider) queryTestStatusCTE(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
	includeVariants map[string][]string,
	spec testStatusSpec,
) (map[string]crstatus.TestStatus, []error) {

	includeVariants = mergeRequestedVariants(includeVariants, reqOptions)
	filters := buildDrilldownFilters(reqOptions)

	setup, err := prepareVariantQuery(ctx, p.dbc, includeVariants, reqOptions.VariantOption.DBGroupBy)
	if err != nil {
		return nil, []error{err}
	}
	if setup == nil {
		return map[string]crstatus.TestStatus{}, nil
	}

	prowJobJoin := prowJobVariantJoin(setup.variantSubquery)

	innerSQL, innerArgs := buildInnerAggregation(spec, prowJobJoin, setup.filterArgs, filters)

	colMapping := buildColumnGroupMapping(setup.groupMapping.groupToVariants, reqOptions.VariantOption.ColumnGroupBy)
	cteSQL, cteArgs := buildStatusCTE("status_agg", innerSQL, innerArgs, "cm", filters)

	fullSQL := fmt.Sprintf("WITH vg(vcid, group_id) AS (%s),\ncm(group_id, col_group_id) AS (%s),\n%s\n%s\nUNION ALL\n%s",
		setup.groupMapping.valuesClause, colMapping.valuesClause,
		cteSQL,
		fmt.Sprintf(testBranchTemplate, "", "status_agg"),
		fmt.Sprintf(placeholderBranchTemplate, "", "status_agg"))

	return p.scanWithParallelHints(ctx, fullSQL, cteArgs, setup.groupMapping)
}

// prefixSumSpec returns a testStatusSpec for querying test_cumulative_summaries
// using CASE WHEN on prefix sums to compute aggregated counts for a date range.
func prefixSumSpec(release string, lookupEnd, lookupStart civil.Date, lifecycles []string) testStatusSpec {
	return testStatusSpec{
		fromTemplate: `
            FROM test_cumulative_summaries e
            %s`,
		selectCols: `SUM(CASE WHEN e.date = ? THEN e.prefix_sum_runs ELSE 0 END)
              - SUM(CASE WHEN e.date = ? THEN e.prefix_sum_runs ELSE 0 END) AS total_count,
                SUM(CASE WHEN e.date = ? THEN e.prefix_sum_successes ELSE 0 END)
              - SUM(CASE WHEN e.date = ? THEN e.prefix_sum_successes ELSE 0 END) AS success_count,
                SUM(CASE WHEN e.date = ? THEN e.prefix_sum_flakes ELSE 0 END)
              - SUM(CASE WHEN e.date = ? THEN e.prefix_sum_flakes ELSE 0 END) AS flake_count,
                MAX(CASE WHEN e.date = ? THEN e.prefix_max_last_failure END) AS last_failure`,
		selectArgs:  []any{lookupEnd, lookupStart, lookupEnd, lookupStart, lookupEnd, lookupStart, lookupEnd},
		whereFilter: "e.release = ? AND e.date IN (?, ?)",
		whereArgs:   []any{release, lookupEnd, lookupStart},
		lifecycles:  lifecycles,
	}
}

// queryTestStatusPrefixSum queries test_cumulative_summaries using CASE WHEN
// on prefix sums to compute aggregated counts for a date range.
func (p *PostgresProvider) queryTestStatusPrefixSum(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
	release string,
	lifecycles []string,
	includeVariants map[string][]string,
	dateRange query.DateRange,
) (map[string]crstatus.TestStatus, []error) {

	if err := query.ResolveDateRanges(p.dbc, release, &dateRange); err != nil {
		return nil, []error{err}
	}
	lookupEnd := dateRange.End.AddDays(-1)
	lookupStart := dateRange.Start.AddDays(-1)

	return p.queryTestStatusCTE(ctx, reqOptions, includeVariants,
		prefixSumSpec(release, lookupEnd, lookupStart, lifecycles))
}

// gaSpec returns a testStatusSpec for querying prow_ga_raw_test_data to compute
// aggregated base test status for GA releases.
func gaSpec(release string, windowDays int) testStatusSpec {
	return testStatusSpec{
		fromTemplate: `
            FROM prow_ga_raw_test_data e
            %s`,
		selectCols: `SUM(e.runs) AS total_count,
                SUM(e.passes) AS success_count,
                SUM(e.flakes) AS flake_count,
                NULL::timestamptz AS last_failure`,
		whereFilter:  "e.release = ? AND e.window_days = ?",
		whereArgs:    []any{release, windowDays},
		havingClause: "\n            HAVING SUM(e.runs) > 0",
	}
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

	return p.queryTestStatusCTE(ctx, reqOptions, reqOptions.VariantOption.IncludeVariants,
		gaSpec(release, windowDays))
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
		if err := tx.Exec(queryPlannerHints).Error; err != nil {
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

// queryCombinedTestStatus executes a single SQL statement that folds sample
// and base queries into two materialized CTEs, then reads failure and
// placeholder results from each via UNION ALL. This eliminates the concurrent
// partition scans that cause buffer cache contention when sample and base
// queries run in parallel.
func (p *PostgresProvider) queryCombinedTestStatus(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
) (baseStatus, sampleStatus map[string]crstatus.TestStatus, errs []error) {

	sampleIncludeVariants := mergeRequestedVariants(
		mergeCompareVariants(reqOptions, reqOptions.VariantOption.IncludeVariants), reqOptions)
	baseIncludeVariants := mergeRequestedVariants(reqOptions.VariantOption.IncludeVariants, reqOptions)

	sampleRelease := reqOptions.SampleRelease.Name
	sampleRange := query.DateRange{
		Start: civil.DateOf(reqOptions.SampleRelease.Start),
		End:   civil.DateOf(reqOptions.SampleRelease.End).AddDays(1),
	}
	if err := query.ResolveDateRanges(p.dbc, sampleRelease, &sampleRange); err != nil {
		return nil, nil, []error{err}
	}
	sampleLookupEnd := sampleRange.End.AddDays(-1)
	sampleLookupStart := sampleRange.Start.AddDays(-1)

	baseRelease := reqOptions.BaseRelease.Name
	baseRange := query.DateRange{
		Start: civil.DateOf(reqOptions.BaseRelease.Start),
		End:   civil.DateOf(reqOptions.BaseRelease.End).AddDays(1),
	}
	baseIsGA := p.baseMatchesGAWindow(ctx, baseRelease, baseRange)

	var baseSpec testStatusSpec
	if baseIsGA {
		baseWindowDays := baseRange.End.AddDays(-1).DaysSince(baseRange.Start)
		baseSpec = gaSpec(baseRelease, baseWindowDays)
	} else {
		if err := query.ResolveDateRanges(p.dbc, baseRelease, &baseRange); err != nil {
			return nil, nil, []error{err}
		}
		baseLookupEnd := baseRange.End.AddDays(-1)
		baseLookupStart := baseRange.Start.AddDays(-1)
		baseSpec = prefixSumSpec(baseRelease, baseLookupEnd, baseLookupStart, nil)
	}

	filters := buildDrilldownFilters(reqOptions)
	dbGroupBy := reqOptions.VariantOption.DBGroupBy
	minimumFailure := reqOptions.AdvancedOption.MinimumFailure

	sampleVF, err := resolveVariantFilter(ctx, p.dbc, sampleIncludeVariants, dbGroupBy)
	if err != nil {
		return nil, nil, []error{err}
	}
	baseVF, err := resolveVariantFilter(ctx, p.dbc, baseIncludeVariants, dbGroupBy)
	if err != nil {
		return nil, nil, []error{err}
	}
	if len(sampleVF.lookup) == 0 && len(baseVF.lookup) == 0 {
		return map[string]crstatus.TestStatus{}, map[string]crstatus.TestStatus{}, nil
	}

	mergedLookup := make(map[uint]map[string]string, len(sampleVF.lookup)+len(baseVF.lookup))
	maps.Copy(mergedLookup, sampleVF.lookup)
	maps.Copy(mergedLookup, baseVF.lookup)
	groupMapping := buildVariantGroupMapping(mergedLookup)
	colMapping := buildColumnGroupMapping(groupMapping.groupToVariants, reqOptions.VariantOption.ColumnGroupBy)

	sampleProwJobJoin := prowJobVariantJoin(sampleVF.variantSubquery)
	baseProwJobJoin := prowJobVariantJoin(baseVF.variantSubquery)

	sampleSpec := prefixSumSpec(sampleRelease, sampleLookupEnd, sampleLookupStart, reqOptions.Lifecycles)
	sampleInnerSQL, sampleInnerArgs := buildInnerAggregation(sampleSpec, sampleProwJobJoin, sampleVF.filterArgs, filters)
	sampleCTE, sampleCTEArgs := buildStatusCTE("sample_agg", sampleInnerSQL, sampleInnerArgs, "cm", filters)

	baseInnerSQL, baseInnerArgs := buildInnerAggregation(baseSpec, baseProwJobJoin, baseVF.filterArgs, filters)
	baseCTE, baseCTEArgs := buildStatusCTE("base_agg", baseInnerSQL, baseInnerArgs, "cm", filters)

	// The combined query bakes the source tag into each branch as a typed
	// literal so the row scanner can split results into sample and base maps.
	//
	// Each side's test branch(es) come from one of two shapes, chosen by
	// reqOptions.IncludeAllTests rather than by which filters happen to be
	// set — a single branch point, not a different code path per parameter
	// combination:
	//
	//   - Default: above-threshold tests (failureBranch) plus below-threshold
	//     tests rescued by a LEFT JOIN against the other side's narrow keys
	//     CTE (belowThresholdRescueBranch, TRT-2883 + PG/BQ parity fix). This
	//     still omits tests that are below threshold on BOTH sides with real
	//     data on both, which is fine here: such a test is never a
	//     regression (a genuine regression would already cross the
	//     threshold and get rescued), and the grid placeholder mechanism
	//     keeps the cell's aggregate status correct regardless.
	//   - IncludeAllTests: every test with runs, unconditionally
	//     (testBranchTemplate, the same one the standalone path already
	//     uses), because the caller wants the full per-test listing (e.g. a
	//     component/capability drill-down), where an invisible-but-real test
	//     is a genuine gap (TRT-2923). This reuses whatever Component/
	//     Capability/TestID filter buildDrilldownFilters already narrowed
	//     the aggregation to, so a scoped drill-down stays cheap; a fully
	//     unscoped IncludeAllTests request (not used by any current UI path)
	//     is allowed to be slow rather than adding a second query/code path.
	sampleSourcePrefix := "'S'::text AS source, "
	baseSourcePrefix := "'B'::text AS source, "

	var sampleTestBranches, baseTestBranches string
	var allArgs []any
	allArgs = append(allArgs, sampleCTEArgs...)
	allArgs = append(allArgs, baseCTEArgs...)

	if reqOptions.IncludeAllTests {
		sampleTestBranches = fmt.Sprintf(testBranchTemplate, sampleSourcePrefix, "sample_agg")
		baseTestBranches = fmt.Sprintf(testBranchTemplate, baseSourcePrefix, "base_agg")
	} else {
		sampleKeysCTE := fmt.Sprintf(keysCTETemplate, "sample_keys", "sample_agg")
		baseKeysCTE := fmt.Sprintf(keysCTETemplate, "base_keys", "base_agg")
		sampleCTE = sampleCTE + ",\n" + sampleKeysCTE
		baseCTE = baseCTE + ",\n" + baseKeysCTE

		sampleTestBranches = fmt.Sprintf(failureBranchTemplate, sampleSourcePrefix, "sample_agg") +
			"\nUNION ALL\n" + fmt.Sprintf(belowThresholdRescueBranchTemplate, sampleSourcePrefix, "sample_agg", "base_keys")
		baseTestBranches = fmt.Sprintf(failureBranchTemplate, baseSourcePrefix, "base_agg") +
			"\nUNION ALL\n" + fmt.Sprintf(belowThresholdRescueBranchTemplate, baseSourcePrefix, "base_agg", "sample_keys")

		allArgs = append(allArgs, minimumFailure)                 // sample failure branch
		allArgs = append(allArgs, minimumFailure, minimumFailure) // sample below-threshold rescue branch
		allArgs = append(allArgs, minimumFailure)                 // base failure branch
		allArgs = append(allArgs, minimumFailure, minimumFailure) // base below-threshold rescue branch
	}

	fullSQL := fmt.Sprintf(
		"WITH vg(vcid, group_id) AS (%s),\ncm(group_id, col_group_id) AS (%s),\n%s,\n%s\n%s\nUNION ALL\n%s\nUNION ALL\n%s\nUNION ALL\n%s",
		groupMapping.valuesClause, colMapping.valuesClause,
		sampleCTE, baseCTE,
		sampleTestBranches,
		fmt.Sprintf(placeholderBranchTemplate, sampleSourcePrefix, "sample_agg"),
		baseTestBranches,
		fmt.Sprintf(placeholderBranchTemplate, baseSourcePrefix, "base_agg"))

	// Execute with parallel hints
	var sampleResult, baseResult map[string]crstatus.TestStatus
	txErr := p.dbc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if txErr := tx.Exec(queryPlannerHints).Error; txErr != nil {
			return fmt.Errorf("setting parallel query hints: %w", txErr)
		}

		type combinedRow struct {
			Source         string         `gorm:"column:source"`
			TestID         string         `gorm:"column:test_id"`
			TestName       string         `gorm:"column:test_name"`
			TestSuite      string         `gorm:"column:test_suite"`
			Component      string         `gorm:"column:component"`
			Capabilities   pq.StringArray `gorm:"column:capabilities;type:text[]"`
			VariantGroupID int            `gorm:"column:variant_group_id"`
			TotalCount     int            `gorm:"column:total_count"`
			SuccessCount   int            `gorm:"column:success_count"`
			FlakeCount     int            `gorm:"column:flake_count"`
			LastFailure    sql.NullTime   `gorm:"column:last_failure"`
		}

		var allRows []combinedRow
		if qErr := tx.Raw(fullSQL, allArgs...).Scan(&allRows).Error; qErr != nil {
			return fmt.Errorf("querying combined test status: %w", qErr)
		}

		sampleFailures := make(map[string]crstatus.TestStatus)
		samplePlaceholders := make(map[string]crstatus.TestStatus)
		baseFailures := make(map[string]crstatus.TestStatus)
		basePlaceholders := make(map[string]crstatus.TestStatus)

		scanStart := time.Now()
		for _, row := range allRows {
			variantMap := groupMapping.groupToVariants[row.VariantGroupID]
			key := crtest.KeyWithVariants{TestID: row.TestID, Variants: variantMap}
			keyStr := key.Encode()

			ts := buildTestStatus(
				row.TestID, row.TestName, row.TestSuite, row.Component, row.Capabilities,
				variantMap, row.TotalCount, row.SuccessCount, row.FlakeCount, row.LastFailure,
			)

			isPlaceholder := strings.HasPrefix(row.TestID, "grid:")
			if row.Source == "S" {
				if isPlaceholder {
					samplePlaceholders[keyStr] = ts
				} else {
					sampleFailures[keyStr] = ts
				}
			} else {
				if isPlaceholder {
					basePlaceholders[keyStr] = ts
				} else {
					baseFailures[keyStr] = ts
				}
			}
		}
		log.WithField("rowCount", len(allRows)).
			WithField("scanDuration", time.Since(scanStart).String()).
			Debug("combined query: row scan complete")

		mergePlaceholders(sampleFailures, samplePlaceholders, "sample")
		mergePlaceholders(baseFailures, basePlaceholders, "base")

		sampleResult = sampleFailures
		baseResult = baseFailures
		return nil
	})
	if txErr != nil {
		return nil, nil, []error{txErr}
	}

	return baseResult, sampleResult, nil
}

func mergePlaceholders(failures, placeholders map[string]crstatus.TestStatus, label string) {
	merged := 0
	for k, v := range placeholders {
		if _, exists := failures[k]; !exists {
			failures[k] = v
			merged++
		}
	}
	log.WithField("side", label).
		WithField("placeholders", len(placeholders)).
		WithField("merged", merged).
		WithField("failures", len(failures)-merged).
		WithField("total", len(failures)).
		Debug("combined query: placeholder merge complete")
}

func buildTestStatus(
	testID, testName, testSuite, component string,
	capabilities pq.StringArray,
	variants map[string]string,
	totalCount, successCount, flakeCount int,
	lastFailure sql.NullTime,
) crstatus.TestStatus {
	ts := crstatus.TestStatus{
		TestID:       testID,
		TestName:     testName,
		TestSuite:    testSuite,
		Component:    component,
		Capabilities: capabilities,
		Variants:     variants,
		Count: crtest.Count{
			TotalCount:   totalCount,
			SuccessCount: successCount,
			FlakeCount:   flakeCount,
		},
	}
	if lastFailure.Valid {
		ts.LastFailure = lastFailure.Time
	}
	return ts
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
		key := crtest.KeyWithVariants{TestID: testID, Variants: variantMap}
		result[key.Encode()] = buildTestStatus(
			testID, testName, testSuite, component, capabilities,
			variantMap, totalCount, successCount, flakeCount, lastFailure,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
