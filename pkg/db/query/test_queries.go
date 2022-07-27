package query

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
)

// TestReportsByVariant returns a test report for every test in the db matching the given substrings, separated by variant.
func TestReportsByVariant(
	dbc *db.DB,
	release string,
	testSubStrings []string,
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
	r := dbc.DB.Raw(q,
		sql.Named("release", release),
		sql.Named("testsubstrings", testSubstringFilter)).Scan(&testReports)
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

	excludeVariantsQuery := ""
	for _, ev := range excludeVariants {
		excludeVariantsQuery += fmt.Sprintf(" AND NOT ('%s'=any(variants))", ev)
	}

	// Query and group by variant:
	var testReport api.Test
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
           sum(previous_flakes)    AS previous_flakes
    FROM prow_test_report_7d_matview
    WHERE release = @release AND name = @testname %s
    GROUP BY name, release
)
SELECT *,
       current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       current_failures * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
       previous_successes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
       (current_successes * 100.0 / NULLIF(current_runs, 0)) - (previous_successes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
FROM results;
`
	q = fmt.Sprintf(q, excludeVariantsQuery)
	r := dbc.DB.Raw(q,
		sql.Named("release", release),
		sql.Named("testname", testName)).First(&testReport)
	if r.Error != nil {
		log.Error(r.Error)
		return testReport, r.Error
	}

	elapsed := time.Since(now)
	log.Infof("TestReportExcludeVariants completed in %s", elapsed)
	return testReport, nil
}

// LoadBugsForTest returns all bugs in the database for the given test, across all releases.
func LoadBugsForTest(dbc *db.DB, testName string) ([]models.Bug, error) {
	results := []models.Bug{}

	test := models.Test{}
	res := dbc.DB.Where("name = ?", testName).Preload("Bugs").First(&test)
	if res.Error != nil {
		return results, res.Error
	}
	log.Infof("found %d bugs for test", len(test.Bugs))
	return test.Bugs, nil
}
