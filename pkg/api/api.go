package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	v12 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"

	"github.com/openshift/sippy/pkg/html"
	"github.com/openshift/sippy/pkg/util"
	"k8s.io/klog"
)

// summary across all job
func summaryAcrossAllJobs(result, resultPrev map[string]v12.SortedAggregateTestsResult, endDay int) *v1.SummaryAcrossAllJobs {
	all := result["all"]
	allPrev := resultPrev["all"]

	summary := v1.SummaryAcrossAllJobs{
		TestExecutions: map[string]int{
			"latest": all.Successes + all.Failures,
			"prev":   allPrev.Successes + allPrev.Failures,
		},
		TestPassPercentage: map[string]float64{
			"latest": all.TestPassPercentage,
			"prev":   allPrev.TestPassPercentage,
		},
	}

	return &summary
}

// stats on failure groups
func failureGroups(failureGroups, failureGroupsPrev []v12.JobRunResult, endDay int) *v1.FailureGroups {

	_, _, median, medianPrev, avg, avgPrev := util.ComputeFailureGroupStats(failureGroups, failureGroupsPrev)

	failureGroupStruct := v1.FailureGroups{
		JobRunsWithFailureGroup: map[string]int{
			"latest": len(failureGroups),
			"prev":   len(failureGroupsPrev),
		},
		AvgFailureGroupSize: map[string]int{
			"latest": avg,
			"prev":   avgPrev,
		},
		MedianFailureGroupSize: map[string]int{
			"latest": median,
			"prev":   medianPrev,
		},
	}
	return &failureGroupStruct
}

func summaryJobsByPlatform(report, reportPrev v12.TestReport, endDay, jobTestCount int) []v1.JobSummaryPlatform {
	jobsByPlatform := util.SummarizeJobsByPlatform(report)
	jobsByPlatformPrev := util.SummarizeJobsByPlatform(reportPrev)

	var jobSummariesByPlatform []v1.JobSummaryPlatform

	for _, v := range jobsByPlatform {
		prev := util.GetPrevPlatform(v.Platform, jobsByPlatformPrev)

		var jobSummaryPlatform v1.JobSummaryPlatform

		if prev != nil {
			jobSummaryPlatform = v1.JobSummaryPlatform{
				Platform: v.Platform,
				PassRates: map[string]v1.PassRate{
					"latest": v1.PassRate{
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithKnownFailures,
						Runs:                v.Successes + v.Failures,
					},
					"prev": v1.PassRate{
						Percentage:          prev.PassPercentage,
						ProjectedPercentage: prev.PassPercentageWithKnownFailures,
						Runs:                prev.Successes + prev.Failures,
					},
				},
			}
		} else {
			jobSummaryPlatform = v1.JobSummaryPlatform{
				Platform: v.Platform,
				PassRates: map[string]v1.PassRate{
					"latest": v1.PassRate{
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
func summaryTopFailingTestsWithBug(topFailingTestsWithBug []*v12.TestResult, resultPrev map[string]v12.SortedAggregateTestsResult, endDay int) []v1.FailingTestBug {

	var topFailingTests []v1.FailingTestBug

	allPrev := resultPrev["all"]

	for _, test := range topFailingTestsWithBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName)
		testPrev := util.GetPrevTest(test.Name, allPrev.TestResults)

		var failedTestWithBug v1.FailingTestBug

		if testPrev != nil {
			failedTestWithBug = v1.FailingTestBug{
				Name: test.Name,
				Url:  testLink,
				PassRates: map[string]v1.PassRate{
					"latest": v1.PassRate{
						Percentage: test.PassPercentage,
						Runs:       test.Successes + test.Failures,
					},
					"prev": v1.PassRate{
						Percentage: testPrev.PassPercentage,
						Runs:       testPrev.Successes + test.Failures,
					},
				},
				Bugs: test.BugList,
			}
		} else {
			failedTestWithBug = v1.FailingTestBug{
				Name: test.Name,
				Url:  testLink,
				PassRates: map[string]v1.PassRate{
					"latest": v1.PassRate{
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
func summaryTopFailingTestsWithoutBug(topFailingTestsWithoutBug []*v12.TestResult, resultPrev map[string]v12.SortedAggregateTestsResult, endDay int) []v1.FailingTestBug {

	allPrev := resultPrev["all"]

	var topFailingTests []v1.FailingTestBug

	for _, test := range topFailingTestsWithoutBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName)
		testPrev := util.GetPrevTest(test.Name, allPrev.TestResults)

		var failedTestWithoutBug v1.FailingTestBug

		if testPrev != nil {

			failedTestWithoutBug = v1.FailingTestBug{
				Name: test.Name,
				Url:  testLink,
				PassRates: map[string]v1.PassRate{
					"latest": v1.PassRate{
						Percentage: test.PassPercentage,
						Runs:       test.Successes + test.Failures,
					},
					"prev": v1.PassRate{
						Percentage: testPrev.PassPercentage,
						Runs:       testPrev.Successes + testPrev.Failures,
					},
				},
			}

		} else {
			failedTestWithoutBug = v1.FailingTestBug{
				Name: test.Name,
				Url:  testLink,
				PassRates: map[string]v1.PassRate{
					"latest": v1.PassRate{
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

func summaryJobPassRatesByJobName(report, reportPrev v12.TestReport, endDay, jobTestCount int) []v1.PassRatesByJobName {
	var passRatesSlice []v1.PassRatesByJobName

	for _, v := range report.JobPassRate {
		prev := util.GetPrevJob(v.Name, reportPrev.JobPassRate)

		var newJobPassRate v1.PassRatesByJobName

		if prev != nil {
			newJobPassRate = v1.PassRatesByJobName{
				Name: v.Name,
				Url:  v.TestGridUrl,
				PassRates: map[string]v1.PassRate{
					"latest": v1.PassRate{
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithKnownFailures,
						Runs:                v.Successes + v.Failures,
					},
					"prev": v1.PassRate{
						Percentage:          prev.PassPercentage,
						ProjectedPercentage: prev.PassPercentageWithKnownFailures,
						Runs:                prev.Successes + prev.Failures,
					},
				},
			}
		} else {
			newJobPassRate = v1.PassRatesByJobName{
				Name: v.Name,
				Url:  v.TestGridUrl,
				PassRates: map[string]v1.PassRate{
					"latest": v1.PassRate{
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
func canaryTestFailures(result map[string]v12.SortedAggregateTestsResult) []v1.CanaryTestFailInstance {
	all := result["all"].TestResults

	var canaryFailures []v1.CanaryTestFailInstance

	if len(all) <= 0 {
		return nil
	}

	for i := len(all) - 1; i > len(all)-10; i-- {
		test := all[i]
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
		canaryFailures = append(canaryFailures,
			v1.CanaryTestFailInstance{
				Name: test.Name,
				Url:  fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName),
				PassRate: v1.PassRate{
					Percentage: test.PassPercentage,
					Runs:       test.Successes + test.Failures,
				},
			})
	}
	return canaryFailures
}

// job runs with failure groups
func failureGroupList(report v12.TestReport) []v1.FailureGroup {

	var failureGroups []v1.FailureGroup
	for _, fg := range report.FailureGroups {
		failureGroups = append(failureGroups, v1.FailureGroup{
			Job:          fg.Job,
			Url:          fg.Url,
			TestFailures: fg.TestFailures,
		})
	}
	return failureGroups
}

func formatJSONReport(report, prevReport v12.TestReport, endDay, jobTestCount int) map[string]interface{} {
	data := html.TestReports{
		Current:      report,
		Prev:         prevReport,
		EndDay:       endDay,
		JobTestCount: jobTestCount,
		Release:      report.Release}

	jsonObject := map[string]interface{}{
		"summaryAllJobs":            summaryAcrossAllJobs(data.Current.All, data.Prev.All, data.EndDay),
		"failureGroupings":          failureGroups(data.Current.FailureGroups, data.Prev.FailureGroups, data.EndDay),
		"jobPassRateByPlatform":     summaryJobsByPlatform(data.Current, data.Prev, data.EndDay, data.JobTestCount),
		"topFailingTestsWithoutBug": summaryTopFailingTestsWithoutBug(data.Current.TopFailingTestsWithoutBug, data.Prev.All, data.EndDay),
		"topFailingTestsWithBug":    summaryTopFailingTestsWithBug(data.Current.TopFailingTestsWithBug, data.Prev.All, data.EndDay),
		"jobPassRatesByName":        summaryJobPassRatesByJobName(data.Current, data.Prev, data.EndDay, data.JobTestCount),
		"canaryTestFailures":        canaryTestFailures(data.Current.All),
		"jobRunsWithFailureGroups":  failureGroupList(data.Current),
		"testImpactingBugs":         data.Current.BugsByFailureCount,
	}
	return jsonObject
}

// PrintJSONReport prints json format of the reports
func PrintJSONReport(w http.ResponseWriter, req *http.Request, releaseReports map[string][]v12.TestReport, endDay, jobTestCount int) {
	reportObjects := make(map[string]interface{})
	for _, reports := range releaseReports {
		report := reports[0]
		prevReport := reports[1]
		reportObjects[report.Release] = formatJSONReport(report, prevReport, endDay, jobTestCount)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "    ")
	err := enc.Encode(reportObjects)
	if err != nil {
		klog.Errorf("unable to render json %v", err)
	}
}
