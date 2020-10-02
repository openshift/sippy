package testreportconversion

import (
	"sort"
	"strings"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

	"github.com/openshift/sippy/pkg/util/sets"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func getTopFailingTestsWithBug(testResultsByName testResultsByName, testResultFilterFn testResultFilterFunc) []sippyprocessingv1.FailingTestResult {
	return getTopFailingTests(testResultsByName, func(testResult sippyprocessingv1.TestResult) bool {
		return len(testResult.BugList) > 0
	}, testResultFilterFn)
}

func getTopFailingTestsWithoutBug(testResultsByName testResultsByName, testResultFilterFn testResultFilterFunc) []sippyprocessingv1.FailingTestResult {
	return getTopFailingTests(testResultsByName, func(testResult sippyprocessingv1.TestResult) bool {
		return len(testResult.BugList) == 0
	}, testResultFilterFn)
}

func getCuratedTests(release string, testResultsByName testResultsByName) []sippyprocessingv1.FailingTestResult {
	return getTopFailingTests(testResultsByName, func(testResult sippyprocessingv1.TestResult) bool {
		return testidentification.IsCuratedTest(release, testResult.Name)
	}, acceptAllTests)
}

type testResultsByName map[string]sippyprocessingv1.FailingTestResult

func (a testResultsByName) toOrderedList() []sippyprocessingv1.FailingTestResult {
	ret := []sippyprocessingv1.FailingTestResult{}

	for _, testResult := range a {
		ret = append(ret, testResult)
	}

	sort.Stable(failingTestResultByPassPercentage(ret))
	return ret
}

// getTestResultsByName takes the job results and returns a map of testNames to results for that particular test across
// all the jobs.
func getTestResultsByName(jobResults []sippyprocessingv1.JobResult) testResultsByName {
	testsByName := testResultsByName{}

	testNames := sets.NewString()
	for _, jobResult := range jobResults {
		for _, testResult := range jobResult.TestResults {
			testNames.Insert(testResult.Name)
		}
	}

	for _, testName := range testNames.UnsortedList() {
		failingTestResult := sippyprocessingv1.FailingTestResult{
			TestName:                testName,
			TestResultAcrossAllJobs: sippyprocessingv1.TestResult{Name: testName},
			JobResults:              nil,
		}

		for _, jobResult := range jobResults {
			for _, testResult := range jobResult.TestResults {
				if testResult.Name != failingTestResult.TestName {
					continue
				}
				// don't include jobs that didn't run the test.
				if testResult.Successes+testResult.Failures == 0 {
					break
				}

				failingTestResult.TestResultAcrossAllJobs = combineTestResult(failingTestResult.TestResultAcrossAllJobs, testResult)

				failingTestResult.JobResults = append(failingTestResult.JobResults, sippyprocessingv1.FailingTestJobResult{
					Name:           jobResult.Name,
					TestFailures:   testResult.Failures,
					TestSuccesses:  testResult.Successes,
					PassPercentage: testResult.PassPercentage,
					TestGridUrl:    jobResult.TestGridUrl,
				})
				break
			}
		}

		sort.Stable(failingTestJobResultByJobPassPercentage(failingTestResult.JobResults))

		testsByName[testName] = failingTestResult
	}

	return testsByName
}

// failingTestJobResultByJobPassPercentage sorts from lowest to highest pass percentage
type failingTestJobResultByJobPassPercentage []sippyprocessingv1.FailingTestJobResult

func (a failingTestJobResultByJobPassPercentage) Len() int      { return len(a) }
func (a failingTestJobResultByJobPassPercentage) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a failingTestJobResultByJobPassPercentage) Less(i, j int) bool {
	if a[i].PassPercentage < a[j].PassPercentage {
		return true
	}
	if a[i].PassPercentage > a[j].PassPercentage {
		return false
	}
	if strings.Compare(a[i].Name, a[j].Name) < 0 {
		return true
	}
	return false
}

// failingTestJobResultByJobPassPercentage sorts from lowest to highest pass percentage
type failingTestResultByPassPercentage []sippyprocessingv1.FailingTestResult

func (a failingTestResultByPassPercentage) Len() int      { return len(a) }
func (a failingTestResultByPassPercentage) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a failingTestResultByPassPercentage) Less(i, j int) bool {
	if a[i].TestResultAcrossAllJobs.PassPercentage < a[j].TestResultAcrossAllJobs.PassPercentage {
		return true
	}
	if a[i].TestResultAcrossAllJobs.PassPercentage > a[j].TestResultAcrossAllJobs.PassPercentage {
		return false
	}
	if strings.Compare(a[i].TestResultAcrossAllJobs.Name, a[j].TestResultAcrossAllJobs.Name) < 0 {
		return true
	}
	return false
}

type testFilterFunc func(sippyprocessingv1.TestResult) bool

func getTopFailingTests(
	testResultsByName testResultsByName,
	testFilterFn testFilterFunc,
	testResultFilterFn testResultFilterFunc,
) []sippyprocessingv1.FailingTestResult {

	topTests := []sippyprocessingv1.FailingTestResult{}

	for _, testResult := range testResultsByName {
		if !testFilterFn(testResult.TestResultAcrossAllJobs) {
			continue
		}
		if !testResultFilterFn(testResult.TestResultAcrossAllJobs) {
			continue
		}

		newInstance := sippyprocessingv1.FailingTestResult{
			TestName:                testResult.TestName,
			TestResultAcrossAllJobs: testResult.TestResultAcrossAllJobs,
			JobResults:              nil,
		}
		for _, jobResult := range testResult.JobResults {
			// if the job hasn't run at least 7 times, don't add it to the list
			if jobResult.TestFailures+jobResult.TestSuccesses < 7 && jobResult.TestFailures == 0 {
				continue
			}
			newInstance.JobResults = append(newInstance.JobResults, jobResult)
		}
		topTests = append(topTests, testResult)
	}

	sort.Stable(failingTestResultByPassPercentage(topTests))

	if len(topTests) < 50 {
		return topTests
	}
	return topTests[:50]
}
