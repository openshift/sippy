package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/klog"

	"github.com/bparees/sippy/pkg/testgrid"
)

var (
	defaultDashboards []string = []string{
		//"redhat-openshift-ocp-release-4.5-blocking",
		//"redhat-openshift-ocp-release-4.5-informing",
		"redhat-openshift-ocp-release-4.4-blocking",
		"redhat-openshift-ocp-release-4.4-informing",
		//"redhat-openshift-ocp-release-4.3-blocking",
		//"redhat-openshift-ocp-release-4.3-informing",
		//"redhat-openshift-ocp-release-4.2-blocking",
		//"redhat-openshift-ocp-release-4.2-informing",
		//"redhat-openshift-ocp-release-4.1-blocking",
		//"redhat-openshift-ocp-release-4.1-informing",
	}
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

	ignoreTestRegex *regexp.Regexp                 = regexp.MustCompile(`operator.Run template|Monitor cluster while tests execute|Overall`)
	ByAll           map[string]AggregateTestResult = make(map[string]AggregateTestResult)
	ByJob           map[string]AggregateTestResult = make(map[string]AggregateTestResult)
	ByPlatform      map[string]AggregateTestResult = make(map[string]AggregateTestResult)
	BySig           map[string]AggregateTestResult = make(map[string]AggregateTestResult)

	FailureGroups map[string]JobRunResult = make(map[string]JobRunResult)
)

type TestMeta struct {
	name  string
	count int
	jobs  map[string]interface{}
	sig   string
	bug   string
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

func jobMatters(jobName, status string, filter *regexp.Regexp) bool {
	if filter != nil && !filter.MatchString(jobName) {
		return false
	}

	switch status {
	case "FAILING", "FLAKY":
		return true
	}
	return false
}

func findBug(testName string) string {
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

// find associated sig from test name
func findSig(name string) string {
	match := sigRegex.FindStringSubmatch(name)
	if len(match) > 1 {
		return match[1]
	}
	return "sig-unknown"
}

func findPlatform(name string) string {
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

func fetchJobSummaries(dashboard string, opts *options) (map[string]testgrid.JobSummary, error) {
	jobs := make(map[string]testgrid.JobSummary)
	url := fmt.Sprintf("https://testgrid.k8s.io/%s/summary", dashboard)

	var buf *bytes.Buffer
	if len(opts.LocalData) != 0 {
		filename := opts.LocalData + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return jobs, fmt.Errorf("Could not read local data file %s: %v", filename, err)
		}
		buf = bytes.NewBuffer(b)
	} else {
		resp, err := http.Get(url)
		if err != nil {
			return jobs, err
		}
		if resp.StatusCode != 200 {
			return jobs, fmt.Errorf("Non-200 response code fetching job summary: %v", resp)
		}
		buf = bytes.NewBuffer([]byte{})
		io.Copy(buf, resp.Body)
	}

	if len(opts.Download) != 0 {
		filename := opts.Download + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
		f, err := os.Create(filename)
		if err != nil {
			return jobs, err
		}
		f.Write(buf.Bytes())
	}

	err := json.NewDecoder(buf).Decode(&jobs)
	if err != nil {
		return nil, err
	}

	return jobs, nil

}

func fetchJobDetails(dashboard, jobName string, opts *options) (testgrid.JobDetails, error) {
	details := testgrid.JobDetails{
		Name: jobName,
	}

	url := fmt.Sprintf("https://testgrid.k8s.io/%s/table?&show-stale-tests=&tab=%s", dashboard, jobName)

	var buf *bytes.Buffer
	if len(opts.LocalData) != 0 {
		filename := opts.LocalData + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return details, fmt.Errorf("Could not read local data file %s: %v", filename, err)
		}
		buf = bytes.NewBuffer(b)

	} else {
		resp, err := http.Get(url)
		if err != nil {
			return details, err
		}
		if resp.StatusCode != 200 {
			return details, fmt.Errorf("Non-200 response code fetching job details: %v", resp)
		}
		buf = bytes.NewBuffer([]byte{})
		io.Copy(buf, resp.Body)
	}

	if len(opts.Download) != 0 {
		filename := opts.Download + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
		f, err := os.Create(filename)
		if err != nil {
			return details, err
		}
		f.Write(buf.Bytes())
	}

	err := json.NewDecoder(buf).Decode(&details)
	if err != nil {
		return details, err
	}

	return details, nil

}

func computeLookback(startday, lookback int, timestamps []int) (int, int) {

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

func processTest(job testgrid.JobDetails, platform string, test testgrid.Test, meta TestMeta, startCol, endCol int) {
	col := startCol
	passed := 0
	failed := 0
	for _, result := range test.Statuses {
		switch result.Value {
		case 1:
			for i := col; i < col+result.Count && i < endCol; i++ {
				passed++
				joburl := fmt.Sprintf("https://prow.svc.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := FailureGroups[joburl]
				if !ok {
					jrr = JobRunResult{
						Job: job.Name,
						Url: joburl,
					}
				}
				jrr.TestNames = append(jrr.TestNames, test.Name)
				if test.Name == "Overall" {
					jrr.Succeeded = true
				}
				FailureGroups[joburl] = jrr
			}
		case 12:
			for i := col; i < col+result.Count && i < endCol; i++ {
				failed++
				joburl := fmt.Sprintf("https://prow.svc.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := FailureGroups[joburl]
				if !ok {
					jrr = JobRunResult{
						Job: job.Name,
						Url: joburl,
					}
				}
				jrr.TestNames = append(jrr.TestNames, test.Name)
				jrr.TestFailures++
				if test.Name == "Overall" {
					jrr.Failed = true
				}
				FailureGroups[joburl] = jrr
			}
		}
		col += result.Count
		if col > endCol {
			break
		}
	}

	addTestResult("all", ByAll, test.Name, meta, passed, failed)
	addTestResult(job.Name, ByJob, test.Name, meta, passed, failed)
	addTestResult(platform, ByPlatform, test.Name, meta, passed, failed)
	addTestResult(meta.sig, BySig, test.Name, meta, passed, failed)
}

func addTestResult(categoryKey string, categories map[string]AggregateTestResult, testName string, meta TestMeta, passed, failed int) {

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
	result.Bug = meta.bug

	category.TestResults[testName] = result

	categories[categoryKey] = category
}

func processJobDetails(job testgrid.JobDetails, opts *options, testMeta map[string]TestMeta) {

	startCol, endCol := computeLookback(opts.StartDay, opts.Lookback, job.Timestamps)
	for _, test := range job.Tests {
		klog.V(2).Infof("Analyzing results from %d to %d from job %s for test %s\n", startCol, endCol, job.Name, test.Name)

		meta, ok := testMeta[test.Name]
		if !ok {
			meta = TestMeta{
				name: test.Name,
				jobs: make(map[string]interface{}),
				sig:  findSig(test.Name),
			}
			if opts.FindBugs {
				meta.bug = findBug(test.Name)
			} else {
				meta.bug = "Bug search not requested"
			}
		}
		meta.count++
		if _, ok := meta.jobs[job.Name]; !ok {
			meta.jobs[job.Name] = struct{}{}
		}

		// update test metadata
		testMeta[test.Name] = meta

		processTest(job, findPlatform(job.Name), test, meta, startCol, endCol)

	}
}

func percent(success, failure int) float64 {
	if success+failure == 0 {
		return math.NaN()
	}
	return float64(success) / float64(success+failure) * 100.0
}

func computePercentages(AggregateTestResults map[string]AggregateTestResult) {
	for k, AggregateTestResult := range AggregateTestResults {
		AggregateTestResult.TestPassPercentage = percent(AggregateTestResult.Successes, AggregateTestResult.Failures)
		for k2, r := range AggregateTestResult.TestResults {
			r.PassPercentage = percent(r.Successes, r.Failures)
			AggregateTestResult.TestResults[k2] = r
		}
		AggregateTestResults[k] = AggregateTestResult
	}
}

func generateSortedResults(AggregateTestResult map[string]AggregateTestResult, opts *options) map[string]SortedAggregateTestResult {
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
			if (result.Successes+result.Failures >= opts.MinRuns) && result.PassPercentage < opts.SuccessThreshold {
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

func summarizeJobsByPlatform(report TestReport) []JobResult {
	jobRunsByPlatform := make(map[string]JobResult)
	platformResults := []JobResult{}

	for _, job := range report.JobPassRate {
		p := findPlatform(job.Name)
		j := jobRunsByPlatform[p]
		j.Successes += job.Successes
		j.Failures += job.Failures
		j.Platform = p
		jobRunsByPlatform[p] = j
	}

	for _, platform := range jobRunsByPlatform {

		platform.PassPercentage = percent(platform.Successes, platform.Failures)
		platformResults = append(platformResults, platform)
	}
	// sort from lowest to highest
	sort.SliceStable(platformResults, func(i, j int) bool {
		return platformResults[i].PassPercentage < platformResults[j].PassPercentage
	})
	return platformResults
}

func summarizeJobsByName(report TestReport) []JobResult {
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

		job.PassPercentage = percent(job.Successes, job.Failures)
		jobResults = append(jobResults, job)
	}
	// sort from lowest to highest
	sort.SliceStable(jobResults, func(i, j int) bool {
		return jobResults[i].PassPercentage < jobResults[j].PassPercentage
	})
	return jobResults
}

func filterFailureGroups(opts *options, jrr map[string]JobRunResult) []JobRunResult {
	filteredJrr := []JobRunResult{}
	// -1 means don't do this reporting.
	if opts.FailureClusterThreshold < 0 {
		return filteredJrr
	}
	for _, v := range jrr {
		if v.TestFailures > opts.FailureClusterThreshold {
			filteredJrr = append(filteredJrr, v)
		}
	}

	// sort from highest to lowest
	sort.SliceStable(filteredJrr, func(i, j int) bool {
		return filteredJrr[i].TestFailures > filteredJrr[j].TestFailures
	})

	return filteredJrr
}

func computeJobPassRate(opts *options, jrr map[string]JobRunResult) []JobResult {
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
		job.PassPercentage = percent(job.Successes, job.Failures)
		jobs = append(jobs, job)
	}

	// sort from lowest to highest
	sort.SliceStable(jobs, func(i, j int) bool {
		return jobs[i].PassPercentage < jobs[j].PassPercentage
	})

	return jobs
}

func prepareTestReport(opts *options) TestReport {
	computePercentages(ByAll)
	computePercentages(ByPlatform)
	computePercentages(ByJob)
	computePercentages(BySig)

	byAll := generateSortedResults(ByAll, opts)
	byPlatform := generateSortedResults(ByPlatform, opts)
	byJob := generateSortedResults(ByJob, opts)
	bySig := generateSortedResults(BySig, opts)

	filteredFailureGroups := filterFailureGroups(opts, FailureGroups)
	jobPassRate := computeJobPassRate(opts, FailureGroups)

	return TestReport{
		All:           byAll,
		ByPlatform:    byPlatform,
		ByJob:         byJob,
		BySig:         bySig,
		FailureGroups: filteredFailureGroups,
		JobPassRate:   jobPassRate,
	}

}

func printReport(opts *options) {
	r := prepareTestReport(opts)
	switch opts.Output {
	case "json":
		printJsonReport(r)
	case "text":
		printTextReport(r)
	case "dashboard":
		printDashboardReport(opts, r)
	}
}
func printJsonReport(report TestReport) {
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(report)
}

func printDashboardReport(opts *options, report TestReport) {
	fmt.Println("================== Summary Across All Jobs ==================")
	all := report.All["all"]
	fmt.Printf("Passing test runs: %d\n", all.Successes)
	fmt.Printf("Failing test runs: %d\n", all.Failures)
	fmt.Printf("Test Pass Percentage: %0.2f\n", all.TestPassPercentage)

	fmt.Println("\n\n================== Top 10 Most Frequently Failing Tests ==================")
	count := 0
	for i := 0; count < 10 && i < len(all.TestResults); i++ {
		test := all.TestResults[i]
		if !ignoreTestRegex.MatchString(test.Name) && (test.Successes+test.Failures) > opts.MinRuns {
			fmt.Printf("Test Name: %s\n", test.Name)
			fmt.Printf("Test Pass Percentage: %0.2f (%d runs)\n\n", test.PassPercentage, test.Successes+test.Failures)
			if test.Successes+test.Failures < 10 {
				fmt.Printf("WARNING: Only %d runs for this test\n", test.Successes+test.Failures)
			}
			count++
		}
	}

	fmt.Println("\n\n================== Top 10 Most Frequently Failing Jobs ==================")
	jobRunsByName := summarizeJobsByName(report)

	for i, v := range jobRunsByName {
		fmt.Printf("Job: %s\n", v.Name)
		fmt.Printf("Job Pass Percentage: %0.2f%% (%d runs)\n", percent(v.Successes, v.Failures), v.Successes+v.Failures)
		if v.Successes+v.Failures < 10 {
			fmt.Printf("WARNING: Only %d runs for this job\n", v.Successes+v.Failures)
		}
		fmt.Printf("\n")
		if i == 9 {
			break
		}
	}

	fmt.Println("\n\n================== Clustered Test Failures ==================")
	count = 0
	for _, group := range report.FailureGroups {
		count += group.TestFailures
	}
	if len(report.FailureGroups) != 0 {
		fmt.Printf("%d Clustered Test Failures with an average size of %d and median of %d\n", len(report.FailureGroups), count/len(report.FailureGroups), report.FailureGroups[len(report.FailureGroups)/2].TestFailures)
	} else {
		fmt.Printf("No clustered test failures observed")
	}

	fmt.Println("\n\n================== Summary By Platform ==================")
	jobsByPlatform := summarizeJobsByPlatform(report)
	for _, v := range jobsByPlatform {
		fmt.Printf("Platform: %s\n", v.Platform)
		fmt.Printf("Platform Job Pass Percentage: %0.2f%% (%d runs)\n", percent(v.Successes, v.Failures), v.Successes+v.Failures)
		if v.Successes+v.Failures < 10 {
			fmt.Printf("WARNING: Only %d runs for this job\n", v.Successes+v.Failures)
		}
		fmt.Printf("\n")
	}
}

func printTextReport(report TestReport) {
	fmt.Println("================== Test Summary Across All Jobs ==================")
	all := report.All["all"]
	fmt.Printf("Passing test runs: %d\n", all.Successes)
	fmt.Printf("Failing test runs: %d\n", all.Failures)
	fmt.Printf("Test Pass Percentage: %0.2f\n", all.TestPassPercentage)
	testCount := 0
	testSuccesses := 0
	testFailures := 0
	for _, test := range all.TestResults {
		fmt.Printf("\tTest Name: %s\n", test.Name)
		//		fmt.Printf("\tPassed: %d\n", test.Successes)
		//		fmt.Printf("\tFailed: %d\n", test.Failures)
		fmt.Printf("\tTest Pass Percentage: %0.2f\n\n", test.PassPercentage)
		testCount++
		testSuccesses += test.Successes
		testFailures += test.Failures
	}

	fmt.Println("\n\n\n================== Test Summary By Platform ==================")
	for key, by := range report.ByPlatform {
		fmt.Printf("Platform: %s\n", key)
		//		fmt.Printf("Passing test runs: %d\n", platform.Successes)
		//		fmt.Printf("Failing test runs: %d\n", platform.Failures)
		fmt.Printf("Test Pass Percentage: %0.2f\n", by.TestPassPercentage)
		for _, test := range by.TestResults {
			fmt.Printf("\tTest Name: %s\n", test.Name)
			//			fmt.Printf("\tPassed: %d\n", test.Successes)
			//			fmt.Printf("\tFailed: %d\n", test.Failures)
			fmt.Printf("\tTest Pass Percentage: %0.2f\n\n", test.PassPercentage)
		}
		fmt.Println("")
	}

	fmt.Println("\n\n\n================== Test Summary By Job ==================")
	for key, by := range report.ByJob {
		fmt.Printf("Job: %s\n", key)
		//		fmt.Printf("Passing test runs: %d\n", platform.Successes)
		//		fmt.Printf("Failing test runs: %d\n", platform.Failures)
		fmt.Printf("Test Pass Percentage: %0.2f\n", by.TestPassPercentage)
		for _, test := range by.TestResults {
			fmt.Printf("\tTest Name: %s\n", test.Name)
			//			fmt.Printf("\tPassed: %d\n", test.Successes)
			//			fmt.Printf("\tFailed: %d\n", test.Failures)
			fmt.Printf("\tTest Pass Percentage: %0.2f\n\n", test.PassPercentage)
		}
		fmt.Println("")
	}

	fmt.Println("\n\n\n================== Test Summary By Sig ==================")
	for key, by := range report.BySig {
		fmt.Printf("\nSig: %s\n", key)
		//		fmt.Printf("Passing test runs: %d\n", platform.Successes)
		//		fmt.Printf("Failing test runs: %d\n", platform.Failures)
		fmt.Printf("Test Pass Percentage: %0.2f\n", by.TestPassPercentage)
		for _, test := range by.TestResults {
			fmt.Printf("\tTest Name: %s\n", test.Name)
			//			fmt.Printf("\tPassed: %d\n", test.Successes)
			//			fmt.Printf("\tFailed: %d\n", test.Failures)
			fmt.Printf("\tTest Pass Percentage: %0.2f\n\n", test.PassPercentage)
		}
		fmt.Println("")
	}

	fmt.Println("\n\n\n================== Clustered Test Failures ==================")
	for _, group := range report.FailureGroups {
		fmt.Printf("Job url: %s\n", group.Url)
		fmt.Printf("Number of test failures: %d\n\n", group.TestFailures)
	}

	fmt.Println("\n\n\n================== Job Pass Rates ==================")
	jobSuccesses := 0
	jobFailures := 0
	jobCount := 0

	for _, job := range report.JobPassRate {
		fmt.Printf("Job: %s\n", job.Name)
		fmt.Printf("Job Successes: %d\n", job.Successes)
		fmt.Printf("Job Failures: %d\n", job.Failures)
		fmt.Printf("Job Pass Percentage: %0.2f\n\n", job.PassPercentage)
		jobSuccesses += job.Successes
		jobFailures += job.Failures
		jobCount++
	}

	fmt.Println("\n\n================== Job Summary By Platform ==================")
	jobsByPlatform := summarizeJobsByPlatform(report)
	for _, v := range jobsByPlatform {
		fmt.Printf("Platform: %s\n", v.Platform)
		fmt.Printf("Job Succeses: %d\n", v.Successes)
		fmt.Printf("Job Failures: %d\n", v.Failures)
		fmt.Printf("Platform Job Pass Percentage: %0.2f%% (%d runs)\n", percent(v.Successes, v.Failures), v.Successes+v.Failures)
		if v.Successes+v.Failures < 10 {
			fmt.Printf("WARNING: Only %d runs for this job\n", v.Successes+v.Failures)
		}
		fmt.Printf("\n")
	}

	fmt.Println("")

	fmt.Println("\n\n================== Overall Summary ==================")
	fmt.Printf("Total Jobs: %d\n", jobCount)
	fmt.Printf("Total Job Successes: %d\n", jobSuccesses)
	fmt.Printf("Total Job Failures: %d\n", jobFailures)
	fmt.Printf("Total Job Pass Percentage: %0.2f\n\n", percent(jobSuccesses, jobFailures))

	fmt.Printf("Total Tests: %d\n", testCount)
	fmt.Printf("Total Test Successes: %d\n", testSuccesses)
	fmt.Printf("Total Test Failures: %d\n", testFailures)
	fmt.Printf("Total Test Pass Percentage: %0.2f\n", percent(testSuccesses, testFailures))

}

type options struct {
	LocalData               string
	Dashboards              []string
	StartDay                int
	Lookback                int
	FindBugs                bool
	SuccessThreshold        float64
	JobFilter               string
	MinRuns                 int
	Output                  string
	FailureClusterThreshold int
	Download                string
}

func main() {
	opt := &options{
		Lookback:                14,
		SuccessThreshold:        99,
		MinRuns:                 10,
		Output:                  "json",
		FailureClusterThreshold: 10,
		StartDay:                0,
	}

	klog.InitFlags(nil)
	flag.CommandLine.Set("skip_headers", "true")

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			if err := opt.Run(); err != nil {
				klog.Exitf("error: %v", err)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.LocalData, "local-data", opt.LocalData, "Path to testgrid data from local disk")
	flags.StringArrayVar(&opt.Dashboards, "dashboard", []string{}, "Which dashboards to analyze (one per arg instance)")
	flags.IntVar(&opt.StartDay, "start-day", opt.StartDay, "Analyze data starting from this day")
	flags.IntVar(&opt.Lookback, "lookback", opt.Lookback, "Number of previous days worth of job runs to analyze")
	flags.Float64Var(&opt.SuccessThreshold, "success-threshold", opt.SuccessThreshold, "Filter results for tests that are more than this percent successful")
	flags.BoolVar(&opt.FindBugs, "find-bugs", opt.FindBugs, "Attempt to find a bug that matches a failing test")
	flags.StringVar(&opt.JobFilter, "job-filter", opt.JobFilter, "Only analyze jobs that match this regex")
	flags.StringVar(&opt.Download, "download", opt.Download, "Download testgrid data to directory specified for use with --local-data")
	flags.IntVar(&opt.MinRuns, "min-runs", opt.MinRuns, "Ignore tests with less than this number of runs")
	flags.IntVar(&opt.FailureClusterThreshold, "failure-cluster-threshold", opt.FailureClusterThreshold, "Include separate report on job runs with more than N test failures, -1 to disable")
	flags.StringVarP(&opt.Output, "output", "o", opt.Output, "Output format for report: json, text")

	flags.AddGoFlag(flag.CommandLine.Lookup("v"))
	flags.AddGoFlag(flag.CommandLine.Lookup("skip_headers"))

	if err := cmd.Execute(); err != nil {
		klog.Exitf("error: %v", err)
	}
}

func (o *options) Run() error {

	testMeta := make(map[string]TestMeta)
	if len(o.Dashboards) == 0 {
		o.Dashboards = defaultDashboards
	}

	for _, dashboard := range o.Dashboards {
		jobs, err := fetchJobSummaries(dashboard, o)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}

		var jobFilter *regexp.Regexp
		if len(o.JobFilter) > 0 {
			jobFilter = regexp.MustCompile(o.JobFilter)
		}
		for jobName, job := range jobs {
			if jobMatters(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := fetchJobDetails(dashboard, jobName, o)
				if err != nil {
					klog.Errorf("Error fetching job details for %s: %v\n", jobName, err)
				}
				processJobDetails(details, o, testMeta)
				klog.V(4).Infoln("\n\n================================================================================")
			}
		}
	}

	printReport(o)

	return nil
}
