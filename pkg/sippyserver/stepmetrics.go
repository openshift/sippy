package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

func (s *Server) getStepMetricsAPIResponse(req *http.Request, validator stepmetrics.Validator) (stepmetrics.Response, error) {
	opts := stepmetrics.RequestOpts{
		URLValues: req.URL.Query(),
	}

	release := req.URL.Query().Get("release")
	if release == "" {
		return stepmetrics.Response{}, fmt.Errorf("no release provided")
	}

	if _, ok := s.currTestReports[release]; !ok {
		return stepmetrics.Response{}, fmt.Errorf("invalid release: %s", release)
	}

	opts.Current = s.currTestReports[release].CurrentPeriodReport
	opts.Previous = s.currTestReports[release].PreviousWeekReport

	request, err := validator(opts)
	if err != nil {
		return stepmetrics.Response{}, err
	}

	api := stepmetrics.NewStepMetricsAPI(opts.Current, opts.Previous)
	return api.Fetch(request)
}

func (s *Server) stepMetrics(w http.ResponseWriter, req *http.Request) {
	resp, err := s.getStepMetricsAPIResponse(req, stepmetrics.ValidateUIRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	timestamp := s.currTestReports[resp.Request.Release].CurrentPeriodReport.Timestamp

	table, err := stepmetricshtml.RenderResponse(resp, timestamp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, table)
}

func (s *Server) stepMetricsAPI(w http.ResponseWriter, req *http.Request) {
	resp, err := s.getStepMetricsAPIResponse(req, stepmetrics.ValidateAPIRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
