package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog"

	"github.com/bparees/sippy/pkg/testgrid"
	"github.com/bparees/sippy/pkg/util"
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

	// ignored for top 10 failing test reporting only.
	ignoreTestRegex *regexp.Regexp = regexp.MustCompile(`operator.Run template|Monitor cluster while tests execute|Overall`)
)

type RawData struct {
	ByAll         map[string]util.AggregateTestResult
	ByJob         map[string]util.AggregateTestResult
	ByPlatform    map[string]util.AggregateTestResult
	BySig         map[string]util.AggregateTestResult
	FailureGroups map[string]util.JobRunResult
}

type Analyzer struct {
	RawData RawData
	Options *Options
	Report  util.TestReport
}

func (a *Analyzer) fetchJobSummaries(dashboard string) (map[string]testgrid.JobSummary, error) {
	jobs := make(map[string]testgrid.JobSummary)
	url := fmt.Sprintf("https://testgrid.k8s.io/%s/summary", dashboard)

	var buf *bytes.Buffer
	if len(a.Options.LocalData) != 0 {
		filename := a.Options.LocalData + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
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

	if len(a.Options.Download) != 0 {
		filename := a.Options.Download + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
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

func (a *Analyzer) fetchJobDetails(dashboard, jobName string) (testgrid.JobDetails, error) {
	details := testgrid.JobDetails{
		Name: jobName,
	}

	url := fmt.Sprintf("https://testgrid.k8s.io/%s/table?&show-stale-tests=&tab=%s", dashboard, jobName)

	var buf *bytes.Buffer
	if len(a.Options.LocalData) != 0 {
		filename := a.Options.LocalData + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
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

	if len(a.Options.Download) != 0 {
		filename := a.Options.Download + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
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

func (a *Analyzer) processTest(job testgrid.JobDetails, platform string, test testgrid.Test, meta util.TestMeta, startCol, endCol int) {
	col := startCol
	passed := 0
	failed := 0
	for _, result := range test.Statuses {
		switch result.Value {
		case 1:
			for i := col; i < col+result.Count && i < endCol; i++ {
				passed++
				joburl := fmt.Sprintf("https://prow.svc.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := a.RawData.FailureGroups[joburl]
				if !ok {
					jrr = util.JobRunResult{
						Job: job.Name,
						Url: joburl,
					}
				}
				jrr.TestNames = append(jrr.TestNames, test.Name)
				if test.Name == "Overall" {
					jrr.Succeeded = true
				}
				a.RawData.FailureGroups[joburl] = jrr
			}
		case 12:
			for i := col; i < col+result.Count && i < endCol; i++ {
				failed++
				joburl := fmt.Sprintf("https://prow.svc.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := a.RawData.FailureGroups[joburl]
				if !ok {
					jrr = util.JobRunResult{
						Job: job.Name,
						Url: joburl,
					}
				}
				jrr.TestNames = append(jrr.TestNames, test.Name)
				jrr.TestFailures++
				if test.Name == "Overall" {
					jrr.Failed = true
				}
				a.RawData.FailureGroups[joburl] = jrr
			}
		}
		col += result.Count
		if col > endCol {
			break
		}
	}

	util.AddTestResult("all", a.RawData.ByAll, test.Name, meta, passed, failed)
	util.AddTestResult(job.Name, a.RawData.ByJob, test.Name, meta, passed, failed)
	util.AddTestResult(platform, a.RawData.ByPlatform, test.Name, meta, passed, failed)
	util.AddTestResult(meta.Sig, a.RawData.BySig, test.Name, meta, passed, failed)
}

func (a *Analyzer) processJobDetails(job testgrid.JobDetails, testMeta map[string]util.TestMeta) {

	startCol, endCol := util.ComputeLookback(a.Options.StartDay, a.Options.Lookback, job.Timestamps)
	for _, test := range job.Tests {
		klog.V(2).Infof("Analyzing results from %d to %d from job %s for test %s\n", startCol, endCol, job.Name, test.Name)

		meta, ok := testMeta[test.Name]
		if !ok {
			meta = util.TestMeta{
				Name: test.Name,
				Jobs: make(map[string]interface{}),
				Sig:  util.FindSig(test.Name),
			}
			if a.Options.FindBugs {
				meta.Bug = util.FindBug(test.Name)
			} else {
				meta.Bug = "Bug search not requested"
			}
		}
		meta.Count++
		if _, ok := meta.Jobs[job.Name]; !ok {
			meta.Jobs[job.Name] = struct{}{}
		}

		// update test metadata
		testMeta[test.Name] = meta

		a.processTest(job, util.FindPlatform(job.Name), test, meta, startCol, endCol)
	}
}

func (a *Analyzer) analyze() {
	testMeta := make(map[string]util.TestMeta)
	for _, dashboard := range a.Options.Dashboards {
		jobs, err := a.fetchJobSummaries(dashboard)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}

		var jobFilter *regexp.Regexp
		if len(a.Options.JobFilter) > 0 {
			jobFilter = regexp.MustCompile(a.Options.JobFilter)
		}
		for jobName, job := range jobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := a.fetchJobDetails(dashboard, jobName)
				if err != nil {
					klog.Errorf("Error fetching job details for %s: %v\n", jobName, err)
				}
				a.processJobDetails(details, testMeta)
				klog.V(4).Infoln("\n\n================================================================================")
			}
		}
	}
}

func (a *Analyzer) prepareTestReport() {
	util.ComputePercentages(a.RawData.ByAll)
	util.ComputePercentages(a.RawData.ByPlatform)
	util.ComputePercentages(a.RawData.ByJob)
	util.ComputePercentages(a.RawData.BySig)

	byAll := util.GenerateSortedResults(a.RawData.ByAll, a.Options.MinRuns, a.Options.SuccessThreshold)
	byPlatform := util.GenerateSortedResults(a.RawData.ByPlatform, a.Options.MinRuns, a.Options.SuccessThreshold)
	byJob := util.GenerateSortedResults(a.RawData.ByJob, a.Options.MinRuns, a.Options.SuccessThreshold)
	bySig := util.GenerateSortedResults(a.RawData.BySig, a.Options.MinRuns, a.Options.SuccessThreshold)

	filteredFailureGroups := util.FilterFailureGroups(a.RawData.FailureGroups, a.Options.FailureClusterThreshold)
	jobPassRate := util.ComputeJobPassRate(a.RawData.FailureGroups)

	a.Report = util.TestReport{
		All:           byAll,
		ByPlatform:    byPlatform,
		ByJob:         byJob,
		BySig:         bySig,
		FailureGroups: filteredFailureGroups,
		JobPassRate:   jobPassRate,
	}
}

func (a *Analyzer) printReport() {
	a.prepareTestReport()
	switch a.Options.Output {
	case "json":
		a.printJsonReport()
	case "text":
		a.printTextReport()
	case "dashboard":
		a.printDashboardReport()
	}
}
func (a *Analyzer) printJsonReport() {
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(a.Report)
}

func (a *Analyzer) printDashboardReport() {
	fmt.Println("================== Summary Across All Jobs ==================")
	all := a.Report.All["all"]
	fmt.Printf("Passing test runs: %d\n", all.Successes)
	fmt.Printf("Failing test runs: %d\n", all.Failures)
	fmt.Printf("Test Pass Percentage: %0.2f\n", all.TestPassPercentage)

	fmt.Println("\n\n================== Top 10 Most Frequently Failing Tests ==================")
	count := 0
	for i := 0; count < 10 && i < len(all.TestResults); i++ {
		test := all.TestResults[i]
		if !ignoreTestRegex.MatchString(test.Name) && (test.Successes+test.Failures) > a.Options.MinRuns {
			fmt.Printf("Test Name: %s\n", test.Name)
			fmt.Printf("Test Pass Percentage: %0.2f (%d runs)\n", test.PassPercentage, test.Successes+test.Failures)
			if test.Successes+test.Failures < 10 {
				fmt.Printf("WARNING: Only %d runs for this test\n", test.Successes+test.Failures)
			}
			count++
			fmt.Printf("\n")
		}
	}

	fmt.Println("\n\n================== Top 10 Most Frequently Failing Jobs ==================")
	jobRunsByName := util.SummarizeJobsByName(a.Report)

	for i, v := range jobRunsByName {
		fmt.Printf("Job: %s\n", v.Name)
		fmt.Printf("Job Pass Percentage: %0.2f%% (%d runs)\n", util.Percent(v.Successes, v.Failures), v.Successes+v.Failures)
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
	for _, group := range a.Report.FailureGroups {
		count += group.TestFailures
	}
	if len(a.Report.FailureGroups) != 0 {
		fmt.Printf("%d Clustered Test Failures with an average size of %d and median of %d\n", len(a.Report.FailureGroups), count/len(a.Report.FailureGroups), a.Report.FailureGroups[len(a.Report.FailureGroups)/2].TestFailures)
	} else {
		fmt.Printf("No clustered test failures observed")
	}

	fmt.Println("\n\n================== Summary By Platform ==================")
	jobsByPlatform := util.SummarizeJobsByPlatform(a.Report)
	for _, v := range jobsByPlatform {
		fmt.Printf("Platform: %s\n", v.Platform)
		fmt.Printf("Platform Job Pass Percentage: %0.2f%% (%d runs)\n", util.Percent(v.Successes, v.Failures), v.Successes+v.Failures)
		if v.Successes+v.Failures < 10 {
			fmt.Printf("WARNING: Only %d runs for this job\n", v.Successes+v.Failures)
		}
		fmt.Printf("\n")
	}
}

func (a *Analyzer) printTextReport() {
	fmt.Println("================== Test Summary Across All Jobs ==================")
	all := a.Report.All["all"]
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
	for key, by := range a.Report.ByPlatform {
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
	for key, by := range a.Report.ByJob {
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
	for key, by := range a.Report.BySig {
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
	for _, group := range a.Report.FailureGroups {
		fmt.Printf("Job url: %s\n", group.Url)
		fmt.Printf("Number of test failures: %d\n\n", group.TestFailures)
	}

	fmt.Println("\n\n\n================== Job Pass Rates ==================")
	jobSuccesses := 0
	jobFailures := 0
	jobCount := 0

	for _, job := range a.Report.JobPassRate {
		fmt.Printf("Job: %s\n", job.Name)
		fmt.Printf("Job Successes: %d\n", job.Successes)
		fmt.Printf("Job Failures: %d\n", job.Failures)
		fmt.Printf("Job Pass Percentage: %0.2f\n\n", job.PassPercentage)
		jobSuccesses += job.Successes
		jobFailures += job.Failures
		jobCount++
	}

	fmt.Println("\n\n================== Job Summary By Platform ==================")
	jobsByPlatform := util.SummarizeJobsByPlatform(a.Report)
	for _, v := range jobsByPlatform {
		fmt.Printf("Platform: %s\n", v.Platform)
		fmt.Printf("Job Succeses: %d\n", v.Successes)
		fmt.Printf("Job Failures: %d\n", v.Failures)
		fmt.Printf("Platform Job Pass Percentage: %0.2f%% (%d runs)\n", util.Percent(v.Successes, v.Failures), v.Successes+v.Failures)
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
	fmt.Printf("Total Job Pass Percentage: %0.2f\n\n", util.Percent(jobSuccesses, jobFailures))

	fmt.Printf("Total Tests: %d\n", testCount)
	fmt.Printf("Total Test Successes: %d\n", testSuccesses)
	fmt.Printf("Total Test Failures: %d\n", testFailures)
	fmt.Printf("Total Test Pass Percentage: %0.2f\n", util.Percent(testSuccesses, testFailures))
}

func (a *Analyzer) printHtmlReport(w http.ResponseWriter, req *http.Request) {

}

func (a *Analyzer) serve() {
	http.DefaultServeMux.HandleFunc("/", a.printHtmlReport)
	go func() {
		klog.Infof("Serving reports on %s ", a.Options.ListenAddr)
		if err := http.ListenAndServe(a.Options.ListenAddr, nil); err != nil {
			klog.Exitf("Server exited: %v", err)
		}
	}()
}

type Options struct {
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
	ListenAddr              string
	Server                  bool
}

func main() {
	opt := &Options{
		Lookback:                14,
		SuccessThreshold:        99,
		MinRuns:                 10,
		Output:                  "json",
		FailureClusterThreshold: 10,
		StartDay:                0,
		ListenAddr:              ":8080",
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
	flag.StringVar(&opt.ListenAddr, "listen", opt.ListenAddr, "The address to serve analysis reports on")
	flags.BoolVar(&opt.Server, "server", opt.Server, "Run in web server mode (serve reports over http)")

	flags.AddGoFlag(flag.CommandLine.Lookup("v"))
	flags.AddGoFlag(flag.CommandLine.Lookup("skip_headers"))

	if err := cmd.Execute(); err != nil {
		klog.Exitf("error: %v", err)
	}
}

func (o *Options) Run() error {

	if len(o.Dashboards) == 0 {
		o.Dashboards = defaultDashboards
	}

	analyzer := Analyzer{
		Options: o,
		RawData: RawData{
			ByAll:         make(map[string]util.AggregateTestResult),
			ByJob:         make(map[string]util.AggregateTestResult),
			ByPlatform:    make(map[string]util.AggregateTestResult),
			BySig:         make(map[string]util.AggregateTestResult),
			FailureGroups: make(map[string]util.JobRunResult),
		},
	}

	if !o.Server {
		analyzer.analyze()
		analyzer.printReport()
	}

	if o.Server {
		analyzer.analyze()
	}

	return nil
}
