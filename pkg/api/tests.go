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

func GetTestOutputsFromBigQuery(ctx context.Context, bigQueryClient *bq.Client, storageBucket, testID string, prowJobRunIDs []string, startDate, endDate time.Time) ([]apitype.TestOutput, error) {
	// Use component_mapping to resolve test_id to test_name/testsuite, which handles test renames.
	// The test_id in junit may be stale (from before a rename), but component_mapping.id is canonical.
	// We join on name/suite to find all junit rows for this test, regardless of when they were created.
	queryStr := `WITH test_mapping AS (
  SELECT name, suite
  FROM ` + "`openshift-gce-devel.ci_analysis_us.component_mapping_latest`" + `
  WHERE id = @testID
)
SELECT junit.prowjob_build_id, junit.test_name, junit.success, junit.test_id, junit.branch, junit.prowjob_name, junit.failure_content
FROM ` + "`openshift-gce-devel.ci_analysis_us.junit`" + ` AS junit
INNER JOIN test_mapping ON junit.test_name = test_mapping.name AND junit.testsuite = test_mapping.suite
WHERE junit.success = false
  AND junit.prowjob_build_id IN UNNEST(@prowJobRunIDs)
  AND junit.modified_time BETWEEN DATETIME(@startDate) AND DATETIME(@endDate)
LIMIT 1000`

	q := bigQueryClient.BQ.Query(queryStr)
	q.Parameters = []bigquery.QueryParameter{
		{
			Name:  "testID",
			Value: testID,
		},
		{
			Name:  "prowJobRunIDs",
			Value: prowJobRunIDs,
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

	// Log the query with parameters substituted for easy copy-paste
	bq.LogQueryWithParamsReplaced(log.WithField("type", "TestOutputs"), q)

	it, err := bq.LoggedRead(ctx, q)
	if err != nil {
		log.WithError(err).Error("error querying test outputs from bigquery")
		return nil, fmt.Errorf("error querying test outputs from bigquery: %w", err)
	}

	type testOutputRow struct {
		ProwJobBuildID string `bigquery:"prowjob_build_id"`
		TestName       string `bigquery:"test_name"`
		Success        bool   `bigquery:"success"`
		TestID         string `bigquery:"test_id"`
		Branch         string `bigquery:"branch"`
		ProwJobName    string `bigquery:"prowjob_name"`
		FailureContent string `bigquery:"failure_content"`
	}

	var outputs []apitype.TestOutput
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

		// Construct the URL to the test output
		url := fmt.Sprintf("https://prow.ci.openshift.org/view/gs/%s/logs/%s/%s", storageBucket, row.ProwJobName, row.ProwJobBuildID)

		outputs = append(outputs, apitype.TestOutput{
			URL:      url,
			Output:   row.FailureContent,
			TestName: row.TestName,
		})
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

	testsResult, overall, err := BuildTestsResultsFromBigQuery(bqc, release, period, collapse, includeOverall, fil)
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
		rawQuery = rawQuery.Select(`name,jira_component,jira_component_id,` + query.QueryTestSummer).Group("name,jira_component,jira_component_id")
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
		Select(`ROW_NUMBER() OVER() as id, name, jira_component, jira_component_id,` + variantSelect + query.QueryTestSummarizer).
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

func BuildTestsResultsFromBigQuery(bqc *bq.Client, release, period string, collapse, includeOverall bool, fil *filter.Filter) (testsAPIResultBQ, *apitype.TestBQ, error) { //lint:ignore
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
			name,
			jira_component,
			jira_component_id,
			release,
			%s
		FROM %s.%s junit
		%s
		GROUP BY name, jira_component, jira_component_id, release
	),
	candidate_query AS (
		SELECT
			ROW_NUMBER() OVER() as id,
			name,
			jira_component,
			jira_component_id,
			%s
		FROM group_stats
	)
	`, query.QueryTestSummer, bqc.Dataset, table, whereStr, query.QueryTestSummarizer)
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
		JOIN test_stats as stats ON stats.test_id = junit.test_id AND stats.stats_suite_name IS NOT DISTINCT FROM junit.testsuite),
	candidate_query AS (
		SELECT
			*
		FROM
			unfiltered_candidate_query
		%s
	)`, bqc.Dataset, table, query.QueryTestSummarizer, bqc.Dataset, table, whereStr)
	}

	queryStr := fmt.Sprintf(`%s
		SELECT *
		FROM candidate_query`, candidateQueryStr)

	q := bqc.BQ.Query(queryStr)
	q.Parameters = queryParams
	testReports, errs := FetchTestResultsFromBQ(context.Background(), q)
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
		q := bqc.BQ.Query(queryStr)
		q.Parameters = queryParams // Reuse the same parameters for the overall query

		overallReports, errs := FetchTestResultsFromBQ(context.Background(), q)
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
func GetTestCapabilitiesFromDB(bqClient *bq.Client) ([]string, error) {
	if bqClient == nil || bqClient.BQ == nil {
		return []string{}, nil
	}

	qFmt := "SELECT ARRAY_AGG(DISTINCT capability ORDER BY capability) AS capabilities FROM `%s.component_mapping_latest`, UNNEST(capabilities) AS capability"
	q := bqClient.BQ.Query(fmt.Sprintf(qFmt, bqClient.Dataset))

	log.Infof("Fetching test capabilities with:\n%s\n", q.Q)

	it, err := q.Read(context.Background())
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
func GetTestLifecyclesFromDB(bqClient *bq.Client) ([]string, error) {
	if bqClient == nil || bqClient.BQ == nil {
		return []string{}, nil
	}

	// Query recent data (last 7 days) to satisfy partition filter requirement on modified_time
	qFmt := `SELECT ARRAY_AGG(DISTINCT lifecycle ORDER BY lifecycle) AS lifecycles
		FROM %s.junit
		WHERE modified_time >= DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 7 DAY)
		AND lifecycle IS NOT NULL AND lifecycle != ''`
	q := bqClient.BQ.Query(fmt.Sprintf(qFmt, bqClient.Dataset))

	log.Infof("Fetching test lifecycles with:\n%s\n", q.Q)

	it, err := q.Read(context.Background())
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
