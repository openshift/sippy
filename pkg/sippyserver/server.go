package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/openshift/sippy/pkg/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/html"
	"k8s.io/klog"
)

func NewServer(o Options) *Server {
	server := &Server{
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

		// most recent 2 day period (days 0-2)
		optCopy := o
		optCopy.EndDay = 2
		optCopy.StartDay = 0
		analyzer = Analyzer{
			Release:  release,
			Options:  o,
			BugCache: server.bugCache,
		}
		server.testReportGeneratorOptions[release+"-days-2"] = analyzer

		// prior 7 day period (days 7-14)
		optCopy = o
		optCopy.EndDay = 14
		optCopy.StartDay = 7
		analyzer = Analyzer{
			Release:  release,
			Options:  optCopy,
			BugCache: server.bugCache,
		}
		server.testReportGeneratorOptions[release+"-prev"] = analyzer
	}

	return server
}

type Server struct {
	bugCache                   buganalysis.BugCache
	testReportGeneratorOptions map[string]Analyzer
	currTestReports            map[string]sippyprocessingv1.TestReport
	options                    Options
}

func (s *Server) refresh(w http.ResponseWriter, req *http.Request) {
	s.RefreshData()

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) RefreshData() {
	klog.Infof("Refreshing data")
	s.bugCache.Clear()
	for k, analyzer := range s.testReportGeneratorOptions {
		s.currTestReports[k] = analyzer.PrepareTestReport()
	}
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

	opt := Options{
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
	currentReport := analyzer.PrepareTestReport()

	// current 2 day period
	optCopy := opt
	optCopy.EndDay = 2
	twoDayAnalyzer := Analyzer{
		Release:  release,
		Options:  optCopy,
		BugCache: s.bugCache,
	}
	twoDayReport := twoDayAnalyzer.PrepareTestReport()

	// prior 7 day period
	optCopy = opt
	optCopy.StartDay = endDay + 1
	optCopy.EndDay = endDay + 8
	prevAnalyzer := Analyzer{
		Release:  release,
		Options:  optCopy,
		BugCache: s.bugCache,
	}
	previousReport := prevAnalyzer.PrepareTestReport()

	html.PrintHtmlReport(w, req, currentReport, twoDayReport, previousReport, opt.EndDay, jobTestCount)

}

func (s *Server) Serve() {
	http.DefaultServeMux.HandleFunc("/", s.printHtmlReport)
	http.DefaultServeMux.HandleFunc("/json", s.printJSONReport)
	http.DefaultServeMux.HandleFunc("/detailed", s.detailed)
	http.DefaultServeMux.HandleFunc("/refresh", s.refresh)
	//go func() {
	klog.Infof("Serving reports on %s ", s.options.ListenAddr)
	if err := http.ListenAndServe(s.options.ListenAddr, nil); err != nil {
		klog.Exitf("Server exited: %v", err)
	}
	//}()
}
