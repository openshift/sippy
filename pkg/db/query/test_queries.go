package query

import (
	"database/sql"
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
           (array_agg(open_bugs))[1] AS open_bugs`

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
		current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
		current_failures * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
		current_flakes * 100.0 / NULLIF(current_runs, 0) AS current_flake_percentage,
		(current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0) AS current_working_percentage,
		previous_successes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
		previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
		previous_flakes * 100.0 / NULLIF(previous_runs, 0) AS previous_flake_percentage,
		(previous_successes + previous_flakes) * 100.0 / NULLIF(previous_runs, 0) AS previous_working_percentage,
		(previous_failures * 100.0 / NULLIF(previous_runs, 0)) - (current_failures * 100.0 / NULLIF(current_runs, 0)) AS net_failure_improvement,
		(previous_flakes * 100.0 / NULLIF(previous_runs, 0)) - (current_flakes * 100.0 / NULLIF(current_runs, 0)) AS net_flake_improvement,
		((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0)) - ((previous_successes + previous_flakes) * 100.0 / NULLIF(previous_runs, 0)) AS net_working_improvement,
		(current_successes * 100.0 / NULLIF(current_runs, 0)) - (previous_successes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement`

	QueryTestSummarizer = QueryTestFields + "," + QueryTestPercentages

	QueryTestAnalysis = `
        select current_successes, current_runs,
               current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage
        from (
            select sum(runs) as current_runs, sum(passes) as current_successes
            from test_analysis_by_job_by_dates 
            where date >= ? AND test_name = ? AND job_name IN ?
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
WITH results AS (
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
          AND NOT(@excluded && variants)
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
func TestReportExcludeVariants(
	dbc *db.DB,
	release string,
	testName string,
	excludeVariants []string,
) (api.Test, error) {
	now := time.Now()

	// Query and group by variant:
	var testReport api.Test
	q := `WITH results AS (
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
          AND NOT(@excluded && variants)
    GROUP BY name, release
) SELECT *, %s FROM results;`

	q = fmt.Sprintf(q, QueryTestPercentages)
	qParams := []interface{}{
		sql.Named("excluded", pq.Array(excludeVariants)),
		sql.Named("release", release),
		sql.Named("testname", testName),
	}
	r := dbc.DB.Raw(q, qParams...).First(&testReport)
	if r.Error != nil {
		log.Error(r.Error)
		return testReport, r.Error
	}

	elapsed := time.Since(now)
	log.Infof("TestReportExcludeVariants completed in %s for release %s and test %q", elapsed, release, testName)
	return testReport, nil
}

// LoadBugsForTest returns all bugs in the database for the given test, across all releases.
func LoadBugsForTest(dbc *db.DB, testName string, filterClosed bool) ([]models.Bug, error) {
	results := []models.Bug{}

	test := models.Test{}
	q := dbc.DB.Where("name = ?", testName)
	if filterClosed {
		q = q.Preload("Bugs", "UPPER(status) != 'CLOSED' and UPPER(status) != 'VERIFIED'")
	} else {
		q = q.Preload("Bugs")
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

// TestsByNURPAndStandardDeviation returns a test report for every test in the db matching the given substrings, separated by variant.
// Result will include current and previous test rates such as passing, flaking, failing rates.
// In addition, it includes the following calculated rates to help identify bad nurps.
// working_average shows the average working percentage among all variants.
// working_standard_deviation shows the standard deviation of the working percentage among variants. The number reflects how much working percentage differs among variants.
// delta_from_working_average shows how much each variant differs from the working_average. This can be used to identify outliers.
// passing_average shows the average passing percentage among all variants.
// passing_standard_deviation shows the standard deviation of the passing percentage among variants. The number reflects how much passing percentage differs among variants.
// delta_from_passing_average shows how much each variant differs from the passing_average. This can be used to identify outliers.
// flake_average shows the average flake percentage among all variants.
// flake_standard_deviation shows the standard deviation of the flake percentage among variants. The number reflects how much flake percentage differs among variants.
// delta_from_flake_average shows how much each variant differs from the flake_average. This can be used to identify outliers.
func TestsByNURPAndStandardDeviation(dbc *db.DB, release, table string) *gorm.DB {
	// 1. Create a virtual stats table. There is a single row for each test.
	stats := dbc.DB.Table(table).
		Select(`
                 id                                                                             AS test_id,
                 suite_name                                                                     AS stats_suite_name,
                 avg((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0))    AS working_average,
                 stddev((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0)) AS working_standard_deviation,
                 avg(current_successes * 100.0 / NULLIF(current_runs, 0))                       AS passing_average,
                 stddev(current_successes * 100.0 / NULLIF(current_runs, 0))                    AS passing_standard_deviation,
                 avg(current_flakes * 100.0 / NULLIF(current_runs, 0))                          AS flake_average,
                 stddev(current_flakes * 100.0 / NULLIF(current_runs, 0))                       AS flake_standard_deviation`).
		Where(`release = ?`, release).
		Group("id, suite_name")

	// 2. Collect standard stats for all tests. Each row applies to one variant of a test.
	passRates := dbc.DB.Table(table).
		Select(`id as test_id, suite_name as pass_rate_suite_name, variants as pass_rate_variants, `+QueryTestPercentages).
		Where(`release = ?`, release)

	// 3. Join the tables to produce test report. Each row represent one variant of a test and contains all stats, both unique to the specific variant and average across all variants.
	return dbc.DB.
		Table(table).
		Select("*, (current_working_percentage - working_average) as delta_from_working_average, (current_pass_percentage - passing_average) as delta_from_passing_average, (current_flake_percentage - flake_average) as delta_from_flake_average").
		Joins(fmt.Sprintf(`INNER JOIN (?) as pass_rates on pass_rates.test_id = %s.id AND pass_rates.pass_rate_suite_name IS NOT DISTINCT FROM %s.suite_name AND pass_rates.pass_rate_variants = %s.variants`, table, table, table), passRates).
		Joins(fmt.Sprintf(`JOIN (?) as stats ON stats.test_id = %s.id AND stats.stats_suite_name IS NOT DISTINCT FROM %s.suite_name`, table, table), stats).
		Where(`release = ?`, release).
		Where(fmt.Sprintf("NOT ('never-stable'=any(%s.variants))", table))
}

func TestOutputs(dbc *db.DB, release, test string, includedVariants, excludedVariants []string, quantity int) ([]api.TestOutput, error) {
	results := make([]api.TestOutput, 0)

	testQuery := dbc.DB.Table("tests").Where("name = ?", test).Select("id")
	q := dbc.DB.Table("prow_job_run_test_outputs").
		Joins("JOIN prow_job_run_tests ON prow_job_run_test_outputs.prow_job_run_test_id = prow_job_run_tests.id").
		Joins("JOIN prow_job_runs ON prow_job_run_tests.prow_job_run_id = prow_job_runs.id").
		Joins("JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_job_runs.timestamp > current_date - interval '14' day").
		Where("prow_job_run_tests.test_id = (?)", testQuery).
		Where("prow_jobs.release = ?", release)

	for _, variant := range includedVariants {
		q = q.Where("? = any(prow_jobs.variants)", variant)
	}

	for _, variant := range excludedVariants {
		q = q.Where("NOT ? = any(prow_jobs.variants)", variant)
	}

	res := q.
		Select("prow_job_runs.url, output").
		Order("prow_job_run_test_outputs.id DESC").
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
		Joins("JOIN prow_job_runs ON prow_job_run_tests.prow_job_run_id = prow_job_runs.id").
		Joins("JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_job_runs.timestamp > current_date - interval '14' day").
		Where("prow_job_run_tests.test_id = (?)", testQuery).
		Where("prow_jobs.release = ?", release)

	for _, variant := range includedVariants {
		q = q.Where("? = any(prow_jobs.variants)", variant)
	}

	for _, variant := range excludedVariants {
		q = q.Where("NOT ? = any(prow_jobs.variants)", variant)
	}

	res := q.
		Select(`
			date("timestamp" AT TIME ZONE 'UTC'::text) as period,
			AVG(prow_job_run_tests.duration) as average_duration`).
		Group(`date("timestamp" AT TIME ZONE 'UTC'::text)`).
		Order(`date("timestamp" AT TIME ZONE 'UTC'::text)`).
		Scan(&rows)

	for _, row := range rows {
		results[row.Period.Format("2006-01-02")] = row.AverageDuration
	}

	return results, res.Error
}
