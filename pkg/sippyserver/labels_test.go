package sippyserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"

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
			name:        "partition keys mismatch",
			outcome:     labels.ApplyOutcomePartitionKeyMismatch,
			result:      labels.Result{RunID: "123", Label: "InfraFailure", Message: "job run partition keys do not match request"},
			wantStatus:  http.StatusConflict,
			wantMessage: "job run partition keys do not match request",
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
				db: &db.DB{DB: &gorm.DB{}},
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
	validRequest := `{"run_id":"123","label":"InfraFailure","prowjob_start":"2026-08-24T11:30:00Z","release":"4.18"}`
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed request", body: `{"run_id":`},
		{name: "second JSON value", body: validRequest + ` {"run_id":"456"}`},
		{name: "trailing garbage", body: validRequest + ` trailing`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			server := &Server{
				db: &db.DB{DB: &gorm.DB{}},
				applyLabel: func(_ context.Context, _ labels.ApplyRequest) (labels.Result, labels.ApplyOutcome) {
					called = true
					return labels.Result{}, labels.ApplyOutcomeRecorded
				},
			}
			req := httptest.NewRequest(http.MethodPost, "/api/job/run/labels", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			server.jsonApplyLabel(recorder, req)

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("HTTP status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if called {
				t.Error("applyLabel must not be called for an invalid request body")
			}
		})
	}
}

func TestJSONApplyLabelAllowsTrailingWhitespace(t *testing.T) {
	called := false
	server := &Server{
		db: &db.DB{DB: &gorm.DB{}},
		applyLabel: func(_ context.Context, _ labels.ApplyRequest) (labels.Result, labels.ApplyOutcome) {
			called = true
			return labels.Result{RunID: "123", Label: "InfraFailure", Message: "label recorded"}, labels.ApplyOutcomeRecorded
		},
	}
	body := `{"run_id":"123","label":"InfraFailure","prowjob_start":"2026-08-24T11:30:00Z","release":"4.18"}` + "\n\t "
	req := httptest.NewRequest(http.MethodPost, "/api/job/run/labels", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	server.jsonApplyLabel(recorder, req)

	if !called {
		t.Fatal("applyLabel was not called")
	}
	if recorder.Code != http.StatusCreated {
		t.Errorf("HTTP status = %d, want %d", recorder.Code, http.StatusCreated)
	}
}

func TestJSONApplyLabelDatabaseUnavailable(t *testing.T) {
	tests := []struct {
		name   string
		server *Server
	}{
		{name: "nil server"},
		{name: "nil database wrapper", server: &Server{}},
		{name: "nil GORM database", server: &Server{db: &db.DB{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			if tt.server != nil {
				tt.server.applyLabel = func(_ context.Context, _ labels.ApplyRequest) (labels.Result, labels.ApplyOutcome) {
					called = true
					return labels.Result{}, labels.ApplyOutcomeRecorded
				}
			}
			req := httptest.NewRequest(http.MethodPost, "/api/job/run/labels", strings.NewReader(`{}`))
			recorder := httptest.NewRecorder()

			tt.server.jsonApplyLabel(recorder, req)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Errorf("HTTP status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
			if !strings.Contains(recorder.Body.String(), "applying labels requires a database connection") {
				t.Errorf("response body = %q, want database unavailable message", recorder.Body.String())
			}
			if called {
				t.Error("applyLabel must not be called without an initialized database")
			}
		})
	}
}

func TestDetermineCapabilitiesSkipsUninitializedDatabase(t *testing.T) {
	server := &Server{db: &db.DB{}, enableWriteAPIs: true}

	server.determineCapabilities()

	if server.hasCapabilities([]string{LocalDBCapability}) {
		t.Errorf("unexpected %q capability with nil GORM database", LocalDBCapability)
	}
	if server.hasCapabilities([]string{WriteEndpointsCapability}) {
		t.Errorf("unexpected %q capability with nil GORM database", WriteEndpointsCapability)
	}
}
