package query

import (
	"database/sql"
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

// TestReportsByVariant returns a test report for every test in the db matching the given substrings, separated by variant.
func TestReportsByVariant(
	dbc *db.DB,
	release string,
	reportType v1.ReportType, // defaults to "current" or last 7 days vs prev 7 days
	testSubStrings []string,
	excludeVariants []string,
) ([]api.Test, error) {
	now := time.Now()

	testSubstringFilter := strings.Join(testSubStrings, "|")
	testSubstringFilter = strings.ReplaceAll(testSubstringFilter, "[", "\\[")
	testSubstringFilter = strings.ReplaceAll(testSubstringFilter, "]", "\\]")

	// Query and group by variant:
	var testReports []api.Test
	q := `
WITH excluded_vc AS (
    SELECT id FROM variant_combinations WHERE @excluded && variants
),
results AS (
    SELECT name,
           release,
           sum(current_runs)       AS current_runs,
           sum(current_successes)  AS current_successes,
           sum(current_failures)   AS current_failures,
           sum(current_flakes)     AS current_flakes,
           sum(previous_runs)      AS previous_runs,
           sum(previous_successes) AS previous_successes,
           sum(previous_failures)  AS previous_failures,
           sum(previous_flakes)    AS previous_flakes,
           unnest(variants)        AS variant
    FROM prow_test_report_7d_matview
	WHERE release = @release AND name ~* @testsubstrings
          AND variant_combination_id NOT IN (SELECT id FROM excluded_vc)
    GROUP BY name, release, variant
)
SELECT *,
       current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       current_failures * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
       previous_successes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
       (current_successes * 100.0 / NULLIF(current_runs, 0)) - (previous_successes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
FROM results;
`
	if reportType == v1.TwoDayReport {
		q = strings.ReplaceAll(q, "prow_test_report_7d_matview", "prow_test_report_2d_matview")
	}

	qParams := []interface{}{
		sql.Named("excluded", pq.Array(excludeVariants)),
		sql.Named("release", release),
		sql.Named("testsubstrings", testSubstringFilter),
	}
	r := dbc.DB.Raw(q, qParams...).Scan(&testReports)
	if r.Error != nil {
		log.Error(r.Error)
		return testReports, r.Error
	}

	elapsed := time.Since(now)
	log.Infof("TestReportsByVariant completed in %s with %d results from db", elapsed, len(testReports))
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

	// Query and group by variant:
	var testReport api.Test
	q := `WITH excluded_vc AS (
    SELECT id FROM variant_combinations WHERE @excluded && variants
),
results AS (
    SELECT name,
           release,
           sum(current_runs)       AS current_runs,
           sum(current_successes)  AS current_successes,
           sum(current_failures)   AS current_failures,
           sum(current_flakes)     AS current_flakes,
           sum(previous_runs)      AS previous_runs,
           sum(previous_successes) AS previous_successes,
           sum(previous_failures)  AS previous_failures,
           sum(previous_flakes)    AS previous_flakes
    FROM prow_test_report_7d_matview
    WHERE release = @release AND name = @testname
          AND variant_combination_id NOT IN (SELECT id FROM excluded_vc)
    GROUP BY name, release
) SELECT *, %s FROM results;`

	q = fmt.Sprintf(q, QueryTestPercentages)
	qParams := []interface{}{
		sql.Named("excluded", pq.Array(excludeVariants)),
		sql.Named("release", release),
		sql.Named("testname", testName),
	}
	if r := dbc.DB.Raw(q, qParams...).First(&testReport); r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			logger.Debug("test not found")
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

// TestsByNURPAndStandardDeviation returns a test report for every test in the db, separated by variant.
// Each row includes current/previous test rates and cross-variant statistics (AVG, STDDEV, delta)
// that are pre-computed in the matview. Only the deltas are computed at query time.
func TestsByNURPAndStandardDeviation(dbc *db.DB, release, table string) *gorm.DB {
	return dbc.DB.
		Table(table).
		Select(`*,
			(current_working_percentage - working_average) AS delta_from_working_average,
			(current_pass_percentage - passing_average) AS delta_from_passing_average,
			(current_flake_percentage - flake_average) AS delta_from_flake_average`).
		Where(`release = ?`, release).
		Where("variant_combination_id NOT IN (SELECT id FROM variant_combinations WHERE 'never-stable' = any(variants))")
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
		q = q.Where("? = any(prow_jobs.variants)", variant)
	}

	for _, variant := range excludedVariants {
		q = q.Where("NOT ? = any(prow_jobs.variants)", variant)
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
		q = q.Where("? = any(prow_jobs.variants)", variant)
	}

	for _, variant := range excludedVariants {
		q = q.Where("NOT ? = any(prow_jobs.variants)", variant)
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
