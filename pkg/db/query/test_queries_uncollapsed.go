package query

// This file holds only UncollapsedTestReportWithStats so that .snyk can exclude this one
// reviewed raw-SQL builder from Snyk Code without excluding the rest of the package.
// Keep other code out of this file so it stays covered by the security scan.

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

// UncollapsedTestReportWithStats builds a per-variant test report with cross-variant
// statistics using a materialized common table expression (CTE) pipeline: filtered -> post_filtered -> stats.
// The filtered CTE applies name matches, variant filters, and the never-stable/zero-run
// exclusions. The post_filtered CTE applies arithmetic processedFilter conditions
// (current_runs >= N, failure_percentage >= M) so the stats CTE only scans test_ids
// that survive all filters. Stats are computed across ALL variant combinations for
// each (test_id, suite_id), regardless of which individual variants passed filtering.
//
// Any processedFilter items with unsupported operators (ILIKE, array membership) are
// returned as remainingFilter for the caller to apply via GORM on the outer query.
func UncollapsedTestReportWithStats(ctx context.Context, dbc *db.DB, release string, sample, base DateRange, nameFilter, variantFilter, processedFilter, lifecycleFilter *filter.Filter) (*gorm.DB, *filter.Filter, error) {
	end, boundary, start, err := resolvePrefixSumDates(ctx, dbc, release, &sample, &base)
	if err != nil {
		return nil, nil, err
	}

	var args []any
	var buf strings.Builder

	nameJoinClause := ""
	var nameJoinArgs []any
	if nameConds, nameArgs := nameFilterConditions(nameFilter); len(nameConds) > 0 {
		joiner := " OR "
		if nameFilter.LinkOperator == filter.LinkOperatorAnd {
			joiner = " AND "
		}
		nameJoinClause = "JOIN tests ON tests.id = e.test_id AND (" + strings.Join(nameConds, joiner) + ")"
		nameJoinArgs = nameArgs
	}

	variantConds, variantArgs := variantFilterConditions(variantFilter)
	lifecycleClause, lifecycleArgs, err := lifecycleWhereClause(lifecycleFilter, "e.lifecycle")
	if err != nil {
		return nil, nil, err
	}
	pfConds, pfArgs, remainingFilter := processedFilterConditions(processedFilter)

	// === filtered CTE: per-variant test report with percentages ===
	buf.WriteString(`WITH filtered AS MATERIALIZED (
  SELECT pre.test_id, tests.name, pre.suite_id, suites.name AS suite_name,
    jira_components.name AS jira_component, jira_components.id AS jira_component_id,
    pre.current_successes, pre.current_failures, pre.current_flakes, pre.current_runs,
    pre.previous_successes, pre.previous_failures, pre.previous_flakes, pre.previous_runs,
    ob.open_bugs, vc.variants, pre.variant_combination_id, pre.release, pre.lifecycles,
    `)
	buf.WriteString(QueryTestPercentages)
	buf.WriteString(`
  FROM (
    SELECT e.test_id, e.suite_id, pj.variant_combination_id, e.release,
      SUM(COALESCE(m.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0), 0))::bigint AS previous_successes,
      SUM(COALESCE(m.prefix_sum_flakes    - COALESCE(s.prefix_sum_flakes,    0), 0))::bigint AS previous_flakes,
      SUM(COALESCE(m.prefix_sum_failures  - COALESCE(s.prefix_sum_failures,  0), 0))::bigint AS previous_failures,
      SUM(COALESCE(m.prefix_sum_runs      - COALESCE(s.prefix_sum_runs,      0), 0))::bigint AS previous_runs,
      SUM(COALESCE(e.prefix_sum_successes - COALESCE(m.prefix_sum_successes, 0), 0))::bigint AS current_successes,
      SUM(COALESCE(e.prefix_sum_flakes    - COALESCE(m.prefix_sum_flakes,    0), 0))::bigint AS current_flakes,
      SUM(COALESCE(e.prefix_sum_failures  - COALESCE(m.prefix_sum_failures,  0), 0))::bigint AS current_failures,
      SUM(COALESCE(e.prefix_sum_runs      - COALESCE(m.prefix_sum_runs,      0), 0))::bigint AS current_runs,
      array_agg(DISTINCT e.lifecycle) AS lifecycles
    FROM test_cumulative_summaries e
    JOIN prow_jobs pj ON e.prow_job_id = pj.id AND pj.variant_combination_id IS NOT NULL
    LEFT JOIN test_cumulative_summaries m ON m.test_id = e.test_id AND m.prow_job_id = e.prow_job_id AND m.suite_id = e.suite_id AND m.lifecycle = e.lifecycle AND m.release = e.release AND m.date = ?
    LEFT JOIN test_cumulative_summaries s ON s.test_id = e.test_id AND s.prow_job_id = e.prow_job_id AND s.suite_id = e.suite_id AND s.lifecycle = e.lifecycle AND s.release = e.release AND s.date = ?
`)
	args = append(args, boundary, start)

	if nameJoinClause != "" {
		buf.WriteString("    ")
		buf.WriteString(nameJoinClause)
		buf.WriteString("\n")
		args = append(args, nameJoinArgs...)
	}

	buf.WriteString(`    WHERE e.date = ? AND e.release = ?`)
	args = append(args, end, release)

	if lifecycleClause != "" {
		buf.WriteString("\n    AND ")
		buf.WriteString(lifecycleClause)
	}
	args = append(args, lifecycleArgs...)

	buf.WriteString(`
    GROUP BY e.test_id, e.suite_id, pj.variant_combination_id, e.release
  ) AS pre
  JOIN tests ON tests.id = pre.test_id
  LEFT JOIN variant_combinations vc ON pre.variant_combination_id = vc.id
  LEFT JOIN suites ON suites.id = pre.suite_id
  LEFT JOIN test_ownerships ON (tests.id = test_ownerships.test_id AND pre.suite_id = test_ownerships.suite_id)
  LEFT JOIN jira_components ON test_ownerships.jira_component = jira_components.name
  LEFT JOIN (
    ` + openBugsSQL + `
  ) AS ob ON tests.id = ob.test_id
  WHERE NOT EXISTS (SELECT 1 FROM variant_combinations WHERE 'never-stable' = any(variants) AND id = variant_combination_id)
    AND (current_runs > 0 OR previous_runs > 0)`)

	for _, cond := range variantConds {
		buf.WriteString("\n    AND ")
		buf.WriteString(cond)
	}
	args = append(args, variantArgs...)

	buf.WriteString("\n)")

	// === post_filtered CTE: narrows filtered rows by arithmetic processedFilter
	// conditions (e.g., current_runs >= 7). This determines which test_ids the stats
	// CTE processes, avoiding a full-release scan when filters are selective.
	statsSource := "filtered"
	resultSource := "filtered f"
	if len(pfConds) > 0 {
		statsSource = "post_filtered"
		resultSource = "post_filtered f"
		buf.WriteString(`,
post_filtered AS MATERIALIZED (
  SELECT * FROM filtered
  WHERE `)
		buf.WriteString(strings.Join(pfConds, " AND "))
		buf.WriteString("\n)")
		args = append(args, pfArgs...)
	}

	// Re-apply the lifecycle filter here: without it, stats would average in
	// rows from lifecycles the caller excluded from the filtered/post_filtered CTE,
	// since only test_id (not lifecycle) scopes this subquery to that CTE's contents.
	// Reusing lifecycleClause verbatim (built above against "e.lifecycle") is only valid
	// because this stats subquery also aliases test_cumulative_summaries as "e" (see the
	// FROM clause a few lines down) -- if that alias ever changes independently of the
	// "pre" subquery's, this reused clause must be rebuilt against the new alias too.
	statsLifecycleClause := ""
	if lifecycleClause != "" {
		statsLifecycleClause = "AND " + lifecycleClause
	}

	// === stats CTE: AVG/STDDEV across all variant combinations per (test_id, suite_id),
	// scoped to test_ids from the filtered/post_filtered CTE. Uses a 2-way prefix sum
	// join (current period only) since base-period counts are not needed for
	// cross-variant statistics.
	fmt.Fprintf(&buf, `,
stats AS (
  SELECT c.test_id, c.suite_id,
    AVG((c.current_successes + c.current_flakes) * 100.0 / NULLIF(c.current_runs, 0)) AS working_average,
    STDDEV((c.current_successes + c.current_flakes) * 100.0 / NULLIF(c.current_runs, 0)) AS working_standard_deviation,
    AVG(c.current_successes * 100.0 / NULLIF(c.current_runs, 0)) AS passing_average,
    STDDEV(c.current_successes * 100.0 / NULLIF(c.current_runs, 0)) AS passing_standard_deviation,
    AVG(c.current_flakes * 100.0 / NULLIF(c.current_runs, 0)) AS flake_average,
    STDDEV(c.current_flakes * 100.0 / NULLIF(c.current_runs, 0)) AS flake_standard_deviation
  FROM (
    SELECT e.test_id, e.suite_id, pj.variant_combination_id,
      SUM(e.prefix_sum_successes - COALESCE(m.prefix_sum_successes, 0))::bigint AS current_successes,
      SUM(e.prefix_sum_flakes    - COALESCE(m.prefix_sum_flakes, 0))::bigint    AS current_flakes,
      SUM(e.prefix_sum_runs      - COALESCE(m.prefix_sum_runs, 0))::bigint      AS current_runs
    FROM test_cumulative_summaries e
    JOIN prow_jobs pj ON e.prow_job_id = pj.id AND pj.variant_combination_id IS NOT NULL
    LEFT JOIN test_cumulative_summaries m ON m.test_id = e.test_id AND m.prow_job_id = e.prow_job_id AND m.suite_id = e.suite_id AND m.lifecycle = e.lifecycle AND m.release = e.release AND m.date = ?
    WHERE e.date = ? AND e.release = ?
      AND e.test_id IN (SELECT DISTINCT test_id FROM %s)
      AND NOT EXISTS (SELECT 1 FROM variant_combinations WHERE 'never-stable' = any(variants) AND id = pj.variant_combination_id)
      %s
    GROUP BY e.test_id, e.suite_id, pj.variant_combination_id
  ) c
  GROUP BY c.test_id, c.suite_id
)`, statsSource, statsLifecycleClause)
	args = append(args, boundary, end, release)
	args = append(args, lifecycleArgs...)

	// === Final SELECT: join filtered/post_filtered rows with their cross-variant statistics ===
	fmt.Fprintf(&buf, `
SELECT f.*,
  COALESCE(s.working_average, 0) AS working_average,
  COALESCE(s.working_standard_deviation, 0) AS working_standard_deviation,
  f.current_working_percentage - COALESCE(s.working_average, 0) AS delta_from_working_average,
  COALESCE(s.passing_average, 0) AS passing_average,
  COALESCE(s.passing_standard_deviation, 0) AS passing_standard_deviation,
  f.current_pass_percentage - COALESCE(s.passing_average, 0) AS delta_from_passing_average,
  COALESCE(s.flake_average, 0) AS flake_average,
  COALESCE(s.flake_standard_deviation, 0) AS flake_standard_deviation,
  f.current_flake_percentage - COALESCE(s.flake_average, 0) AS delta_from_flake_average
FROM %s
LEFT JOIN stats s ON f.test_id = s.test_id AND f.suite_id = s.suite_id`, resultSource)

	return dbc.DB.WithContext(ctx).Raw(buf.String(), args...), remainingFilter, nil
}
