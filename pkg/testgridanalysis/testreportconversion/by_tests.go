package testreportconversion

import (
	"sort"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func getTopFailingTestsWithBug(jobResults []sippyprocessingv1.JobResult, testResultFilterFn testResultFilterFunc) []sippyprocessingv1.FailingTestResult {
	return getTopFailingTests(jobResults, func(testResult sippyprocessingv1.TestResult) bool {
		return len(testResult.BugList) > 0
	}, testResultFilterFn)
}

func getTopFailingTestsWithoutBug(jobResults []sippyprocessingv1.JobResult, testResultFilterFn testResultFilterFunc) []sippyprocessingv1.FailingTestResult {
	return getTopFailingTests(jobResults, func(testResult sippyprocessingv1.TestResult) bool {
		return len(testResult.BugList) == 0
	}, testResultFilterFn)
}

type testFilterFunc func(sippyprocessingv1.TestResult) bool

func getTopFailingTests(
	jobResults []sippyprocessingv1.JobResult,
	testFilterFn testFilterFunc,
	testResultFilterFn testResultFilterFunc,
) []sippyprocessingv1.FailingTestResult {

	topTests := []sippyprocessingv1.FailingTestResult{}

	type passFailPercent struct {
		name            string
		pass            int
		fail            int
		passFailPercent float64
	}
	testsToFailurePatterns := map[string]passFailPercent{}

	for _, jobResult := range jobResults {
		for _, testResult := range jobResult.TestResults {
			if !testFilterFn(testResult) {
				continue
			}
			currPassFailPercent := testsToFailurePatterns[testResult.Name]
			currPassFailPercent.name = testResult.Name
			currPassFailPercent.pass += testResult.Successes
			currPassFailPercent.fail += testResult.Failures
			currPassFailPercent.passFailPercent = percent(currPassFailPercent.pass, currPassFailPercent.fail)
			testsToFailurePatterns[testResult.Name] = currPassFailPercent
		}
	}

	failingTests := []passFailPercent{}
	for _, curr := range testsToFailurePatterns {
		if curr.fail == 0 {
			continue
		}
		if curr.fail+curr.pass < 10 {
			continue
		}
		failingTests = append(failingTests, curr)
	}
	sort.SliceStable(failingTests, func(i, j int) bool {
		return failingTests[i].passFailPercent < failingTests[j].passFailPercent
	})

	for _, failingTest := range failingTests {
		if len(topTests) >= 50 {
			break
		}

		failingTestResult := sippyprocessingv1.FailingTestResult{
			TestName:                failingTest.name,
			TestResultAcrossAllJobs: sippyprocessingv1.TestResult{Name: failingTest.name},
			JobResults:              nil,
		}
		for _, jobResult := range jobResults {
			for _, testResult := range jobResult.TestResults {
				if testResult.Name != failingTestResult.TestName {
					continue
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
		if !testResultFilterFn(failingTestResult.TestResultAcrossAllJobs) {
			continue
		}
		topTests = append(topTests, failingTestResult)
	}

	return topTests
}
