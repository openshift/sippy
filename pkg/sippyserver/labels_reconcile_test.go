package sippyserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/openshift/sippy/pkg/api/labels"
	"github.com/openshift/sippy/pkg/db"
	"gorm.io/gorm"
)

func TestJSONReconcileLabelsRequiresRunIDPathVariable(t *testing.T) {
	server := &Server{db: &db.DB{DB: &gorm.DB{}}}
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/runs//labels/reconcile", nil)
	req = mux.SetURLVars(req, map[string]string{"run_id": ""})
	recorder := httptest.NewRecorder()

	server.jsonReconcileLabels(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestJSONReconcileLabelsForwardsRunIDFromPath(t *testing.T) {
	var got labels.ReconcileRequest
	server := &Server{
		db: &db.DB{DB: &gorm.DB{}},
		reconcileLabels: func(_ context.Context, request labels.ReconcileRequest) (labels.ReconcileResult, labels.ReconcileOutcome) {
			got = request
			return labels.ReconcileResult{RunID: "123", Message: "labels reconciled"}, labels.ReconcileOutcomeApplied
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/runs/123/labels/reconcile", nil)
	req = mux.SetURLVars(req, map[string]string{"run_id": "123"})
	recorder := httptest.NewRecorder()

	server.jsonReconcileLabels(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got.RunID != "123" {
		t.Fatalf("request = %#v, want run ID from path", got)
	}
}
