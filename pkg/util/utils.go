package util

import (
	"fmt"
	gohtml "html"
	"math"
	"regexp"
	"sort"
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util/sets"
	"k8s.io/klog"
)

var (
	sigRegex      *regexp.Regexp = regexp.MustCompile(`\[(sig-.*?)\]`)
	bugzillaRegex *regexp.Regexp = regexp.MustCompile(`(https://bugzilla.redhat.com/show_bug.cgi\?id=\d+)`)

	// platform regexes
	awsRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-aws-`)
	azureRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-azure-`)
	fipsRegex      *regexp.Regexp = regexp.MustCompile(`(?i)-fips-`)
	metalRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-metal-`)
	metalIPIRegex  *regexp.Regexp = regexp.MustCompile(`(?i)-metal-ipi`)
	gcpRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-gcp`)
	ocpRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-ocp-`)
	openstackRegex *regexp.Regexp = regexp.MustCompile(`(?i)-openstack-`)
	originRegex    *regexp.Regexp = regexp.MustCompile(`(?i)-origin-`)
	ovirtRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-ovirt-`)
	ovnRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-ovn-`)
	proxyRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-proxy`)
	ppc64leRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-ppc64le-`)
	rtRegex        *regexp.Regexp = regexp.MustCompile(`(?i)-rt-`)
	s390xRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-s390x-`)
	serialRegex    *regexp.Regexp = regexp.MustCompile(`(?i)-serial-`)
	upgradeRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-upgrade-`)
	vsphereRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-vsphere-`)

	// Tests we are already tracking an issue for
	//	KnownIssueTestRegex *regexp.Regexp = regexp.MustCompile(`Application behind service load balancer with PDB is not disrupted|Kubernetes and OpenShift APIs remain available|Cluster frontend ingress remain available|OpenShift APIs remain available|Kubernetes APIs remain available|Cluster upgrade should maintain a functioning cluster`)
)

func GetPrevTest(test string, testResults []sippyprocessingv1.TestResult) *sippyprocessingv1.TestResult {
	for _, v := range testResults {
		if v.Name == test {
			return &v
		}
	}
	return nil
}

func GetPrevJob(job string, jobRunsByJob []sippyprocessingv1.JobResult) *sippyprocessingv1.JobResult {
	for _, v := range jobRunsByJob {
		if v.Name == job {
			return &v
		}
	}
	return nil
}

func GetPrevPlatform(platform string, jobsByPlatform []sippyprocessingv1.JobResult) *sippyprocessingv1.JobResult {
	for _, v := range jobsByPlatform {
		if v.Platform == platform {
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

func SummarizeTestResults(
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
			passPercentage := Percent(rawTestResult.Successes, rawTestResult.Failures)

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

			testSearchUrl := gohtml.EscapeString(regexp.QuoteMeta(rawTestResult.Name))
			testSearchLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.ci.openshift.org/?maxAge=48h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", testSearchUrl, rawTestResult.Name)

			s := sorted[k]
			s.TestResults = append(s.TestResults, sippyprocessingv1.TestResult{
				Name:           rawTestResult.Name,
				Successes:      rawTestResult.Successes,
				Failures:       rawTestResult.Failures,
				Flakes:         rawTestResult.Flakes,
				PassPercentage: passPercentage,
				BugList:        bugCache.ListBugs(release, "", rawTestResult.Name),
				SearchLink:     testSearchLink,
			})
			sorted[k] = s
		}

		s := sorted[k]
		s.Successes = passedCount
		s.Failures = failedCount
		s.TestPassPercentage = Percent(passedCount, failedCount)
		sorted[k] = s

		// sort from lowest to highest
		sort.SliceStable(sorted[k].TestResults, func(i, j int) bool {
			return sorted[k].TestResults[i].PassPercentage < sorted[k].TestResults[j].PassPercentage
		})
	}
	return sorted
}

func GenerateSortedBugFailureCounts(
	allJobRuns map[string]testgridanalysisapi.RawJobRunResult,
	byAll map[string]sippyprocessingv1.SortedAggregateTestsResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
) []bugsv1.Bug {
	bugs := map[string]bugsv1.Bug{}

	failedTestNamesAcrossAllJobRuns := sets.NewString()
	for _, jobrun := range allJobRuns {
		failedTestNamesAcrossAllJobRuns.Insert(jobrun.FailedTestNames...)
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

func FilterFailureGroups(
	rawJRRs map[string]testgridanalysisapi.RawJobRunResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
	failureClusterThreshold int,
) []sippyprocessingv1.JobRunResult {
	filteredJrr := []sippyprocessingv1.JobRunResult{}
	// -1 means don't do this reporting.
	if failureClusterThreshold < 0 {
		return filteredJrr
	}
	for _, rawJRR := range rawJRRs {
		if rawJRR.TestFailures < failureClusterThreshold {
			continue
		}

		allFailuresKnown := areAllFailuresKnown(rawJRR, bugCache, release)
		hasUnknownFailure := rawJRR.Failed && !allFailuresKnown

		filteredJrr = append(filteredJrr, sippyprocessingv1.JobRunResult{
			Job:                rawJRR.Job,
			Url:                rawJRR.Url,
			TestGridJobUrl:     rawJRR.TestGridJobUrl,
			TestFailures:       rawJRR.TestFailures,
			FailedTestNames:    rawJRR.FailedTestNames,
			Failed:             rawJRR.Failed,
			HasUnknownFailures: hasUnknownFailure,
			Succeeded:          rawJRR.Succeeded,
		})
	}

	// sort from highest to lowest
	sort.SliceStable(filteredJrr, func(i, j int) bool {
		return filteredJrr[i].TestFailures > filteredJrr[j].TestFailures
	})

	return filteredJrr
}

func areAllFailuresKnown(
	rawJRR testgridanalysisapi.RawJobRunResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question,
) bool {
	// check if all the test failures in the run can be attributed to
	// known bugs.  If not, the job run was an "unknown failure" that we cannot pretend
	// would have passed if all our bugs were fixed.
	allFailuresKnown := true
	for _, testName := range rawJRR.FailedTestNames {
		bugs := bugCache.ListBugs(release, "", testName)
		isKnownFailure := len(bugs) > 0
		if !isKnownFailure {
			allFailuresKnown = false
			break
		}
	}
	return allFailuresKnown
}

func SummarizeJobRunResults(
	rawJRRs map[string]testgridanalysisapi.RawJobRunResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question,
) []sippyprocessingv1.JobResult {
	jobsMap := make(map[string]sippyprocessingv1.JobResult)

	for _, rawJRR := range rawJRRs {
		job, ok := jobsMap[rawJRR.Job]
		if !ok {
			job = sippyprocessingv1.JobResult{
				Name:        rawJRR.Job,
				TestGridUrl: rawJRR.TestGridJobUrl,
			}
		}
		if rawJRR.Failed {
			job.Failures++
		} else if rawJRR.Succeeded {
			job.Successes++
		}
		if rawJRR.Failed && areAllFailuresKnown(rawJRR, bugCache, release) {
			job.KnownFailures++
		}
		jobsMap[rawJRR.Job] = job
	}
	jobs := []sippyprocessingv1.JobResult{}
	for _, job := range jobsMap {
		job.PassPercentage = Percent(job.Successes, job.Failures)
		job.PassPercentageWithKnownFailures = Percent(job.Successes+job.KnownFailures, job.Failures-job.KnownFailures)
		jobs = append(jobs, job)
	}

	// sort from lowest to highest
	sort.SliceStable(jobs, func(i, j int) bool {
		return jobs[i].PassPercentage < jobs[j].PassPercentage
	})

	return jobs
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

// find associated sig from test name
func FindSig(name string) string {
	match := sigRegex.FindStringSubmatch(name)
	if len(match) > 1 {
		return match[1]
	}
	return "sig-unknown"
}

func FindPlatform(name string) []string {
	platforms := []string{}
	if ocpRegex.MatchString(name) {
		platforms = append(platforms, "ocp")
	}
	if originRegex.MatchString(name) {
		platforms = append(platforms, "origin")
	}
	if awsRegex.MatchString(name) {
		platforms = append(platforms, "aws")
	}
	if azureRegex.MatchString(name) {
		platforms = append(platforms, "azure")
	}
	if gcpRegex.MatchString(name) {
		platforms = append(platforms, "gcp")
	}
	if openstackRegex.MatchString(name) {
		platforms = append(platforms, "openstack")
	}

	// Without support for negative lookbacks in the native
	// regexp library, it's easiest to differentiate these
	// two by seeing if it's metal-ipi, and then fall through
	// to check if it's UPI metal.
	if metalIPIRegex.MatchString(name) {
		platforms = append(platforms, "metal-ipi")
	} else if metalRegex.MatchString(name) {
		platforms = append(platforms, "metal")
	}

	if ovirtRegex.MatchString(name) {
		platforms = append(platforms, "ovirt")
	}
	if vsphereRegex.MatchString(name) {
		platforms = append(platforms, "vsphere")
	}
	if upgradeRegex.MatchString(name) {
		platforms = append(platforms, "upgrade")
	}
	if serialRegex.MatchString(name) {
		platforms = append(platforms, "serial")
	}
	if ovnRegex.MatchString(name) {
		platforms = append(platforms, "ovn")
	}
	if fipsRegex.MatchString(name) {
		platforms = append(platforms, "fips")
	}
	if ppc64leRegex.MatchString(name) {
		platforms = append(platforms, "ppc64le")
	}
	if s390xRegex.MatchString(name) {
		platforms = append(platforms, "s390x")
	}
	if rtRegex.MatchString(name) {
		platforms = append(platforms, "rt")
	}
	if proxyRegex.MatchString(name) {
		platforms = append(platforms, "proxy")
	}

	if len(platforms) == 0 {
		klog.V(2).Infof("unknown platform for job: %s\n", name)
		return []string{"unknown platform"}
	}
	return platforms
}

func AddTestResult(categoryKey string, categories map[string]testgridanalysisapi.AggregateTestsResult, testName string, passed, failed, flaked int) {

	klog.V(4).Infof("Adding test %s to category %s, passed: %d, failed: %d\n", testName, categoryKey, passed, failed)
	category, ok := categories[categoryKey]
	if !ok {
		category = testgridanalysisapi.AggregateTestsResult{
			RawTestResults: make(map[string]testgridanalysisapi.RawTestResult),
		}
	}

	result, ok := category.RawTestResults[testName]
	if !ok {
		result = testgridanalysisapi.RawTestResult{}
	}
	result.Name = testName
	result.Successes += passed
	result.Failures += failed
	result.Flakes += flaked

	category.RawTestResults[testName] = result

	categories[categoryKey] = category
}

func SummarizeJobsByPlatform(report sippyprocessingv1.TestReport) []sippyprocessingv1.JobResult {
	jobRunsByPlatform := make(map[string]sippyprocessingv1.JobResult)
	platformResults := []sippyprocessingv1.JobResult{}

	for _, job := range report.JobPassRate {
		platforms := FindPlatform(job.Name)
		for _, p := range platforms {
			j := jobRunsByPlatform[p]
			j.Successes += job.Successes
			j.Failures += job.Failures
			j.KnownFailures += job.KnownFailures
			j.Platform = p
			jobRunsByPlatform[p] = j
		}
	}

	for _, platform := range jobRunsByPlatform {

		platform.PassPercentage = Percent(platform.Successes, platform.Failures)
		platform.PassPercentageWithKnownFailures = Percent(platform.Successes+platform.KnownFailures, platform.Failures-platform.KnownFailures)
		platformResults = append(platformResults, platform)
	}
	// sort from lowest to highest
	sort.SliceStable(platformResults, func(i, j int) bool {
		return platformResults[i].PassPercentage < platformResults[j].PassPercentage
	})
	return platformResults
}

func SummarizeJobsByName(report sippyprocessingv1.TestReport) []sippyprocessingv1.JobResult {
	jobRunsByName := make(map[string]sippyprocessingv1.JobResult)
	jobResults := []sippyprocessingv1.JobResult{}

	for _, job := range report.JobPassRate {
		j := jobRunsByName[job.Name]
		j.Name = job.Name
		j.TestGridUrl = job.TestGridUrl
		j.Successes += job.Successes
		j.Failures += job.Failures
		j.KnownFailures += job.KnownFailures
		jobRunsByName[job.Name] = j
	}

	for _, job := range jobRunsByName {

		job.PassPercentage = Percent(job.Successes, job.Failures)
		job.PassPercentageWithKnownFailures = Percent(job.Successes+job.KnownFailures, job.Failures-job.KnownFailures)
		jobResults = append(jobResults, job)
	}
	// sort from lowest to highest
	sort.SliceStable(jobResults, func(i, j int) bool {
		return jobResults[i].PassPercentage < jobResults[j].PassPercentage
	})
	return jobResults
}
