package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	gosort "sort"
	"strconv"
	"strings"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/html/installhtml"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
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

func BuildTestsResults(dbc *db.DB, release, period string, fil *filter.Filter) (testsAPIResult, error) {
	now := time.Now()

	var testReports []apitype.Test
	// This query takes the variants out of the picture and adds in percentages
	q := `
WITH results AS (
    SELECT name,
           sum(current_runs)       AS current_runs,
           sum(current_successes)  AS current_successes,
           sum(current_failures)   AS current_failures,
           sum(current_flakes)     AS current_flakes,
           sum(previous_runs)      AS previous_runs,
           sum(previous_successes) AS previous_successes,
           sum(previous_failures)  AS previous_failures,
           sum(previous_flakes)    AS previous_flakes
    FROM prow_test_report_7d_matview
	WHERE release = @release
    GROUP BY name
)
SELECT *,
       current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       current_failures * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
       previous_successes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
       (current_successes * 100.0 / NULLIF(current_runs, 0)) - (previous_successes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
FROM results;
`
	if period == "twoDay" {
		q = strings.ReplaceAll(q, testReport7dMatView, testReport2dMatView)
	}

	r := dbc.DB.Raw(q,
		sql.Named("release", release)).Scan(&testReports)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test reports")
		return []apitype.Test{}, r.Error
	}

	// Apply filtering to what we pulled from the db. Perfect world we'd incorporate this into the query instead.
	filteredReports := make([]apitype.Test, 0, len(testReports))
	fakeIDCtr := 1
	for _, testReport := range testReports {
		if fil != nil {
			include, err := fil.Filter(testReport)
			if err != nil {
				return []apitype.Test{}, err
			}

			if !include {
				continue
			}
		}

		// Need fake IDs for the javscript tables:
		testReport.ID = fakeIDCtr
		fakeIDCtr++

		// TODO: do we need bugs linked here?
		testReport.Bugs = []v1.Bug{}
		testReport.AssociatedBugs = []v1.Bug{}

		filteredReports = append(filteredReports, testReport)
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{
		"elapsed":         elapsed,
		"reports":         len(testReports),
		"filteredReports": len(filteredReports),
	}).Info("BuildTestsResults completed")

	return filteredReports, nil
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
