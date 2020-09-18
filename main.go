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
	"strconv"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"

	"github.com/openshift/sippy/pkg/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/html"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testreportconversion"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/spf13/cobra"
	"k8s.io/klog"
)

var (
	dashboardTemplate = "redhat-openshift-ocp-release-%s-%s"
)

type Analyzer struct {
	Options        *Options
	LastUpdateTime time.Time
	Release        string

	BugCache buganalysis.BugCache
}

func loadJobSummaries(dashboard string, storagePath string) (map[string]testgridv1.JobSummary, time.Time, error) {
	jobs := make(map[string]testgridv1.JobSummary)
	url := fmt.Sprintf("https://testgrid.k8s.io/%s/summary", dashboard)

	var buf *bytes.Buffer
	filename := storagePath + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return jobs, time.Time{}, fmt.Errorf("Could not read local data file %s: %v", filename, err)
	}
	buf = bytes.NewBuffer(b)
	f, _ := os.Stat(filename)
	f.ModTime()

	err = json.NewDecoder(buf).Decode(&jobs)
	if err != nil {
		return nil, time.Time{}, err
	}

	return jobs, f.ModTime(), nil

}

func downloadJobSummaries(dashboard string, storagePath string) error {
	url := fmt.Sprintf("https://testgrid.k8s.io/%s/summary", dashboard)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Non-200 response code fetching job summary: %v", resp)
	}
	filename := storagePath + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer([]byte{})
	io.Copy(buf, resp.Body)

	_, err = f.Write(buf.Bytes())
	return err
}

func loadJobDetails(dashboard, jobName, storagePath string) (testgridv1.JobDetails, error) {
	details := testgridv1.JobDetails{
		Name: jobName,
	}

	url := fmt.Sprintf("https://testgrid.k8s.io/%s/table?&show-stale-tests=&tab=%s&grid=old", dashboard, jobName)

	var buf *bytes.Buffer
	filename := storagePath + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return details, fmt.Errorf("Could not read local data file %s: %v", filename, err)
	}
	buf = bytes.NewBuffer(b)

	err = json.NewDecoder(buf).Decode(&details)
	if err != nil {
		return details, err
	}
	details.TestGridUrl = fmt.Sprintf("https://testgrid.k8s.io/%s#%s", dashboard, jobName)
	return details, nil
}

// https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/table?&show-stale-tests=&tab=release-openshift-origin-installer-e2e-azure-compact-4.4

// https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing#release-openshift-origin-installer-e2e-azure-compact-4.4&show-stale-tests=&sort-by-failures=

func downloadJobDetails(dashboard, jobName, storagePath string) error {
	url := fmt.Sprintf("https://testgrid.k8s.io/%s/table?&show-stale-tests=&tab=%s&grid=old", dashboard, jobName)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Non-200 response code fetching job details: %v", resp)
	}

	filename := storagePath + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer([]byte{})
	io.Copy(buf, resp.Body)

	_, err = f.Write(buf.Bytes())
	return err

}

func getFailedTestNamesFromJobResults(jobResults map[string]testgridanalysisapi.RawJobResult) sets.String {
	failedTestNames := sets.NewString()
	for _, jobResult := range jobResults {
		for _, jobrun := range jobResult.JobRunResults {
			failedTestNames.Insert(jobrun.FailedTestNames...)
		}
	}
	return failedTestNames
}

// updateBugCacheForJobResults looks up all the bugs related to every failing test in the jobResults and returns a list of
// warnings/errors that happened looking up the data
func updateBugCacheForJobResults(bugCache buganalysis.BugCache, rawJobResults testgridanalysisapi.RawData) []string {
	warnings := []string{}

	// now that we have all the test failures (remember we added sythentics), use that to update the bugzilla cache
	failedTestNamesAcrossAllJobRuns := getFailedTestNamesFromJobResults(rawJobResults.JobResults)
	err := bugCache.UpdateForFailedTests(failedTestNamesAcrossAllJobRuns.List()...)
	if err != nil {
		klog.Error(err)
		warnings = append(warnings, fmt.Sprintf("Bugzilla Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error()))
	}

	return warnings
}

func (a *Analyzer) getTestGridData(releases []string, storagePath string) []testgridv1.JobDetails {
	testGridJobDetails := []testgridv1.JobDetails{}

	var jobFilter *regexp.Regexp
	if len(a.Options.JobFilter) > 0 {
		jobFilter = regexp.MustCompile(a.Options.JobFilter)
	}

	for _, release := range releases {
		dashboard := fmt.Sprintf(dashboardTemplate, release, "blocking")
		blockingJobs, ts, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error loading dashboard page %s: %v\n", dashboard, err)
			continue
		}
		a.LastUpdateTime = ts
		for jobName, job := range blockingJobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := loadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error loading job details for %s: %v\n", jobName, err)
				} else {
					testGridJobDetails = append(testGridJobDetails, details)
				}
			}
		}
	}
	for _, release := range releases {
		dashboard := fmt.Sprintf(dashboardTemplate, release, "informing")
		informingJobs, _, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error load dashboard page %s: %v\n", dashboard, err)
			continue
		}

		for jobName, job := range informingJobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := loadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error loading job details for %s: %v\n", jobName, err)
				} else {
					testGridJobDetails = append(testGridJobDetails, details)
				}
			}
		}
	}

	return testGridJobDetails
}

func downloadData(releases []string, filter string, storagePath string) {
	var jobFilter *regexp.Regexp
	if len(filter) > 0 {
		jobFilter = regexp.MustCompile(filter)
	}

	for _, release := range releases {

		dashboard := fmt.Sprintf(dashboardTemplate, release, "blocking")
		err := downloadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}
		blockingJobs, _, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error loading dashboard page %s: %v\n", dashboard, err)
			continue
		}

		for jobName, job := range blockingJobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				err := downloadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error fetching job details for %s: %v\n", jobName, err)
				}
			}
		}
	}

	for _, release := range releases {

		dashboard := fmt.Sprintf(dashboardTemplate, release, "informing")
		err := downloadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}
		informingJobs, _, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}

		for jobName, job := range informingJobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				err := downloadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error fetching job details for %s: %v\n", jobName, err)
				}
			}
		}
	}
}

func (a *Analyzer) prepareTestReport() sippyprocessingv1.TestReport {
	testGridJobDetails := a.getTestGridData([]string{a.Release}, a.Options.LocalData)
	rawJobResultOptions := testgridconversion.ProcessingOptions{StartDay: a.Options.StartDay, EndDay: a.Options.EndDay}
	rawJobResults := rawJobResultOptions.ProcessTestGridDataIntoRawJobResults(testGridJobDetails)
	bugCacheWarnings := updateBugCacheForJobResults(a.BugCache, rawJobResults)

	return testreportconversion.PrepareTestReport(
		rawJobResults,
		a.BugCache,
		a.Release,
		a.Options.MinTestRuns,
		a.Options.TestSuccessThreshold,
		a.Options.EndDay,
		bugCacheWarnings,
		a.LastUpdateTime,
		a.Options.FailureClusterThreshold,
	)
}

func (a *Analyzer) printJSONReport(testReport sippyprocessingv1.TestReport) {
	switch a.Options.Output {
	case "json":
		printJsonReport(testReport)
	}
}
func printJsonReport(testReport sippyprocessingv1.TestReport) {
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(testReport)
}

type Server struct {
	bugCache                   buganalysis.BugCache
	testReportGeneratorOptions map[string]Analyzer
	currTestReports            map[string]sippyprocessingv1.TestReport
	options                    *Options
}

func (s *Server) refresh(w http.ResponseWriter, req *http.Request) {
	klog.Infof("Refreshing data")
	s.bugCache.Clear()

	for k, analyzer := range s.testReportGeneratorOptions {
		s.currTestReports[k] = analyzer.prepareTestReport()
	}

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	klog.Infof("Refresh complete")
}

func (s *Server) printHtmlReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[release]; !ok {
		html.WriteLandingPage(w, s.options.Releases)
		return
	}
	html.PrintHtmlReport(w, req, s.currTestReports[release], s.currTestReports[release+"-days-2"], s.currTestReports[release+"-prev"], s.options.EndDay, 15)
}

func (s *Server) printJSONReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	releaseReports := make(map[string][]sippyprocessingv1.TestReport)
	if release == "all" {
		// return all available json reports
		// store [currentReport, prevReport] in a slice
		for _, r := range s.options.Releases {
			if _, ok := s.currTestReports[r]; ok {
				releaseReports[r] = []sippyprocessingv1.TestReport{s.currTestReports[r], s.currTestReports[r+"-prev"]}
			} else {
				klog.Errorf("unable to load test report for release version %s", r)
				continue
			}
		}
		api.PrintJSONReport(w, req, releaseReports, s.options.EndDay, 15)
		return
	} else if _, ok := s.currTestReports[release]; !ok {
		// return a 404 error along with the list of available releases in the detail section
		errMsg := map[string]interface{}{
			"code":   "404",
			"detail": fmt.Sprintf("No valid release specified, valid releases are: %v", s.options.Releases),
		}
		errMsgBytes, _ := json.Marshal(errMsg)
		w.WriteHeader(http.StatusNotFound)
		w.Write(errMsgBytes)
		return
	}
	releaseReports[release] = []sippyprocessingv1.TestReport{s.currTestReports[release], s.currTestReports[release+"-prev"]}
	api.PrintJSONReport(w, req, releaseReports, s.options.EndDay, 15)
}

func (s *Server) detailed(w http.ResponseWriter, req *http.Request) {

	release := "4.5"
	t := req.URL.Query().Get("release")
	if t != "" {
		release = t
	}

	startDay := 0
	t = req.URL.Query().Get("startDay")
	if t != "" {
		startDay, _ = strconv.Atoi(t)
	}

	endDay := startDay + 7
	t = req.URL.Query().Get("endDay")
	if t != "" {
		endDay, _ = strconv.Atoi(t)
	}

	testSuccessThreshold := 98.0
	t = req.URL.Query().Get("testSuccessThreshold")
	if t != "" {
		testSuccessThreshold, _ = strconv.ParseFloat(t, 64)
	}

	jobFilter := ""
	t = req.URL.Query().Get("jobFilter")
	if t != "" {
		jobFilter = t
	}

	minTestRuns := 10
	t = req.URL.Query().Get("minTestRuns")
	if t != "" {
		minTestRuns, _ = strconv.Atoi(t)
	}

	fct := 10
	t = req.URL.Query().Get("failureClusterThreshold")
	if t != "" {
		fct, _ = strconv.Atoi(t)
	}

	jobTestCount := 10
	t = req.URL.Query().Get("jobTestCount")
	if t != "" {
		jobTestCount, _ = strconv.Atoi(t)
	}

	opt := &Options{
		StartDay:                startDay,
		EndDay:                  endDay,
		TestSuccessThreshold:    testSuccessThreshold,
		JobFilter:               jobFilter,
		MinTestRuns:             minTestRuns,
		FailureClusterThreshold: fct,
		LocalData:               s.options.LocalData,
	}

	analyzer := Analyzer{
		Release:  release,
		Options:  opt,
		BugCache: s.bugCache,
	}
	currentReport := analyzer.prepareTestReport()

	// current 2 day period
	optCopy := *opt
	optCopy.EndDay = 2
	twoDayAnalyzer := Analyzer{
		Release:  release,
		Options:  &optCopy,
		BugCache: s.bugCache,
	}
	twoDayReport := twoDayAnalyzer.prepareTestReport()

	// prior 7 day period
	optCopy = *opt
	optCopy.StartDay = endDay + 1
	optCopy.EndDay = endDay + 8
	prevAnalyzer := Analyzer{
		Release:  release,
		Options:  &optCopy,
		BugCache: s.bugCache,
	}
	previousReport := prevAnalyzer.prepareTestReport()

	html.PrintHtmlReport(w, req, currentReport, twoDayReport, previousReport, opt.EndDay, jobTestCount)

}

func (s *Server) serve(opts *Options) {
	http.DefaultServeMux.HandleFunc("/", s.printHtmlReport)
	http.DefaultServeMux.HandleFunc("/json", s.printJSONReport)
	http.DefaultServeMux.HandleFunc("/detailed", s.detailed)
	http.DefaultServeMux.HandleFunc("/refresh", s.refresh)
	//go func() {
	klog.Infof("Serving reports on %s ", opts.ListenAddr)
	if err := http.ListenAndServe(opts.ListenAddr, nil); err != nil {
		klog.Exitf("Server exited: %v", err)
	}
	//}()
}

type Options struct {
	LocalData               string
	Releases                []string
	StartDay                int
	EndDay                  int
	TestSuccessThreshold    float64
	JobFilter               string
	MinTestRuns             int
	Output                  string
	FailureClusterThreshold int
	FetchData               string
	ListenAddr              string
	Server                  bool
}

func main() {
	opt := &Options{
		EndDay:                  7,
		TestSuccessThreshold:    99.99,
		MinTestRuns:             10,
		Output:                  "json",
		FailureClusterThreshold: 10,
		StartDay:                0,
		ListenAddr:              ":8080",
		Releases:                []string{"4.4"},
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
	flags.StringArrayVar(&opt.Releases, "release", opt.Releases, "Which releases to analyze (one per arg instance)")
	flags.IntVar(&opt.StartDay, "start-day", opt.StartDay, "Analyze data starting from this day")
	flags.IntVar(&opt.EndDay, "end-day", opt.EndDay, "Look at job runs going back to this day")
	flags.Float64Var(&opt.TestSuccessThreshold, "test-success-threshold", opt.TestSuccessThreshold, "Filter results for tests that are more than this percent successful")
	flags.StringVar(&opt.JobFilter, "job-filter", opt.JobFilter, "Only analyze jobs that match this regex")
	flags.StringVar(&opt.FetchData, "fetch-data", opt.FetchData, "Download testgrid data to directory specified for future use with --local-data")
	flags.IntVar(&opt.MinTestRuns, "min-test-runs", opt.MinTestRuns, "Ignore tests with less than this number of runs")
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
	switch o.Output {
	case "json", "text", "dashboard":
	default:
		return fmt.Errorf("invalid output type: %s\n", o.Output)
	}

	if len(o.FetchData) != 0 {
		downloadData(o.Releases, o.JobFilter, o.FetchData)
		return nil
	}
	if !o.Server {
		analyzer := Analyzer{
			Options:  o,
			BugCache: buganalysis.NewBugCache(),
		}

		testReport := analyzer.prepareTestReport()
		analyzer.printJSONReport(testReport)
	}

	if o.Server {
		server := Server{
			bugCache:                   buganalysis.NewBugCache(),
			testReportGeneratorOptions: make(map[string]Analyzer),
			currTestReports:            make(map[string]sippyprocessingv1.TestReport),
			options:                    o,
		}
		for _, release := range o.Releases {
			// most recent 7 day period (days 0-7)
			analyzer := Analyzer{
				Release:  release,
				Options:  o,
				BugCache: server.bugCache,
			}

			server.testReportGeneratorOptions[release] = analyzer
			server.currTestReports[release] = analyzer.prepareTestReport()

			// most recent 2 day period (days 0-2)
			optCopy := *o
			optCopy.EndDay = 2
			optCopy.StartDay = 0
			analyzer = Analyzer{
				Release:  release,
				Options:  o,
				BugCache: server.bugCache,
			}
			server.testReportGeneratorOptions[release+"-days-2"] = analyzer
			server.currTestReports[release+"-days-2"] = analyzer.prepareTestReport()

			// prior 7 day period (days 7-14)
			optCopy = *o
			optCopy.EndDay = 14
			optCopy.StartDay = 7
			analyzer = Analyzer{
				Release:  release,
				Options:  &optCopy,
				BugCache: server.bugCache,
			}
			server.testReportGeneratorOptions[release+"-prev"] = analyzer
			server.currTestReports[release+"-prev"] = analyzer.prepareTestReport()
		}
		server.serve(o)
	}

	return nil
}
