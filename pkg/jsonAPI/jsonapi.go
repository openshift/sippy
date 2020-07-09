package jsonAPI

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/openshift/sippy/pkg/html"
	"github.com/openshift/sippy/pkg/util"
)

// NOTE: these functions are mirrored in html.go
// Copied over here as a quick fix
func getPrevTest(test string, testResults []util.TestResult) *util.TestResult {
	for _, v := range testResults {
		if v.Name == test {
			return &v
		}
	}
	return nil
}

func getPrevJob(job string, jobRunsByJob []util.JobResult) *util.JobResult {
	for _, v := range jobRunsByJob {
		if v.Name == job {
			return &v
		}
	}
	return nil
}

func getPrevPlatform(platform string, jobsByPlatform []util.JobResult) *util.JobResult {
	for _, v := range jobsByPlatform {
		if v.Platform == platform {
			return &v
		}
	}
	return nil
}

// PassRate describes statistics on a pass rate
type PassRate struct {
	Percentage          float64 `json:"percentage"`
	ProjectedPercentage float64 `json:"projectedPercentage,omitempty"`
	Runs                int     `json:"runs"`
}

// SummaryAcrossAllJobs describes the category failuregroups
// valid keys are latestXDays, and prevXDays
type SummaryAcrossAllJobs struct {
	TestExecutions     map[string]int     `json:"testExecutions"`
	TestPassPercentage map[string]float64 `json:"testPassPercentage"`
}

// FailureGroups describes the category failuregroups
// valid keys are latestXDays, and prevXDays
type FailureGroups struct {
	JobRunsWithFailureGroup map[string]int `json:"jobRunsWithFailureGroup"`
	AvgFailureGroupSize     map[string]int `json:"avgFailureGroupSize"`
	MedianFailureGroupSize  map[string]int `json:"medianFailureGroupSize"`
}

// CanaryTestFailInstance describes one single instance of a canary test failure
// passRate should have percentage (float64) and number of runs (int)
type CanaryTestFailInstance struct {
	Name     string   `json:"name"`
	Url      string   `json:"url"`
	PassRate PassRate `json:"passRate"`
}

// PassRatesByJobName is responsible for the section job pass rates by job name
type PassRatesByJobName struct {
	Name         string              `json:"name"`
	Url          string              `json:"url"`
	PassRates    map[string]PassRate `json:"passRates"`
	FailingTests []FailingTest       `json:"failingTests"`
}

// FailingTest describes one single instance of a failed test and associated bugs
// passRate may include projected pass rate (float64), percentage (float64), and number of runs (int)
type FailingTest struct {
	Name     string     `json:"name"`
	Url      string     `json:"url"`
	PassRate PassRate   `json:"passRate"`
	Bugs     []util.Bug `json:"bugs"`
}

// FailingTestBug describes a single instance of failed test with bug or failed test without bug
// differs from failingtest in that it includes pass rates for previous days and latest days
type FailingTestBug struct {
	Name      string              `json:"name"`
	Url       string              `json:"url"`
	PassRates map[string]PassRate `json:"passRates"`
	Bugs      []util.Bug          `json:"bugs,omitempty"`
}

// JobSummaryPlatformdescribes a single platform and its associated jobs, their pass rates, and failing tests
type JobSummaryPlatform struct {
	Platform     string              `json:"platform"`
	PassRates    map[string]PassRate `json:"passRates"`
	FailingTests []FailingTest       `json:"failingTests"`
}

// FailureGroup describes a single failure group - does not show the associated failed job names
type FailureGroup struct {
	Job          string `json:"job"`
	Url          string `json:"url"`
	TestFailures int    `json:"testFailures"`
}

// summary across all job
func jsonSummaryAcrossAllJobs(result, resultPrev map[string]util.SortedAggregateTestResult, endDay int) *SummaryAcrossAllJobs {
	all := result["all"]
	allPrev := resultPrev["all"]

	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"

	summary := SummaryAcrossAllJobs{
		TestExecutions: map[string]int{
			latestDays: all.Successes + all.Failures,
			prevDays:   allPrev.Successes + allPrev.Failures,
		},
		TestPassPercentage: map[string]float64{
			latestDays: all.TestPassPercentage,
			prevDays:   allPrev.TestPassPercentage,
		},
	}

	return &summary
}

// stats on failure groups
func jsonFailureGroups(failureGroups, failureGroupsPrev []util.JobRunResult, endDay int) *FailureGroups {
	count, countPrev, median, medianPrev, avg, avgPrev := 0, 0, 0, 0, 0, 0
	for _, group := range failureGroups {
		count += group.TestFailures
	}
	for _, group := range failureGroupsPrev {
		countPrev += group.TestFailures
	}
	if len(failureGroups) != 0 {
		median = failureGroups[len(failureGroups)/2].TestFailures
		avg = count / len(failureGroups)
	}
	if len(failureGroupsPrev) != 0 {
		medianPrev = failureGroupsPrev[len(failureGroupsPrev)/2].TestFailures
		avgPrev = count / len(failureGroupsPrev)
	}

	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"

	failureGroupStruct := FailureGroups{
		JobRunsWithFailureGroup: map[string]int{
			latestDays: len(failureGroups),
			prevDays:   len(failureGroupsPrev),
		},
		AvgFailureGroupSize: map[string]int{
			latestDays: avg,
			prevDays:   avgPrev,
		},
		MedianFailureGroupSize: map[string]int{
			latestDays: median,
			prevDays:   medianPrev,
		},
	}
	return &failureGroupStruct
}

func jsonSummaryJobsByPlatform(report, reportPrev util.TestReport, endDay, jobTestCount int) []JobSummaryPlatform {
	jobsByPlatform := util.SummarizeJobsByPlatform(report)
	jobsByPlatformPrev := util.SummarizeJobsByPlatform(reportPrev)

	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"

	var jobSummariesByPlatform []JobSummaryPlatform

	for _, v := range jobsByPlatform {
		prev := getPrevPlatform(v.Platform, jobsByPlatformPrev)

		var jobSummaryPlatform JobSummaryPlatform

		if prev != nil {
			jobSummaryPlatform = JobSummaryPlatform{
				Platform: v.Platform,
				PassRates: map[string]PassRate{
					latestDays: PassRate{
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithKnownFailures,
						Runs:                v.Successes + v.Failures,
					},
					prevDays: PassRate{
						Percentage:          prev.PassPercentage,
						ProjectedPercentage: prev.PassPercentageWithKnownFailures,
						Runs:                prev.Successes + prev.Failures,
					},
				},
			}
		} else {
			jobSummaryPlatform = JobSummaryPlatform{
				Platform: v.Platform,
				PassRates: map[string]PassRate{
					latestDays: PassRate{
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithKnownFailures,
						Runs:                v.Successes + v.Failures,
					},
				},
			}
		}

		platformTests := report.ByPlatform[v.Platform]
		for _, test := range platformTests.TestResults {
			if util.IgnoreTestRegex.MatchString(test.Name) {
				continue
			}

			encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
			// NOTE: not really sure what this represents
			// jobQuery := fmt.Sprintf("%s.*%s|%s.*%s", report.Release, v.Platform, v.Platform, report.Release)

			bugList := util.TestBugCache[test.Name]

			testLink := fmt.Sprintf("https://search.svc.ci.openshift.org/?context=1&type=bug&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s", encodedTestName)

			failingTest := FailingTest{
				Name: test.Name,
				Url:  testLink,
				PassRate: PassRate{
					Percentage: test.PassPercentage,
					Runs:       test.Successes + test.Failures,
				},
				Bugs: bugList,
			}

			jobSummaryPlatform.FailingTests = append(jobSummaryPlatform.FailingTests, failingTest)
		}

		jobSummariesByPlatform = append(jobSummariesByPlatform, jobSummaryPlatform)
	}
	return jobSummariesByPlatform
}

// top failing tests with a bug
func jsonSummaryTopFailingTestsWithBug(topFailingTestsWithBug []*util.TestResult, resultPrev map[string]util.SortedAggregateTestResult, endDay int) []FailingTestBug {

	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"
	var topFailingTests []FailingTestBug

	allPrev := resultPrev["all"]

	for _, test := range topFailingTestsWithBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("https://search.svc.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s", encodedTestName)
		testPrev := getPrevTest(test.Name, allPrev.TestResults)

		var failedTestWithBug FailingTestBug

		if testPrev != nil {
			failedTestWithBug = FailingTestBug{
				Name: test.Name,
				Url:  testLink,
				PassRates: map[string]PassRate{
					latestDays: PassRate{
						Percentage: test.PassPercentage,
						Runs:       test.Successes + test.Failures,
					},
					prevDays: PassRate{
						Percentage: testPrev.PassPercentage,
						Runs:       testPrev.Successes + test.Failures,
					},
				},
				Bugs: test.BugList,
			}
		} else {
			failedTestWithBug = FailingTestBug{
				Name: test.Name,
				Url:  testLink,
				PassRates: map[string]PassRate{
					latestDays: PassRate{
						Percentage: test.PassPercentage,
						Runs:       test.Successes + test.Failures,
					},
				},
				Bugs: test.BugList,
			}
		}

		topFailingTests = append(topFailingTests, failedTestWithBug)
	}

	return topFailingTests

}

// top failing tests without a bug
func jsonSummaryTopFailingTestsWithoutBug(topFailingTestsWithoutBug []*util.TestResult, resultPrev map[string]util.SortedAggregateTestResult, endDay int) []FailingTestBug {

	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"

	allPrev := resultPrev["all"]

	var topFailingTests []FailingTestBug

	for _, test := range topFailingTestsWithoutBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("https://search.svc.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s", encodedTestName)
		testPrev := getPrevTest(test.Name, allPrev.TestResults)

		var failedTestWithoutBug FailingTestBug

		if testPrev != nil {

			failedTestWithoutBug = FailingTestBug{
				Name: test.Name,
				Url:  testLink,
				PassRates: map[string]PassRate{
					latestDays: PassRate{
						Percentage: test.PassPercentage,
						Runs:       test.Successes + test.Failures,
					},
					prevDays: PassRate{
						Percentage: testPrev.PassPercentage,
						Runs:       testPrev.Successes + testPrev.Failures,
					},
				},
			}

		} else {
			failedTestWithoutBug = FailingTestBug{
				Name: test.Name,
				Url:  testLink,
				PassRates: map[string]PassRate{
					latestDays: PassRate{
						Percentage: test.PassPercentage,
						Runs:       test.Successes + test.Failures,
					},
				},
			}
		}

		topFailingTests = append(topFailingTests, failedTestWithoutBug)
	}
	return topFailingTests
}

func jsonSummaryJobPassRatesByJobName(report, reportPrev util.TestReport, endDay, jobTestCount int) []PassRatesByJobName {
	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"
	jobRunsByName := util.SummarizeJobsByName(report)
	jobRunsByNamePrev := util.SummarizeJobsByName(reportPrev)

	var passRatesSlice []PassRatesByJobName

	for _, v := range jobRunsByName {
		prev := getPrevJob(v.Name, jobRunsByNamePrev)

		var newJobPassRate PassRatesByJobName

		if prev != nil {
			newJobPassRate = PassRatesByJobName{
				Name: v.Name,
				Url:  v.TestGridUrl,
				PassRates: map[string]PassRate{
					latestDays: PassRate{
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithKnownFailures,
						Runs:                v.Successes + v.Failures,
					},
					prevDays: PassRate{
						Percentage:          prev.PassPercentage,
						ProjectedPercentage: prev.PassPercentageWithKnownFailures,
						Runs:                prev.Successes + prev.Failures,
					},
				},
			}
		} else {
			newJobPassRate = PassRatesByJobName{
				Name: v.Name,
				Url:  v.TestGridUrl,
				PassRates: map[string]PassRate{
					latestDays: PassRate{
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithKnownFailures,
						Runs:                v.Successes + v.Failures,
					},
				},
			}
		}

		// FIXME: not sure if I need this
		count := jobTestCount
		// deleted additionalCount since we want the API to return everything
		jobTests := report.ByJob[v.Name]
		for _, test := range jobTests.TestResults {
			if util.IgnoreTestRegex.MatchString(test.Name) {
				continue
			}
			if count == 0 {
				continue
			}
			count--

			encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

			bugList := util.TestBugCache[test.Name]

			failingTest := FailingTest{
				Name: test.Name,
				Url:  fmt.Sprintf("https://search.svc.ci.openshift.org/?context=1&type=bug&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s", encodedTestName),
				PassRate: PassRate{
					Percentage: test.PassPercentage,
					Runs:       test.Successes + test.Failures,
				},
				Bugs: bugList,
			}

			newJobPassRate.FailingTests = append(newJobPassRate.FailingTests, failingTest)
		}

		// NOTE - not sure if this needs to be declared as a pointer so future modifications are made by reference
		passRatesSlice = append(passRatesSlice, newJobPassRate)

	}

	return passRatesSlice

}

// canaryTestFailures section
func jsonCanaryTestFailures(result map[string]util.SortedAggregateTestResult) []CanaryTestFailInstance {
	all := result["all"].TestResults

	var canaryFailures []CanaryTestFailInstance

	for i := len(all) - 1; i > len(all)-10; i-- {
		test := all[i]
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
		canaryFailures = append(canaryFailures,
			CanaryTestFailInstance{
				Name: test.Name,
				Url:  fmt.Sprintf("https://search.svc.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s", encodedTestName),
				PassRate: PassRate{
					Percentage: test.PassPercentage,
					Runs:       test.Successes + test.Failures,
				},
			})
	}
	return canaryFailures
}

// job runs with failure groups
func jsonFailureGroupList(report util.TestReport) []FailureGroup {

	var failureGroups []FailureGroup
	for _, fg := range report.FailureGroups {
		failureGroups = append(failureGroups, FailureGroup{
			Job:          fg.Job,
			Url:          fg.Url,
			TestFailures: fg.TestFailures,
		})
	}
	return failureGroups
}

func jsonTestImpactingBugs(testImpactingBugs []util.Bug) []util.Bug {
	return testImpactingBugs
}

// PrintJSONReport prints the json format of the report
// follows conventions from jsonapi.org
func PrintJSONReport(w http.ResponseWriter, req *http.Request, report, prevReport util.TestReport, endDay, jobTestCount int) {

	data := html.TestReports{report, prevReport, endDay, jobTestCount}

	jsonObject := map[string]interface{}{
		"releaseHealthData": map[string]interface{}{
			"summaryAllJobs":            jsonSummaryAcrossAllJobs(data.Current.All, data.Prev.All, data.EndDay),
			"failureGroupings":          jsonFailureGroups(data.Current.FailureGroups, data.Prev.FailureGroups, data.EndDay),
			"jobPassRateByPlatform":     jsonSummaryJobsByPlatform(data.Current, data.Prev, data.EndDay, data.JobTestCount),
			"topFailingTestsWithoutBug": jsonSummaryTopFailingTestsWithoutBug(data.Current.TopFailingTestsWithoutBug, data.Prev.All, data.EndDay),
			"topFailingTestsWithBug":    jsonSummaryTopFailingTestsWithBug(data.Current.TopFailingTestsWithBug, data.Prev.All, data.EndDay),
			"jobPassRatesByName":        jsonSummaryJobPassRatesByJobName(data.Current, data.Prev, data.EndDay, data.JobTestCount),
			"canaryTestFailures":        jsonCanaryTestFailures(data.Current.All),
			"jobRunsWithFailureGroups":  jsonFailureGroupList(data.Current),
			"testImpactingBugs":         jsonTestImpactingBugs(data.Current.BugsByFailureCount),
		},
	}

	enc := json.NewEncoder(w)
	err := enc.Encode(jsonObject)
	if err != nil {
		// klog.Errorf("unable to render json %v", err)
		fmt.Printf("unable to render json %v", err)
	}

}
