package testreportconversion

import (
	"sort"
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util/sets"
)

func PrepareTestReport(
	rawData testgridanalysisapi.RawData,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
	// TODO refactor into a test run filter
	minRuns int, // indicates how many runs are required for a test is included in overall percentages
	// TODO deads2k wants to eliminate the successThreshold
	successThreshold float64, // indicates an upper bound on how successful a test can be before it is excluded
	endDay int, // indicates how many days of data to collect
	analysisWarnings []string,
	reportTimestamp time.Time, // TODO seems like we could derive this from our raw data
	failureClusterThreshold int, // TODO I don't think we even display this anymore
) sippyprocessingv1.TestReport {

	// allJobResults holds all the job results with all the test results.  It contains complete frequency information and
	allJobResults := convertRawJobResultsToProcessedJobResults(rawData.JobResults, bugCache, release)

	standardTestResultFilterFn := standardTestResultFilter(minRuns, successThreshold)

	byAll := summarizeTestResults(rawData.ByAll, bugCache, release, minRuns, successThreshold)
	byPlatform := convertRawDataToByPlatform(rawData.JobResults, bugCache, release, standardTestResultFilterFn)

	filteredFailureGroups := filterFailureGroups(rawData.JobResults, bugCache, release, failureClusterThreshold)
	frequentJobResults := filterPertinentFrequentJobResults(allJobResults, endDay, standardTestResultFilterFn)
	infrequentJobResults := filterPertinentInfrequentJobResults(allJobResults, endDay, standardTestResultFilterFn)

	bugFailureCounts := generateSortedBugFailureCounts(rawData.JobResults, byAll, bugCache, release)
	bugzillaComponentResults := generateAllJobFailuresByBugzillaComponent(rawData.JobResults, allJobResults)

	testReport := sippyprocessingv1.TestReport{
		Release:                        release,
		All:                            byAll,
		ByPlatform:                     byPlatform,
		FailureGroups:                  filteredFailureGroups,
		FrequentJobResults:             frequentJobResults,
		InfrequentJobResults:           infrequentJobResults,
		Timestamp:                      reportTimestamp,
		BugsByFailureCount:             bugFailureCounts,
		JobFailuresByBugzillaComponent: bugzillaComponentResults,

		AnalysisWarnings: analysisWarnings,
	}

	testReport.TopFailingTestsWithBug = getTopFailingTestsWithBug(allJobResults, standardTestResultFilterFn)
	testReport.TopFailingTestsWithoutBug = getTopFailingTestsWithoutBug(allJobResults, standardTestResultFilterFn)

	return testReport
}

func summarizeTestResults(
	aggregateTestResult map[string]testgridanalysisapi.AggregateTestsResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
	minRuns int, // indicates how many runs are required for a test is included in overall percentages
	// TODO deads2k wants to eliminate the successThreshold
	successThreshold float64, // indicates an upper bound on how successful a test can be before it is excluded
) map[string]sippyprocessingv1.SortedAggregateTestsResult {
	sorted := make(map[string]sippyprocessingv1.SortedAggregateTestsResult)

	for k, v := range aggregateTestResult {
		sorted[k] = sippyprocessingv1.SortedAggregateTestsResult{}

		passedCount := 0
		failedCount := 0
		for _, rawTestResult := range v.RawTestResults {
			passPercentage := percent(rawTestResult.Successes, rawTestResult.Failures)

			// strip out tests are more than N% successful
			if passPercentage > successThreshold {
				continue
			}
			// strip out tests that have less than N total runs
			if rawTestResult.Successes+rawTestResult.Failures < minRuns {
				continue
			}

			passedCount += rawTestResult.Successes
			failedCount += rawTestResult.Failures

			s := sorted[k]
			s.TestResults = append(s.TestResults,
				convertRawTestResultToProcessedTestResult(rawTestResult, bugCache, release))
			sorted[k] = s
		}

		s := sorted[k]
		s.Successes = passedCount
		s.Failures = failedCount
		s.TestPassPercentage = percent(passedCount, failedCount)
		sorted[k] = s

		sort.Stable(testResultsByPassPercentage(sorted[k].TestResults))
	}
	return sorted
}

func filterFailureGroups(
	rawJobResults map[string]testgridanalysisapi.RawJobResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
	failureClusterThreshold int,
) []sippyprocessingv1.JobRunResult {
	filteredJrr := []sippyprocessingv1.JobRunResult{}
	// -1 means don't do this reporting.
	if failureClusterThreshold < 0 {
		return filteredJrr
	}
	for _, jobResult := range rawJobResults {
		for _, rawJRR := range jobResult.JobRunResults {
			if rawJRR.TestFailures < failureClusterThreshold {
				continue
			}

			allFailuresKnown := areAllFailuresKnown(rawJRR, bugCache, release)
			hasUnknownFailure := rawJRR.Failed && !allFailuresKnown

			filteredJrr = append(filteredJrr, sippyprocessingv1.JobRunResult{
				Job:                jobResult.JobName,
				Url:                rawJRR.JobRunURL,
				TestFailures:       rawJRR.TestFailures,
				FailedTestNames:    rawJRR.FailedTestNames,
				Failed:             rawJRR.Failed,
				HasUnknownFailures: hasUnknownFailure,
				Succeeded:          rawJRR.Succeeded,
			})
		}
	}

	// sort from highest to lowest
	sort.SliceStable(filteredJrr, func(i, j int) bool {
		return filteredJrr[i].TestFailures > filteredJrr[j].TestFailures
	})

	return filteredJrr
}

func generateSortedBugFailureCounts(
	allJobResults map[string]testgridanalysisapi.RawJobResult,
	byAll map[string]sippyprocessingv1.SortedAggregateTestsResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
) []bugsv1.Bug {
	bugs := map[string]bugsv1.Bug{}

	failedTestNamesAcrossAllJobRuns := sets.NewString()
	for _, jobResult := range allJobResults {
		for _, jobrun := range jobResult.JobRunResults {
			failedTestNamesAcrossAllJobRuns.Insert(jobrun.FailedTestNames...)
		}
	}

	// for every test that failed in some job run, look up the bug(s) associated w/ the test
	// and attribute the number of times the test failed+flaked to that bug(s)
	for _, testResult := range byAll["all"].TestResults {
		testName := testResult.Name
		bugList := bugCache.ListBugs(release, "", testName)
		for _, bug := range bugList {
			if b, found := bugs[bug.Url]; found {
				b.FailureCount += testResult.Failures
				b.FlakeCount += testResult.Flakes
				bugs[bug.Url] = b
			} else {
				bug.FailureCount = testResult.Failures
				bug.FlakeCount = testResult.Flakes
				bugs[bug.Url] = bug
			}
		}
	}

	sortedBugs := []bugsv1.Bug{}
	for _, bug := range bugs {
		sortedBugs = append(sortedBugs, bug)
	}
	// sort from highest to lowest
	sort.SliceStable(sortedBugs, func(i, j int) bool {
		return sortedBugs[i].FailureCount > sortedBugs[j].FailureCount
	})
	return sortedBugs
}

func percent(success, failure int) float64 {
	if success+failure == 0 {
		//return math.NaN()
		return 0.0
	}
	return float64(success) / float64(success+failure) * 100.0
}
