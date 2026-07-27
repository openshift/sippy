package query

import (
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

// TestNameMatches specifies which tests to include in a report query.
// When all fields are empty, all tests are included.
type TestNameMatches struct {
	ExactNames []string // matched with = (exact equality)
	Prefixes   []string // matched with LIKE 'prefix%'
	Substrings []string // matched with LIKE '%substring%'
}

// HasConditions returns true when at least one match criterion is set.
func (m TestNameMatches) HasConditions() bool {
	return len(m.ExactNames) > 0 || len(m.Prefixes) > 0 || len(m.Substrings) > 0
}

// nameFilterConditions converts a name filter into SQL conditions and args.
// Positive items produce =, LIKE, ILIKE conditions; negative items (Not=true)
// produce NOT(...) wrapped versions of the same.
func nameFilterConditions(f *filter.Filter) (conditions []string, args []any) {
	if f == nil || len(f.Items) == 0 {
		return nil, nil
	}
	var positive, negative TestNameMatches
	for _, item := range f.Items {
		target := &positive
		if item.Not {
			target = &negative
		}
		switch item.Operator {
		case filter.OperatorEquals:
			target.ExactNames = append(target.ExactNames, item.Value)
		case filter.OperatorStartsWith:
			target.Prefixes = append(target.Prefixes, item.Value)
		case filter.OperatorContains:
			target.Substrings = append(target.Substrings, item.Value)
		}
	}
	conditions, args = nameMatchConditions(positive)
	negConds, negArgs := nameMatchConditions(negative)
	conditions = append(conditions, negateConditions(negConds)...)
	args = append(args, negArgs...)
	return conditions, args
}

// nameMatchConditions returns SQL conditions and args for a TestNameMatches.
func nameMatchConditions(matches TestNameMatches) (conditions []string, args []any) {
	for _, name := range matches.ExactNames {
		conditions = append(conditions, "tests.name = ?")
		args = append(args, name)
	}
	for _, prefix := range matches.Prefixes {
		conditions = append(conditions, "tests.name LIKE ?")
		args = append(args, escapeLikeMetachars(prefix)+"%")
	}
	for _, sub := range matches.Substrings {
		conditions = append(conditions, "tests.name ILIKE ?")
		args = append(args, "%"+escapeLikeMetachars(sub)+"%")
	}
	return conditions, args
}

func negateConditions(conditions []string) []string {
	negated := make([]string, len(conditions))
	for i, c := range conditions {
		negated[i] = "NOT(" + c + ")"
	}
	return negated
}

// buildTestsJoinCondition constructs a tests JOIN clause from the match criteria.
// An empty TestNameMatches produces a plain JOIN with no filter. Conditions are
// combined with OR.
func buildTestsJoinCondition(matches TestNameMatches) (string, []any) {
	const testsJoinClause = "JOIN tests ON tests.id = e.test_id"
	conditions, args := nameMatchConditions(matches)
	if len(conditions) == 0 {
		return testsJoinClause, nil
	}
	return testsJoinClause + " AND (" + strings.Join(conditions, " OR ") + ")", args
}

// escapeLikeMetachars escapes LIKE/ILIKE metacharacters (%, _, \) so they
// match literally.
func escapeLikeMetachars(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// DateRange defines a half-open date interval [Start, End) used to compute
// period counts from prefix sums in test_cumulative_summaries.
//
// Given prefix sums P(d) = cumulative total through date d:
//
//	count for [Start, End) = P(End-1) - P(Start-1)
//
// Query builders convert internally with AddDays(-1) to get the prefix sum
// lookup dates.
type DateRange struct {
	Start civil.Date // first date of the period (inclusive)
	End   civil.Date // first date after the period (exclusive)
}

// PeriodsForReportType returns the sample and base date ranges for the given
// report type. The sample period is the "current" window; the base period is
// the "previous" comparison window.
func PeriodsForReportType(reportType v1.ReportType) (sample, base DateRange) {
	tomorrow := civil.DateOf(time.Now().UTC()).AddDays(1)
	if reportType == v1.TwoDayReport {
		boundary := tomorrow.AddDays(-3)
		return DateRange{Start: boundary, End: tomorrow},
			DateRange{Start: tomorrow.AddDays(-10), End: boundary}
	}
	boundary := tomorrow.AddDays(-8)
	return DateRange{Start: boundary, End: tomorrow},
		DateRange{Start: tomorrow.AddDays(-15), End: boundary}
}

// TestReportQuery returns per-variant test report rows for a single release with
// metadata (test name, suite, jira component, open bugs, variants). It does NOT
// include percentages or cross-variant statistics; callers that need those should
// use UncollapsedTestReportWithStats instead.
func TestReportQuery(dbc *db.DB, release string, sample, base DateRange, nameMatches TestNameMatches) (*gorm.DB, error) {
	return testReportPreAgg(dbc, release, sample, base, nameMatches)
}

// ResolveDateRanges clamps the Start and End of each DateRange to the latest
// available date (+1, since DateRange uses half-open intervals) for the release
// in test_cumulative_summaries. This ensures the planner sees literal dates it
// can use for partition pruning, and handles cases where data hasn't been
// backfilled up to the requested dates.
func ResolveDateRanges(dbc *db.DB, release string, ranges ...*DateRange) error {
	var maxDate *civil.Date
	row := dbc.DB.Table("test_cumulative_summaries").
		Select("MAX(date)").
		Where("release = ?", release).
		Row()
	if err := row.Scan(&maxDate); err != nil {
		return fmt.Errorf("resolving max date for release %s: %w", release, err)
	}
	if maxDate == nil {
		return nil
	}
	clampTo := maxDate.AddDays(1)
	for _, dr := range ranges {
		if dr.End.After(clampTo) {
			dr.End = clampTo
		}
		if dr.Start.After(clampTo) {
			dr.Start = clampTo
		}
	}
	return nil
}

func openBugsSubquery(dbc *db.DB) *gorm.DB {
	return dbc.DB.Table("bug_tests").
		Select("bug_tests.test_id, COUNT(DISTINCT bugs.id) AS open_bugs").
		Joins("INNER JOIN bugs ON bug_tests.bug_id = bugs.id").
		Where("LOWER(bugs.status) <> 'closed'").
		Group("bug_tests.test_id")
}

// variantFilterConditions returns raw SQL fragments and args for variant filter items.
// Each fragment is a standalone condition (e.g., "EXISTS (...)") that assumes
// variant_combination_id is in scope.
func variantFilterConditions(variantFilter *filter.Filter) (conditions []string, args []any) {
	if variantFilter == nil || len(variantFilter.Items) == 0 {
		return nil, nil
	}
	for _, item := range variantFilter.Items {
		switch item.Operator {
		case filter.OperatorHasEntry:
			if item.Not {
				conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM variant_combinations WHERE id = variant_combination_id AND ? = ANY(variants))")
			} else {
				conditions = append(conditions, "EXISTS (SELECT 1 FROM variant_combinations WHERE id = variant_combination_id AND ? = ANY(variants))")
			}
			args = append(args, item.Value)
		case filter.OperatorHasEntryContaining, filter.OperatorContains:
			pattern := "%" + escapeLikeMetachars(strings.ToLower(item.Value)) + "%"
			if item.Not {
				conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM variant_combinations vc, LATERAL unnest(vc.variants) AS v(item) WHERE vc.id = variant_combination_id AND LOWER(v.item) LIKE ?)")
			} else {
				conditions = append(conditions, "EXISTS (SELECT 1 FROM variant_combinations vc, LATERAL unnest(vc.variants) AS v(item) WHERE vc.id = variant_combination_id AND LOWER(v.item) LIKE ?)")
			}
			args = append(args, pattern)
		}
	}
	return conditions, args
}

// pushdownSafeFields lists columns available in the filtered CTE (before the
// stats join). Only these fields can be pushed into post_filtered; stats-derived
// fields like working_average and delta_from_* exist only after the final SELECT
// and must stay in remaining for the caller to apply on the outer query.
var pushdownSafeFields = sets.New[string](
	"current_runs", "current_successes", "current_failures", "current_flakes",
	"previous_runs", "previous_successes", "previous_failures", "previous_flakes",
	"open_bugs",
	"current_pass_percentage", "current_failure_percentage", "current_flake_percentage", "current_working_percentage",
	"previous_pass_percentage", "previous_failure_percentage", "previous_flake_percentage", "previous_working_percentage",
	"net_failure_improvement", "net_flake_improvement", "net_working_improvement", "net_improvement",
)

// arithmeticOps maps filter operators to SQL comparison operators.
var arithmeticOps = map[filter.Operator]string{
	filter.OperatorEquals:                        "=",
	filter.OperatorArithmeticEquals:              "=",
	filter.OperatorArithmeticNotEquals:           "<>",
	filter.OperatorArithmeticGreaterThan:         ">",
	filter.OperatorArithmeticGreaterThanOrEquals: ">=",
	filter.OperatorArithmeticLessThan:            "<",
	filter.OperatorArithmeticLessThanOrEquals:    "<=",
}

// processedFilterConditions converts arithmetic filter items to raw SQL WHERE
// conditions with parameterized args. Items with unsupported operators (ILIKE,
// array membership, etc.) are returned in the remaining filter for the caller to
// apply via GORM. Splitting is only safe for AND-linked filters; OR-linked
// filters are returned entirely as remaining.
func processedFilterConditions(f *filter.Filter) (conditions []string, args []any, remaining *filter.Filter) {
	if f == nil || len(f.Items) == 0 {
		return nil, nil, nil
	}
	if f.LinkOperator == filter.LinkOperatorOr {
		return nil, nil, f
	}
	var unsupported []filter.FilterItem
	for _, item := range f.Items {
		op, isArithmetic := arithmeticOps[item.Operator]
		if isArithmetic && pushdownSafeFields.Has(item.Field) {
			field := pq.QuoteIdentifier(item.Field)
			cond := fmt.Sprintf("%s %s ?", field, op)
			if item.Not {
				cond = negateConditions([]string{cond})[0]
			}
			conditions = append(conditions, cond)
			args = append(args, item.Value)
		} else {
			unsupported = append(unsupported, item)
		}
	}
	if len(unsupported) > 0 {
		remaining = &filter.Filter{Items: unsupported, LinkOperator: f.LinkOperator}
	}
	return conditions, args, remaining
}

// resolvePrefixSumDates clamps the sample and base DateRanges to available data,
// warns if their boundaries don't align, and converts the half-open interval
// dates to the three prefix sum lookup dates used by the 3-way self-join
// (each shifted by -1 day): end (e), boundary (m), and start (s).
func resolvePrefixSumDates(dbc *db.DB, release string, sample, base *DateRange) (end, boundary, start civil.Date, err error) {
	if err = ResolveDateRanges(dbc, release, sample, base); err != nil {
		return
	}
	if sample.Start != base.End {
		log.WithFields(log.Fields{
			"sample_start": sample.Start,
			"base_end":     base.End,
		}).Warn("sample.Start != base.End: query uses sample.Start as the shared boundary; base.End is ignored")
	}
	return sample.End.AddDays(-1), sample.Start.AddDays(-1), base.Start.AddDays(-1), nil
}

// testReportCoreJoin builds the 3-way self-join on test_cumulative_summaries
// and aggregates per (test_id, suite_id, variant_combination_id, release).
// Multiple prow_jobs can share the same variant_combination_id, so summing
// here produces one row per variant combination (matching the old matview
// granularity). Keeping rows narrow lets callers add percentages before
// metadata joins.
func testReportCoreJoin(dbc *db.DB, release string, sample, base DateRange, nameMatches TestNameMatches) (*gorm.DB, error) {
	end, boundary, start, err := resolvePrefixSumDates(dbc, release, &sample, &base)
	if err != nil {
		return nil, err
	}

	query := dbc.DB.
		Table("test_cumulative_summaries e").
		Select(`e.test_id, e.suite_id, pj.variant_combination_id, e.release,
			SUM(COALESCE(m.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0), 0))::bigint AS previous_successes,
			SUM(COALESCE(m.prefix_sum_flakes    - COALESCE(s.prefix_sum_flakes,    0), 0))::bigint AS previous_flakes,
			SUM(COALESCE(m.prefix_sum_failures  - COALESCE(s.prefix_sum_failures,  0), 0))::bigint AS previous_failures,
			SUM(COALESCE(m.prefix_sum_runs      - COALESCE(s.prefix_sum_runs,      0), 0))::bigint AS previous_runs,
			SUM(COALESCE(e.prefix_sum_successes - COALESCE(m.prefix_sum_successes, 0), 0))::bigint AS current_successes,
			SUM(COALESCE(e.prefix_sum_flakes    - COALESCE(m.prefix_sum_flakes,    0), 0))::bigint AS current_flakes,
			SUM(COALESCE(e.prefix_sum_failures  - COALESCE(m.prefix_sum_failures,  0), 0))::bigint AS current_failures,
			SUM(COALESCE(e.prefix_sum_runs      - COALESCE(m.prefix_sum_runs,      0), 0))::bigint AS current_runs`).
		Joins("JOIN prow_jobs pj ON e.prow_job_id = pj.id AND pj.variant_combination_id IS NOT NULL").
		Joins("LEFT JOIN test_cumulative_summaries m ON m.test_id = e.test_id AND m.prow_job_id = e.prow_job_id AND m.suite_id = e.suite_id AND m.release = e.release AND m.date = ?", boundary).
		Joins("LEFT JOIN test_cumulative_summaries s ON s.test_id = e.test_id AND s.prow_job_id = e.prow_job_id AND s.suite_id = e.suite_id AND s.release = e.release AND s.date = ?", start).
		Where("e.date = ? AND e.release = ?", end, release).
		Group("e.test_id, e.suite_id, pj.variant_combination_id, e.release")

	if nameMatches.HasConditions() {
		testsJoin, testsJoinArgs := buildTestsJoinCondition(nameMatches)
		query = query.Joins(testsJoin, testsJoinArgs...)
	}

	return query, nil
}

// testReportPreAgg wraps testReportCoreJoin with metadata joins (tests, suites,
// test_ownerships, jira_components, variant_combinations, open_bugs). The planner
// flattens the core subquery so the resulting plan is identical to a single-level
// query with all joins together.
func testReportPreAgg(dbc *db.DB, release string, sample, base DateRange, nameMatches TestNameMatches) (*gorm.DB, error) {
	core, err := testReportCoreJoin(dbc, release, sample, base, nameMatches)
	if err != nil {
		return nil, err
	}
	openBugs := openBugsSubquery(dbc)

	return dbc.DB.
		Table("(?) AS pre", core).
		Select(`tests.name, pre.suite_id, suites.name AS suite_name,
			jira_components.name AS jira_component, jira_components.id AS jira_component_id,
			pre.current_successes, pre.current_failures, pre.current_flakes, pre.current_runs,
			pre.previous_successes, pre.previous_failures, pre.previous_flakes, pre.previous_runs,
			ob.open_bugs, vc.variants, pre.variant_combination_id, pre.release`).
		Joins("JOIN tests ON tests.id = pre.test_id").
		Joins("LEFT JOIN variant_combinations vc ON pre.variant_combination_id = vc.id").
		Joins("LEFT JOIN suites ON suites.id = pre.suite_id").
		Joins("LEFT JOIN test_ownerships ON (tests.id = test_ownerships.test_id AND pre.suite_id = test_ownerships.suite_id)").
		Joins("LEFT JOIN jira_components ON test_ownerships.jira_component = jira_components.name").
		Joins("LEFT JOIN (?) AS ob ON tests.id = ob.test_id", openBugs), nil
}

// TestReportQueryCollapsed builds collapsed test report rows keyed by (test_id, suite_id).
// It aggregates prefix sums per date partition separately (~16K groups each), then joins
// the three small results to compute period counts. This avoids the expensive 3-way
// self-join on all ~1.8M per-prow_job rows that the uncollapsed path requires.
func TestReportQueryCollapsed(dbc *db.DB, release string, sample, base DateRange, variantFilter, nameFilter *filter.Filter) (*gorm.DB, error) {
	end, boundary, start, err := resolvePrefixSumDates(dbc, release, &sample, &base)
	if err != nil {
		return nil, err
	}
	nameConds, nameArgs := nameFilterConditions(nameFilter)
	variantConds, variantArgs := variantFilterConditions(variantFilter)

	var nameJoinClause string
	var nameJoinArgs []any
	if len(nameConds) > 0 {
		joiner := " OR "
		if nameFilter.LinkOperator == filter.LinkOperatorAnd {
			joiner = " AND "
		}
		nameJoinClause = "JOIN tests ON tests.id = tcs.test_id AND (" + strings.Join(nameConds, joiner) + ")"
		nameJoinArgs = nameArgs
	}

	var args []any
	var buf strings.Builder

	writeDateAgg := func(alias string, date civil.Date) {
		buf.WriteString(`(SELECT tcs.test_id, tcs.suite_id, tcs.release,
    SUM(tcs.prefix_sum_successes) AS ps_successes,
    SUM(tcs.prefix_sum_failures)  AS ps_failures,
    SUM(tcs.prefix_sum_flakes)    AS ps_flakes,
    SUM(tcs.prefix_sum_runs)      AS ps_runs
  FROM test_cumulative_summaries tcs
  JOIN prow_jobs pj ON tcs.prow_job_id = pj.id AND pj.variant_combination_id IS NOT NULL
`)
		if nameJoinClause != "" {
			buf.WriteString("  ")
			buf.WriteString(nameJoinClause)
			buf.WriteString("\n")
			args = append(args, nameJoinArgs...)
		}
		buf.WriteString("  WHERE tcs.date = ? AND tcs.release = ?")
		args = append(args, date, release)

		for _, cond := range variantConds {
			buf.WriteString("\n    AND ")
			buf.WriteString(cond)
		}
		args = append(args, variantArgs...)

		fmt.Fprintf(&buf, "\n  GROUP BY tcs.test_id, tcs.suite_id, tcs.release\n) AS %s", alias)
	}

	buf.WriteString(`SELECT t.name, su.name AS suite_name,
  jc.name AS jira_component, jc.id AS jira_component_id, e.release,
  COALESCE(e.ps_successes - COALESCE(m.ps_successes, 0), 0)::bigint AS current_successes,
  COALESCE(e.ps_failures  - COALESCE(m.ps_failures,  0), 0)::bigint AS current_failures,
  COALESCE(e.ps_flakes    - COALESCE(m.ps_flakes,    0), 0)::bigint AS current_flakes,
  COALESCE(e.ps_runs      - COALESCE(m.ps_runs,      0), 0)::bigint AS current_runs,
  COALESCE(m.ps_successes - COALESCE(s.ps_successes, 0), 0)::bigint AS previous_successes,
  COALESCE(m.ps_failures  - COALESCE(s.ps_failures,  0), 0)::bigint AS previous_failures,
  COALESCE(m.ps_flakes    - COALESCE(s.ps_flakes,    0), 0)::bigint AS previous_flakes,
  COALESCE(m.ps_runs      - COALESCE(s.ps_runs,      0), 0)::bigint AS previous_runs,
  ob.open_bugs
FROM `)

	writeDateAgg("e", end)

	buf.WriteString("\nLEFT JOIN ")
	writeDateAgg("m", boundary)
	buf.WriteString(" ON m.test_id = e.test_id AND m.suite_id = e.suite_id AND m.release = e.release")

	buf.WriteString("\nLEFT JOIN ")
	writeDateAgg("s", start)
	buf.WriteString(` ON s.test_id = e.test_id AND s.suite_id = e.suite_id AND s.release = e.release
JOIN tests t ON t.id = e.test_id
LEFT JOIN suites su ON su.id = e.suite_id
LEFT JOIN test_ownerships too ON too.test_id = e.test_id AND too.suite_id = e.suite_id
LEFT JOIN jira_components jc ON jc.name = too.jira_component
LEFT JOIN (
  ` + openBugsSQL + `
) AS ob ON t.id = ob.test_id`)

	return dbc.DB.Raw(buf.String(), args...), nil
}
