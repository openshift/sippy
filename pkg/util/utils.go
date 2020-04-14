package util

import (
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"regexp"
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
	ovirtRegex     *regexp.Regexp = regexp.MustCompile(`(?i)-ovirt-`)
	vsphereRegex   *regexp.Regexp = regexp.MustCompile(`(?i)-vsphere-`)
)

type TestMeta struct {
	Name  string
	Count int
	Jobs  map[string]interface{}
	Sig   string
	Bug   string
}

type TestReport struct {
	All           map[string]SortedAggregateTestResult `json:"all"`
	ByPlatform    map[string]SortedAggregateTestResult `json:"byPlatform`
	ByJob         map[string]SortedAggregateTestResult `json:"byJob`
	BySig         map[string]SortedAggregateTestResult `json:"bySig`
	FailureGroups []JobRunResult                       `json:"failureGroups"`
	JobPassRate   []JobResult                          `json:"jobPassRate"`
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
	Bug            string  `json:"bug"`
}

type JobRunResult struct {
	Job          string   `json:"job"`
	Url          string   `json:"url"`
	TestFailures int      `json:"testFailures"`
	TestNames    []string `json:"testNames"`
	Failed       bool     `json:"failed"`
	Succeeded    bool     `json:"succeeded"`
}

type JobResult struct {
	Name           string  `json:"name"`
	Platform       string  `json:"platform"`
	Failures       int     `json:"failures"`
	Successes      int     `json:"successes"`
	PassPercentage float64 `json:"PassPercentage"`
}

func Percent(success, failure int) float64 {
	if success+failure == 0 {
		return math.NaN()
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
				Name: run.Job,
			}
		}
		if run.Failed {
			job.Failures++
		} else if run.Succeeded {
			job.Successes++
		}
		jobsMap[run.Job] = job
	}
	jobs := []JobResult{}
	for _, job := range jobsMap {
		job.PassPercentage = Percent(job.Successes, job.Failures)
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

	switch status {
	case "FAILING", "FLAKY":
		return true
	}
	return false
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

func FindPlatform(name string) string {
	switch {
	case awsRegex.MatchString(name):
		return "aws"
	case azureRegex.MatchString(name):
		return "azure"
	case gcpRegex.MatchString(name):
		return "gcp"
	case openstackRegex.MatchString(name):
		return "openstack"
	case metalRegex.MatchString(name):
		return "metal"
	case ovirtRegex.MatchString(name):
		return "ovirt"
	case ovirtRegex.MatchString(name):
		return "ovirt"
	case vsphereRegex.MatchString(name):
		return "vsphere"
	}
	klog.V(2).Infof("unknown platform for job: %s\n", name)
	return "unknown platform"
}

func FindBug(testName string) string {
	testName = strings.ReplaceAll(testName, "[", "\\[")
	testName = strings.ReplaceAll(testName, "]", "\\]")
	klog.V(4).Infof("Searching bugs for test name: %s\n", testName)

	query := url.QueryEscape(testName)
	resp, err := http.Get(fmt.Sprintf("https://search.svc.ci.openshift.org/?search=%s&maxAge=48h&context=-1&type=bug", query))
	if err != nil {
		return fmt.Sprintf("error during bug retrieval: %v", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Sprintf("Non-200 response code doing bug search: %v", resp)
	}
	body, err := ioutil.ReadAll(resp.Body)
	match := bugzillaRegex.FindStringSubmatch(string(body))
	if len(match) > 1 {
		return match[1]
	}

	return "no bug found"
}

func AddTestResult(categoryKey string, categories map[string]AggregateTestResult, testName string, meta TestMeta, passed, failed int) {

	klog.V(2).Infof("Adding test %s to category %s, passed: %d, failed: %d\n", testName, categoryKey, passed, failed)
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
	result.Bug = meta.Bug

	category.TestResults[testName] = result

	categories[categoryKey] = category
}

func SummarizeJobsByPlatform(report TestReport) []JobResult {
	jobRunsByPlatform := make(map[string]JobResult)
	platformResults := []JobResult{}

	for _, job := range report.JobPassRate {
		p := FindPlatform(job.Name)
		j := jobRunsByPlatform[p]
		j.Successes += job.Successes
		j.Failures += job.Failures
		j.Platform = p
		jobRunsByPlatform[p] = j
	}

	for _, platform := range jobRunsByPlatform {

		platform.PassPercentage = Percent(platform.Successes, platform.Failures)
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
		j.Successes += job.Successes
		j.Failures += job.Failures
		jobRunsByName[job.Name] = j
	}

	for _, job := range jobRunsByName {

		job.PassPercentage = Percent(job.Successes, job.Failures)
		jobResults = append(jobResults, job)
	}
	// sort from lowest to highest
	sort.SliceStable(jobResults, func(i, j int) bool {
		return jobResults[i].PassPercentage < jobResults[j].PassPercentage
	})
	return jobResults
}
