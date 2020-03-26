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

	"github.com/spf13/cobra"
	"k8s.io/klog"
)

var (
	dashboard_urls []string = []string{
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-informing",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.3-informing",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.2-informing",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.1-informing",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-blocking",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-blocking",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.3-blocking",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.2-blocking",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.1-blocking",
	}
	sigRegex      *regexp.Regexp = regexp.MustCompile(`\[(sig-.*?)\]`)
	bugzillaRegex *regexp.Regexp = regexp.MustCompile(`(https://bugzilla.redhat.com/show_bug.cgi\?id=\d+)`)
)

type options struct {
	SampleData     string
	SortByFlakes   bool
	SortByFailures bool
	FailureCount   int
}

type TestReport struct {
	TestName        string   `json:"testName"`
	OwningSig       string   `json:"owningSig"`
	JobsFailedCount int      `json:"jobsFailedCount"`
	JobsFailedNames []string `json:"jobsFailedNames"`
	AssociatedBug   string   `json:"associatedBug"`
}
type TestFailureMeta struct {
	name  string
	count int
	jobs  map[string]interface{}
	sig   string
	bug   string
}

type Job struct {
	OverallStatus string `json:"overall_status"`
}

type JobDetails struct {
	Name  string
	Tests []Test `json:"tests"`
}

type Test struct {
	Name string `json:"name"`
}

func badStatus(status string) bool {
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

func fetchJobs(dashboard_url string) (map[string]Job, error) {
	resp, err := http.Get(dashboard_url + "/summary")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 response code fetching dashboard page: %v", resp)
	}

	var jobs map[string]Job
	err = json.NewDecoder(resp.Body).Decode(&jobs)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func fetchJobDetails(dashboard_url, jobName string, opts *options) (JobDetails, error) {
	details := JobDetails{
		Name: jobName,
	}
	if len(opts.SampleData) == 0 {
		sortBy := ""
		switch {
		case opts.SortByFlakes:
			sortBy = "sort-by-flakiness="
		case opts.SortByFailures:
			sortBy = "sort-by-failures="
		}

		url := fmt.Sprintf("%s/table?tab=%s&exclude-filter-by-regex=Monitor%%5Cscluster&exclude-filter-by-regex=%%5Eoperator.Run%%20template.*container%%20test%%24&%s", dashboard_url, jobName, sortBy)
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

func processJobDetails(details JobDetails, opts *options, testFailures map[string]TestFailureMeta) {
	for i, test := range details.Tests {
		if i > opts.FailureCount {
			break
		}
		if test.Name == "Overall" {
			continue
		}

		klog.V(4).Infof("Found a top failing test: %q\n\n", test.Name)

		meta, ok := testFailures[test.Name]
		if !ok {
			bug := findBug(test.Name)
			meta = TestFailureMeta{
				jobs: make(map[string]interface{}),
				name: test.Name,
				bug:  bug,
			}
		}
		meta.count++
		if _, ok := meta.jobs[details.Name]; !ok {
			meta.jobs[details.Name] = struct{}{}
		}

		// find associated sig from test name
		match := sigRegex.FindStringSubmatch(test.Name)
		if len(match) > 1 {
			meta.sig = match[1]
		} else {
			meta.sig = "sig-unknown"
		}

		// update testfailures metadata
		testFailures[test.Name] = meta
	}
}

func printReport(testFailures map[string]TestFailureMeta) {
	klog.V(4).Infof("====================== Printing test report ======================\n")
	sigCount := make(map[string]int)
	var failures []TestFailureMeta
	for _, meta := range testFailures {
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
}

func main() {
	opt := &options{
		SortByFlakes:   false,
		SortByFailures: false,
		FailureCount:   1,
	}

	klog.InitFlags(nil)
	flag.CommandLine.Set("skip_headers", "true")

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			if opt.SortByFlakes && opt.SortByFailures {
				klog.Exitf("Cannot set both sort-by-flakes and sort-by-failures")
			}
			if len(opt.SampleData) > 0 && (opt.SortByFlakes || opt.SortByFailures) {
				klog.Exitf("Cannot sort tests when using sample data")
			}
			if err := opt.Run(); err != nil {
				klog.Exitf("error: %v", err)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.SampleData, "sample-data", opt.SampleData, "Path to sample testgrid data from local disk")
	flags.BoolVar(&opt.SortByFlakes, "sort-by-flakes", opt.SortByFlakes, "Sort tests by flakiness")
	flags.BoolVar(&opt.SortByFailures, "sort-by-failures", opt.SortByFailures, "Sort tests by failures")
	flags.IntVar(&opt.FailureCount, "failure-count", opt.FailureCount, "Number of test failures to report on per test job")

	flags.AddGoFlag(flag.CommandLine.Lookup("v"))
	flags.AddGoFlag(flag.CommandLine.Lookup("skip_headers"))

	if err := cmd.Execute(); err != nil {
		klog.Exitf("error: %v", err)
	}
}

func (o *options) Run() error {

	testFailures := make(map[string]TestFailureMeta)
	i := 0

	if len(o.SampleData) > 0 {
		details, err := fetchJobDetails("", "sample-job", o)
		if err != nil {
			klog.Errorf("Error fetching job details for %s: %v\n", o.SampleData, err)
		}
		processJobDetails(details, o, testFailures)
		printReport(testFailures)
		return nil
	}

	for _, dashboard_url := range dashboard_urls {
		jobs, err := fetchJobs(dashboard_url)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard_url, err)
			continue
		}

		for jobName, job := range jobs {
			if badStatus(job.OverallStatus) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := fetchJobDetails(dashboard_url, jobName, o)
				if err != nil {
					klog.Errorf("Error fetching job details for %s: %v\n", jobName, err)
				}
				processJobDetails(details, o, testFailures)
				klog.V(4).Infoln("\n\n================================================================================")
			}
			i++
			if i > 5 {
				break
			}
		}
	}

	printReport(testFailures)

	return nil
}

/*
https://testgrid.k8s.io/redhat-openshift-informing


release page:
https://testgrid.k8s.io/redhat-openshift-informing/summary

job:
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/table?tab=release-openshift-ocp-installer-e2e-aws-4.4&width=10&exclude-filter-by-regex=Monitor%5Cscluster&exclude-filter-by-regex=%5Eoperator.Run%20template.*container%20test%24&dashboard=redhat-openshift-ocp-release-4.4-informing
*/
