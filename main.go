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
	dashboards []string = []string{
		"redhat-openshift-ocp-release-4.5-informing",
		"redhat-openshift-ocp-release-4.4-informing",
		"redhat-openshift-ocp-release-4.3-informing",
		"redhat-openshift-ocp-release-4.2-informing",
		"redhat-openshift-ocp-release-4.1-informing",
		"redhat-openshift-ocp-release-4.5-blocking",
		"redhat-openshift-ocp-release-4.4-blocking",
		"redhat-openshift-ocp-release-4.3-blocking",
		"redhat-openshift-ocp-release-4.2-blocking",
		"redhat-openshift-ocp-release-4.1-blocking",
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

	ByAll      map[string]AggregateResult = make(map[string]AggregateResult)
	ByJob      map[string]AggregateResult = make(map[string]AggregateResult)
	ByPlatform map[string]AggregateResult = make(map[string]AggregateResult)
	BySig      map[string]AggregateResult = make(map[string]AggregateResult)
)

/*
type TestReport struct {
	TestName        string   `json:"testName"`
	OwningSig       string   `json:"owningSig"`
	JobsFailedCount int      `json:"jobsFailedCount"`
	JobsFailedNames []string `json:"jobsFailedNames"`
	AssociatedBug   string   `json:"associatedBug"`
}
*/

type TestReport struct {
	All        map[string]SortedAggregateResult `json:"all"`
	ByPlatform map[string]SortedAggregateResult `json:"byPlatform`
	ByJob      map[string]SortedAggregateResult `json:"byJob`
	BySig      map[string]SortedAggregateResult `json:"bySig`
}

type SortedAggregateResult struct {
	Successes      int      `json:"successes"`
	Failures       int      `json:"failures"`
	PassPercentage float32  `json:"PassPercentage"`
	Results        []Result `json:"results"`
}

type TestMeta struct {
	name     string
	count    int
	jobs     map[string]interface{}
	sig      string
	bug      string
	platform string
}

type AggregateResult struct {
	Successes      int               `json:"successes"`
	Failures       int               `json:"failures"`
	PassPercentage float32           `json:"PassPercentage"`
	Results        map[string]Result `json:"results"`
}

type Result struct {
	Name           string  `json:"name"`
	Successes      int     `json:"successes"`
	Failures       int     `json:"failures"`
	PassPercentage float32 `json:"PassPercentage"`
	Bug            string  `json:"Bug"`
}

func jobHasBadStatus(status string) bool {
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

	/*
		sortBy := ""
		switch {
		case opts.SortByFlakes:
			sortBy = "sort-by-flakiness="
		case opts.SortByFailures:
			sortBy = "sort-by-failures="
		}
	*/

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

func processTest(job string, test testgrid.Test, meta TestMeta, cols int) {
	col := 0
	passed := 0
	failed := 0
	total := 0
	for _, result := range test.Statuses {
		//klog.V(2).Infof("processing test %s status %d of %d\n", test.Name, col, cols)

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

	/*
		overall := AggregateResult{}

		overall.Successes += passed
		overall.Failures += failed
		if f, ok := overall.Results[test.Name]; !ok {
			overall.Results[test.Name] = Result{}
		}
		overall.Results[test.Name].Name = test.Name
		overall.Results[test.Name].Successes += passed
		overall.Results[test.Name].Failures += failed
	*/

	addTestResult("all", ByAll, test.Name, meta, passed, failed)
	addTestResult(job, ByJob, test.Name, meta, passed, failed)
	addTestResult(meta.platform, ByPlatform, test.Name, meta, passed, failed)
	addTestResult(meta.sig, BySig, test.Name, meta, passed, failed)
}

func addTestResult(categoryKey string, categories map[string]AggregateResult, testName string, meta TestMeta, passed, failed int) {

	klog.V(2).Infof("Adding test %s to category %s, passed: %d, failed: %d\n", testName, categoryKey, passed, failed)
	category, ok := categories[categoryKey]
	if !ok {
		category = AggregateResult{
			Results: make(map[string]Result),
		}
	}

	category.Successes += passed
	category.Failures += failed

	result, ok := category.Results[testName]
	if !ok {
		result = Result{}
	}
	result.Name = testName
	result.Successes += passed
	result.Failures += failed
	result.Bug = meta.bug

	category.Results[testName] = result

	categories[categoryKey] = category
}

func processJobDetails(job testgrid.JobDetails, opts *options, testMeta map[string]TestMeta) {

	cols := computeLookback(opts.Lookback, job.Timestamps)

	for _, test := range job.Tests {
		klog.V(2).Infof("Analyzing results from job %s for test %s\n", job.Name, test.Name)
		/*
			if i > opts.FailureCount {
				break
			}
		*/
		/*
			if test.Name == "Overall" {
				continue
			}
		*/
		//klog.V(4).Infof("Found a top failing test: %q\n\n", test.Name)

		meta, ok := testMeta[test.Name]
		if !ok {
			meta = TestMeta{
				name:     test.Name,
				jobs:     make(map[string]interface{}),
				platform: findPlatform(test.Name),
				sig:      findSig(test.Name),
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

		processTest(job.Name, test, meta, cols)

	}
}

func computePercentages(aggregateResults map[string]AggregateResult) {
	for k, aggregateResult := range aggregateResults {

		aggregateResult.PassPercentage = float32(aggregateResult.Successes) / float32(aggregateResult.Successes+aggregateResult.Failures) * 100
		for k, r := range aggregateResult.Results {
			r.PassPercentage = float32(r.Successes) / float32(r.Successes+r.Failures) * 100
			aggregateResult.Results[k] = r
		}
		aggregateResults[k] = aggregateResult
	}
}

func generateSortedResults(aggregateResult map[string]AggregateResult) map[string]SortedAggregateResult {
	sorted := make(map[string]SortedAggregateResult)

	for k, v := range aggregateResult {
		sorted[k] = SortedAggregateResult{
			Failures:       v.Failures,
			Successes:      v.Successes,
			PassPercentage: v.PassPercentage,
		}

		for _, result := range v.Results {
			s := sorted[k]
			s.Results = append(s.Results, result)
			sorted[k] = s
		}
		// sort from lowest to highest
		sort.SliceStable(sorted[k].Results, func(i, j int) bool {
			return sorted[k].Results[i].PassPercentage < sorted[k].Results[j].PassPercentage
		})
	}

	return sorted

}

func printReport() {

	computePercentages(ByAll)
	computePercentages(ByPlatform)
	computePercentages(ByJob)
	computePercentages(BySig)

	byAll := generateSortedResults(ByAll)
	byPlatform := generateSortedResults(ByPlatform)
	byJob := generateSortedResults(ByJob)
	bySig := generateSortedResults(BySig)

	enc := json.NewEncoder(os.Stdout)
	enc.Encode(TestReport{
		All:        byAll,
		ByPlatform: byPlatform,
		ByJob:      byJob,
		BySig:      bySig})

	/*

		klog.V(4).Infof("====================== Printing test report ======================\n")
		sigCount := make(map[string]int)
		var failures []TestMeta
		for _, meta := range testMeta {
			failures = append(failures, meta)
		}

		// sort from highest count to lowest
		sort.SliceStable(failures, func(i, j int) bool {
			return failures[i].count > failures[j].count
		})
		var report []TestReport

		for _, meta := range failures {
			klog.V(4).Infof("Test: %s\nCount: %d\nSig: %s\nJobs: %v\n\n", meta.name, meta.count, meta.sig, meta.jobs)

			var jobs []string
			for k := range meta.jobs {
				jobs = append(jobs, k)
			}
			testReport := TestReport{
				TestName:        meta.name,
				OwningSig:       meta.sig,
				AssociatedBug:   meta.bug,
				JobsFailedCount: meta.count,
				JobsFailedNames: jobs,
			}
			report = append(report, testReport)
			if _, ok := sigCount[meta.sig]; !ok {
				sigCount[meta.sig] = 0
			}
			sigCount[meta.sig] += meta.count
		}

		enc := json.NewEncoder(os.Stdout)
		enc.Encode(report)
		for s, c := range sigCount {
			klog.V(4).Infof("Sig %s is responsible for the top flake in %d job definitions\n", s, c)
		}
	*/
}

type options struct {
	SampleData string
	//SortByFlakes   bool
	//SortByFailures bool
	FailureCount int
	Dashboards   []string
	Lookback     int
	FindBugs     bool
}

func main() {
	opt := &options{
		//SortByFlakes:   false,
		//SortByFailures: false,
		FailureCount: 1,
		Lookback:     14,
	}

	klog.InitFlags(nil)
	flag.CommandLine.Set("skip_headers", "true")

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			/*
				if opt.SortByFlakes && opt.SortByFailures {
					klog.Exitf("Cannot set both sort-by-flakes and sort-by-failures")
				}

				if len(opt.SampleData) > 0 && (opt.SortByFlakes || opt.SortByFailures) {
					klog.Exitf("Cannot sort tests when using sample data")
				}
			*/
			if err := opt.Run(); err != nil {
				klog.Exitf("error: %v", err)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.SampleData, "sample-data", opt.SampleData, "Path to sample testgrid data from local disk")
	//flags.BoolVar(&opt.SortByFlakes, "sort-by-flakes", opt.SortByFlakes, "Sort tests by flakiness")
	//flags.BoolVar(&opt.SortByFailures, "sort-by-failures", opt.SortByFailures, "Sort tests by failures")
	flags.IntVar(&opt.FailureCount, "failure-count", opt.FailureCount, "Number of test failures to report on per test job")
	flags.StringArrayVar(&opt.Dashboards, "dashboard", []string{}, "Which dashboards to analyze (one per arg instance)")
	flags.IntVar(&opt.FailureCount, "lookback", opt.Lookback, "Number of previous days worth of job runs to analyze")
	flags.BoolVar(&opt.FindBugs, "find-bugs", opt.FindBugs, "Attempt to find a bug that matches a failing test")

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
		printReport()
		return nil
	}
	if len(o.Dashboards) == 0 {
		o.Dashboards = dashboards
	}

	for _, dashboard := range o.Dashboards {
		jobs, err := fetchJobSummaries(dashboard)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}

		for jobName, job := range jobs {
			if jobHasBadStatus(job.OverallStatus) {
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

	printReport()

	return nil
}

/*
https://testgrid.k8s.io/redhat-openshift-informing


release page:
https://testgrid.k8s.io/redhat-openshift-informing/summary

job:
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/table?tab=release-openshift-ocp-installer-e2e-aws-4.4&width=10&exclude-filter-by-regex=Monitor%5Cscluster&exclude-filter-by-regex=%5Eoperator.Run%20template.*container%20test%24&dashboard=redhat-openshift-ocp-release-4.4-informing
*/
