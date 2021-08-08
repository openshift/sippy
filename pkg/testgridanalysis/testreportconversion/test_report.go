package testreportconversion

import (
	"sort"
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
)

func PrepareTestReport(
	reportName string,
	rawData testgridanalysisapi.RawData,
	variantManager testidentification.VariantManager,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	bugzillaRelease string, // required to limit bugs to those that apply to the release in question
	// TODO refactor into a test run filter
	minRuns int, // indicates how many runs are required for a test is included in overall percentages
	// TODO deads2k wants to eliminate the successThreshold
	successThreshold float64, // indicates an upper bound on how successful a test can be before it is excluded
	numDays int, // indicates how many days of data to collect
	analysisWarnings []string,
	reportTimestamp time.Time, // TODO seems like we could derive this from our raw data
	failureClusterThreshold int, // TODO I don't think we even display this anymore
) sippyprocessingv1.TestReport {

	// allJobResults holds all the job results with all the test results.  It contains complete frequency information and
	allJobResults := convertRawJobResultsToProcessedJobResults(rawData, bugCache, bugzillaRelease, variantManager)
	allTestResultsByName := getTestResultsByName(allJobResults)

	standardTestResultFilterFn := StandardTestResultFilter(minRuns, successThreshold)
	infrequentJobsTestResultFilterFn := StandardTestResultFilter(2, successThreshold)

	byVariant := convertRawDataToByVariant(allJobResults, standardTestResultFilterFn, variantManager)
	variantHealth := convertVariantResultsToHealth(byVariant)

	filteredFailureGroups := filterFailureGroups(rawData.JobResults, allTestResultsByName, failureClusterThreshold)
	frequentJobResults := filterPertinentFrequentJobResults(allJobResults, numDays, standardTestResultFilterFn)
	infrequentJobResults := filterPertinentInfrequentJobResults(allJobResults, numDays, infrequentJobsTestResultFilterFn)

	bugFailureCounts := generateSortedBugFailureCounts(allTestResultsByName)
	bugzillaComponentResults := generateAllJobFailuresByBugzillaComponent(rawData.JobResults, allJobResults)

	topFailingTestsWithBug := getTopFailingTestsWithBug(allTestResultsByName, standardTestResultFilterFn)
	topFailingTestsWithoutBug := getTopFailingTestsWithoutBug(allTestResultsByName, standardTestResultFilterFn)
	curatedTests := getCuratedTests(bugzillaRelease, allTestResultsByName)

	// the top level indicators should exclude jobs that are not yet stable, because those failures are not informative
	infra := excludeNeverStableJobs(allTestResultsByName[testgridanalysisapi.InfrastructureTestName], variantManager)
	install := excludeNeverStableJobs(allTestResultsByName[testgridanalysisapi.InstallTestName], variantManager)
	upgrade := excludeNeverStableJobs(allTestResultsByName[testgridanalysisapi.UpgradeTestName], variantManager)
	finalOperatorHealth := excludeNeverStableJobs(allTestResultsByName[testgridanalysisapi.FinalOperatorHealthTestName], variantManager)

	testReport := sippyprocessingv1.TestReport{
		Release:   reportName,
		Timestamp: reportTimestamp,
		TopLevelIndicators: sippyprocessingv1.TopLevelIndicators{
			Infrastructure:      infra,
			Install:             install,
			Upgrade:             upgrade,
			FinalOperatorHealth: finalOperatorHealth,
			Variant:             variantHealth,
		},

		ByTest:        allTestResultsByName.toOrderedList(),
		ByVariant:     byVariant,
		FailureGroups: filteredFailureGroups,

		ByJob:                allJobResults,
		FrequentJobResults:   frequentJobResults,
		InfrequentJobResults: infrequentJobResults,

		BugsByFailureCount:             bugFailureCounts,
		JobFailuresByBugzillaComponent: bugzillaComponentResults,

		TopFailingTestsWithBug:    topFailingTestsWithBug,
		TopFailingTestsWithoutBug: topFailingTestsWithoutBug,
		CuratedTests:              curatedTests,

		AnalysisWarnings: analysisWarnings,
	}

	return testReport
}

func filterFailureGroups(
	rawJobResults map[string]testgridanalysisapi.RawJobResult,
	allTestResultsByName testResultsByName, // we look up individual tests to find their list of bugs
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

			allFailuresKnown := areAllFailuresKnownFromProcessedResults(rawJRR, allTestResultsByName)
			hasUnknownFailure := rawJRR.Failed && !allFailuresKnown

			filteredJrr = append(filteredJrr, sippyprocessingv1.JobRunResult{
				Job:                jobResult.JobName,
				URL:                rawJRR.JobRunURL,
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
			if b, found := bugs[bug.URL]; found {
				b.FailureCount += testResult.TestResultAcrossAllJobs.Failures
				b.FlakeCount += testResult.TestResultAcrossAllJobs.Flakes
				bugs[bug.URL] = b
			} else {
				bug.FailureCount = testResult.TestResultAcrossAllJobs.Failures
				bug.FlakeCount = testResult.TestResultAcrossAllJobs.Flakes
				bugs[bug.URL] = bug
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
		return 0.0
	}
	return float64(success) / float64(success+failure) * 100.0
}
