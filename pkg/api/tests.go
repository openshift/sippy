package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	gosort "sort"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/html/installhtml"
)

const (
	testReport7dMatView          = "prow_test_report_7d_matview"
	testReport2dMatView          = "prow_test_report_2d_matview"
	payloadFailedTests14dMatView = "payload_test_failures_14d_matview"

	queryTestSummer = `
           sum(current_runs)       AS current_runs,
           sum(current_successes)  AS current_successes,
           sum(current_failures)   AS current_failures,
           sum(current_flakes)     AS current_flakes,
           sum(previous_runs)      AS previous_runs,
           sum(previous_successes) AS previous_successes,
           sum(previous_failures)  AS previous_failures,
           sum(previous_flakes)    AS previous_flakes`

	queryTestSummarizer = `current_runs,
		current_successes,
		current_failures,
		current_flakes,
		previous_runs,
		previous_successes,
		previous_failures,
		previous_flakes,
		current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
		current_failures * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
		current_flakes * 100.0 / NULLIF(current_runs, 0) AS current_flake_percentage,
		(current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0) AS current_working_percentage,

		previous_successes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
		previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
		previous_flakes * 100.0 / NULLIF(previous_runs, 0) AS previous_flake_percentage,
		(previous_successes + previous_flakes) * 100.0 / NULLIF(previous_runs, 0) AS previous_working_percentage,

		(current_failures * 100.0 / NULLIF(current_runs, 0) - (previous_failures * 100.0 / NULLIF(previous_runs, 0))) AS net_failure_improvement,
		(current_flakes * 100.0 / NULLIF(current_runs, 0) - (previous_flakes * 100.0 / NULLIF(previous_runs, 0))) AS net_flake_improvement,
		((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0)) - ((previous_successes + previous_flakes) * 100.0 / NULLIF(previous_runs, 0)) AS net_working_improvement,
		(current_successes * 100.0 / NULLIF(current_runs, 0)) - (previous_successes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement`
)

func PrintTestsDetailsJSONFromDB(w http.ResponseWriter, release string, testSubstrings []string, dbc *db.DB) {
	responseStr, err := installhtml.TestDetailTestsFromDB(dbc, release, testSubstrings)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": err.Error()})
		return
	}
	RespondWithJSON(http.StatusOK, w, responseStr)
}

type testsAPIResult []apitype.Test

func (tests testsAPIResult) sort(req *http.Request) testsAPIResult {
	sortField := req.URL.Query().Get("sortField")
	sort := req.URL.Query().Get("sort")

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

	testsResult, overall, err := BuildTestsResults(dbc, release, period, collapse, fil)
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

	results, _, err := BuildTestsResults(dbc, release, "default", true, &f)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building test report:" + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	for _, result := range results {
		fmt.Fprintf(w, "%q:struct{}{},\n", result.Name)
	}
}

func BuildTestsResults(dbc *db.DB, release, period string, collapse bool, fil *filter.Filter) (testsAPIResult, *apitype.Test, error) { //lint:ignore
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
		rawQuery = rawQuery.Select(`name,` + queryTestSummer).Group("name")
	} else {
		variantSelect = "suite_name,variants, "
	}

	if rawFilter != nil {
		rawQuery = rawFilter.ToSQL(rawQuery, apitype.Test{})
	}

	testReports := make([]apitype.Test, 0)
	// FIXME: Add test id to matview, for now generate with ROW_NUMBER OVER
	processedResults := dbc.DB.Table("(?) as results", rawQuery).
		Select(`ROW_NUMBER() OVER() as id, name,` + variantSelect + queryTestSummarizer)

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
	if !collapse {
		finalResults := dbc.DB.Table("(?) as final_results", finalResults)
		finalResults = finalResults.Select(queryTestSummer)
		summaryResult := dbc.DB.Table("(?) as overall", finalResults).Select(queryTestSummarizer)
		overallTest = &apitype.Test{
			ID:   math.MaxInt32,
			Name: "Overall",
		}
		summaryResult.Scan(overallTest)
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{
		"elapsed": elapsed,
		"reports": len(testReports),
	}).Info("BuildTestsResults completed")

	return testReports, overallTest, nil
}

type testDetail struct {
	Name    string                         `json:"name"`
	Results []v1sippyprocessing.TestResult `json:"results"`
}

type testsDetailAPIResult struct {
	Tests []testDetail `json:"tests"`
	Start int          `json:"start"`
	End   int          `json:"end"`
}

func (tests testsDetailAPIResult) limit(req *http.Request) testsDetailAPIResult {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(tests.Tests) >= limit {
		tests.Tests = tests.Tests[:limit]
	}

	return tests
}
