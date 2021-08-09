package sippyserver

import (
	"net/http"

	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

func (s *Server) stepMetrics(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	multistageJobName := req.URL.Query().Get("multistageJobName")

	if release == "" {
		generichtml.PrintStatusMessage(w, http.StatusBadRequest, "Please specify a release.")
		return
	}

	if multistageJobName == "" {
		multistageJobName = "all"
	}

	stepmetricshtml.PrintMultistageJobTable(w,
		s.currTestReports[release].CurrentPeriodReport)
	//s.currTestReports[release].PreviousWeekReport)
	//multistageJobName)
}
