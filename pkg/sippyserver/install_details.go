package sippyserver

import (
	"net/http"

	"github.com/openshift/sippy/pkg/html/installhtml"
)

func (s *Server) printInstallHtmlReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	if _, ok := s.currTestReports[release]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	installhtml.PrintInstallHtmlReport(w, req,
		s.currTestReports[release].CurrentPeriodReport,
		s.currTestReports[release].PreviousWeekReport,
		release,
	)
}
