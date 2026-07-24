package query

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

	"github.com/openshift/sippy/pkg/apis/api"
	jira "github.com/openshift/sippy/pkg/apis/jira/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/util"
)

const (
	QueryTestSummer = `
           sum(current_runs)       AS current_runs,
           sum(current_successes)  AS current_successes,
           sum(current_failures)   AS current_failures,
           sum(current_flakes)     AS current_flakes,
           sum(previous_runs)      AS previous_runs,
           sum(previous_successes) AS previous_successes,
           sum(previous_failures)  AS previous_failures,
           sum(previous_flakes)    AS previous_flakes,
           (array_agg(open_bugs))[0] AS open_bugs`

	QueryTestFields = `
		current_runs,
		current_successes,
		current_failures,
		current_flakes,
		previous_runs,
		previous_successes,
		previous_failures,
		previous_flakes,
		open_bugs`

	QueryTestPercentages = `
		COALESCE(current_successes * 100.0 / NULLIF(current_runs, 0), 0) AS current_pass_percentage,
		COALESCE(current_failures * 100.0 / NULLIF(current_runs, 0), 0) AS current_failure_percentage,
		COALESCE(current_flakes * 100.0 / NULLIF(current_runs, 0), 0) AS current_flake_percentage,
		COALESCE((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0), 0) AS current_working_percentage,
		COALESCE(previous_successes * 100.0 / NULLIF(previous_runs, 0), 0) AS previous_pass_percentage,
		COALESCE(previous_failures * 100.0 / NULLIF(previous_runs, 0), 0) AS previous_failure_percentage,
		COALESCE(previous_flakes * 100.0 / NULLIF(previous_runs, 0), 0) AS previous_flake_percentage,
		COALESCE((previous_successes + previous_flakes) * 100.0 / NULLIF(previous_runs, 0), 0) AS previous_working_percentage,
		COALESCE((previous_failures * 100.0 / NULLIF(previous_runs, 0)), 0) - COALESCE((current_failures * 100.0 / NULLIF(current_runs, 0)), 0) AS net_failure_improvement,
		COALESCE((previous_flakes * 100.0 / NULLIF(previous_runs, 0)), 0) - COALESCE((current_flakes * 100.0 / NULLIF(current_runs, 0)), 0) AS net_flake_improvement,
		COALESCE(((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0)), 0) - COALESCE(((previous_successes + previous_flakes) * 100.0 / NULLIF(previous_runs, 0)), 0) AS net_working_improvement,
		COALESCE((current_successes * 100.0 / NULLIF(current_runs, 0)), 0) - COALESCE((previous_successes * 100.0 / NULLIF(previous_runs, 0)), 0) AS net_improvement`

	QueryTestSummarizer = QueryTestFields + "," + QueryTestPercentages

	openBugsSQL = `SELECT bug_tests.test_id, COUNT(DISTINCT bugs.id) AS open_bugs
    FROM bug_tests
    INNER JOIN bugs ON bug_tests.bug_id = bugs.id
    WHERE LOWER(bugs.status) <> 'closed'
    GROUP BY bug_tests.test_id`

	QueryTestAnalysis = `
        select current_successes, current_runs,
               current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage
        from (
            select sum(runs) as current_runs, sum(passes) as current_successes
            from test_analysis_by_job_by_dates
            where date >= ? AND test_name = ? AND job_name IN ? AND release = ?
        ) t`
)

func LoadTestCache(dbc *db.DB, preloads []string) (map[string]*models.Test, error) {
	// Cache all tests by name to their ID, used for the join object.
	testCache := map[string]*models.Test{}
	q := dbc.DB.Model(&models.Test{})
	for _, p := range preloads {
		q = q.Preload(p)
	}

	// Kube exceeds 60000 tests, more than postgres can load at once:
	testsBatch := []*models.Test{}
	res := q.FindInBatches(&testsBatch, 5000, func(tx *gorm.DB, batch int) error {
		for _, idn := range testsBatch {
			if _, ok := testCache[idn.Name]; !ok {
				testCache[idn.Name] = idn
			}
		}
		return nil
	})

	if res.Error != nil {
		return map[string]*models.Test{}, res.Error
	}

	log.Infof("test cache created with %d entries from database", len(testCache))
	return testCache, nil
}

// TestReportsByVariant returns per-variant test report rows for every test matching
// the given name criteria. When includeAll is true, an additional "All" aggregate row
// per test is included via UNION ALL (Variant = "All", counting runs across all
// variant_combination_ids without unnesting).
func TestReportsByVariant(
	dbc *db.DB,
	release string,
	reportType v1.ReportType,
	nameMatches TestNameMatches,
	excludeVariants []string,
	includeAll bool,
) ([]api.Test, error) {
	now := time.Now()

	sample, base := PeriodsForReportType(reportType)
	inner, err := TestReportQuery(dbc, release, sample, base, nameMatches)
	if err != nil {
		return nil, err
	}
	excludeArr := pq.Array(excludeVariants)

	perVariant := dbc.DB.Table("(?) AS r", inner).
		Select(`name, release,
			sum(current_runs)       AS current_runs,
			sum(current_successes)  AS current_successes,
			sum(current_failures)   AS current_failures,
			sum(current_flakes)     AS current_flakes,
			sum(previous_runs)      AS previous_runs,
			sum(previous_successes) AS previous_successes,
			sum(previous_failures)  AS previous_failures,
			sum(previous_flakes)    AS previous_flakes,
			unnest(variants)        AS variant`).
		Where("NOT EXISTS (SELECT 1 FROM variant_combinations WHERE ? && variants AND id = variant_combination_id)", excludeArr).
		Group("name, release, variant")

	var aggregated *gorm.DB
	if includeAll {
		allArm := dbc.DB.Table("(?) AS r", inner).
			Select(`name, release,
				sum(current_runs)       AS current_runs,
				sum(current_successes)  AS current_successes,
				sum(current_failures)   AS current_failures,
				sum(current_flakes)     AS current_flakes,
				sum(previous_runs)      AS previous_runs,
				sum(previous_successes) AS previous_successes,
				sum(previous_failures)  AS previous_failures,
				sum(previous_flakes)    AS previous_flakes,
				'All'::text             AS variant`).
			Where("NOT EXISTS (SELECT 1 FROM variant_combinations WHERE ? && variants AND id = variant_combination_id)", excludeArr).
			Group("name, release")
		aggregated = dbc.DB.Raw("? UNION ALL ?", perVariant, allArm)
	} else {
		aggregated = perVariant
	}

	var testReports []api.Test
	r := dbc.DB.Table("(?) AS agg", aggregated).
		Select(fmt.Sprintf("*, %s", QueryTestPercentages)).
		Where("agg.current_runs > 0 OR agg.previous_runs > 0").
		Scan(&testReports)
	if r.Error != nil {
		log.Error(r.Error)
		return testReports, r.Error
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{"elapsed": elapsed, "count": len(testReports)}).Info("TestReportsByVariant completed")
	return testReports, nil
}

// TestReportExcludeVariants returns a single test report the given test name in the db,
// all variants collapsed, optionally with some excluded.
// If the query fails, it is logged and the bool is left false.
func TestReportExcludeVariants(dbc *db.DB, release, testName string, excludeVariants []string) (api.Test, bool) {
	now := time.Now()
	logger := log.WithField("func", "TestReportExcludeVariants").
		WithField("release", release).
		WithField("test", testName)

	sample, base := PeriodsForReportType(v1.CurrentReport)
	inner, err := TestReportQuery(dbc, release, sample, base, TestNameMatches{ExactNames: []string{testName}})
	if err != nil {
		logger.WithError(err).Error("failed to build test report query")
		return api.Test{}, false
	}

	aggregated := dbc.DB.Table("(?) AS r", inner).
		Select(`name, release,
			sum(current_runs)       AS current_runs,
			sum(current_successes)  AS current_successes,
			sum(current_failures)   AS current_failures,
			sum(current_flakes)     AS current_flakes,
			sum(previous_runs)      AS previous_runs,
			sum(previous_successes) AS previous_successes,
			sum(previous_failures)  AS previous_failures,
			sum(previous_flakes)    AS previous_flakes`).
		Where("NOT EXISTS (SELECT 1 FROM variant_combinations WHERE ? && variants AND id = variant_combination_id)", pq.Array(excludeVariants)).
		Group("name, release")

	var testReports []api.Test
	r := dbc.DB.Table("(?) AS agg", aggregated).
		Select(fmt.Sprintf("*, %s", QueryTestPercentages)).
		Limit(1).
		Find(&testReports)
	if r.Error == nil && len(testReports) == 0 {
		r.Error = gorm.ErrRecordNotFound
	}
	var testReport api.Test
	if len(testReports) > 0 {
		testReport = testReports[0]
	}
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			logger.WithField("test", testName).Warn("test not found in cumulative summaries")
		} else {
			logger.WithError(r.Error).Error("query failed")
		}
		return testReport, false
	}

	logger.Debugf("completed in %s", time.Since(now))
	return testReport, true
}

// LoadBugsForTest returns all bugs in the database for the given test, across all releases.
func LoadBugsForTest(dbc *db.DB, testName string, filterClosed bool) ([]models.Bug, error) {
	results := []models.Bug{}

	test := models.Test{}
	q := dbc.DB.Where("name = ?", testName)
	timeLimit := "NOW() - last_change_time < interval '14 days'" // filter bugs since we no longer delete them
	if filterClosed {
		q = q.Preload("Bugs", timeLimit+" and UPPER(status) != 'CLOSED' and UPPER(status) != 'VERIFIED'")
	} else {
		q = q.Preload("Bugs", timeLimit)
	}
	res := q.First(&test)
	if res.Error != nil {
		return results, res.Error
	}
	// issues with LabelJiraAutomator are placeholders for multiple tests. Filter them out.
	for _, b := range test.Bugs {
		if !util.StrSliceContains(b.Labels, jira.LabelJiraAutomator) {
			results = append(results, b)
		}
	}
	log.Debugf("LoadBugsForTest found %d bugs for test '%s'", len(results), testName)

	return results, nil
}

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
func UncollapsedTestReportWithStats(dbc *db.DB, release string, sample, base DateRange, nameFilter, variantFilter, processedFilter *filter.Filter) (*gorm.DB, *filter.Filter, error) {
	end, boundary, start, err := resolvePrefixSumDates(dbc, release, &sample, &base)
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
	pfConds, pfArgs, remainingFilter := processedFilterConditions(processedFilter)

	// === filtered CTE: per-variant test report with percentages ===
	buf.WriteString(`WITH filtered AS MATERIALIZED (
  SELECT pre.test_id, tests.name, pre.suite_id, suites.name AS suite_name,
    jira_components.name AS jira_component, jira_components.id AS jira_component_id,
    pre.current_successes, pre.current_failures, pre.current_flakes, pre.current_runs,
    pre.previous_successes, pre.previous_failures, pre.previous_flakes, pre.previous_runs,
    ob.open_bugs, vc.variants, pre.variant_combination_id, pre.release,
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
      SUM(COALESCE(e.prefix_sum_runs      - COALESCE(m.prefix_sum_runs,      0), 0))::bigint AS current_runs
    FROM test_cumulative_summaries e
    JOIN prow_jobs pj ON e.prow_job_id = pj.id AND pj.variant_combination_id IS NOT NULL
    LEFT JOIN test_cumulative_summaries m ON m.test_id = e.test_id AND m.prow_job_id = e.prow_job_id AND m.suite_id = e.suite_id AND m.release = e.release AND m.date = ?
    LEFT JOIN test_cumulative_summaries s ON s.test_id = e.test_id AND s.prow_job_id = e.prow_job_id AND s.suite_id = e.suite_id AND s.release = e.release AND s.date = ?
`)
	args = append(args, boundary, start)

	if nameJoinClause != "" {
		buf.WriteString("    ")
		buf.WriteString(nameJoinClause)
		buf.WriteString("\n")
		args = append(args, nameJoinArgs...)
	}

	buf.WriteString(`    WHERE e.date = ? AND e.release = ?
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
	args = append(args, end, release)

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
    LEFT JOIN test_cumulative_summaries m ON m.test_id = e.test_id AND m.prow_job_id = e.prow_job_id AND m.suite_id = e.suite_id AND m.release = e.release AND m.date = ?
    WHERE e.date = ? AND e.release = ?
      AND e.test_id IN (SELECT DISTINCT test_id FROM %s)
      AND NOT EXISTS (SELECT 1 FROM variant_combinations WHERE 'never-stable' = any(variants) AND id = pj.variant_combination_id)
    GROUP BY e.test_id, e.suite_id, pj.variant_combination_id
  ) c
  GROUP BY c.test_id, c.suite_id
)`, statsSource)
	args = append(args, boundary, end, release)

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

	return dbc.DB.Raw(buf.String(), args...), remainingFilter, nil
}

func TestOutputs(dbc *db.DB, release, test string, includedVariants, excludedVariants []string, quantity int) ([]api.TestOutput, error) {
	results := make([]api.TestOutput, 0)

	testQuery := dbc.DB.Table("tests").Where("name = ?", test).Select("id")
	q := dbc.DB.Table("prow_job_run_tests").
		Joins("JOIN prow_job_run_test_outputs ON prow_job_run_test_outputs.prow_job_run_test_id = prow_job_run_tests.id AND prow_job_run_test_outputs.prow_job_run_test_timestamp = prow_job_run_tests.prow_job_run_timestamp").
		Joins("JOIN prow_job_runs ON prow_job_run_tests.prow_job_run_id = prow_job_runs.id").
		Joins("JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_job_run_tests.test_id = (?)", testQuery).
		Where("prow_job_run_tests.status IN ?", []int{int(v1.TestStatusFailure), int(v1.TestStatusFlake)}).
		Where("prow_job_run_tests.prow_job_run_timestamp > current_date - interval '14' day").
		Where("prow_job_run_tests.prow_job_run_release = ?", release).
		Where("prow_job_run_test_outputs.prow_job_run_test_timestamp > current_date - interval '14' day").
		Where("prow_job_run_test_outputs.prow_job_run_test_release = ?", release)

	for _, variant := range includedVariants {
		q = q.Where("prow_jobs.variant_combination_id IN (SELECT id FROM variant_combinations WHERE ? = any(variants))", variant)
	}

	for _, variant := range excludedVariants {
		q = q.Where("NOT EXISTS (SELECT 1 FROM variant_combinations WHERE ? = any(variants) AND id = prow_jobs.variant_combination_id)", variant)
	}

	res := q.
		Select("prow_job_runs.url as prow_job_url, prow_job_run_test_outputs.output").
		Order("prow_job_run_tests.prow_job_run_timestamp DESC, prow_job_run_test_outputs.id DESC").
		Limit(quantity).
		Scan(&results)

	return results, res.Error
}

func TestDurations(dbc *db.DB, release, test string, includedVariants, excludedVariants []string) (map[string]float64, error) {
	type testDuration struct {
		Period          time.Time `json:"period"`
		AverageDuration float64   `json:"average_duration"`
	}
	rows := make([]testDuration, 0)
	results := make(map[string]float64)

	testQuery := dbc.DB.Table("tests").Where("name = ?", test).Select("id")
	q := dbc.DB.Table("prow_job_run_tests").
		Joins("JOIN tests ON prow_job_run_tests.test_id = tests.id").
		Joins("JOIN prow_jobs ON prow_jobs.id = prow_job_run_tests.prow_job_id").
		Where("prow_job_run_tests.prow_job_run_timestamp > current_date - interval '14' day").
		Where("prow_job_run_tests.test_id = (?)", testQuery).
		Where("prow_job_run_tests.prow_job_run_release = ?", release)

	for _, variant := range includedVariants {
		q = q.Where("prow_jobs.variant_combination_id IN (SELECT id FROM variant_combinations WHERE ? = any(variants))", variant)
	}

	for _, variant := range excludedVariants {
		q = q.Where("NOT EXISTS (SELECT 1 FROM variant_combinations WHERE ? = any(variants) AND id = prow_jobs.variant_combination_id)", variant)
	}

	res := q.
		Select(`
			date(prow_job_run_tests.prow_job_run_timestamp AT TIME ZONE 'UTC'::text) as period,
			AVG(prow_job_run_tests.duration) as average_duration`).
		Group(`date(prow_job_run_tests.prow_job_run_timestamp AT TIME ZONE 'UTC'::text)`).
		Order(`date(prow_job_run_tests.prow_job_run_timestamp AT TIME ZONE 'UTC'::text)`).
		Scan(&rows)

	for _, row := range rows {
		results[row.Period.Format("2006-01-02")] = row.AverageDuration
	}

	return results, res.Error
}
