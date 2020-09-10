package testreportconversion

import (
	"sort"
	"strings"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

// testResultsByPassPercentage sorts from lowest to highest pass percentage
type testResultsByPassPercentage []sippyprocessingv1.TestResult

func (a testResultsByPassPercentage) Len() int      { return len(a) }
func (a testResultsByPassPercentage) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a testResultsByPassPercentage) Less(i, j int) bool {
	return a[i].PassPercentage < a[j].PassPercentage
}

func combineTestResults(lhs, rhs []sippyprocessingv1.TestResult) []sippyprocessingv1.TestResult {
	byTestName := map[string]sippyprocessingv1.TestResult{}

	for i := range lhs {
		currTestResult := lhs[i]
		byTestName[currTestResult.Name] = currTestResult
	}

	for i := range rhs {
		currTestResult := rhs[i]
		existing, ok := byTestName[currTestResult.Name]
		if !ok {
			byTestName[currTestResult.Name] = currTestResult
			continue
		}

		existing.Failures += currTestResult.Failures
		existing.Successes += currTestResult.Successes
		existing.Flakes += currTestResult.Flakes
		existing.PassPercentage = percent(existing.Successes, existing.Failures)
		// bugs should be the same for now.
		byTestName[currTestResult.Name] = existing
	}

	combined := []sippyprocessingv1.TestResult{}
	for _, currTestResult := range byTestName {
		combined = append(combined, currTestResult)
	}

	sort.Stable(testResultsByPassPercentage(combined))

	return combined
}

func convertRawTestResultToProcessedTestResult(
	rawTestResult testgridanalysisapi.RawTestResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
) sippyprocessingv1.TestResult {
	return sippyprocessingv1.TestResult{
		Name:           rawTestResult.Name,
		Successes:      rawTestResult.Successes,
		Failures:       rawTestResult.Failures,
		Flakes:         rawTestResult.Flakes,
		PassPercentage: percent(rawTestResult.Successes, rawTestResult.Failures),
		BugList:        bugCache.ListBugs(release, "", rawTestResult.Name),
	}
}

func convertRawTestResultsToProcessedTestResults(
	rawTestResults map[string]testgridanalysisapi.RawTestResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
) []sippyprocessingv1.TestResult {

	ret := []sippyprocessingv1.TestResult{}

	for _, rawTestResult := range rawTestResults {
		ret = append(ret, convertRawTestResultToProcessedTestResult(rawTestResult, bugCache, release))
	}

	sort.Stable(testResultsByPassPercentage(ret))

	return ret
}

func filterTestResults(
	testResults []sippyprocessingv1.TestResult,
	minRuns int, // indicates how many runs are required for a test is included in overall percentages
	// TODO deads2k wants to eliminate the successThreshold
	successThreshold float64, // indicates an upper bound on how successful a test can be before it is excluded
) []sippyprocessingv1.TestResult {

	filteredResults := []sippyprocessingv1.TestResult{}

	for i := range testResults {
		testResult := testResults[i]
		// we filter these our for display
		if testResult.Name == "Overall" || strings.HasSuffix(testResult.Name, "container setup") {
			continue
		}

		// strip out tests are more than N% successful
		if passPercentage := percent(testResult.Successes, testResult.Failures); passPercentage > successThreshold {
			continue
		}
		// strip out tests that have less than N total runs
		if testResult.Successes+testResult.Failures < minRuns {
			continue
		}

		filteredResults = append(filteredResults, testResult)
	}

	return filteredResults
}
