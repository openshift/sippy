package api

import (
	"net/http"
	"regexp"
	gosort "sort"
	"strconv"
	"strings"

	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/installhtml"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util"
)

func PrintTestsDetailsJSON(w http.ResponseWriter, req *http.Request, current, previous v1sippyprocessing.TestReport) {
	RespondWithJSON(http.StatusOK, w, installhtml.TestDetailTests(installhtml.JSON, current, previous, req.URL.Query()["test"]))
}

func testFilter(req *http.Request, release string) []func(result v1sippyprocessing.FailingTestResult) bool {
	filterBy := req.URL.Query()["filterBy"]
	runs, _ := strconv.Atoi(req.URL.Query().Get("runs"))
	names := req.URL.Query()["test"]

	var filter []func(result v1sippyprocessing.FailingTestResult) bool
	for _, filterName := range filterBy {
		switch filterName {
		case "name":
			filter = append(filter, func(test v1sippyprocessing.FailingTestResult) bool {
				regex := regexp.QuoteMeta(strings.Join(names, "|"))
				match, err := regexp.Match(regex, []byte(test.TestName))
				if err != nil {
					return false
				}
				return match
			})
		case "install":
			filter = append(filter, func(test v1sippyprocessing.FailingTestResult) bool {
				return testidentification.IsInstallRelatedTest(test.TestName)
			})
		case "upgrade":
			filter = append(filter, func(test v1sippyprocessing.FailingTestResult) bool {
				return testidentification.IsUpgradeRelatedTest(test.TestName)
			})
		case "runs":
			filter = append(filter, func(test v1sippyprocessing.FailingTestResult) bool {
				return (test.TestResultAcrossAllJobs.Failures + test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Flakes) > runs
			})
		case "trt":
			filter = append(filter, func(test v1sippyprocessing.FailingTestResult) bool {
				return testidentification.IsCuratedTest(release, test.TestName)
			})
		case "hasBug":
			filter = append(filter, func(test v1sippyprocessing.FailingTestResult) bool {
				return len(test.TestResultAcrossAllJobs.BugList) > 0
			})
		case "noBug":
			filter = append(filter, func(test v1sippyprocessing.FailingTestResult) bool {
				return len(test.TestResultAcrossAllJobs.BugList) == 0
			})
		}
	}
	return filter
}

type testsAPIResult []v1.Test

func (tests testsAPIResult) sort(req *http.Request) testsAPIResult {
	sortBy := req.URL.Query().Get("sortBy")

	switch sortBy {
	case "regression":
		gosort.Slice(tests, func(i, j int) bool {
			return tests[i].NetImprovement < tests[j].NetImprovement
		})
	case "improvement":
		gosort.Slice(tests, func(i, j int) bool {
			return tests[i].NetImprovement > tests[j].NetImprovement
		})
	}

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
	filters := testFilter(req, release)

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

buildTests:
	for idx, test := range current {

		for _, filter := range filters {
			if !filter(test) {
				continue buildTests
			}
		}

		testPrev := util.FindFailedTestResult(test.TestName, previous)

		row := v1.Test{
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

		tests = append(tests, row)
	}

	RespondWithJSON(http.StatusOK, w, tests.
		sort(req).
		limit(req))
}
