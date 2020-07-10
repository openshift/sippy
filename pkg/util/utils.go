package util

import (
	"fmt"
	//"io/ioutil"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
	"time"

	"k8s.io/klog"
)

var (
	sigRegex      *regexp.Regexp = regexp.MustCompile(`\[(sig-.*?)\]`)
	bugzillaRegex *regexp.Regexp = regexp.MustCompile(`(https://bugzilla.redhat.com/show_bug.cgi\?id=\d+)`)

	// platform regexes
	awsRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-aws-`)
	azureRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-azure-`)
	gcpRegex       *regexp.Regexp = regexp.MustCompile(`(?i)-gcp-`)
	openstackRegex *regexp.Regexp = regexp.MustCompile(`(?i)-openstack-`)
	metalRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-metal-`)
	metalIPIRegex  *regexp.Regexp = regexp.MustCompile(`(?i)-metal-ipi`)
	ovirtRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-ovirt-`)
	vsphereRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-vsphere-`)
	upgradeRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-upgrade-`)

	// ignored for top 10 failing test reporting
	// also ignored for doing bug lookup to determine if this is a known failure or not (these failures will typically not
	// have bugs associated, but we don't want the entire run marked as an unknown failure if one of them fails)
	IgnoreTestRegex *regexp.Regexp = regexp.MustCompile(`Run multi-stage test|operator.Import a release payload|operator.Run template|operator.Build image|Monitor cluster while tests execute|Overall|job.initialize`)
	// Tests we are already tracking an issue for
	//	KnownIssueTestRegex *regexp.Regexp = regexp.MustCompile(`Application behind service load balancer with PDB is not disrupted|Kubernetes and OpenShift APIs remain available|Cluster frontend ingress remain available|OpenShift APIs remain available|Kubernetes APIs remain available|Cluster upgrade should maintain a functioning cluster`)

	// TestBugCache is a map of test names to known bugs tied to those tests
	TestBugCache    map[string][]Bug = make(map[string][]Bug)
	TestBugCacheErr error
)

type TestMeta struct {
	Name       string
	Count      int
	Jobs       map[string]interface{}
	Sig        string
	BugList    []string
	BugErr     error
	BugFetched bool
}

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
	PassPercentage float64 `json:"passPercentage"`
	BugList        []Bug   `json:"BugList"`
	BugErr         error   `json:"BugErr"`
	SearchLink     string  `json:"searchLink"`
}

type JobRunResult struct {
	Job                string   `json:"job"`
	Url                string   `json:"url"`
	TestGridJobUrl     string   `json:"url"`
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

type BugList map[string]BugResult

type BugResult map[string][]Bug

type Bug struct {
	Summary      string `json:"name,omitempty"`
	ID           string `json:"id"`
	Url          string `json:"url"`
	FailureCount int32  `json:"failureCount,omitempty"`
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

	if len(platforms) == 0 {
		klog.V(2).Infof("unknown platform for job: %s\n", name)
		return []string{"unknown platform"}
	}
	return platforms
}

/*
func FindBug(testName string) ([]string, bool, error) {
	testName = regexp.QuoteMeta(testName)
	klog.V(4).Infof("Searching bugs for test name: %s\n", testName)

	bugs := []string{}
	query := url.QueryEscape(testName)
	resp, err := http.Get(fmt.Sprintf("https://search.svc.ci.openshift.org/search?search=%s&maxAge=48h&context=-1&type=bug", query))
	if err != nil {
		e := fmt.Errorf("error during bug search: %v", err)
		klog.Errorf(e.Error())
		return bugs, false, e
	}
	if resp.StatusCode != 200 {
		e := fmt.Errorf("Non-200 response code during bug search: %v", resp)
		klog.Errorf(e.Error())
		return bugs, false, e
	}

	//body, err := ioutil.ReadAll(resp.Body)
	bugList := BugList{}
	err = json.NewDecoder(resp.Body).Decode(&bugList)
	for k := range bugList {
		bugs = append(bugs, k)
	}
	klog.V(2).Infof("Found bugs: %v", bugs)
	return bugs, true, nil
}
*/

// GET
/*
func FindBugs(testNames []string) (map[string][]Bug, error) {
	searchResults := make(map[string][]Bug)

	query := []string{}
	for _, testName := range testNames {
		testName = regexp.QuoteMeta(testName)
		klog.V(4).Infof("Searching bugs for test name: %s\n", testName)
		query = append(query, fmt.Sprintf("search=%s", url.QueryEscape(testName)))
	}
	resp, err := http.Get(fmt.Sprintf("https://search.svc.ci.openshift.org/search?%s&maxAge=48h&context=-1&type=bug", strings.Join(query, "&")))
	if err != nil {
		e := fmt.Errorf("error during bug search: %v", err)
		klog.Errorf(e.Error())
		return searchResults, e
	}
	if resp.StatusCode != 200 {
		e := fmt.Errorf("Non-200 response code during bug search: %v", resp)
		klog.Errorf(e.Error())
		return searchResults, e
	}

	//body, err := ioutil.ReadAll(resp.Body)
	bugList := BugList{}
	err = json.NewDecoder(resp.Body).Decode(&bugList)

	for bugUrl, bugResult := range bugList {
		for searchString, results := range bugResult {
			// reverse the regex escaping we did earlier, so we get back the pure test name string.
			r, _ := syntax.Parse(searchString, 0)
			searchString = string(r.Rune)
			results[0].Url = bugUrl
			results[0].ID = strings.TrimPrefix(bugUrl, "https://bugzilla.redhat.com/show_bug.cgi?id=")
			searchResults[searchString] = append(searchResults[searchString], results[0])
		}
	}
	klog.V(2).Infof("Found bugs: %v", searchResults)
	return searchResults, nil
}
*/

// POST
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
	searchUrl := "https://search.ci.openshift.org/search"
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

	bugList := BugList{}
	err = json.NewDecoder(resp.Body).Decode(&bugList)

	for bugUrl, bugResult := range bugList {
		for searchString, results := range bugResult {
			// reverse the regex escaping we did earlier, so we get back the pure test name string.
			r, _ := syntax.Parse(searchString, 0)
			searchString = string(r.Rune)
			results[0].Url = bugUrl
			results[0].ID = strings.TrimPrefix(bugUrl, "https://bugzilla.redhat.com/show_bug.cgi?id=")
			searchResults[searchString] = append(searchResults[searchString], results[0])
		}
	}
	klog.V(2).Infof("Found bugs: %v", searchResults)
	return searchResults, nil
}

func AddTestResult(categoryKey string, categories map[string]AggregateTestResult, testName string, meta TestMeta, passed, failed int) {

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
	result.BugErr = meta.BugErr

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
