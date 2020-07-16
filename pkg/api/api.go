package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/openshift/sippy/pkg/html"
	"github.com/openshift/sippy/pkg/util"
	"k8s.io/klog"
)

// PassRate describes statistics on a pass rate
type PassRate struct {
	Percentage          float64 `json:"percentage"`
	ProjectedPercentage float64 `json:"projectedPercentage,omitempty"`
	Runs                int     `json:"runs"`
}

// SummaryAcrossAllJobs describes the category summaryacrossalljobs
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
	Name      string              `json:"name"`
	Url       string              `json:"url"`
	PassRates map[string]PassRate `json:"passRates"`
}

// FailingTestBug describes a single instance of failed test with bug or failed test without bug
// differs from failingtest in that it includes pass rates for previous days and latest days
type FailingTestBug struct {
	Name      string              `json:"name"`
	Url       string              `json:"url"`
	PassRates map[string]PassRate `json:"passRates"`
	Bugs      []util.Bug          `json:"bugs,omitempty"`
}

// JobSummaryPlatform describes a single platform and its associated jobs, their pass rates, and failing tests
type JobSummaryPlatform struct {
	Platform  string              `json:"platform"`
	PassRates map[string]PassRate `json:"passRates"`
}

// FailureGroup describes a single failure group - does not show the associated failed job names
type FailureGroup struct {
	Job          string `json:"job"`
	Url          string `json:"url"`
	TestFailures int    `json:"testFailures"`
}

// summary across all job
func summaryAcrossAllJobs(result, resultPrev map[string]util.SortedAggregateTestResult, endDay int) *SummaryAcrossAllJobs {
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
func failureGroups(failureGroups, failureGroupsPrev []util.JobRunResult, endDay int) *FailureGroups {

	_, _, median, medianPrev, avg, avgPrev := util.ComputeFailureGroupStats(failureGroups, failureGroupsPrev)

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

func summaryJobsByPlatform(report, reportPrev util.TestReport, endDay, jobTestCount int) []JobSummaryPlatform {
	jobsByPlatform := util.SummarizeJobsByPlatform(report)
	jobsByPlatformPrev := util.SummarizeJobsByPlatform(reportPrev)

	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"

	var jobSummariesByPlatform []JobSummaryPlatform

	for _, v := range jobsByPlatform {
		prev := util.GetPrevPlatform(v.Platform, jobsByPlatformPrev)

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

		jobSummariesByPlatform = append(jobSummariesByPlatform, jobSummaryPlatform)
	}
	return jobSummariesByPlatform
}

// top failing tests with a bug
func summaryTopFailingTestsWithBug(topFailingTestsWithBug []*util.TestResult, resultPrev map[string]util.SortedAggregateTestResult, endDay int) []FailingTestBug {

	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"
	var topFailingTests []FailingTestBug

	allPrev := resultPrev["all"]

	for _, test := range topFailingTestsWithBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName)
		testPrev := util.GetPrevTest(test.Name, allPrev.TestResults)

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
func summaryTopFailingTestsWithoutBug(topFailingTestsWithoutBug []*util.TestResult, resultPrev map[string]util.SortedAggregateTestResult, endDay int) []FailingTestBug {

	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"

	allPrev := resultPrev["all"]

	var topFailingTests []FailingTestBug

	for _, test := range topFailingTestsWithoutBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName)
		testPrev := util.GetPrevTest(test.Name, allPrev.TestResults)

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

func summaryJobPassRatesByJobName(report, reportPrev util.TestReport, endDay, jobTestCount int) []PassRatesByJobName {
	latestDays := fmt.Sprintf("latest%dDays", endDay)
	prevDays := "prev7Days"
	jobRunsByName := util.SummarizeJobsByName(report)
	jobRunsByNamePrev := util.SummarizeJobsByName(reportPrev)

	var passRatesSlice []PassRatesByJobName

	for _, v := range jobRunsByName {
		prev := util.GetPrevJob(v.Name, jobRunsByNamePrev)

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

		passRatesSlice = append(passRatesSlice, newJobPassRate)

	}

	return passRatesSlice

}

// canaryTestFailures section
func canaryTestFailures(result map[string]util.SortedAggregateTestResult) []CanaryTestFailInstance {
	all := result["all"].TestResults

	var canaryFailures []CanaryTestFailInstance

	if len(all) <= 0 {
		return nil
	}

	for i := len(all) - 1; i > len(all)-10; i-- {
		test := all[i]
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
		canaryFailures = append(canaryFailures,
			CanaryTestFailInstance{
				Name: test.Name,
				Url:  fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName),
				PassRate: PassRate{
					Percentage: test.PassPercentage,
					Runs:       test.Successes + test.Failures,
				},
			})
	}
	return canaryFailures
}

// job runs with failure groups
func failureGroupList(report util.TestReport) []FailureGroup {

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

// PrintJSONReport prints the json format of the report
// follows conventions from jsonapi.org
func PrintJSONReport(w http.ResponseWriter, req *http.Request, report, prevReport util.TestReport, endDay, jobTestCount int) {

	data := html.TestReports{
		Current:      report,
		Prev:         prevReport,
		EndDay:       endDay,
		JobTestCount: jobTestCount,
		Release:      report.Release}

	jsonObject := map[string]interface{}{
		"releaseHealthData": map[string]interface{}{
			"summaryAllJobs":            summaryAcrossAllJobs(data.Current.All, data.Prev.All, data.EndDay),
			"failureGroupings":          failureGroups(data.Current.FailureGroups, data.Prev.FailureGroups, data.EndDay),
			"jobPassRateByPlatform":     summaryJobsByPlatform(data.Current, data.Prev, data.EndDay, data.JobTestCount),
			"topFailingTestsWithoutBug": summaryTopFailingTestsWithoutBug(data.Current.TopFailingTestsWithoutBug, data.Prev.All, data.EndDay),
			"topFailingTestsWithBug":    summaryTopFailingTestsWithBug(data.Current.TopFailingTestsWithBug, data.Prev.All, data.EndDay),
			"jobPassRatesByName":        summaryJobPassRatesByJobName(data.Current, data.Prev, data.EndDay, data.JobTestCount),
			"canaryTestFailures":        canaryTestFailures(data.Current.All),
			"jobRunsWithFailureGroups":  failureGroupList(data.Current),
			"testImpactingBugs":         data.Current.BugsByFailureCount,
		},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	err := enc.Encode(jsonObject)
	if err != nil {
		klog.Errorf("unable to render json %v", err)
	}

}
