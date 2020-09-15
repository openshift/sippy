package testreportconversion

import (
	"sort"
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
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
	allTestResultsByName := getTestResultsByName(allJobResults)

	standardTestResultFilterFn := standardTestResultFilter(minRuns, successThreshold)

	byPlatform := convertRawDataToByPlatform(rawData.JobResults, bugCache, release, standardTestResultFilterFn)

	filteredFailureGroups := filterFailureGroups(rawData.JobResults, bugCache, release, failureClusterThreshold)
	frequentJobResults := filterPertinentFrequentJobResults(allJobResults, endDay, standardTestResultFilterFn)
	infrequentJobResults := filterPertinentInfrequentJobResults(allJobResults, endDay, standardTestResultFilterFn)

	bugFailureCounts := generateSortedBugFailureCounts(allTestResultsByName)
	bugzillaComponentResults := generateAllJobFailuresByBugzillaComponent(rawData.JobResults, allJobResults)

	topFailingTestsWithBug := getTopFailingTestsWithBug(allTestResultsByName, standardTestResultFilterFn)
	topFailingTestsWithoutBug := getTopFailingTestsWithoutBug(allTestResultsByName, standardTestResultFilterFn)

	testReport := sippyprocessingv1.TestReport{
		Release:   release,
		Timestamp: reportTimestamp,

		ByTest:        allTestResultsByName.toOrderedList(),
		ByPlatform:    byPlatform,
		FailureGroups: filteredFailureGroups,

		ByJob:                allJobResults,
		FrequentJobResults:   frequentJobResults,
		InfrequentJobResults: infrequentJobResults,

		BugsByFailureCount:             bugFailureCounts,
		JobFailuresByBugzillaComponent: bugzillaComponentResults,

		TopFailingTestsWithBug:    topFailingTestsWithBug,
		TopFailingTestsWithoutBug: topFailingTestsWithoutBug,

		AnalysisWarnings: analysisWarnings,
	}

	return testReport
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

func generateSortedBugFailureCounts(allTestResultsByName testResultsByName) []bugsv1.Bug {
	bugs := map[string]bugsv1.Bug{}

	// for every test that failed in some job run, look up the bug(s) associated w/ the test
	// and attribute the number of times the test failed+flaked to that bug(s)
	for _, testResult := range allTestResultsByName {
		bugList := testResult.TestResultAcrossAllJobs.BugList
		for _, bug := range bugList {
			if b, found := bugs[bug.Url]; found {
				b.FailureCount += testResult.TestResultAcrossAllJobs.Failures
				b.FlakeCount += testResult.TestResultAcrossAllJobs.Flakes
				bugs[bug.Url] = b
			} else {
				bug.FailureCount = testResult.TestResultAcrossAllJobs.Failures
				bug.FlakeCount = testResult.TestResultAcrossAllJobs.Flakes
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
