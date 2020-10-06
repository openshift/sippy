package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/openshift/sippy/pkg/html/releasehtml"

	"github.com/openshift/sippy/pkg/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"k8s.io/klog"
)

func NewServer(
	testGridLoadingOptions TestGridLoadingConfig,
	rawJobResultsAnalysisOptions RawJobResultsAnalysisConfig,
	displayDataOptions DisplayDataConfig,
	releases []string,
	listenAddr string,
) *Server {
	server := &Server{
		listenAddr: listenAddr,
		releases:   releases,
		bugCache:   buganalysis.NewBugCache(),
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
	listenAddr string
	releases   []string

	bugCache                  buganalysis.BugCache
	testReportGeneratorConfig TestReportGeneratorConfig
	currTestReports           map[string]StandardReport
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
	for _, release := range s.releases {
		s.currTestReports[release] = s.testReportGeneratorConfig.PrepareStandardTestReports(release, s.bugCache)
	}
	klog.Infof("Refresh complete")
}

func (s *Server) printHtmlReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[release]; !ok {
		releasehtml.WriteLandingPage(w, s.releases)
		return
	}
	releasehtml.PrintHtmlReport(w, req,
		s.currTestReports[release].CurrentPeriodReport,
		s.currTestReports[release].CurrentTwoDayReport,
		s.currTestReports[release].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		15)
}

func (s *Server) printJSONReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	releaseReports := make(map[string][]sippyprocessingv1.TestReport)
	if release == "all" {
		// return all available json reports
		// store [currentReport, prevReport] in a slice
		for _, r := range s.releases {
			if _, ok := s.currTestReports[r]; ok {
				releaseReports[r] = []sippyprocessingv1.TestReport{s.currTestReports[r].CurrentPeriodReport, s.currTestReports[r].PreviousWeekReport}
			} else {
				klog.Errorf("unable to load test report for release version %s", r)
				continue
			}
		}
		api.PrintJSONReport(w, req, releaseReports, s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays, 15)
		return
	} else if _, ok := s.currTestReports[release]; !ok {
		// return a 404 error along with the list of available releases in the detail section
		errMsg := map[string]interface{}{
			"code":   "404",
			"detail": fmt.Sprintf("No valid release specified, valid releases are: %v", s.releases),
		}
		errMsgBytes, _ := json.Marshal(errMsg)
		w.WriteHeader(http.StatusNotFound)
		w.Write(errMsgBytes)
		return
	}
	releaseReports[release] = []sippyprocessingv1.TestReport{s.currTestReports[release].CurrentPeriodReport, s.currTestReports[release].PreviousWeekReport}
	api.PrintJSONReport(w, req, releaseReports, s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays, 15)
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
	testReports := testReportConfig.PrepareStandardTestReports(release, s.bugCache)

	releasehtml.PrintHtmlReport(w, req, testReports.CurrentPeriodReport, testReports.CurrentTwoDayReport, testReports.PreviousWeekReport, numDays, jobTestCount)

}

func (s *Server) Serve() {
	http.DefaultServeMux.HandleFunc("/", s.printHtmlReport)
	http.DefaultServeMux.HandleFunc("/install", s.printInstallHtmlReport)
	http.DefaultServeMux.HandleFunc("/json", s.printJSONReport)
	http.DefaultServeMux.HandleFunc("/detailed", s.detailed)
	http.DefaultServeMux.HandleFunc("/refresh", s.refresh)
	//go func() {
	klog.Infof("Serving reports on %s ", s.listenAddr)
	if err := http.ListenAndServe(s.listenAddr, nil); err != nil {
		klog.Exitf("Server exited: %v", err)
	}
	//}()
}
