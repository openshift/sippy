package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

func (s *Server) validateRelease(req *http.Request) (string, error) {
	release := req.URL.Query().Get("release")
	if release == "" {
		return "", fmt.Errorf("no release provided")
	}

	if _, ok := s.currTestReports[release]; !ok {
		return "", fmt.Errorf("invalid release: %s", release)
	}

	return release, nil
}

func (s *Server) validateStepMetricsQuery(req *http.Request) (stepmetrics.Request, error) {
	release, err := s.validateRelease(req)
	if err != nil {
		return stepmetrics.Request{}, err
	}

	opts := stepmetrics.RequestOpts{
		URLValues: req.URL.Query(),
		Current:   s.currTestReports[release].CurrentPeriodReport,
		Previous:  s.currTestReports[release].PreviousWeekReport,
	}

	return stepmetrics.ValidateRequest(opts)
}

func (s *Server) stepMetrics(w http.ResponseWriter, req *http.Request) {
	request, err := s.validateStepMetricsQuery(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	curr := s.currTestReports[request.Release].CurrentPeriodReport
	prev := s.currTestReports[request.Release].PreviousWeekReport

	api := stepmetrics.NewStepMetricsAPI(curr, prev)

	resp, err := api.Fetch(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	table := stepmetricshtml.NewStepMetricsHTMLTable(request.Release, curr.Timestamp)
	if err := table.RenderResponse(w, resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) stepMetricsAPI(w http.ResponseWriter, req *http.Request) {
	request, err := s.validateStepMetricsQuery(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	curr := s.currTestReports[request.Release].CurrentPeriodReport
	prev := s.currTestReports[request.Release].PreviousWeekReport

	api := stepmetrics.NewStepMetricsAPI(curr, prev)
	resp, err := api.Fetch(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	b, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(b); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
