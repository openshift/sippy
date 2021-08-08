package sippyserver

import (
	"net/http"

	"github.com/openshift/sippy/pkg/api"

	"github.com/openshift/sippy/pkg/html/installhtml"
)

func (s *Server) printInstallHTMLReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintInstallHTMLReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}

func (s *Server) printUpgradeHTMLReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintUpgradeHTMLReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}

func (s *Server) jsonUpgradeReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	api.PrintUpgradeJSONReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}

func (s *Server) jsonInstallReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	api.PrintInstallJSONReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}

func (s *Server) printOperatorHealthHTMLReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintOperatorHealthHTMLReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}

func (s *Server) printTestDetailHTMLReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[reportName]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintTestDetailHTMLReport(w, req,
		s.currTestReports[reportName].CurrentPeriodReport,
		s.currTestReports[reportName].PreviousWeekReport,
		req.URL.Query()["test"],
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		reportName,
	)
}
