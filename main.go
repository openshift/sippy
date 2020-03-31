package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
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
		//		"redhat-openshift-ocp-release-4.5-informing",
		"redhat-openshift-ocp-release-4.4-informing",
		//		"redhat-openshift-ocp-release-4.3-informing",
		//		"redhat-openshift-ocp-release-4.2-informing",
		//		"redhat-openshift-ocp-release-4.1-informing",
		//		"redhat-openshift-ocp-release-4.5-blocking",
		"redhat-openshift-ocp-release-4.4-blocking",
		//		"redhat-openshift-ocp-release-4.3-blocking",
		//		"redhat-openshift-ocp-release-4.2-blocking",
		//		"redhat-openshift-ocp-release-4.1-blocking",
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

	ByAll      map[string]AggregateTestResult = make(map[string]AggregateTestResult)
	ByJob      map[string]AggregateTestResult = make(map[string]AggregateTestResult)
	ByPlatform map[string]AggregateTestResult = make(map[string]AggregateTestResult)
	BySig      map[string]AggregateTestResult = make(map[string]AggregateTestResult)
)

type TestMeta struct {
	name  string
	count int
	jobs  map[string]interface{}
	sig   string
	bug   string
}

type TestReport struct {
	All        map[string]SortedAggregateTestResult `json:"all"`
	ByPlatform map[string]SortedAggregateTestResult `json:"byPlatform`
	ByJob      map[string]SortedAggregateTestResult `json:"byJob`
	BySig      map[string]SortedAggregateTestResult `json:"bySig`
}

type SortedAggregateTestResult struct {
	Successes      int          `json:"successes"`
	Failures       int          `json:"failures"`
	PassPercentage float32      `json:"PassPercentage"`
	TestResults    []TestResult `json:"results"`
}

type AggregateTestResult struct {
	Successes      int                   `json:"successes"`
	Failures       int                   `json:"failures"`
	PassPercentage float32               `json:"PassPercentage"`
	TestResults    map[string]TestResult `json:"results"`
}

type TestResult struct {
	Name           string  `json:"name"`
	Successes      int     `json:"successes"`
	Failures       int     `json:"failures"`
	PassPercentage float32 `json:"PassPercentage"`
	Bug            string  `json:"Bug"`
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
	case vsphereRegex.MatchString(name):
		return "vsphere"
	}
	return "unknown platform"
}

func fetchJobSummaries(dashboard string) (map[string]testgrid.JobSummary, error) {
	resp, err := http.Get(fmt.Sprintf("https://testgrid.k8s.io/%s/summary", dashboard))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 response code fetching dashboard page: %v", resp)
	}

	var jobs map[string]testgrid.JobSummary
	err = json.NewDecoder(resp.Body).Decode(&jobs)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func fetchJobDetails(dashboard, jobName string, opts *options) (testgrid.JobDetails, error) {
	details := testgrid.JobDetails{
		Name: jobName,
	}

	if len(opts.SampleData) != 0 {
		f, err := os.Open(opts.SampleData)
		if err != nil {
			return details, fmt.Errorf("Could not open sample data file %s: %v", opts.SampleData, err)
		}
		err = json.NewDecoder(f).Decode(&details)
		if err != nil {
			return details, err
		}
		return details, nil
	}

	url := fmt.Sprintf("https://testgrid.k8s.io/%s/table?tab=%s&exclude-filter-by-regex=Monitor%%5Cscluster&exclude-filter-by-regex=%%5Eoperator.Run%%20template.*container%%20test%%24", dashboard, jobName)
	resp, err := http.Get(url)
	if err != nil {
		return details, err
	}
	if resp.StatusCode != 200 {
		return details, fmt.Errorf("Non-200 response code fetching job details: %v", resp)
	}

	err = json.NewDecoder(resp.Body).Decode(&details)
	if err != nil {
		return details, err
	}

	return details, nil

}

func computeLookback(lookback int, timestamps []int) int {

	stop := time.Now().Add(time.Duration(-1*lookback*24)*time.Hour).Unix() * 1000

	for i, t := range timestamps {
		if int64(t) < stop {
			return i
		}
	}
	return 0
}

func processTest(job, platform string, test testgrid.Test, meta TestMeta, cols int) {
	col := 0
	passed := 0
	failed := 0
	total := 0
	for _, result := range test.Statuses {
		col += result.Count
		switch result.Value {
		case 1:
			passed += result.Count
		case 12:
			failed += result.Count
		}
		total += result.Count
		if col > cols {
			break
		}
	}

	addTestResult("all", ByAll, test.Name, meta, passed, failed)
	addTestResult(job, ByJob, test.Name, meta, passed, failed)
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

	cols := computeLookback(opts.Lookback, job.Timestamps)

	for _, test := range job.Tests {
		klog.V(2).Infof("Analyzing results from job %s for test %s\n", job.Name, test.Name)

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

		processTest(job.Name, findPlatform(job.Name), test, meta, cols)

	}
}

func computePercentages(AggregateTestResults map[string]AggregateTestResult) {
	for k, AggregateTestResult := range AggregateTestResults {

		if AggregateTestResult.Successes+AggregateTestResult.Failures > 0 {
			AggregateTestResult.PassPercentage = float32(AggregateTestResult.Successes) / float32(AggregateTestResult.Successes+AggregateTestResult.Failures) * 100
		}
		for k, r := range AggregateTestResult.TestResults {
			if r.Successes+r.Failures > 0 {
				r.PassPercentage = float32(r.Successes) / float32(r.Successes+r.Failures) * 100
				AggregateTestResult.TestResults[k] = r
			}
		}
		AggregateTestResults[k] = AggregateTestResult
	}
}

func generateSortedResults(AggregateTestResult map[string]AggregateTestResult, opts *options) map[string]SortedAggregateTestResult {
	sorted := make(map[string]SortedAggregateTestResult)

	for k, v := range AggregateTestResult {
		sorted[k] = SortedAggregateTestResult{
			Failures:       v.Failures,
			Successes:      v.Successes,
			PassPercentage: v.PassPercentage,
		}

		for _, result := range v.TestResults {
			// ignore the "Overall" test.
			if result.Name == "Overall" {
				continue
			}
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

func prepareTestReport(opts *options) TestReport {
	computePercentages(ByAll)
	computePercentages(ByPlatform)
	computePercentages(ByJob)
	computePercentages(BySig)

	byAll := generateSortedResults(ByAll, opts)
	byPlatform := generateSortedResults(ByPlatform, opts)
	byJob := generateSortedResults(ByJob, opts)
	bySig := generateSortedResults(BySig, opts)

	return TestReport{
		All:        byAll,
		ByPlatform: byPlatform,
		ByJob:      byJob,
		BySig:      bySig}

}

func printReport(opts *options) {
	r := prepareTestReport(opts)
	switch opts.Output {
	case "json":
		printJsonReport(r)
	case "text":
		printTextReport(r)
	}
}
func printJsonReport(report TestReport) {
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(report)
}

func printTextReport(report TestReport) {
	fmt.Println("================== Summary Across All Jobs ==================")
	all := report.All["all"]
	//	fmt.Printf("Passing test runs: %d\n", all.Successes)
	//	fmt.Printf("Failing test runs: %d\n", all.Failures)
	fmt.Printf("Percentage: %f\n", all.PassPercentage)

	for _, test := range all.TestResults {
		fmt.Printf("\tTest Name: %s\n", test.Name)
		//		fmt.Printf("\tPassed: %d\n", test.Successes)
		//		fmt.Printf("\tFailed: %d\n", test.Failures)
		fmt.Printf("\tPercentage: %f\n", test.PassPercentage)
	}

	fmt.Println("\n\n\n================== Summary By Platform ==================")
	for key, by := range report.ByPlatform {
		fmt.Printf("\nPlatform: %s\n", key)
		//		fmt.Printf("Passing test runs: %d\n", platform.Successes)
		//		fmt.Printf("Failing test runs: %d\n", platform.Failures)
		fmt.Printf("Percentage: %f\n", by.PassPercentage)

		for _, test := range by.TestResults {
			fmt.Printf("\tTest Name: %s\n", test.Name)
			//			fmt.Printf("\tPassed: %d\n", test.Successes)
			//			fmt.Printf("\tFailed: %d\n", test.Failures)
			fmt.Printf("\tPercentage: %f\n", test.PassPercentage)
		}
	}

	fmt.Println("\n\n\n================== Summary By Job ==================")
	for key, by := range report.ByJob {
		fmt.Printf("\nJob: %s\n", key)
		//		fmt.Printf("Passing test runs: %d\n", platform.Successes)
		//		fmt.Printf("Failing test runs: %d\n", platform.Failures)
		fmt.Printf("Percentage: %f\n", by.PassPercentage)

		for _, test := range by.TestResults {
			fmt.Printf("\tTest Name: %s\n", test.Name)
			//			fmt.Printf("\tPassed: %d\n", test.Successes)
			//			fmt.Printf("\tFailed: %d\n", test.Failures)
			fmt.Printf("\tPercentage: %f\n", test.PassPercentage)
		}
	}

	fmt.Println("\n\n\n================== Summary By Sig ==================")
	for key, by := range report.BySig {
		fmt.Printf("\nSig: %s\n", key)
		//		fmt.Printf("Passing test runs: %d\n", platform.Successes)
		//		fmt.Printf("Failing test runs: %d\n", platform.Failures)
		fmt.Printf("Percentage: %f\n", by.PassPercentage)

		for _, test := range by.TestResults {
			fmt.Printf("\tTest Name: %s\n", test.Name)
			//			fmt.Printf("\tPassed: %d\n", test.Successes)
			//			fmt.Printf("\tFailed: %d\n", test.Failures)
			fmt.Printf("\tPercentage: %f\n", test.PassPercentage)
		}
	}

}

type options struct {
	SampleData       string
	Dashboards       []string
	Lookback         int
	FindBugs         bool
	SuccessThreshold float32
	JobFilter        string
	MinRuns          int
	Output           string
}

func main() {
	opt := &options{
		Lookback:         14,
		SuccessThreshold: 99,
		MinRuns:          10,
		Output:           "json",
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
	flags.StringVar(&opt.SampleData, "sample-data", opt.SampleData, "Path to sample testgrid data from local disk")
	flags.StringArrayVar(&opt.Dashboards, "dashboard", []string{}, "Which dashboards to analyze (one per arg instance)")
	flags.IntVar(&opt.Lookback, "lookback", opt.Lookback, "Number of previous days worth of job runs to analyze")
	flags.Float32Var(&opt.SuccessThreshold, "success-threshold", opt.SuccessThreshold, "Filter results for tests that are more than this percent successful")
	flags.BoolVar(&opt.FindBugs, "find-bugs", opt.FindBugs, "Attempt to find a bug that matches a failing test")
	flags.StringVar(&opt.JobFilter, "job-filter", opt.JobFilter, "Only analyze jobs that match this regex")
	flags.IntVar(&opt.MinRuns, "min-runs", opt.MinRuns, "Ignore tests with less than this number of runs")
	flags.StringVarP(&opt.Output, "output", "o", opt.Output, "Output format for report: json, text")

	flags.AddGoFlag(flag.CommandLine.Lookup("v"))
	flags.AddGoFlag(flag.CommandLine.Lookup("skip_headers"))

	if err := cmd.Execute(); err != nil {
		klog.Exitf("error: %v", err)
	}
}

func (o *options) Run() error {

	testMeta := make(map[string]TestMeta)
	if len(o.SampleData) > 0 {
		details, err := fetchJobDetails("", "sample-job", o)
		if err != nil {
			klog.Errorf("Error fetching job details for %s: %v\n", o.SampleData, err)
		}
		processJobDetails(details, o, testMeta)
		printReport(o)
		return nil
	}
	if len(o.Dashboards) == 0 {
		o.Dashboards = defaultDashboards
	}

	for _, dashboard := range o.Dashboards {
		jobs, err := fetchJobSummaries(dashboard)
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
