package sippyserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/labels"
)

type applyLabelFunc func(context.Context, labels.ApplyRequest) (labels.Result, labels.ApplyOutcome)

// applyLabelEndpoint keeps the label route's enablement separate from its
// runtime database availability check.
func (s *Server) applyLabelEndpoint() apiEndpoint {
	return apiEndpoint{
		EndpointPath: "/api/job/run/labels",
		Description:  "Apply one externally-sourced label to a job run in PostgreSQL (every label uses the common append path; InfraFailure also triggers summary subtraction in the same transaction)",
		Methods:      []string{http.MethodPost},
		Capabilities: []string{WriteEndpointsCapability},
		HandlerFunc:  s.jsonApplyLabel,
	}
}

// jsonApplyLabel handles POST /api/job/run/labels. It applies a single
// externally-sourced job run label request to PostgreSQL: every label is
// appended to the run's prow_job_runs.labels array, and an InfraFailure label
// additionally removes the run's contribution from the summary tables (via
// pkg/db/infrafailure.SubtractInfraFailureFromSummaries) in the same transaction.
// The endpoint is gated by the write-endpoints capability.
func (s *Server) jsonApplyLabel(w http.ResponseWriter, req *http.Request) {
	log.Info("labels apply POST")

	if !s.hasDatabase() {
		failureResponse(w, http.StatusServiceUnavailable, "applying labels requires a database connection")
		return
	}

	var request labels.ApplyRequest
	req.Body = http.MaxBytesReader(w, req.Body, 1<<20) // 1 MiB limit to prevent DoS
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields() // catch client errors faster
	if err := dec.Decode(&request); err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		failureResponse(w, http.StatusBadRequest, "invalid request body: request must contain a single JSON object")
		return
	}

	if err := labels.ValidateRequest(request); err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	applyLabel := s.applyLabel
	if applyLabel == nil {
		applyLabel = labels.NewApplier(s.db).Apply
	}
	result, outcome := applyLabel(req.Context(), request)

	api.RespondWithJSON(httpStatusForApplyOutcome(outcome), w, result)
}

func httpStatusForApplyOutcome(outcome labels.ApplyOutcome) int {
	switch outcome {
	case labels.ApplyOutcomeRecorded:
		return http.StatusCreated
	case labels.ApplyOutcomeAlreadyLabeled:
		return http.StatusOK
	case labels.ApplyOutcomeRunNotFound:
		return http.StatusNotFound
	case labels.ApplyOutcomePartitionKeyMismatch:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
