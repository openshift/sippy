package sippyserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openshift/sippy/pkg/api/labels"
	"github.com/openshift/sippy/pkg/db"
)

func TestJSONApplyLabelHTTPOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		outcome     labels.ApplyOutcome
		result      labels.Result
		wantStatus  int
		wantMessage string
		wantError   string
	}{
		{
			name:        "new label",
			outcome:     labels.ApplyOutcomeRecorded,
			result:      labels.Result{RunID: "123", Label: "InfraFailure", Message: "label recorded"},
			wantStatus:  http.StatusCreated,
			wantMessage: "label recorded",
		},
		{
			name:        "idempotent label",
			outcome:     labels.ApplyOutcomeAlreadyLabeled,
			result:      labels.Result{RunID: "123", Label: "InfraFailure", Message: "label already present"},
			wantStatus:  http.StatusOK,
			wantMessage: "label already present",
		},
		{
			name:        "run missing",
			outcome:     labels.ApplyOutcomeRunNotFound,
			result:      labels.Result{RunID: "123", Label: "InfraFailure", Message: "job run not found"},
			wantStatus:  http.StatusNotFound,
			wantMessage: "job run not found",
		},
		{
			name:       "application error",
			outcome:    labels.ApplyOutcomeError,
			result:     labels.Result{RunID: "123", Label: "InfraFailure", Error: "database write failed"},
			wantStatus: http.StatusInternalServerError,
			wantError:  "database write failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			server := &Server{
				db: &db.DB{},
				applyLabel: func(_ context.Context, request labels.ApplyRequest) (labels.Result, labels.ApplyOutcome) {
					called = true
					if request.Release != "4.18" {
						t.Errorf("request release = %q, want 4.18", request.Release)
					}
					return tt.result, tt.outcome
				},
			}
			requestBody := `{"run_id":"123","label":"InfraFailure","prowjob_start":"2026-08-24T11:30:00Z","release":"4.18"}`
			req := httptest.NewRequest(http.MethodPost, "https://sippy.example.com/api/job/run/labels", strings.NewReader(requestBody))
			recorder := httptest.NewRecorder()

			server.jsonApplyLabel(recorder, req)

			if !called {
				t.Fatal("applyLabel was not called")
			}
			if recorder.Code != tt.wantStatus {
				t.Errorf("HTTP status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("response body is not JSON: %v", err)
			}
			if _, ok := body["status"]; ok {
				t.Errorf("response body must not contain status: %s", recorder.Body.String())
			}
			if tt.wantMessage == "" {
				if _, ok := body["message"]; ok {
					t.Errorf("unexpected message = %v", body["message"])
				}
			} else if got := body["message"]; got != tt.wantMessage {
				t.Errorf("message = %v, want %q", got, tt.wantMessage)
			}
			if tt.wantError != "" && body["error"] != tt.wantError {
				t.Errorf("error = %v, want %q", body["error"], tt.wantError)
			}
			if _, ok := body["links"]; ok {
				t.Errorf("response body must not contain links: %s", recorder.Body.String())
			}
		})
	}
}

func TestJSONApplyLabelBadRequest(t *testing.T) {
	called := false
	server := &Server{
		db: &db.DB{},
		applyLabel: func(_ context.Context, _ labels.ApplyRequest) (labels.Result, labels.ApplyOutcome) {
			called = true
			return labels.Result{}, labels.ApplyOutcomeRecorded
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/job/run/labels", strings.NewReader(`{"run_id":`))
	recorder := httptest.NewRecorder()

	server.jsonApplyLabel(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("HTTP status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if called {
		t.Error("applyLabel must not be called for malformed JSON")
	}
}

func TestJSONApplyLabelDatabaseUnavailable(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/job/run/labels", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()

	server.jsonApplyLabel(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("HTTP status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}
