package query

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

	"github.com/openshift/sippy/pkg/apis/api"
	jira "github.com/openshift/sippy/pkg/apis/jira/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
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
        SELECT current_successes, current_runs,
               current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage
        FROM (
            SELECT
                SUM(e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0)) AS current_successes,
                SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)) AS current_runs
            -- e = latest cumulative snapshot for the release
            FROM test_cumulative_summaries e
            JOIN tests t ON t.id = e.test_id
            JOIN prow_jobs pj ON pj.id = e.prow_job_id
            -- s = prior-day baseline; the difference e - s gives the windowed totals
            LEFT JOIN test_cumulative_summaries s
                ON s.test_id = e.test_id
                AND s.prow_job_id = e.prow_job_id
                AND s.suite_id = e.suite_id
                AND s.lifecycle = e.lifecycle
                AND s.release = e.release
                AND s.date = (?::date - INTERVAL '1 day')::date
            WHERE e.date = (SELECT MAX(date) FROM test_cumulative_summaries WHERE release = ?)
                AND e.release = ?
                AND t.name = ?
                AND pj.name IN ?
        ) t`
)

// TestReportsByVariant returns per-variant test report rows for every test matching
// the given name criteria. When includeAll is true, an additional "All" aggregate row
// per test is included via UNION ALL (Variant = "All", counting runs across all
// variant_combination_ids without unnesting).
func TestReportsByVariant(
	ctx context.Context,
	dbc *db.DB,
	release string,
	reportType v1.ReportType,
	nameMatches TestNameMatches,
	excludeVariants []string,
	includeAll bool,
) ([]api.Test, error) {
	now := time.Now()

	sample, base := PeriodsForReportType(reportType)
	inner, err := TestReportQuery(ctx, dbc, release, sample, base, nameMatches)
	if err != nil {
		return nil, err
	}
	excludeArr := pq.Array(excludeVariants)

	perVariant := dbc.DB.WithContext(ctx).Table("(?) AS r", inner).
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
		allArm := dbc.DB.WithContext(ctx).Table("(?) AS r", inner).
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
		aggregated = dbc.DB.WithContext(ctx).Raw("? UNION ALL ?", perVariant, allArm)
	} else {
		aggregated = perVariant
	}

	var testReports []api.Test
	q := dbc.DB.WithContext(ctx).Table("(?) AS agg", aggregated).
		Select(fmt.Sprintf("*, %s", QueryTestPercentages)).
		Where("agg.current_runs > 0 OR agg.previous_runs > 0")
	if err := ScanAll(q, &testReports); err != nil {
		log.Error(err)
		return testReports, err
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{"elapsed": elapsed, "count": len(testReports)}).Info("TestReportsByVariant completed")
	return testReports, nil
}

// TestReportExcludeVariants returns a single test report the given test name in the db,
// all variants collapsed, optionally with some excluded.
// The bool reports whether the test was found; a missing test is not an error, so callers
// can omit it, while a failed query is returned so callers do not mistake it for absence.
func TestReportExcludeVariants(ctx context.Context, dbc *db.DB, release, testName string, excludeVariants []string) (api.Test, bool, error) {
	now := time.Now()
	logger := log.WithField("func", "TestReportExcludeVariants").
		WithField("release", release).
		WithField("test", testName)

	sample, base := PeriodsForReportType(v1.CurrentReport)
	inner, err := TestReportQuery(ctx, dbc, release, sample, base, TestNameMatches{ExactNames: []string{testName}})
	if err != nil {
		return api.Test{}, false, fmt.Errorf("building test report query for %q: %w", testName, err)
	}

	aggregated := dbc.DB.WithContext(ctx).Table("(?) AS r", inner).
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
	r := dbc.DB.WithContext(ctx).Table("(?) AS agg", aggregated).
		Select(fmt.Sprintf("*, %s", QueryTestPercentages)).
		Limit(1).
		Find(&testReports)
	if r.Error != nil {
		return api.Test{}, false, fmt.Errorf("querying test report for %q: %w", testName, r.Error)
	}
	if len(testReports) == 0 {
		logger.Warn("test not found in cumulative summaries")
		return api.Test{}, false, nil
	}

	logger.Debugf("completed in %s", time.Since(now))
	return testReports[0], true, nil
}

// LoadBugsForTest returns all bugs in the database for the given test, across all releases.
func LoadBugsForTest(ctx context.Context, dbc *db.DB, testName string, filterClosed bool) ([]models.Bug, error) {
	results := []models.Bug{}

	test := models.Test{}
	q := dbc.DB.WithContext(ctx).Where("name = ?", testName)
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

func TestOutputs(ctx context.Context, dbc *db.DB, release, test string, includedVariants, excludedVariants []string, quantity int) ([]api.TestOutput, error) {
	results := make([]api.TestOutput, 0)

	testQuery := dbc.DB.WithContext(ctx).Table("tests").Where("name = ?", test).Select("id")
	q := dbc.DB.WithContext(ctx).Table("prow_job_run_tests").
		Joins("JOIN prow_job_run_test_outputs ON prow_job_run_test_outputs.prow_job_run_test_id = prow_job_run_tests.id AND prow_job_run_test_outputs.prow_job_run_test_timestamp = prow_job_run_tests.prow_job_run_timestamp").
		Joins("JOIN prow_job_runs ON prow_job_run_tests.prow_job_run_id = prow_job_runs.id AND prow_job_runs.prow_job_release = prow_job_run_tests.prow_job_run_release AND prow_job_runs.timestamp = prow_job_run_tests.prow_job_run_timestamp").
		Joins("JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_job_run_tests.test_id = (?)", testQuery).
		Where("prow_job_run_tests.status IN ?", []int{int(v1.TestStatusFailure), int(v1.TestStatusFlake)}).
		Where("prow_job_run_tests.prow_job_run_timestamp > current_date - interval '14' day").
		Where("prow_job_run_tests.prow_job_run_release = ?", release).
		Where("prow_job_run_test_outputs.prow_job_run_test_timestamp > current_date - interval '14' day").
		Where("prow_job_run_test_outputs.prow_job_run_test_release = ?", release).
		Where("prow_job_runs.prow_job_release = ?", release).
		Where("prow_job_runs.timestamp > current_date - interval '14' day").
		Where("prow_job_runs.labels IS NULL OR NOT (prow_job_runs.labels @> ARRAY['InfraFailure'])")

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

func TestDurations(ctx context.Context, dbc *db.DB, release, test string, includedVariants, excludedVariants []string) (map[civil.Date]float64, error) {
	type testDuration struct {
		Period          civil.Date `json:"period"`
		AverageDuration float64    `json:"average_duration"`
	}
	rows := make([]testDuration, 0)
	results := make(map[civil.Date]float64)

	testQuery := dbc.DB.WithContext(ctx).Table("tests").Where("name = ?", test).Select("id")
	q := dbc.DB.WithContext(ctx).Table("prow_job_run_tests").
		Joins("JOIN tests ON prow_job_run_tests.test_id = tests.id").
		Joins("JOIN prow_jobs ON prow_jobs.id = prow_job_run_tests.prow_job_id").
		Joins("JOIN prow_job_runs ON prow_job_run_tests.prow_job_run_id = prow_job_runs.id AND prow_job_runs.prow_job_release = prow_job_run_tests.prow_job_run_release AND prow_job_runs.timestamp = prow_job_run_tests.prow_job_run_timestamp").
		Where("prow_job_run_tests.prow_job_run_timestamp > current_date - interval '14' day").
		Where("prow_job_run_tests.test_id = (?)", testQuery).
		Where("prow_job_run_tests.prow_job_run_release = ?", release).
		Where("prow_job_runs.labels IS NULL OR NOT (prow_job_runs.labels @> ARRAY['InfraFailure'])")

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
		results[row.Period] = row.AverageDuration
	}

	return results, res.Error
}
