package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

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
	syntheticTestManager testgridconversion.SythenticTestManager,
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

	syntheticTestManager      testgridconversion.SythenticTestManager
	variantManager            testidentification.VariantManager
	bugCache                  buganalysis.BugCache
	testReportGeneratorConfig TestReportGeneratorConfig
	currTestReports           map[string]StandardReport
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

func (s *Server) printHtmlReport(w http.ResponseWriter, req *http.Request) {
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

	releasehtml.PrintHtmlReport(w, req,
		s.currTestReports[dashboard.ReportName].CurrentPeriodReport,
		s.currTestReports[dashboard.ReportName].CurrentTwoDayReport,
		s.currTestReports[dashboard.ReportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		15)
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
		w.Write(errMsgBytes)
		return
	}
	releaseReports[reportName] = []sippyprocessingv1.TestReport{s.currTestReports[reportName].CurrentPeriodReport, s.currTestReports[reportName].PreviousWeekReport}
	api.PrintJSONReport(w, req, releaseReports, s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays, 15)
}

func (s *Server) detailed(w http.ResponseWriter, req *http.Request) {
	reportName := "4.5"
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
			// TODO add warning
		}
	}

	testReportConfig := TestReportGeneratorConfig{
		TestGridLoadingConfig: TestGridLoadingConfig{
			LocalData: s.testReportGeneratorConfig.TestGridLoadingConfig.LocalData,
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
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}
	testReports := testReportConfig.PrepareStandardTestReports(dashboardCoordinates, s.syntheticTestManager, s.variantManager, s.bugCache)

	releasehtml.PrintHtmlReport(w, req, testReports.CurrentPeriodReport, testReports.CurrentTwoDayReport, testReports.PreviousWeekReport, numDays, jobTestCount)

}

func (s *Server) Serve() {
	http.DefaultServeMux.HandleFunc("/", s.printHtmlReport)
	http.DefaultServeMux.HandleFunc("/install", s.printInstallHtmlReport)
	http.DefaultServeMux.HandleFunc("/upgrade", s.printUpgradeHtmlReport)
	http.DefaultServeMux.HandleFunc("/operator-health", s.printOperatorHealthHtmlReport)
	http.DefaultServeMux.HandleFunc("/testdetails", s.printTestDetailHtmlReport)
	http.DefaultServeMux.HandleFunc("/json", s.printJSONReport)
	http.DefaultServeMux.HandleFunc("/detailed", s.detailed)
	http.DefaultServeMux.HandleFunc("/refresh", s.refresh)
	http.DefaultServeMux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	//go func() {
	klog.Infof("Serving reports on %s ", s.listenAddr)
	if err := http.ListenAndServe(s.listenAddr, nil); err != nil {
		klog.Exitf("Server exited: %v", err)
	}
	//}()
}
