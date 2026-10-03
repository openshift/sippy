package sippyserver

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/labels"
)

type reconcileLabelsFunc func(context.Context, labels.ReconcileRequest) (labels.ReconcileResult, labels.ReconcileOutcome)

// jsonReconcileLabels handles the job-level GCS label reconciliation endpoint.
// The request contains only job identity and notification context. Sippy reads
// the complete authoritative label set from GCS before changing PostgreSQL.
func (s *Server) jsonReconcileLabels(w http.ResponseWriter, req *http.Request) {
	if !s.hasDatabase() {
		failureResponse(w, http.StatusServiceUnavailable, "reconciling labels requires a database connection")
		return
	}

	request := labels.ReconcileRequest{RunID: mux.Vars(req)["run_id"]}
	if err := labels.ValidateReconcileRequest(request); err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	reconcile := s.reconcileLabels
	if reconcile == nil {
		reconcile = labels.NewReconciler(s.db, s.gcsClient).Reconcile
	}
	result, outcome := reconcile(req.Context(), request)
	api.RespondWithJSON(httpStatusForReconcileOutcome(outcome), w, result)
}

func httpStatusForReconcileOutcome(outcome labels.ReconcileOutcome) int {
	switch outcome {
	case labels.ReconcileOutcomeApplied, labels.ReconcileOutcomeNoOp, labels.ReconcileOutcomeRunNotFound:
		return http.StatusOK
	default:
		return http.StatusInternalServerError
	}
}
