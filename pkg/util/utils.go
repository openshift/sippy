package util

import (
	"regexp"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func FindFailedTestResult(test string, testResults []sippyprocessingv1.FailingTestResult) *sippyprocessingv1.FailingTestResult {
	for _, v := range testResults {
		if v.TestName == test {
			return &v
		}
	}
	return nil
}

func FindJobResultForJobName(job string, jobRunsByJob []sippyprocessingv1.JobResult) *sippyprocessingv1.JobResult {
	for _, v := range jobRunsByJob {
		if v.Name == job {
			return &v
		}
	}
	return nil
}

func FindPlatformResultsForName(platform string, allPlatforms []sippyprocessingv1.PlatformResults) *sippyprocessingv1.PlatformResults {
	for _, v := range allPlatforms {
		if v.PlatformName == platform {
			return &v
		}
	}
	return nil
}

func FindBugzillaJobFailures(bzComponent string, bugzillaJobFailures []sippyprocessingv1.SortedBugzillaComponentResult) *sippyprocessingv1.SortedBugzillaComponentResult {
	for _, v := range bugzillaJobFailures {
		if v.Name == bzComponent {
			return &v
		}
	}
	return nil
}

// ComputeFailureGroupStats computes count, median, and average number of failuregroups
// returns count, countPrev, median, medianPrev, avg, avgPrev
func ComputeFailureGroupStats(failureGroups, failureGroupsPrev []sippyprocessingv1.JobRunResult) (int, int, int, int, int, int) {
	count, countPrev, median, medianPrev, avg, avgPrev := 0, 0, 0, 0, 0, 0
	for _, group := range failureGroups {
		count += group.TestFailures
	}
	for _, group := range failureGroupsPrev {
		countPrev += group.TestFailures
	}
	if len(failureGroups) != 0 {
		median = failureGroups[len(failureGroups)/2].TestFailures
		avg = count / len(failureGroups)
	}
	if len(failureGroupsPrev) != 0 {
		medianPrev = failureGroupsPrev[len(failureGroupsPrev)/2].TestFailures
		avgPrev = count / len(failureGroupsPrev)
	}
	return count, countPrev, median, medianPrev, avg, avgPrev
}

func RelevantJob(jobName, status string, filter *regexp.Regexp) bool {
	if filter != nil && !filter.MatchString(jobName) {
		return false
	}
	return true
	/*
		switch status {
		case "FAILING", "FLAKY":
			return true
		}
		return false
	*/
}
