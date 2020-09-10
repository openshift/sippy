package util

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"k8s.io/klog"
)

var (
	bugzillaRegex *regexp.Regexp = regexp.MustCompile(`(https://bugzilla.redhat.com/show_bug.cgi\?id=\d+)`)
)

func GetPrevTest(test string, testResults []sippyprocessingv1.TestResult) *sippyprocessingv1.TestResult {
	for _, v := range testResults {
		if v.Name == test {
			return &v
		}
	}
	return nil
}

func GetJobResultForJobName(job string, jobRunsByJob []sippyprocessingv1.JobResult) *sippyprocessingv1.JobResult {
	for _, v := range jobRunsByJob {
		if v.Name == job {
			return &v
		}
	}
	return nil
}

func GetPlatform(platform string, allPlatforms []sippyprocessingv1.PlatformResults) *sippyprocessingv1.PlatformResults {
	for _, v := range allPlatforms {
		if v.PlatformName == platform {
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

func Percent(success, failure int) float64 {
	if success+failure == 0 {
		//return math.NaN()
		return 0.0
	}
	return float64(success) / float64(success+failure) * 100.0
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

func ComputeLookback(startday, lookback int, timestamps []int) (int, int) {

	stopTs := time.Now().Add(time.Duration(-1*lookback*24)*time.Hour).Unix() * 1000
	startTs := time.Now().Add(time.Duration(-1*startday*24)*time.Hour).Unix() * 1000
	klog.V(2).Infof("starttime: %d\nendtime: %d\n", startTs, stopTs)
	start := math.MaxInt32 // start is an int64 so leave overhead for wrapping to negative in case this gets incremented(it does).
	for i, t := range timestamps {
		if int64(t) < startTs && i < start {
			start = i
		}
		if int64(t) < stopTs {
			return start, i
		}
	}
	return start, len(timestamps)
}

func AddTestResultToCategory(categoryKey string, categories map[string]testgridanalysisapi.AggregateTestsResult, testName string, passed, failed, flaked int) {

	klog.V(4).Infof("Adding test %s to category %s, passed: %d, failed: %d\n", testName, categoryKey, passed, failed)
	category, ok := categories[categoryKey]
	if !ok {
		category = testgridanalysisapi.AggregateTestsResult{
			RawTestResults: make(map[string]testgridanalysisapi.RawTestResult),
		}
	}

	AddTestResult(category.RawTestResults, testName, passed, failed, flaked)

	categories[categoryKey] = category
}

func AddTestResult(testResults map[string]testgridanalysisapi.RawTestResult, testName string, passed, failed, flaked int) {
	result, ok := testResults[testName]
	if !ok {
		result = testgridanalysisapi.RawTestResult{}
	}
	result.Name = testName
	result.Successes += passed
	result.Failures += failed
	result.Flakes += flaked

	testResults[testName] = result
}

func SummarizeJobsFailuresByBugzillaComponent(report sippyprocessingv1.TestReport) []sippyprocessingv1.SortedBugzillaComponentResult {
	bzComponentResults := []sippyprocessingv1.SortedBugzillaComponentResult{}

	for _, bzJobFailures := range report.JobFailuresByBugzillaComponent {
		bzComponentResults = append(bzComponentResults, bzJobFailures)
	}
	// sort from highest to lowest
	sort.SliceStable(bzComponentResults, func(i, j int) bool {
		if bzComponentResults[i].JobsFailed[0].FailPercentage > bzComponentResults[j].JobsFailed[0].FailPercentage {
			return true
		}
		if bzComponentResults[i].JobsFailed[0].FailPercentage < bzComponentResults[j].JobsFailed[0].FailPercentage {
			return false
		}
		if strings.Compare(strings.ToLower(bzComponentResults[i].Name), strings.ToLower(bzComponentResults[j].Name)) < 0 {
			return true
		}
		return false
	})
	return bzComponentResults
}

func GetPrevBugzillaJobFailures(bzComponent string, bugzillaJobFailures []sippyprocessingv1.SortedBugzillaComponentResult) *sippyprocessingv1.SortedBugzillaComponentResult {
	for _, v := range bugzillaJobFailures {
		if v.Name == bzComponent {
			return &v
		}
	}
	return nil
}
