package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/openshift/sippy/pkg/api/stepmetrics"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

func (s *Server) getStepMetricsRequest(req *http.Request, validator stepmetrics.Validator) (stepmetrics.Request, error) {
	opts := stepmetrics.RequestOpts{
		URLValues: req.URL.Query(),
	}

	release := req.URL.Query().Get("release")
	if release == "" {
		return stepmetrics.Request{}, fmt.Errorf("no release provided")
	}

	if _, ok := s.currTestReports[release]; !ok {
		return stepmetrics.Request{}, fmt.Errorf("invalid release: %s", release)
	}

	opts.Current = s.currTestReports[release].CurrentPeriodReport
	opts.Previous = s.currTestReports[release].PreviousWeekReport

	return validator(opts)
}

func (s *Server) stepMetrics(w http.ResponseWriter, req *http.Request) {
	request, err := s.getStepMetricsRequest(req, stepmetrics.ValidateUIRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	timestamp := s.currTestReports[request.Release].CurrentPeriodReport.Timestamp

	tr := stepmetricshtml.NewTableRequest(
		s.currTestReports[request.Release].CurrentPeriodReport,
		s.currTestReports[request.Release].PreviousWeekReport,
		request,
	)

	table, err := stepmetricshtml.RenderRequest(tr, timestamp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Fprint(w, table)
}

func (s *Server) stepMetricsAPI(w http.ResponseWriter, req *http.Request) {
	request, err := s.getStepMetricsRequest(req, stepmetrics.ValidateAPIRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	api := stepmetrics.NewStepMetricsAPI(
		s.currTestReports[request.Release].CurrentPeriodReport,
		s.currTestReports[request.Release].PreviousWeekReport,
	)

	resp, err := api.Fetch(request)
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
