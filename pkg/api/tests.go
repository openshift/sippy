package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	gosort "sort"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/html/installhtml"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util"
)

const (
	testReport7dMatView = "prow_test_report_7d_matview"
	testReport2dMatView = "prow_test_report_2d_matview"
)

func PrintTestsDetailsJSON(w http.ResponseWriter, req *http.Request, current, previous v1sippyprocessing.TestReport) {
	RespondWithJSON(http.StatusOK, w, installhtml.TestDetailTests(installhtml.JSON, current, previous, req.URL.Query()["test"]))
}

func PrintTestsDetailsJSONFromDB(w http.ResponseWriter, release string, testSubstrings []string, dbc *db.DB) {
	responseStr, err := installhtml.TestDetailTestsFromDB(dbc, installhtml.JSON, release, testSubstrings)
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

// PrintTestsJSON renders the list of matching tests.
func PrintTestsJSON(release string, w http.ResponseWriter, req *http.Request, currentPeriod, twoDayPeriod, previousPeriod []v1sippyprocessing.FailingTestResult) {
	tests := testsAPIResult{}
	var fil *filter.Filter

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
	var current, previous []v1sippyprocessing.FailingTestResult
	switch req.URL.Query().Get("period") {
	case periodTwoDay:
		current = twoDayPeriod
		previous = currentPeriod
	default:
		current = currentPeriod
		previous = previousPeriod
	}

	for idx, test := range current {
		testPrev := util.FindFailedTestResult(test.TestName, previous)

		row := apitype.Test{
			ID:                    idx + 1,
			Name:                  test.TestName,
			CurrentSuccesses:      test.TestResultAcrossAllJobs.Successes,
			CurrentFailures:       test.TestResultAcrossAllJobs.Failures,
			CurrentFlakes:         test.TestResultAcrossAllJobs.Flakes,
			CurrentPassPercentage: test.TestResultAcrossAllJobs.PassPercentage,
			CurrentRuns:           test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures + test.TestResultAcrossAllJobs.Flakes,
			Bugs:                  []v1.Bug{},
			AssociatedBugs:        []v1.Bug{},
		}

		if testPrev != nil {
			row.PreviousSuccesses = testPrev.TestResultAcrossAllJobs.Successes
			row.PreviousFlakes = testPrev.TestResultAcrossAllJobs.Flakes
			row.PreviousFailures = testPrev.TestResultAcrossAllJobs.Failures
			row.PreviousPassPercentage = testPrev.TestResultAcrossAllJobs.PassPercentage
			row.PreviousRuns = testPrev.TestResultAcrossAllJobs.Successes + testPrev.TestResultAcrossAllJobs.Failures + testPrev.TestResultAcrossAllJobs.Flakes
			row.NetImprovement = row.CurrentPassPercentage - row.PreviousPassPercentage
		}

		if test.TestResultAcrossAllJobs.BugList != nil {
			row.Bugs = test.TestResultAcrossAllJobs.BugList
		}
		if test.TestResultAcrossAllJobs.AssociatedBugList != nil {
			row.AssociatedBugs = test.TestResultAcrossAllJobs.AssociatedBugList
		}

		if testidentification.IsCuratedTest(release, row.Name) {
			row.Tags = append(row.Tags, "trt")
		}

		if testidentification.IsInstallRelatedTest(row.Name) {
			row.Tags = append(row.Tags, "install")
		}

		if testidentification.IsUpgradeRelatedTest(row.Name) {
			row.Tags = append(row.Tags, "upgrade")
		}

		if fil != nil {
			include, err := fil.Filter(row)
			if err != nil {
				RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Filter error:" + err.Error()})
				return
			}

			if !include {
				continue
			}
		}

		tests = append(tests, row)
	}

	RespondWithJSON(http.StatusOK, w, tests.
		sort(req).
		limit(req))
}

func PrintTestsJSONFromDB(release string, w http.ResponseWriter, req *http.Request, dbc *db.DB) {
	var fil *filter.Filter

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

	testsResult, err := BuildTestsResults(dbc, release, period, fil)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job report:" + err.Error()})
		return
	}

	RespondWithJSON(http.StatusOK, w, testsResult.
		sort(req).
		limit(req))
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

	results, err := BuildTestsResults(dbc, release, "default", &f)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building test report:" + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	for _, result := range results {
		fmt.Fprintf(w, "%q:struct{}{},\n", result.Name)
	}
}

func BuildTestsResults(dbc *db.DB, release, period string, fil *filter.Filter) (testsAPIResult, error) {
	now := time.Now()

	// Test results are generated by using two subqueries, which need to be filtered separately. Once during
	// pre-processing where we're evaluating summed variant results, and in post-processing after we've
	// assembled our final temporary table.
	var rawFilter, processedFilter *filter.Filter
	if fil != nil {
		rawFilter, processedFilter = fil.Split([]string{"variants"})
	}

	table := testReport7dMatView
	if period == "twoDay" {
		table = testReport2dMatView
	}

	rawQuery := dbc.DB.
		Table(table).
		Where("release = ?", release).
		Select(`name,
           sum(current_runs)       AS current_runs,
           sum(current_successes)  AS current_successes,
           sum(current_failures)   AS current_failures,
           sum(current_flakes)     AS current_flakes,
           sum(previous_runs)      AS previous_runs,
           sum(previous_successes) AS previous_successes,
           sum(previous_failures)  AS previous_failures,
           sum(previous_flakes)    AS previous_flakes`).
		Group("name")

	if rawFilter != nil {
		rawQuery = rawFilter.ToSQL(rawQuery, apitype.Test{})
	}

	testReports := make([]apitype.Test, 0)
	// FIXME: Add test id to matview, for now generate with ROW_NUMBER OVER
	processedResults := dbc.DB.Table("(?) as results", rawQuery).Select(
		`ROW_NUMBER() OVER() as id,
				name,
				current_runs,
				current_successes,
				current_failures,
				current_flakes,
				previous_runs,
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

			    (previous_failures * 100.0 / NULLIF(previous_runs, 0)) - (current_failures * 100.0 / NULLIF(current_runs, 0)) AS net_failure_improvement,
				(previous_flakes * 100.0 / NULLIF(previous_runs, 0)) - (current_flakes * 100.0 / NULLIF(current_runs, 0)) AS net_flake_improvement,
				((current_successes + current_flakes) * 100.0 / NULLIF(current_runs, 0)) - ((previous_successes + previous_flakes) * 100.0 / NULLIF(previous_runs, 0)) AS net_working_improvement,
				(current_successes * 100.0 / NULLIF(current_runs, 0)) - (previous_successes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement`)

	finalResults := dbc.DB.Table("(?) as final_results", processedResults)
	if processedFilter != nil {
		finalResults = processedFilter.ToSQL(finalResults, apitype.Test{})
	}

	finalResults = finalResults.Scan(&testReports)
	if finalResults.Error != nil {
		log.WithError(finalResults.Error).Error("error querying test reports")
		return []apitype.Test{}, finalResults.Error
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{
		"elapsed": elapsed,
		"reports": len(testReports),
	}).Info("BuildTestsResults completed")

	return testReports, nil
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
