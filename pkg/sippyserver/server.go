package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/openshift/sippy/pkg/html/generichtml"

	"github.com/openshift/sippy/pkg/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/html/releasehtml"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"k8s.io/klog"
)

func NewServer(
	testGridLoadingOptions TestGridLoadingConfig,
	rawJobResultsAnalysisOptions RawJobResultsAnalysisConfig,
	displayDataOptions DisplayDataConfig,
	dashboardCoordinates []TestGridDashboardCoordinates,
	listenAddr string,
	syntheticTestManager testgridconversion.SyntheticTestManager,
	variantManager testidentification.VariantManager,
	bugCache buganalysis.BugCache,
) *Server {

	server := &Server{
		listenAddr:           listenAddr,
		dashboardCoordinates: dashboardCoordinates,

		syntheticTestManager: syntheticTestManager,
		variantManager:       variantManager,
		bugCache:             bugCache,
		testReportGeneratorConfig: TestReportGeneratorConfig{
			TestGridLoadingConfig:       testGridLoadingOptions,
			RawJobResultsAnalysisConfig: rawJobResultsAnalysisOptions,
			DisplayDataConfig:           displayDataOptions,
		},
		currTestReports: map[string]StandardReport{},
	}

	return server
}

type Server struct {
	listenAddr           string
	dashboardCoordinates []TestGridDashboardCoordinates

	syntheticTestManager      testgridconversion.SyntheticTestManager
	variantManager            testidentification.VariantManager
	bugCache                  buganalysis.BugCache
	testReportGeneratorConfig TestReportGeneratorConfig
	currTestReports           map[string]StandardReport
	httpServer                *http.Server
}

type TestGridDashboardCoordinates struct {
	// this is how we index and display.  it gets wired to ?release for now
	ReportName string
	// this is generic and is required
	TestGridDashboardNames []string
	// this is openshift specific, used for BZ lookup and not required
	BugzillaRelease string
}

type StandardReport struct {
	CurrentPeriodReport sippyprocessingv1.TestReport
	CurrentTwoDayReport sippyprocessingv1.TestReport
	PreviousWeekReport  sippyprocessingv1.TestReport
}

func (s *Server) refresh(w http.ResponseWriter, req *http.Request) {
	s.RefreshData()

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) RefreshData() {
	klog.Infof("Refreshing data")
	s.bugCache.Clear()
	for _, dashboard := range s.dashboardCoordinates {
		s.currTestReports[dashboard.ReportName] = s.testReportGeneratorConfig.PrepareStandardTestReports(dashboard, s.syntheticTestManager, s.variantManager, s.bugCache)
	}
	klog.Infof("Refresh complete")
}

func (s *Server) defaultHandler(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/" {
		s.printHTMLReport(w, req)
	} else {
		generichtml.PrintStatusMessage(w, http.StatusNotFound, "Page not found.")
	}
}

func (s *Server) printHTMLReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	dashboard, found := s.reportNameToDashboardCoordinates(reportName)
	if !found {
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}
	if _, hasReport := s.currTestReports[dashboard.ReportName]; !hasReport {
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}

	releasehtml.PrintHTMLReport(w, req,
		s.currTestReports[dashboard.ReportName].CurrentPeriodReport,
		s.currTestReports[dashboard.ReportName].CurrentTwoDayReport,
		s.currTestReports[dashboard.ReportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		15,
		s.reportNames())
}

func (s *Server) printCanaryReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	dashboard, found := s.reportNameToDashboardCoordinates(reportName)
	if !found {
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}
	if _, hasReport := s.currTestReports[dashboard.ReportName]; !hasReport {
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}

	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	testReport := s.currTestReports[dashboard.ReportName].CurrentPeriodReport
	for i := len(testReport.ByTest) - 1; i >= 0; i-- {
		t := testReport.ByTest[i]
		if t.TestResultAcrossAllJobs.PassPercentage > 99 {
			fmt.Fprintf(w, "%q:struct{}{},\n", t.TestName)
		} else {
			break
		}
	}
}

func (s *Server) reportNameToDashboardCoordinates(reportName string) (TestGridDashboardCoordinates, bool) {
	for _, dashboard := range s.dashboardCoordinates {
		if dashboard.ReportName == reportName {
			return dashboard, true
		}
	}
	return TestGridDashboardCoordinates{}, false
}

func (s *Server) reportNames() []string {
	ret := []string{}
	for _, dashboard := range s.dashboardCoordinates {
		ret = append(ret, dashboard.ReportName)
	}
	return ret
}

func (s *Server) printJSONReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	releaseReports := make(map[string][]sippyprocessingv1.TestReport)
	if reportName == "all" {
		// return all available json reports
		// store [currentReport, prevReport] in a slice
		for _, reportName := range s.reportNames() {
			if _, ok := s.currTestReports[reportName]; ok {
				releaseReports[reportName] = []sippyprocessingv1.TestReport{s.currTestReports[reportName].CurrentPeriodReport, s.currTestReports[reportName].PreviousWeekReport}
			} else {
				klog.Errorf("unable to load test report for reportName version %s", reportName)
				continue
			}
		}
		api.PrintJSONReport(w, req, releaseReports, s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays, 15)
		return
	} else if _, ok := s.currTestReports[reportName]; !ok {
		// return a 404 error along with the list of available openshiftReleases in the detail section
		errMsg := map[string]interface{}{
			"code":   "404",
			"detail": fmt.Sprintf("No valid reportName specified, valid reportNames are: %v", s.reportNames()),
		}
		errMsgBytes, _ := json.Marshal(errMsg)
		w.WriteHeader(http.StatusNotFound)
		if _, err := w.Write(errMsgBytes); err != nil {
			klog.Errorf(err.Error())
		}
		return
	}
	releaseReports[reportName] = []sippyprocessingv1.TestReport{s.currTestReports[reportName].CurrentPeriodReport, s.currTestReports[reportName].PreviousWeekReport}
	api.PrintJSONReport(w, req, releaseReports, s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays, 15)
}

func (s *Server) detailed(w http.ResponseWriter, req *http.Request) {
	reportName := "4.8"

	// Default to the first release given on the command-line
	reportNames := s.reportNames()
	if len(reportNames) > 0 {
		reportName = reportNames[0]
	}

	t := req.URL.Query().Get("release")
	if t != "" {
		reportName = t
	}

	startDay := 0
	t = req.URL.Query().Get("startDay")
	if t != "" {
		startDay, _ = strconv.Atoi(t)
	}

	numDays := 7
	t = req.URL.Query().Get("endDay")
	if t != "" {
		endDay, _ := strconv.Atoi(t)
		numDays = endDay - startDay
	}

	testSuccessThreshold := 98.0
	t = req.URL.Query().Get("testSuccessThreshold")
	if t != "" {
		testSuccessThreshold, _ = strconv.ParseFloat(t, 64)
	}

	jobFilterString := ""
	t = req.URL.Query().Get("jobFilter")
	if t != "" {
		jobFilterString = t
	}

	minTestRuns := 10
	t = req.URL.Query().Get("minTestRuns")
	if t != "" {
		minTestRuns, _ = strconv.Atoi(t)
	}

	failureClusterThreshold := 10
	t = req.URL.Query().Get("failureClusterThreshold")
	if t != "" {
		failureClusterThreshold, _ = strconv.Atoi(t)
	}

	jobTestCount := 10
	t = req.URL.Query().Get("jobTestCount")
	if t != "" {
		jobTestCount, _ = strconv.Atoi(t)
	}

	var jobFilter *regexp.Regexp
	if len(jobFilterString) > 0 {
		var err error
		jobFilter, err = regexp.Compile(jobFilterString)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid jobFilter: %s", err), http.StatusBadRequest)
			return
		}
	}

	testReportConfig := TestReportGeneratorConfig{
		TestGridLoadingConfig: TestGridLoadingConfig{
			LocalData: s.testReportGeneratorConfig.TestGridLoadingConfig.LocalData,
			Loader:    s.testReportGeneratorConfig.TestGridLoadingConfig.Loader,
			JobFilter: jobFilter,
		},
		RawJobResultsAnalysisConfig: RawJobResultsAnalysisConfig{
			StartDay: startDay,
			NumDays:  numDays,
		},
		DisplayDataConfig: DisplayDataConfig{
			MinTestRuns:             minTestRuns,
			TestSuccessThreshold:    testSuccessThreshold,
			FailureClusterThreshold: failureClusterThreshold,
		},
	}

	dashboardCoordinates, found := s.reportNameToDashboardCoordinates(reportName)
	if !found {
		releasehtml.WriteLandingPage(w, reportNames)
		return
	}
	testReports := testReportConfig.PrepareStandardTestReports(dashboardCoordinates, s.syntheticTestManager, s.variantManager, s.bugCache)

	releasehtml.PrintHTMLReport(w, req,
		testReports.CurrentPeriodReport,
		testReports.CurrentTwoDayReport,
		testReports.PreviousWeekReport,
		numDays, jobTestCount, reportNames)

}

func (s *Server) jobs(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	jobFilterString := req.URL.Query().Get("jobFilter")

	var jobFilter *regexp.Regexp
	if len(jobFilterString) > 0 {
		var err error
		jobFilter, err = regexp.Compile(jobFilterString)
		if err != nil {
			http.Error(w, fmt.Sprintf("jobFilter: %s", err), http.StatusBadRequest)
			return
		}
	}

	dashboardCoordinates, found := s.reportNameToDashboardCoordinates(reportName)
	if !found {
		http.Error(w, fmt.Sprintf("release %s not found", reportName), http.StatusBadRequest)
		return
	}

	testGridJobDetails, lastUpdateTime := s.testReportGeneratorConfig.TestGridLoadingConfig.loadWithFilter(
		dashboardCoordinates.TestGridDashboardNames, jobFilter)

	api.PrintJobsReport(w, s.syntheticTestManager, testGridJobDetails, lastUpdateTime)
}

func (s *Server) jobsReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	releasehtml.PrintJobsReport(w, reportName)
}

func (s *Server) variantsReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	variant := req.URL.Query().Get("variant")
	reports := s.currTestReports

	if variant == "" || release == "" {
		generichtml.PrintStatusMessage(w, http.StatusBadRequest, "Please specify a variant and release.")
		return
	}

	if _, ok := reports[release]; !ok {
		generichtml.PrintStatusMessage(w, http.StatusNotFound, fmt.Sprintf("Release %q not found.", release))
		return
	}

	var currentWeek *sippyprocessingv1.VariantResults
	for i, report := range reports[release].CurrentPeriodReport.ByVariant {
		if report.VariantName == variant {
			currentWeek = &reports[release].CurrentPeriodReport.ByVariant[i]
			break
		}
	}

	var previousWeek *sippyprocessingv1.VariantResults
	for i, report := range reports[release].PreviousWeekReport.ByVariant {
		if report.VariantName == variant {
			previousWeek = &reports[release].PreviousWeekReport.ByVariant[i]
			break
		}
	}

	if currentWeek == nil {
		generichtml.PrintStatusMessage(w, http.StatusNotFound, fmt.Sprintf("Variant %q not found.", variant))
		return
	}

	timestamp := reports[release].CurrentPeriodReport.Timestamp

	releasehtml.PrintVariantsReport(w, release, variant, currentWeek, previousWeek, timestamp)
}

func (s *Server) Serve() {
	// Use private ServeMux to prevent tests from stomping on http.DefaultServeMux
	serveMux := http.NewServeMux()

	serveMux.HandleFunc("/", s.printHTMLReport)
	serveMux.HandleFunc("/install", s.printInstallHTMLReport)
	serveMux.HandleFunc("/upgrade", s.printUpgradeHTMLReport)
	serveMux.HandleFunc("/operator-health", s.printOperatorHealthHTMLReport)
	serveMux.HandleFunc("/testdetails", s.printTestDetailHTMLReport)
	serveMux.HandleFunc("/json", s.printJSONReport)
	serveMux.HandleFunc("/detailed", s.detailed)
	serveMux.HandleFunc("/refresh", s.refresh)
	serveMux.HandleFunc("/canary", s.printCanaryReport)
	serveMux.HandleFunc("/api/jobs", s.jobs)
	serveMux.HandleFunc("/jobs", s.jobsReport)
	serveMux.HandleFunc("/variants", s.variantsReport)
	serveMux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Store a pointer to the HTTP server for later retrieval.
	s.httpServer = &http.Server{
		Addr:    s.listenAddr,
		Handler: serveMux,
	}

	klog.Infof("Serving reports on %s ", s.listenAddr)

	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		klog.Exitf("Server exited: %v", err)
	}
}

func (s *Server) GetHTTPServer() *http.Server {
	return s.httpServer
}
