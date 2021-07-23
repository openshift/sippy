package util

import (
	"regexp"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
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

func FindTestResult(test string, testResults []sippyprocessingv1.TestResult) *sippyprocessingv1.TestResult {
	for _, v := range testResults {
		if v.Name == test {
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

func FindVariantResultsForName(variant string, allVariants []sippyprocessingv1.VariantResults) *sippyprocessingv1.VariantResults {
	for _, v := range allVariants {
		if v.VariantName == variant {
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

type FailureGroupStats struct {
	Count      int
	CountPrev  int
	Median     int
	MedianPrev int
	Avg        int
	AvgPrev    int
}

// ComputeFailureGroupStats computes count, median, and average number of failuregroups
// returns FailureGroupStats containing count, countPrev, median, medianPrev, avg, avgPrev
func ComputeFailureGroupStats(failureGroups, failureGroupsPrev []sippyprocessingv1.JobRunResult) FailureGroupStats {
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

	return FailureGroupStats{
		Count:      count,
		CountPrev:  countPrev,
		Median:     median,
		MedianPrev: medianPrev,
		Avg:        avg,
		AvgPrev:    avgPrev,
	}
}

func RelevantJob(jobName, status string, filter *regexp.Regexp) bool {
	if filter != nil && !filter.MatchString(jobName) {
		return false
	}
	return true
}

func IsActiveBug(bug bugsv1.Bug) bool {
	switch bug.Status {
	case "VERIFIED", "RELEASE_PENDING", "CLOSED":
		return false
	default:
		return true
	}
}
