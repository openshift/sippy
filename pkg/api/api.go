package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html"
	"github.com/openshift/sippy/pkg/util"
	"k8s.io/klog"
)

// summary across all job
func summaryAcrossAllJobs(result, resultPrev map[string]sippyprocessingv1.SortedAggregateTestsResult, endDay int) *sippyv1.SummaryAcrossAllJobs {
	all := result["all"]
	allPrev := resultPrev["all"]

	summary := sippyv1.SummaryAcrossAllJobs{
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
func failureGroups(failureGroups, failureGroupsPrev []sippyprocessingv1.JobRunResult, endDay int) *sippyv1.FailureGroups {

	_, _, median, medianPrev, avg, avgPrev := util.ComputeFailureGroupStats(failureGroups, failureGroupsPrev)

	failureGroupStruct := sippyv1.FailureGroups{
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

func summaryJobsByPlatform(report, reportPrev sippyprocessingv1.TestReport, endDay, jobTestCount int) []sippyv1.JobSummaryPlatform {
	var jobSummariesByPlatform []sippyv1.JobSummaryPlatform

	for _, v := range report.ByPlatform {
		prev := util.GetPlatform(v.PlatformName, reportPrev.ByPlatform)

		var jobSummaryPlatform sippyv1.JobSummaryPlatform

		if prev != nil {
			jobSummaryPlatform = sippyv1.JobSummaryPlatform{
				Platform: v.PlatformName,
				PassRates: map[string]sippyv1.PassRate{
					"latest": sippyv1.PassRate{
						Percentage:          v.JobRunPassPercentage,
						ProjectedPercentage: v.JobRunPassPercentageWithKnownFailures,
						Runs:                v.JobRunSuccesses + v.JobRunFailures,
					},
					"prev": sippyv1.PassRate{
						Percentage:          prev.JobRunPassPercentage,
						ProjectedPercentage: prev.JobRunPassPercentageWithKnownFailures,
						Runs:                prev.JobRunSuccesses + prev.JobRunFailures,
					},
				},
			}
		} else {
			jobSummaryPlatform = sippyv1.JobSummaryPlatform{
				Platform: v.PlatformName,
				PassRates: map[string]sippyv1.PassRate{
					"latest": sippyv1.PassRate{
						Percentage:          v.JobRunPassPercentage,
						ProjectedPercentage: v.JobRunPassPercentageWithKnownFailures,
						Runs:                v.JobRunSuccesses + v.JobRunFailures,
					},
				},
			}
		}

		jobSummariesByPlatform = append(jobSummariesByPlatform, jobSummaryPlatform)
	}
	return jobSummariesByPlatform
}

// top failing tests with a bug
func summaryTopFailingTestsWithBug(topFailingTestsWithBug []sippyprocessingv1.FailingTestResult, resultPrev map[string]sippyprocessingv1.SortedAggregateTestsResult, endDay int) []sippyv1.FailingTestBug {

	var topFailingTests []sippyv1.FailingTestBug

	allPrev := resultPrev["all"]

	for _, test := range topFailingTestsWithBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.TestName))

		testLink := fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName)
		testPrev := util.GetPrevTest(test.TestName, allPrev.TestResults)

		var failedTestWithBug sippyv1.FailingTestBug

		if testPrev != nil {
			failedTestWithBug = sippyv1.FailingTestBug{
				Name: test.TestName,
				Url:  testLink,
				PassRates: map[string]sippyv1.PassRate{
					"latest": sippyv1.PassRate{
						Percentage: test.TestResultAcrossAllJobs.PassPercentage,
						Runs:       test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures,
					},
					"prev": sippyv1.PassRate{
						Percentage: testPrev.PassPercentage,
						Runs:       testPrev.Successes + test.TestResultAcrossAllJobs.Failures,
					},
				},
				Bugs: test.TestResultAcrossAllJobs.BugList,
			}
		} else {
			failedTestWithBug = sippyv1.FailingTestBug{
				Name: test.TestResultAcrossAllJobs.Name,
				Url:  testLink,
				PassRates: map[string]sippyv1.PassRate{
					"latest": sippyv1.PassRate{
						Percentage: test.TestResultAcrossAllJobs.PassPercentage,
						Runs:       test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures,
					},
				},
				Bugs: test.TestResultAcrossAllJobs.BugList,
			}
		}

		topFailingTests = append(topFailingTests, failedTestWithBug)
	}

	return topFailingTests

}

// top failing tests without a bug
func summaryTopFailingTestsWithoutBug(topFailingTestsWithoutBug []sippyprocessingv1.FailingTestResult, resultPrev map[string]sippyprocessingv1.SortedAggregateTestsResult, endDay int) []sippyv1.FailingTestBug {

	allPrev := resultPrev["all"]

	var topFailingTests []sippyv1.FailingTestBug

	for _, test := range topFailingTestsWithoutBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.TestName))

		testLink := fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName)
		testPrev := util.GetPrevTest(test.TestName, allPrev.TestResults)

		var failedTestWithoutBug sippyv1.FailingTestBug

		if testPrev != nil {

			failedTestWithoutBug = sippyv1.FailingTestBug{
				Name: test.TestName,
				Url:  testLink,
				PassRates: map[string]sippyv1.PassRate{
					"latest": sippyv1.PassRate{
						Percentage: test.TestResultAcrossAllJobs.PassPercentage,
						Runs:       test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures,
					},
					"prev": sippyv1.PassRate{
						Percentage: testPrev.PassPercentage,
						Runs:       testPrev.Successes + testPrev.Failures,
					},
				},
			}

		} else {
			failedTestWithoutBug = sippyv1.FailingTestBug{
				Name: test.TestName,
				Url:  testLink,
				PassRates: map[string]sippyv1.PassRate{
					"latest": sippyv1.PassRate{
						Percentage: test.TestResultAcrossAllJobs.PassPercentage,
						Runs:       test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures,
					},
				},
			}
		}

		topFailingTests = append(topFailingTests, failedTestWithoutBug)
	}
	return topFailingTests
}

func summaryJobPassRatesByJobName(report, reportPrev sippyprocessingv1.TestReport, endDay, jobTestCount int) []sippyv1.PassRatesByJobName {
	var passRatesSlice []sippyv1.PassRatesByJobName

	for _, v := range report.JobResults {
		prev := util.GetJobResultForJobName(v.Name, reportPrev.JobResults)

		var newJobPassRate sippyv1.PassRatesByJobName

		if prev != nil {
			newJobPassRate = sippyv1.PassRatesByJobName{
				Name: v.Name,
				Url:  v.TestGridUrl,
				PassRates: map[string]sippyv1.PassRate{
					"latest": sippyv1.PassRate{
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithKnownFailures,
						Runs:                v.Successes + v.Failures,
					},
					"prev": sippyv1.PassRate{
						Percentage:          prev.PassPercentage,
						ProjectedPercentage: prev.PassPercentageWithKnownFailures,
						Runs:                prev.Successes + prev.Failures,
					},
				},
			}
		} else {
			newJobPassRate = sippyv1.PassRatesByJobName{
				Name: v.Name,
				Url:  v.TestGridUrl,
				PassRates: map[string]sippyv1.PassRate{
					"latest": sippyv1.PassRate{
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
func canaryTestFailures(result map[string]sippyprocessingv1.SortedAggregateTestsResult) []sippyv1.CanaryTestFailInstance {
	all := result["all"].TestResults

	var canaryFailures []sippyv1.CanaryTestFailInstance

	if len(all) <= 0 {
		return nil
	}

	for i := len(all) - 1; i > len(all)-10; i-- {
		test := all[i]
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
		canaryFailures = append(canaryFailures,
			sippyv1.CanaryTestFailInstance{
				Name: test.Name,
				Url:  fmt.Sprintf("%s%s", html.BugSearchUrl, encodedTestName),
				PassRate: sippyv1.PassRate{
					Percentage: test.PassPercentage,
					Runs:       test.Successes + test.Failures,
				},
			})
	}
	return canaryFailures
}

// job runs with failure groups
func failureGroupList(report sippyprocessingv1.TestReport) []sippyv1.FailureGroup {

	var failureGroups []sippyv1.FailureGroup
	for _, fg := range report.FailureGroups {
		failureGroups = append(failureGroups, sippyv1.FailureGroup{
			Job:          fg.Job,
			Url:          fg.Url,
			TestFailures: fg.TestFailures,
		})
	}
	return failureGroups
}

func formatJSONReport(report, prevReport sippyprocessingv1.TestReport, endDay, jobTestCount int) map[string]interface{} {
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
func PrintJSONReport(w http.ResponseWriter, req *http.Request, releaseReports map[string][]sippyprocessingv1.TestReport, endDay, jobTestCount int) {
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
