package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	gosort "sort"
	"strconv"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	bq "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/html/installhtml"
	"github.com/openshift/sippy/pkg/util/param"
)

const (
	testReport7dMatView          = "prow_test_report_7d_matview"
	testReport2dMatView          = "prow_test_report_2d_matview"
	payloadFailedTests14dMatView = "payload_test_failures_14d_matview"
)

func PrintTestsDetailsJSONFromDB(w http.ResponseWriter, release string, testSubstrings []string, dbc *db.DB) {
	responseStr, err := installhtml.TestDetailTestsFromDB(dbc, release, testSubstrings)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": err.Error()})
		return
	}
	RespondWithJSON(http.StatusOK, w, responseStr)
}

func GetTestOutputsFromDB(dbc *db.DB, release, test string, filters *filter.Filter, quantity int) ([]apitype.TestOutput, error) {
	var includedVariants, excludedVariants []string
	if filters != nil {
		for _, f := range filters.Items {
			if f.Field == "variants" {
				if f.Not {
					excludedVariants = append(excludedVariants, f.Value)
				} else {
					includedVariants = append(includedVariants, f.Value)
				}
			}
		}
	}

	return query.TestOutputs(dbc, release, test, includedVariants, excludedVariants, quantity)
}

func GetTestRunsAndOutputsFromBigQuery(ctx context.Context, bigQueryClient *bq.Client, testID string, prowJobRunIDs, prowJobNames []string, includeSuccess bool, startDate, endDate time.Time) ([]apitype.TestOutputBigQuery, error) {
	// Use component_mapping to resolve test_id to test_name/testsuite, which handles test renames.
	// The test_id in junit may be stale (from before a rename), but component_mapping.id is canonical.
	// We join on name/suite to find all junit rows for this test, regardless of when they were created.
	// We also join jobs to get prowjob_url, and prowjob_start.
	// Build optional filter clauses used by both the matching_runs CTE and main query.
	filterStr := ""
	if !includeSuccess {
		filterStr += `
  AND junit.success = false`
	}
	if len(prowJobRunIDs) > 0 {
		filterStr += `
  AND junit.prowjob_build_id IN UNNEST(@prowJobRunIDs)`
	}
	for i := range prowJobNames {
		filterStr += fmt.Sprintf(`
  AND LOWER(junit.prowjob_name) LIKE CONCAT('%%', LOWER(@prowJobName%d), '%%')`, i)
	}

	queryStr := `WITH test_mapping AS (
  SELECT name, suite
  FROM ` + "`openshift-gce-devel.ci_analysis_us.component_mapping_latest`" + `
  WHERE id = @testID
),
matching_runs AS (
  SELECT DISTINCT junit.prowjob_build_id
  FROM ` + "`openshift-gce-devel.ci_analysis_us.junit`" + ` AS junit
  INNER JOIN test_mapping ON junit.test_name = test_mapping.name AND junit.testsuite = test_mapping.suite
  INNER JOIN ` + "`openshift-gce-devel.ci_analysis_us.jobs`" + ` AS jobs ON junit.prowjob_build_id = jobs.prowjob_build_id
  WHERE junit.modified_time BETWEEN DATETIME(@startDate) AND DATETIME(@endDate)
    AND jobs.prowjob_job_name NOT LIKE '%aggregat%'` + filterStr + `
),
failed_test_counts AS (
  SELECT prowjob_build_id,
    COUNTIF(adjusted_success_val = 0 AND adjusted_flake_count = 0) AS failed_tests
  FROM (
    SELECT d.prowjob_build_id,
      ROW_NUMBER() OVER(PARTITION BY d.prowjob_build_id, d.file_path, d.test_name, d.testsuite ORDER BY
        CASE
          WHEN d.flake_count > 0 THEN 0
          WHEN d.success_val > 0 THEN 1
          ELSE 2
        END) AS row_num,
      CASE WHEN d.flake_count > 0 THEN 0 ELSE d.success_val END AS adjusted_success_val,
      CASE WHEN d.flake_count > 0 THEN 1 ELSE 0 END AS adjusted_flake_count
    FROM ` + "`openshift-gce-devel.ci_analysis_us.junit`" + ` d
    INNER JOIN matching_runs mr ON d.prowjob_build_id = mr.prowjob_build_id
    WHERE d.modified_time BETWEEN DATETIME(@startDate) AND DATETIME(@endDate)
      AND d.skipped = false
  ) deduped
  WHERE deduped.row_num = 1
  GROUP BY prowjob_build_id
)
SELECT junit.prowjob_build_id, junit.test_name, junit.success, junit.test_id, junit.branch, junit.prowjob_name, junit.failure_content,
       jobs.prowjob_url, jobs.prowjob_start, COALESCE(ftc.failed_tests, 0) AS failed_tests
FROM ` + "`openshift-gce-devel.ci_analysis_us.junit`" + ` AS junit
INNER JOIN test_mapping ON junit.test_name = test_mapping.name AND junit.testsuite = test_mapping.suite
INNER JOIN ` + "`openshift-gce-devel.ci_analysis_us.jobs`" + ` AS jobs ON junit.prowjob_build_id = jobs.prowjob_build_id
LEFT JOIN failed_test_counts ftc ON junit.prowjob_build_id = ftc.prowjob_build_id
WHERE junit.modified_time BETWEEN DATETIME(@startDate) AND DATETIME(@endDate)
  AND jobs.prowjob_job_name NOT LIKE '%aggregat%'` + filterStr + `
ORDER BY jobs.prowjob_start DESC
LIMIT 500`

	q := bigQueryClient.Query(ctx, bqlabel.TestOutputs, queryStr)
	q.Parameters = []bigquery.QueryParameter{
		{
			Name:  "testID",
			Value: testID,
		},
		{
			Name:  "startDate",
			Value: startDate,
		},
		{
			Name:  "endDate",
			Value: endDate,
		},
	}

	if len(prowJobRunIDs) > 0 {
		q.Parameters = append(q.Parameters, bigquery.QueryParameter{
			Name:  "prowJobRunIDs",
			Value: prowJobRunIDs,
		})
	}

	for i, name := range prowJobNames {
		q.Parameters = append(q.Parameters, bigquery.QueryParameter{
			Name:  fmt.Sprintf("prowJobName%d", i),
			Value: name,
		})
	}

	// Log the query with parameters substituted for easy copy-paste
	bq.LogQueryWithParamsReplaced(log.WithField("type", "TestOutputs"), q)

	it, err := bq.LoggedRead(ctx, q)
	if err != nil {
		log.WithError(err).Error("error querying test outputs from bigquery")
		return nil, fmt.Errorf("error querying test outputs from bigquery: %w", err)
	}

	type testOutputRow struct {
		ProwJobBuildID string                `bigquery:"prowjob_build_id"`
		TestName       string                `bigquery:"test_name"`
		Success        bool                  `bigquery:"success"`
		TestID         string                `bigquery:"test_id"`
		Branch         string                `bigquery:"branch"`
		ProwJobName    string                `bigquery:"prowjob_name"`
		FailureContent string                `bigquery:"failure_content"`
		ProwJobURL     bigquery.NullString   `bigquery:"prowjob_url"`
		ProwJobStart   bigquery.NullDateTime `bigquery:"prowjob_start"`
		FailedTests    bigquery.NullInt64    `bigquery:"failed_tests"`
	}

	var outputs []apitype.TestOutputBigQuery
	for {
		var row testOutputRow
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error reading test output row from bigquery")
			continue
		}

		output := apitype.TestOutputBigQuery{
			Output:      row.FailureContent,
			TestName:    row.TestName,
			Success:     row.Success,
			ProwJobName: row.ProwJobName,
			FailedTests: int(row.FailedTests.Int64),
		}
		if row.ProwJobURL.Valid {
			output.ProwJobURL = row.ProwJobURL.StringVal
		}
		if row.ProwJobStart.Valid {
			t := row.ProwJobStart.DateTime.In(time.UTC)
			output.StartTime = &t
		}
		outputs = append(outputs, output)
	}

	return outputs, nil
}

func GetTestDurationsFromDB(dbc *db.DB, release, test string, filters *filter.Filter) (map[string]float64, error) {
	var includedVariants, excludedVariants []string
	if filters != nil {
		for _, f := range filters.Items {
			if f.Field == "variants" {
				if f.Not {
					excludedVariants = append(excludedVariants, f.Value)
				} else {
					includedVariants = append(includedVariants, f.Value)
				}
			}
		}
	}

	return query.TestDurations(dbc, release, test, includedVariants, excludedVariants)
}

type testsAPIResult []apitype.Test

func (tests testsAPIResult) sort(req *http.Request) testsAPIResult {
	sortField := param.SafeRead(req, "sortField")
	sort := param.SafeRead(req, "sort")

	if sortField == "" {
		sortField = "current_pass_percentage"
	}

	if sort == "" {
		sort = "asc"
	}

	gosort.Slice(tests, func(i, j int) bool {
		if sort == "asc" {
			return filter.Compare(tests[i], tests[j], sortField)
		}
		return filter.Compare(tests[j], tests[i], sortField)
	})

	return tests
}

func (tests testsAPIResult) limit(req *http.Request) testsAPIResult {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit == 0 || len(tests) < limit {
		return tests
	}

	return tests[:limit]
}

type testsAPIResultBQ []apitype.TestBQ

func (tests testsAPIResultBQ) sort(req *http.Request) testsAPIResultBQ {
	sortField := param.SafeRead(req, "sortField")
	sort := param.SafeRead(req, "sort")

	if sortField == "" {
		sortField = "current_pass_percentage"
	}

	if sort == "" {
		sort = "asc"
	}

	gosort.Slice(tests, func(i, j int) bool {
		if sort == "asc" {
			return filter.Compare(tests[i], tests[j], sortField)
		}
		return filter.Compare(tests[j], tests[i], sortField)
	})

	return tests
}

func (tests testsAPIResultBQ) limit(req *http.Request) testsAPIResultBQ {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit == 0 || len(tests) < limit {
		return tests
	}

	return tests[:limit]
}

func PrintTestsJSONFromDB(release string, w http.ResponseWriter, req *http.Request, dbc *db.DB) {
	var fil *filter.Filter

	// Collapse means to produce an aggregated test result of all variant (NURP+ - network, upgrade, release, platform)
	// combos. Uncollapsed results shows you the per-NURP+ result for each test (currently approx. 50,000 rows: filtering
	// is advised)
	collapseStr := req.URL.Query().Get("collapse")
	collapse := true
	if collapseStr == "false" {
		collapse = false
	}

	overallStr := req.URL.Query().Get("overall")
	includeOverall := !collapse
	if overallStr != "" {
		includeOverall, _ = strconv.ParseBool(overallStr)
	}

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		fil = &filter.Filter{}
		if err := json.Unmarshal([]byte(queryFilter), fil); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	// If requesting a two day report, we make the comparison between the last
	// period (typically 7 days) and the last two days.
	period := req.URL.Query().Get("period")
	if period != "" && period != "default" && period != "current" && period != "twoDay" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Unknown period"})
		return
	}

	testsResult, overall, err := BuildTestsResults(dbc, release, period, collapse, includeOverall, fil)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job report:" + err.Error()})
		return
	}

	testsResult = testsResult.sort(req).limit(req)
	if overall != nil {
		testsResult = append([]apitype.Test{*overall}, testsResult...)
	}

	RespondWithJSON(http.StatusOK, w, testsResult)
}

func PrintTestsJSONFromBigQuery(release string, w http.ResponseWriter, req *http.Request, bqc *bq.Client) {
	var fil *filter.Filter

	// Collapse means to produce an aggregated test result of all variant (NURP+ - network, upgrade, release, platform)
	// combos. Uncollapsed results shows you the per-NURP+ result for each test (currently approx. 50,000 rows: filtering
	// is advised)
	collapseStr := req.URL.Query().Get("collapse")
	collapse := true
	if collapseStr == "false" {
		collapse = false
	}

	overallStr := req.URL.Query().Get("overall")
	includeOverall := !collapse
	if overallStr != "" {
		includeOverall, _ = strconv.ParseBool(overallStr)
	}

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		fil = &filter.Filter{}
		if err := json.Unmarshal([]byte(queryFilter), fil); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	// If requesting a two day report, we make the comparison between the last
	// period (typically 7 days) and the last two days.
	period := req.URL.Query().Get("period")
	if period != "" && period != "default" && period != "current" && period != "twoDay" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Unknown period"})
		return
	}

	testsResult, overall, err := BuildTestsResultsFromBigQuery(req.Context(), bqc, release, period, collapse, includeOverall, fil)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job report:" + err.Error()})
		return
	}

	testsResult = testsResult.sort(req).limit(req)
	if overall != nil {
		testsResult = append([]apitype.TestBQ{*overall}, testsResult...)
	}

	RespondWithJSON(http.StatusOK, w, testsResult)
}

func PrintCanaryTestsFromDB(release string, w http.ResponseWriter, dbc *db.DB) {
	f := filter.Filter{
		Items: []filter.FilterItem{
			{
				Field:    "current_pass_percentage",
				Operator: ">=",
				Value:    "99",
			},
		},
	}

	results, _, err := BuildTestsResults(dbc, release, "default", true, false, &f)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building test report:" + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	for _, result := range results {
		fmt.Fprintf(w, "%q:struct{}{},\n", result.Name)
	}
}

func GetJobRunTestsCountByLookback(dbc *db.DB, lookbackDays int) (int64, int64, error) {
	if lookbackDays < 1 {
		return -1, -1, errors.New("Lookback Days must be greater than zero")
	}
	// Calculate the truncated time
	now := time.Now().UTC()
	truncatedTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -lookbackDays)

	type counts = struct {
		JobRunsCount int64 `json:"job_runs_count"`
		TestIDsCount int64 `json:"test_ids_count"`
	}

	queryCounts := counts{}
	timeStart := time.Now()

	log.Infof("Starting tests count query for lookback: %d", lookbackDays)

	err := dbc.DB.Table("prow_job_run_tests").
		Select("count(distinct prow_job_run_id) as job_runs_count, count(distinct test_id) as test_ids_count").
		Where("created_at > ?", truncatedTime).
		Scan(&queryCounts).
		Error

	timeFinish := time.Now()
	log.Infof("Finished tests count query for lookback: %d, duration: %s", lookbackDays, timeFinish.Sub(timeStart).String())

	if err != nil {
		return -1, -1, err
	}

	return queryCounts.JobRunsCount, queryCounts.TestIDsCount, nil
}

func BuildTestsResults(dbc *db.DB, release, period string, collapse, includeOverall bool, fil *filter.Filter) (testsAPIResult, *apitype.Test, error) { //lint:ignore
	now := time.Now()

	// Test results are generated by using two subqueries, which need to be filtered separately. Once during
	// pre-processing where we're evaluating summed variant results, and in post-processing after we've
	// assembled our final temporary table.
	var rawFilter, processedFilter *filter.Filter
	if fil != nil {
		rawFilter, processedFilter = fil.Split([]string{"name", "variants"})
	}

	table := testReport7dMatView
	if period == "twoDay" {
		table = testReport2dMatView
	}

	rawQuery := dbc.DB.
		Table(table).
		Where("release = ?", release)

	// Collapse groups the test results together -- otherwise we return the test results per-variant combo (NURP+)
	variantSelect := ""
	if collapse {
		rawQuery = rawQuery.Select(`suite_name,name,jira_component,jira_component_id,` + query.QueryTestSummer).Group("suite_name,name,jira_component,jira_component_id")
	} else {
		rawQuery = query.TestsByNURPAndStandardDeviation(dbc, release, table)
		variantSelect = "suite_name, variants," +
			"delta_from_working_average, working_average, working_standard_deviation, " +
			"delta_from_passing_average, passing_average, passing_standard_deviation, " +
			"delta_from_flake_average, flake_average, flake_standard_deviation, "

	}

	if rawFilter != nil {
		rawQuery = rawFilter.ToSQL(rawQuery, apitype.Test{})
	}

	testReports := make([]apitype.Test, 0)
	// FIXME: Add test id to matview, for now generate with ROW_NUMBER OVER
	processedResults := dbc.DB.Table("(?) as results", rawQuery).
		Select(`ROW_NUMBER() OVER() as id, suite_name, name, jira_component, jira_component_id,` + variantSelect + query.QueryTestSummarizer).
		Where("current_runs > 0 or previous_runs > 0")

	finalResults := dbc.DB.Table("(?) as final_results", processedResults)
	if processedFilter != nil {
		finalResults = processedFilter.ToSQL(finalResults, apitype.Test{})
	}

	frr := finalResults.Scan(&testReports)
	if frr.Error != nil {
		log.WithError(finalResults.Error).Error("error querying test reports")
		return []apitype.Test{}, nil, frr.Error
	}

	// Produce a special "overall" test that has a summary of all the selected tests.
	var overallTest *apitype.Test
	if includeOverall {
		finalResults := dbc.DB.Table("(?) as final_results", finalResults)
		finalResults = finalResults.Select(query.QueryTestSummer)
		summaryResult := dbc.DB.Table("(?) as overall", finalResults).Select(query.QueryTestSummarizer)
		overallTest = &apitype.Test{
			ID:   math.MaxInt32,
			Name: "Overall",
		}
		// TODO: column open_bugs does not exist here?
		summaryResult.Scan(overallTest)
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{
		"elapsed": elapsed,
		"reports": len(testReports),
	}).Debug("BuildTestsResults completed")

	return testReports, overallTest, nil
}

func BuildTestsResultsFromBigQuery(ctx context.Context, bqc *bq.Client, release, period string, collapse, includeOverall bool, fil *filter.Filter) (testsAPIResultBQ, *apitype.TestBQ, error) { //lint:ignore
	now := time.Now()

	// Test results are generated by using two subqueries, which need to be filtered separately. Once during
	// pre-processing where we're evaluating summed variant results, and in post-processing after we've
	// assembled our final temporary table.
	var rawFilter, processedFilter *filter.Filter
	if fil != nil {
		rawFilter, processedFilter = fil.Split([]string{"name", "variants"})
	}

	table := "junit_7day_comparison"
	if period == "twoDay" {
		table = "junit_2day_comparison"
	}

	// Collapse groups the test results together -- otherwise we return the test results per-variant combo (NURP+)
	candidateQueryStr := ""
	whereStr := `
		WHERE release=@release AND (current_runs > 0 or previous_runs > 0)`

	// Collect all query parameters
	queryParams := []bigquery.QueryParameter{
		{Name: "release", Value: release},
	}
	paramIndex := 0

	if rawFilter != nil && len(rawFilter.Items) > 0 {
		filterResult := rawFilter.ToBQStr(apitype.Test{}, &paramIndex)
		whereStr += " AND " + filterResult.SQL
		// Add filter parameters directly from the filter result
		queryParams = append(queryParams, filterResult.Parameters...)
	}

	if collapse {
		candidateQueryStr = fmt.Sprintf(`WITH group_stats AS (
		SELECT
			cm.cm_id as test_id,
			name,
			jira_component,
			jira_component_id,
			release,
			%s
		FROM %s.%s junit
		INNER JOIN (SELECT id AS cm_id, name AS cm_name, suite AS cm_suite FROM %s.component_mapping_latest) cm ON junit.name = cm.cm_name AND junit.testsuite = cm.cm_suite
		%s
		GROUP BY cm.cm_id, name, jira_component, jira_component_id, release
	),
	candidate_query AS (
		SELECT
			ROW_NUMBER() OVER() as id,
			test_id,
			name,
			jira_component,
			jira_component_id,
			%s
		FROM group_stats
	)
	`, query.QueryTestSummer, bqc.Dataset, table, bqc.Dataset, whereStr, query.QueryTestSummarizer)
	} else {
		if processedFilter != nil && len(processedFilter.Items) > 0 {
			filterResult := processedFilter.ToBQStr(apitype.Test{}, &paramIndex)
			whereStr += " AND " + filterResult.SQL
			// Add processed filter parameters directly from the filter result
			queryParams = append(queryParams, filterResult.Parameters...)
		}
		candidateQueryStr = fmt.Sprintf(`WITH test_stats AS (
		SELECT
			 test_id,
			 testsuite                                                                                   AS stats_suite_name,
			 COALESCE(avg((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0)), 0)    AS working_average,
			 COALESCE(stddev((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0)), 0) AS working_standard_deviation,
			 COALESCE(avg(current_successes * 100.0 / NULLIF(current_runs, 0)), 0)                       AS passing_average,
			 COALESCE(stddev(current_successes * 100.0 / NULLIF(current_runs, 0)), 0)                    AS passing_standard_deviation,
			 COALESCE(avg(current_flakes * 100.0 / NULLIF(current_runs, 0)), 0)                          AS flake_average,
			 COALESCE(stddev(current_flakes * 100.0 / NULLIF(current_runs, 0)), 0)                       AS flake_standard_deviation
		FROM %s.%s junit
		WHERE release=@release
		GROUP BY test_id, testsuite),
	unfiltered_candidate_query AS (
		SELECT
			ROW_NUMBER() OVER() as id,
			cm.cm_id as test_id,
			name,
			jira_component,
			jira_component_id,
			testsuite as suite_name,
			variants,
			release,
			(current_working_percentage - working_average) as delta_from_working_average,
			working_average,
			working_standard_deviation,
			(current_pass_percentage - passing_average) as delta_from_passing_average,
			passing_average,
			passing_standard_deviation,
			(current_flake_percentage - flake_average) as delta_from_flake_average,
			flake_average,
			flake_standard_deviation,
			%s
		FROM %s.%s junit
		JOIN test_stats as stats ON stats.test_id = junit.test_id AND stats.stats_suite_name IS NOT DISTINCT FROM junit.testsuite
		INNER JOIN (SELECT id AS cm_id, name AS cm_name, suite AS cm_suite FROM %s.component_mapping_latest) cm ON junit.name = cm.cm_name AND junit.testsuite = cm.cm_suite),
	candidate_query AS (
		SELECT
			*
		FROM
			unfiltered_candidate_query
		%s
	)`, bqc.Dataset, table, query.QueryTestSummarizer, bqc.Dataset, table, bqc.Dataset, whereStr)
	}

	queryStr := fmt.Sprintf(`%s
		SELECT *
		FROM candidate_query`, candidateQueryStr)

	q := bqc.Query(ctx, bqlabel.TestResults, queryStr)
	q.Parameters = queryParams
	testReports, errs := FetchTestResultsFromBQ(ctx, q)
	if len(errs) > 0 {
		return []apitype.TestBQ{}, nil, errs[0]
	}

	// Produce a special "overall" test that has a summary of all the selected tests.
	var overallTest *apitype.TestBQ
	if includeOverall {
		queryStr := fmt.Sprintf(`%s,
	group_stats AS (
		SELECT
			%s
		FROM candidate_query
	)
	SELECT %s
	FROM group_stats`, candidateQueryStr, query.QueryTestSummer, query.QueryTestSummarizer)
		q := bqc.Query(ctx, bqlabel.TestResultsOverall, queryStr)
		q.Parameters = queryParams // Reuse the same parameters for the overall query

		overallReports, errs := FetchTestResultsFromBQ(ctx, q)
		if len(errs) > 0 {
			return testReports, nil, errs[0]
		}

		overallTest = &overallReports[0]
		overallTest.ID = math.MaxInt32
		overallTest.Name = "Overall"
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{
		"elapsed": elapsed,
		"reports": len(testReports),
	}).Debug("BuildTestsResults completed")

	return testReports, overallTest, nil
}

func FetchTestResultsFromBQ(ctx context.Context, q *bigquery.Query) ([]apitype.TestBQ, []error) {
	errs := []error{}
	result := []apitype.TestBQ{}
	log.Infof("Fetching test result with:\n%s\nParameters:\n%+v\n", q.Q, q.Parameters)

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying test result from bigquery")
		errs = append(errs, err)
		return result, errs
	}

	for {
		row := apitype.TestBQ{}
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing test result from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing test result from bigquery"))
			continue
		}
		result = append(result, row)
	}
	return result, errs
}

// GetTestCapabilitiesFromDB returns a sorted list of capabilities from the BQ component_mapping_latest table
func GetTestCapabilitiesFromDB(ctx context.Context, bqClient *bq.Client) ([]string, error) {
	if bqClient == nil || bqClient.BQ == nil {
		return []string{}, nil
	}

	qFmt := "SELECT ARRAY_AGG(DISTINCT capability ORDER BY capability) AS capabilities FROM `%s.component_mapping_latest`, UNNEST(capabilities) AS capability"
	q := bqClient.Query(ctx, bqlabel.TestCapabilities, fmt.Sprintf(qFmt, bqClient.Dataset))

	log.Infof("Fetching test capabilities with:\n%s\n", q.Q)

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying test capabilities from bigquery")
		return []string{}, err
	}

	var row struct {
		Capabilities []string `bigquery:"capabilities"`
	}
	err = it.Next(&row)
	if err != nil {
		log.WithError(err).Error("error retrieving test capabilities from bigquery")
		return []string{}, errors.Wrap(err, "error retrieving test capabilities from bigquery")
	}

	return row.Capabilities, nil
}

// GetTestLifecyclesFromDB returns a sorted list of lifecycles from the BQ junit table
func GetTestLifecyclesFromDB(ctx context.Context, bqClient *bq.Client) ([]string, error) {
	if bqClient == nil || bqClient.BQ == nil {
		return []string{}, nil
	}

	// Query recent data (last 7 days) to satisfy partition filter requirement on modified_time
	qFmt := `SELECT ARRAY_AGG(DISTINCT lifecycle ORDER BY lifecycle) AS lifecycles
		FROM %s.junit
		WHERE modified_time >= DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 7 DAY)
		AND lifecycle IS NOT NULL AND lifecycle != ''`
	q := bqClient.Query(ctx, bqlabel.TestLifecycles, fmt.Sprintf(qFmt, bqClient.Dataset))

	log.Infof("Fetching test lifecycles with:\n%s\n", q.Q)

	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying test lifecycles from bigquery")
		return []string{}, err
	}

	var row struct {
		Lifecycles []string `bigquery:"lifecycles"`
	}
	err = it.Next(&row)
	if err != nil {
		log.WithError(err).Error("error retrieving test lifecycles from bigquery")
		return []string{}, errors.Wrap(err, "error retrieving test lifecycles from bigquery")
	}

	return row.Lifecycles, nil
}
