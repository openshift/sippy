package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/openshift/sippy/pkg/html/releasehtml"

	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/util"
)

// stats on failure groups
func failureGroups(failureGroups, failureGroupsPrev []sippyprocessingv1.JobRunResult) *sippyv1.FailureGroups {

	stats := util.ComputeFailureGroupStats(failureGroups, failureGroupsPrev)

	failureGroupStruct := sippyv1.FailureGroups{
		JobRunsWithFailureGroup: map[string]int{
			"latest": len(failureGroups),
			"prev":   len(failureGroupsPrev),
		},
		AvgFailureGroupSize: map[string]int{
			"latest": stats.Avg,
			"prev":   stats.AvgPrev,
		},
		MedianFailureGroupSize: map[string]int{
			"latest": stats.Median,
			"prev":   stats.MedianPrev,
		},
	}
	return &failureGroupStruct
}

func summaryJobsByVariant(report, reportPrev sippyprocessingv1.TestReport) []sippyv1.JobSummaryVariant {
	var jobSummariesByVariant []sippyv1.JobSummaryVariant

	for _, v := range report.ByVariant {
		prev := util.FindVariantResultsForName(v.VariantName, reportPrev.ByVariant)

		var jobSummaryVariant sippyv1.JobSummaryVariant

		if prev != nil {
			jobSummaryVariant = sippyv1.JobSummaryVariant{
				Variant: v.VariantName,
				PassRates: map[string]sippyv1.PassRate{
					"latest": {
						Percentage:          v.JobRunPassPercentage,
						ProjectedPercentage: v.JobRunPassPercentageWithKnownFailures,
						Runs:                v.JobRunSuccesses + v.JobRunFailures,
					},
					"prev": {
						Percentage:          prev.JobRunPassPercentage,
						ProjectedPercentage: prev.JobRunPassPercentageWithKnownFailures,
						Runs:                prev.JobRunSuccesses + prev.JobRunFailures,
					},
				},
			}
		} else {
			jobSummaryVariant = sippyv1.JobSummaryVariant{
				Variant: v.VariantName,
				PassRates: map[string]sippyv1.PassRate{
					"latest": {
						Percentage:          v.JobRunPassPercentage,
						ProjectedPercentage: v.JobRunPassPercentageWithKnownFailures,
						Runs:                v.JobRunSuccesses + v.JobRunFailures,
					},
				},
			}
		}

		jobSummariesByVariant = append(jobSummariesByVariant, jobSummaryVariant)
	}
	return jobSummariesByVariant
}

// top failing tests with a bug
func summaryTopFailingTestsWithBug(topFailingTestsWithBug, prevTestResults []sippyprocessingv1.FailingTestResult) []sippyv1.FailingTestBug {

	var topFailingTests []sippyv1.FailingTestBug

	for _, test := range topFailingTestsWithBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.TestName))

		testLink := fmt.Sprintf("%s%s", releasehtml.BugSearchURL, encodedTestName)
		testPrev := util.FindFailedTestResult(test.TestName, prevTestResults)

		var failedTestWithBug sippyv1.FailingTestBug

		if testPrev != nil {
			failedTestWithBug = sippyv1.FailingTestBug{
				Name: test.TestName,
				URL:  testLink,
				PassRates: map[string]sippyv1.PassRate{
					"latest": {
						Percentage: test.TestResultAcrossAllJobs.PassPercentage,
						Runs:       test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures,
					},
					"prev": {
						Percentage: testPrev.TestResultAcrossAllJobs.PassPercentage,
						Runs:       testPrev.TestResultAcrossAllJobs.Successes + testPrev.TestResultAcrossAllJobs.Failures,
					},
				},
				Bugs:           test.TestResultAcrossAllJobs.BugList,
				AssociatedBugs: test.TestResultAcrossAllJobs.AssociatedBugList,
			}
		} else {
			failedTestWithBug = sippyv1.FailingTestBug{
				Name: test.TestResultAcrossAllJobs.Name,
				URL:  testLink,
				PassRates: map[string]sippyv1.PassRate{
					"latest": {
						Percentage: test.TestResultAcrossAllJobs.PassPercentage,
						Runs:       test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures,
					},
				},
				Bugs:           test.TestResultAcrossAllJobs.BugList,
				AssociatedBugs: test.TestResultAcrossAllJobs.AssociatedBugList,
			}
		}

		topFailingTests = append(topFailingTests, failedTestWithBug)
	}

	return topFailingTests

}

// top failing tests without a bug
func summaryTopFailingTestsWithoutBug(topFailingTestsWithoutBug, prevTopFailingTestsWithoutBug []sippyprocessingv1.FailingTestResult) []sippyv1.FailingTestBug {
	var topFailingTests []sippyv1.FailingTestBug

	for _, test := range topFailingTestsWithoutBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.TestName))

		testLink := fmt.Sprintf("%s%s", releasehtml.BugSearchURL, encodedTestName)
		testPrev := util.FindFailedTestResult(test.TestName, prevTopFailingTestsWithoutBug)

		var failedTestWithoutBug sippyv1.FailingTestBug

		if testPrev != nil {
			failedTestWithoutBug = sippyv1.FailingTestBug{
				Name:           test.TestName,
				URL:            testLink,
				AssociatedBugs: test.TestResultAcrossAllJobs.AssociatedBugList,
				PassRates: map[string]sippyv1.PassRate{
					"latest": {
						Percentage: test.TestResultAcrossAllJobs.PassPercentage,
						Runs:       test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures,
					},
					"prev": {
						Percentage: testPrev.TestResultAcrossAllJobs.PassPercentage,
						Runs:       testPrev.TestResultAcrossAllJobs.Successes + testPrev.TestResultAcrossAllJobs.Failures,
					},
				},
			}

		} else {
			failedTestWithoutBug = sippyv1.FailingTestBug{
				Name:           test.TestName,
				URL:            testLink,
				AssociatedBugs: test.TestResultAcrossAllJobs.AssociatedBugList,
				PassRates: map[string]sippyv1.PassRate{
					"latest": {
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

func summaryJobPassRatesByJobName(report, reportPrev sippyprocessingv1.TestReport) []sippyv1.PassRatesByJobName {
	var passRatesSlice []sippyv1.PassRatesByJobName

	for _, v := range report.FrequentJobResults {
		prev := util.FindJobResultForJobName(v.Name, reportPrev.FrequentJobResults)

		var newJobPassRate sippyv1.PassRatesByJobName

		if prev != nil {
			newJobPassRate = sippyv1.PassRatesByJobName{
				Name: v.Name,
				URL:  v.TestGridURL,
				PassRates: map[string]sippyv1.PassRate{
					"latest": {
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithoutInfrastructureFailures,
						Runs:                v.Successes + v.Failures,
					},
					"prev": {
						Percentage:          prev.PassPercentage,
						ProjectedPercentage: prev.PassPercentageWithoutInfrastructureFailures,
						Runs:                prev.Successes + prev.Failures,
					},
				},
			}
		} else {
			newJobPassRate = sippyv1.PassRatesByJobName{
				Name: v.Name,
				URL:  v.TestGridURL,
				PassRates: map[string]sippyv1.PassRate{
					"latest": {
						Percentage:          v.PassPercentage,
						ProjectedPercentage: v.PassPercentageWithoutInfrastructureFailures,
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
func canaryTestFailures(all []sippyprocessingv1.FailingTestResult) []sippyv1.CanaryTestFailInstance {
	var canaryFailures []sippyv1.CanaryTestFailInstance

	if len(all) == 0 {
		return nil
	}

	foundCount := 0
	for i := len(all) - 1; i >= 0; i-- {
		test := all[i]
		if test.TestResultAcrossAllJobs.Failures == 0 {
			continue
		}
		foundCount++
		if foundCount > 10 {
			break
		}
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.TestName))
		canaryFailures = append(canaryFailures,
			sippyv1.CanaryTestFailInstance{
				Name: test.TestName,
				URL:  fmt.Sprintf("%s%s", releasehtml.BugSearchURL, encodedTestName),
				PassRate: sippyv1.PassRate{
					Percentage: test.TestResultAcrossAllJobs.PassPercentage,
					Runs:       test.TestResultAcrossAllJobs.Successes + test.TestResultAcrossAllJobs.Failures,
				},
			})
	}
	return canaryFailures
}

func minimumJobPassRateByBugzillaComponent(report, prev sippyprocessingv1.TestReport) []sippyv1.MinimumPassRatesByComponent {
	var result []sippyv1.MinimumPassRatesByComponent
	for c, failures := range report.JobFailuresByBugzillaComponent {
		passRate := sippyv1.MinimumPassRatesByComponent{
			Name: c,
			PassRates: map[string]sippyv1.PassRate{
				"latest": {
					Percentage: 100.0 - failures.JobsFailed[0].FailPercentage,
					Runs:       failures.JobsFailed[0].TotalRuns,
				},
			},
		}
		if prev, found := prev.JobFailuresByBugzillaComponent[c]; found {
			passRate.PassRates["prev"] = sippyv1.PassRate{
				Percentage: 100.0 - prev.JobsFailed[0].FailPercentage,
				Runs:       prev.JobsFailed[0].TotalRuns,
			}
		}
		result = append(result, passRate)
	}

	return result
}

// job runs with failure groups
func failureGroupList(report sippyprocessingv1.TestReport) []sippyv1.FailureGroup {

	var failureGroups []sippyv1.FailureGroup
	for _, fg := range report.FailureGroups {
		failureGroups = append(failureGroups, sippyv1.FailureGroup{
			Job:          fg.Job,
			URL:          fg.URL,
			TestFailures: fg.TestFailures,
		})
	}
	return failureGroups
}

func formatJSONReport(report, prevReport sippyprocessingv1.TestReport, numDays, jobTestCount int) map[string]interface{} {
	data := releasehtml.TestReports{
		Current:      report,
		Prev:         prevReport,
		NumDays:      numDays,
		JobTestCount: jobTestCount,
		Release:      report.Release}

	jsonObject := map[string]interface{}{
		"failureGroupings":               failureGroups(data.Current.FailureGroups, data.Prev.FailureGroups),
		"jobPassRateByVariant":           summaryJobsByVariant(data.Current, data.Prev),
		"topFailingTestsWithoutBug":      summaryTopFailingTestsWithoutBug(data.Current.TopFailingTestsWithoutBug, data.Prev.TopFailingTestsWithoutBug),
		"topFailingTestsWithBug":         summaryTopFailingTestsWithBug(data.Current.TopFailingTestsWithBug, data.Prev.ByTest),
		"jobPassRatesByName":             summaryJobPassRatesByJobName(data.Current, data.Prev),
		"minimumJobPassRatesByComponent": minimumJobPassRateByBugzillaComponent(data.Current, data.Prev),
		"canaryTestFailures":             canaryTestFailures(data.Current.ByTest),
		"jobRunsWithFailureGroups":       failureGroupList(data.Current),
		"testImpactingBugs":              data.Current.BugsByFailureCount,
	}
	return jsonObject
}

// PrintJSONReport prints json format of the reports
func PrintJSONReport(w http.ResponseWriter, req *http.Request, releaseReports map[string][]sippyprocessingv1.TestReport, numDays, jobTestCount int) {
	reportObjects := make(map[string]interface{})
	for _, reports := range releaseReports {
		report := reports[0]
		prevReport := reports[1]
		reportObjects[report.Release] = formatJSONReport(report, prevReport, numDays, jobTestCount)
	}

	RespondWithJSON(http.StatusOK, w, reportObjects)
}

func RespondWithJSON(statusCode int, w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(statusCode)

	if jsonString, ok := data.(string); ok {
		fmt.Fprint(w, jsonString)
		return
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		fmt.Fprintf(w, `{message: "could not marshal results: %s"}`, err)
	}
}
