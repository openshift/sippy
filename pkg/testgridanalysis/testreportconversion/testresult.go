package testreportconversion

import (
	"fmt"
	"sort"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

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

func combineTestResult(lhs, rhs sippyprocessingv1.TestResult) sippyprocessingv1.TestResult {
	if lhs.Name != rhs.Name {
		panic(fmt.Sprintf("coding error: %q %q", lhs.Name, rhs.Name))
	}

	// shallow copy
	combined := lhs
	combined.Failures += rhs.Failures
	combined.Successes += rhs.Successes
	combined.Flakes += rhs.Flakes
	combined.PassPercentage = percent(combined.Successes, combined.Failures)
	combined.BugList = combineBugLists(lhs.BugList, rhs.BugList)

	return combined
}

func combineBugLists(lhs, rhs []bugsv1.Bug) []bugsv1.Bug {
	combined := []bugsv1.Bug{}
	for _, curr := range lhs {
		combined = append(combined, curr)
	}
	for _, curr := range rhs {
		if existing := findBug(curr.ID, combined); existing != nil {
			continue
		}
		combined = append(combined, curr)
	}

	return combined
}

func findBug(id int64, haystack []bugsv1.Bug) *bugsv1.Bug {
	for _, curr := range haystack {
		if curr.ID == id {
			return &curr
		}
	}
	return nil
}

func convertRawTestResultToProcessedTestResult(
	jobName string,
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
		BugList:        bugCache.ListBugs(release, jobName, rawTestResult.Name),
	}
}

func convertRawTestResultsToProcessedTestResults(
	jobName string,
	rawTestResults map[string]testgridanalysisapi.RawTestResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
) []sippyprocessingv1.TestResult {

	ret := []sippyprocessingv1.TestResult{}

	for _, rawTestResult := range rawTestResults {
		ret = append(ret, convertRawTestResultToProcessedTestResult(jobName, rawTestResult, bugCache, release))
	}

	sort.Stable(testResultsByPassPercentage(ret))

	return ret
}

type TestResultFilterFunc func(sippyprocessingv1.TestResult) bool

func acceptAllTests(testResult sippyprocessingv1.TestResult) bool {
	return true
}

func FilterSuccessfulTestResults(successThreshold float64 /*indicates an upper bound on how successful a test can be before it is excluded*/) TestResultFilterFunc {
	return func(testResult sippyprocessingv1.TestResult) bool {
		if testResult.PassPercentage > successThreshold {
			return false
		}
		return true
	}
}

func FilterLowValueTestsByName(testResult sippyprocessingv1.TestResult) bool {
	if testResult.Name == "Overall" || testidentification.IsSetupContainerEquivalent(testResult.Name) {
		return false
	}
	return true
}

func IsHighValueTestsByName(testResult sippyprocessingv1.TestResult) bool {
	if testidentification.IsInstallOperatorTest(testResult.Name) {
		return true
	}
	if testidentification.IsUpgradeRelatedTest(testResult.Name) {
		return true
	}
	return false
}

func FilterTooFewTestRuns(minRuns int /*indicates how many runs are required for a test is included in overall percentages*/) TestResultFilterFunc {
	return func(testResult sippyprocessingv1.TestResult) bool {
		if testResult.Successes+testResult.Failures < minRuns {
			return false
		}
		return true
	}
}

func FilterTestResultsByFilters(fns ...TestResultFilterFunc) TestResultFilterFunc {
	return func(testResult sippyprocessingv1.TestResult) bool {
		for _, fn := range fns {
			if !fn(testResult) {
				return false
			}
		}
		return true
	}
}

type TestResultFilterFuncs []TestResultFilterFunc

func (fns TestResultFilterFuncs) And(testResult sippyprocessingv1.TestResult) bool {
	for _, fn := range fns {
		if !fn(testResult) {
			return false
		}
	}
	return true
}

func (fns TestResultFilterFuncs) Or(testResult sippyprocessingv1.TestResult) bool {
	for _, fn := range fns {
		if fn(testResult) {
			return true
		}
	}
	return false
}

func StandardTestResultFilter(
	minRuns int, // indicates how many runs are required for a test is included in overall percentages
	// TODO deads2k wants to eliminate the successThreshold
	successThreshold float64, // indicates an upper bound on how successful a test can be before it is excluded
) TestResultFilterFunc {
	return TestResultFilterFuncs{
		IsHighValueTestsByName,
		TestResultFilterFuncs{
			FilterLowValueTestsByName,
			FilterTooFewTestRuns(minRuns),
			FilterSuccessfulTestResults(successThreshold),
		}.And,
	}.Or
}

func (filterFn TestResultFilterFunc) FilterTestResults(testResults []sippyprocessingv1.TestResult) []sippyprocessingv1.TestResult {
	filteredResults := []sippyprocessingv1.TestResult{}

	for i := range testResults {
		testResult := testResults[i]
		if !filterFn(testResult) {
			continue
		}
		filteredResults = append(filteredResults, testResult)
	}

	return filteredResults
}

func excludeNeverStableJobs(in sippyprocessingv1.FailingTestResult) sippyprocessingv1.FailingTestResult {
	filteredFailingTestResult := sippyprocessingv1.FailingTestResult{
		TestName:                in.TestName,
		TestResultAcrossAllJobs: sippyprocessingv1.TestResult{Name: in.TestName},
		JobResults:              nil,
	}

	for _, jobResult := range in.JobResults {
		if testidentification.IsJobNeverStable(jobResult.Name) {
			continue
		}
		filteredFailingTestResult.JobResults = append(filteredFailingTestResult.JobResults, jobResult)
	}
	sort.Stable(failingTestJobResultByJobPassPercentage(filteredFailingTestResult.JobResults))

	for _, jobResult := range filteredFailingTestResult.JobResults {
		filteredFailingTestResult.TestResultAcrossAllJobs.BugList = in.TestResultAcrossAllJobs.BugList
		filteredFailingTestResult.TestResultAcrossAllJobs.Successes += jobResult.TestSuccesses
		filteredFailingTestResult.TestResultAcrossAllJobs.Failures += jobResult.TestFailures
	}
	filteredFailingTestResult.TestResultAcrossAllJobs.PassPercentage = percent(filteredFailingTestResult.TestResultAcrossAllJobs.Successes, filteredFailingTestResult.TestResultAcrossAllJobs.Failures)

	return filteredFailingTestResult
}
