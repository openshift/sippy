package sippyserver

import (
	"net/http"

	"github.com/openshift/sippy/pkg/html/installhtml"
)

func (s *Server) printInstallHtmlReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintInstallHtmlReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}

func (s *Server) printUpgradeHtmlReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintUpgradeHtmlReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}

func (s *Server) printOperatorHealthHtmlReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintOperatorHealthHtmlReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}

func (s *Server) printTestDetailHtmlReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintTestDetailHtmlReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		req.URL.Query()["test"],
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}
