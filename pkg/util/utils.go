package util

import (
	"fmt"
	//	"io/ioutil"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"regexp/syntax"
	"sort"

	//	"strings"
	"time"

	"k8s.io/klog"
)

var (
	sigRegex      *regexp.Regexp = regexp.MustCompile(`\[(sig-.*?)\]`)
	bugzillaRegex *regexp.Regexp = regexp.MustCompile(`(https://bugzilla.redhat.com/show_bug.cgi\?id=\d+)`)

	// platform regexes
	ocpRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-ocp-`)
	originRegex    *regexp.Regexp = regexp.MustCompile(`(?i)-origin-`)
	awsRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-aws-`)
	azureRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-azure-`)
	gcpRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-gcp`)
	openstackRegex *regexp.Regexp = regexp.MustCompile(`(?i)-openstack-`)
	fipsRegex      *regexp.Regexp = regexp.MustCompile(`(?i)-fips-`)
	ovnRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-ovn-`)
	metalRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-metal-`)
	metalIPIRegex  *regexp.Regexp = regexp.MustCompile(`(?i)-metal-ipi`)
	ovirtRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-ovirt-`)
	vsphereRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-vsphere-`)
	upgradeRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-upgrade-`)
	serialRegex    *regexp.Regexp = regexp.MustCompile(`(?i)-serial-`)
	ppc64leRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-ppc64le-`)
	s390xRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-s390x-`)

	// ignored for top 10 failing test reporting
	// also ignored for doing bug lookup to determine if this is a known failure or not (these failures will typically not
	// have bugs associated, but we don't want the entire run marked as an unknown failure if one of them fails)
	IgnoreTestRegex *regexp.Regexp = regexp.MustCompile(`^Run template.*container test|^Import the release payload|^Import a release payload|^Build image.*from the repository$|Monitor cluster while tests execute|Overall|job.initialize|\[sig-arch\]\[Feature:ClusterUpgrade\] Cluster should remain functional during upgrade`)
	// Tests we are already tracking an issue for
	//	KnownIssueTestRegex *regexp.Regexp = regexp.MustCompile(`Application behind service load balancer with PDB is not disrupted|Kubernetes and OpenShift APIs remain available|Cluster frontend ingress remain available|OpenShift APIs remain available|Kubernetes APIs remain available|Cluster upgrade should maintain a functioning cluster`)

	// TestBugCache is a map of test names to known bugs tied to those tests
	TestBugCache    map[string][]Bug = make(map[string][]Bug)
	TestBugCacheErr error
)

type TestReport struct {
	Release                   string                               `json:"release"`
	All                       map[string]SortedAggregateTestResult `json:"all"`
	ByPlatform                map[string]SortedAggregateTestResult `json:"byPlatform`
	ByJob                     map[string]SortedAggregateTestResult `json:"byJob`
	BySig                     map[string]SortedAggregateTestResult `json:"bySig`
	FailureGroups             []JobRunResult                       `json:"failureGroups"`
	JobPassRate               []JobResult                          `json:"jobPassRate"`
	Timestamp                 time.Time                            `json:"timestamp"`
	TopFailingTestsWithBug    []*TestResult                        `json:"topFailingTestsWithBug"`
	TopFailingTestsWithoutBug []*TestResult                        `json:"topFailingTestsWithoutBug"`
	BugsByFailureCount        []Bug                                `json:"bugsByFailureCount"`
}

type SortedAggregateTestResult struct {
	Successes          int          `json:"successes"`
	Failures           int          `json:"failures"`
	TestPassPercentage float64      `json:"testPassPercentage"`
	TestResults        []TestResult `json:"results"`
}

type AggregateTestResult struct {
	Successes          int                   `json:"successes"`
	Failures           int                   `json:"failures"`
	TestPassPercentage float64               `json:"testPassPercentage"`
	TestResults        map[string]TestResult `json:"results"`
}

type TestResult struct {
	Name           string  `json:"name"`
	Successes      int     `json:"successes"`
	Failures       int     `json:"failures"`
	Flakes         int     `json:"flakes"`
	PassPercentage float64 `json:"passPercentage"`
	BugList        []Bug   `json:"BugList"`
	SearchLink     string  `json:"searchLink"`
}

type JobRunResult struct {
	Job                string   `json:"job"`
	Url                string   `json:"url"`
	TestGridJobUrl     string   `json:"testGridJobUrl"`
	TestFailures       int      `json:"testFailures"`
	FailedTestNames    []string `json:"failedTestNames"`
	Failed             bool     `json:"failed"`
	HasUnknownFailures bool     `json:"hasUnknownFailures"`
	Succeeded          bool     `json:"succeeded"`
}

type JobResult struct {
	Name                            string  `json:"name"`
	Platform                        string  `json:"platform"`
	Failures                        int     `json:"failures"`
	KnownFailures                   int     `json:"knownFailures"`
	Successes                       int     `json:"successes"`
	PassPercentage                  float64 `json:"PassPercentage"`
	PassPercentageWithKnownFailures float64 `json:"PassPercentageWithKnownFailures"`
	TestGridUrl                     string  `json:"TestGridUrl"`
}

type Search struct {
	Results Results `json:"results"`
}

// search string is the key
type Results map[string]Result

type Result struct {
	Matches []Match `json:"matches"`
}

type Match struct {
	Bug Bug `json:"bugInfo"`
}

type Bug struct {
	ID             int64     `json:"id"`
	Status         string    `json:"status"`
	LastChangeTime time.Time `json:"last_change_time"`
	Summary        string    `json:"summary"`
	TargetRelease  []string  `json:"target_release"`
	Component      []string  `json:"component"`
	Url            string    `json:"url"`
	FailureCount   int       `json:"failureCount,omitempty"`
	FlakeCount     int       `json:"flakeCount,omitempty"`
}

func GetPrevTest(test string, testResults []TestResult) *TestResult {
	for _, v := range testResults {
		if v.Name == test {
			return &v
		}
	}
	return nil
}

func GetPrevJob(job string, jobRunsByJob []JobResult) *JobResult {
	for _, v := range jobRunsByJob {
		if v.Name == job {
			return &v
		}
	}
	return nil
}

func GetPrevPlatform(platform string, jobsByPlatform []JobResult) *JobResult {
	for _, v := range jobsByPlatform {
		if v.Platform == platform {
			return &v
		}
	}
	return nil
}

// ComputeFailureGroupStats computes count, median, and average number of failuregroups
// returns count, countPrev, median, medianPrev, avg, avgPrev
func ComputeFailureGroupStats(failureGroups, failureGroupsPrev []JobRunResult) (int, int, int, int, int, int) {
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

func ComputePercentages(AggregateTestResults map[string]AggregateTestResult) {
	for k, AggregateTestResult := range AggregateTestResults {
		AggregateTestResult.TestPassPercentage = Percent(AggregateTestResult.Successes, AggregateTestResult.Failures)
		for k2, r := range AggregateTestResult.TestResults {
			r.PassPercentage = Percent(r.Successes, r.Failures)
			AggregateTestResult.TestResults[k2] = r
		}
		AggregateTestResults[k] = AggregateTestResult
	}
}

func GenerateSortedResults(AggregateTestResult map[string]AggregateTestResult, minRuns int, successThreshold float64) map[string]SortedAggregateTestResult {
	sorted := make(map[string]SortedAggregateTestResult)

	for k, v := range AggregateTestResult {
		sorted[k] = SortedAggregateTestResult{
			Failures:           v.Failures,
			Successes:          v.Successes,
			TestPassPercentage: v.TestPassPercentage,
		}

		for _, result := range v.TestResults {
			// strip out tests are more than N% successful
			// strip out tests that have less than N total runs
			if (result.Successes+result.Failures >= minRuns) && result.PassPercentage < successThreshold {
				s := sorted[k]
				s.TestResults = append(s.TestResults, result)
				sorted[k] = s
			}

		}
		// sort from lowest to highest
		sort.SliceStable(sorted[k].TestResults, func(i, j int) bool {
			return sorted[k].TestResults[i].PassPercentage < sorted[k].TestResults[j].PassPercentage
		})
	}
	return sorted
}

func GenerateSortedBugFailureCounts(bugs map[string]Bug) []Bug {
	sortedBugs := []Bug{}
	for _, bug := range bugs {
		sortedBugs = append(sortedBugs, bug)
	}
	// sort from highest to lowest
	sort.SliceStable(sortedBugs, func(i, j int) bool {
		return sortedBugs[i].FailureCount > sortedBugs[j].FailureCount
	})
	return sortedBugs
}

func FilterFailureGroups(jrr map[string]JobRunResult, failureClusterThreshold int) []JobRunResult {
	filteredJrr := []JobRunResult{}
	// -1 means don't do this reporting.
	if failureClusterThreshold < 0 {
		return filteredJrr
	}
	for _, v := range jrr {
		if v.TestFailures > failureClusterThreshold {
			filteredJrr = append(filteredJrr, v)
		}
	}

	// sort from highest to lowest
	sort.SliceStable(filteredJrr, func(i, j int) bool {
		return filteredJrr[i].TestFailures > filteredJrr[j].TestFailures
	})

	return filteredJrr
}

func ComputeJobPassRate(jrr map[string]JobRunResult) []JobResult {
	jobsMap := make(map[string]JobResult)

	for _, run := range jrr {
		job, ok := jobsMap[run.Job]
		if !ok {
			job = JobResult{
				Name:        run.Job,
				TestGridUrl: run.TestGridJobUrl,
			}
		}
		if run.Failed {
			job.Failures++
		} else if run.Succeeded {
			job.Successes++
		}
		if run.Failed && !run.HasUnknownFailures {
			job.KnownFailures++
		}
		jobsMap[run.Job] = job
	}
	jobs := []JobResult{}
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

	if len(platforms) == 0 {
		klog.V(2).Infof("unknown platform for job: %s\n", name)
		return []string{"unknown platform"}
	}
	return platforms
}

func FindBugs(testNames []string) (map[string][]Bug, error) {
	searchResults := make(map[string][]Bug)

	v := url.Values{}
	v.Set("type", "bug")
	v.Set("context", "-1")
	for _, testName := range testNames {
		testName = regexp.QuoteMeta(testName)
		klog.V(4).Infof("Searching bugs for test name: %s\n", testName)
		v.Add("search", testName)
	}

	//searchUrl:="https://search.apps.build01.ci.devcluster.openshift.com/search"
	searchUrl := "https://search.ci.openshift.org/v2/search"
	resp, err := http.PostForm(searchUrl, v)
	if err != nil {
		e := fmt.Errorf("error during bug search against %s: %s", searchUrl, err)
		klog.Errorf(e.Error())
		return searchResults, e
	}
	if resp.StatusCode != 200 {
		e := fmt.Errorf("Non-200 response code during bug search against %s: %s", searchUrl, resp.Status)
		klog.Errorf(e.Error())
		return searchResults, e
	}

	search := Search{}
	err = json.NewDecoder(resp.Body).Decode(&search)

	for search, result := range search.Results {
		// reverse the regex escaping we did earlier, so we get back the pure test name string.
		r, _ := syntax.Parse(search, 0)
		search = string(r.Rune)
		for _, match := range result.Matches {
			bug := match.Bug
			bug.Url = fmt.Sprintf("https://bugzilla.redhat.com/show_bug.cgi?id=%d", bug.ID)

			// ignore any bugs verified over a week ago, they cannot be responsible for test failures
			// (or the bug was incorrectly verified and needs to be revisited)
			if bug.Status == "VERIFIED" {
				if bug.LastChangeTime.Add(time.Hour * 24 * 7).Before(time.Now()) {
					continue
				}
			}
			searchResults[search] = append(searchResults[search], bug)
		}
	}

	klog.V(2).Infof("Found bugs: %v", searchResults)
	return searchResults, nil
}

func AddTestResult(categoryKey string, categories map[string]AggregateTestResult, testName string, passed, failed, flaked int) {

	klog.V(4).Infof("Adding test %s to category %s, passed: %d, failed: %d\n", testName, categoryKey, passed, failed)
	category, ok := categories[categoryKey]
	if !ok {
		category = AggregateTestResult{
			TestResults: make(map[string]TestResult),
		}
	}

	category.Successes += passed
	category.Failures += failed

	result, ok := category.TestResults[testName]
	if !ok {
		result = TestResult{}
	}
	result.Name = testName
	result.Successes += passed
	result.Failures += failed
	result.Flakes += flaked

	category.TestResults[testName] = result

	categories[categoryKey] = category
}

func SummarizeJobsByPlatform(report TestReport) []JobResult {
	jobRunsByPlatform := make(map[string]JobResult)
	platformResults := []JobResult{}

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

func SummarizeJobsByName(report TestReport) []JobResult {
	jobRunsByName := make(map[string]JobResult)
	jobResults := []JobResult{}

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
