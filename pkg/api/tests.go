package api

import (
	"encoding/json"
	"net/http"
	gosort "sort"
	"strconv"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/installhtml"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util"
)

func PrintTestsDetailsJSON(w http.ResponseWriter, req *http.Request, current, previous v1sippyprocessing.TestReport) {
	RespondWithJSON(http.StatusOK, w, installhtml.TestDetailTests(installhtml.JSON, current, previous, req.URL.Query()["test"]))
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
			return compare(tests[i], tests[j], sortField)
		}
		return compare(tests[j], tests[i], sortField)
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
	var filter *Filter

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		filter = &Filter{}
		if err := json.Unmarshal([]byte(queryFilter), filter); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	// If requesting a two day report, we make the comparison between the last
	// period (typically 7 days) and the last two days.
	var current, previous []v1sippyprocessing.FailingTestResult
	switch req.URL.Query().Get("period") {
	case "twoDay":
		current = twoDayPeriod
		previous = currentPeriod
	default:
		current = currentPeriod
		previous = previousPeriod
	}

	for idx, test := range current {
		testPrev := util.FindFailedTestResult(test.TestName, previous)

		row := apitype.Test{
			ID:                    idx,
			Name:                  test.TestName,
			CurrentSuccesses:      test.TestResultAcrossAllJobs.Successes,
			CurrentFailures:       test.TestResultAcrossAllJobs.Failures,
			CurrentFlakes:         test.TestResultAcrossAllJobs.Flakes,
			CurrentPassPercentage: test.TestResultAcrossAllJobs.PassPercentage,
			CurrentRuns:           test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures + test.TestResultAcrossAllJobs.Flakes,
		}

		if testPrev != nil {
			row.PreviousSuccesses = testPrev.TestResultAcrossAllJobs.Successes
			row.PreviousFlakes = testPrev.TestResultAcrossAllJobs.Flakes
			row.PreviousFailures = testPrev.TestResultAcrossAllJobs.Failures
			row.PreviousPassPercentage = testPrev.TestResultAcrossAllJobs.PassPercentage
			row.PreviousRuns = testPrev.TestResultAcrossAllJobs.Successes + testPrev.TestResultAcrossAllJobs.Failures + testPrev.TestResultAcrossAllJobs.Flakes
			row.NetImprovement = row.CurrentPassPercentage - row.PreviousPassPercentage
		}

		row.Bugs = test.TestResultAcrossAllJobs.BugList
		row.AssociatedBugs = test.TestResultAcrossAllJobs.AssociatedBugList

		if testidentification.IsCuratedTest(release, row.Name) {
			row.Tags = append(row.Tags, "trt")
		}

		if testidentification.IsInstallRelatedTest(row.Name) {
			row.Tags = append(row.Tags, "install")
		}

		if testidentification.IsUpgradeRelatedTest(row.Name) {
			row.Tags = append(row.Tags, "upgrade")
		}

		if filter != nil {
			include, err := filter.Filter(row)
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
