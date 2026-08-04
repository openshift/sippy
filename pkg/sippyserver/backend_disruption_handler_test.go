package sippyserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBackendDisruptionByRun_NoBigQueryClient(t *testing.T) {
	s := &Server{bigQueryClient: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/runs/disruption?job_run_names=123", nil)
	w := httptest.NewRecorder()

	s.jsonBackendDisruptionByRun(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	msg, _ := body["message"].(string)
	if msg == "" {
		t.Error("expected error message in response body")
	}
}

func TestBackendDisruptionByRun_MissingJobRunNames(t *testing.T) {
	s := &Server{bigQueryClient: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/runs/disruption", nil)
	w := httptest.NewRecorder()

	s.jsonBackendDisruptionByRun(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	msg, _ := body["message"].(string)
	if msg == "" {
		t.Error("expected error message in response body")
	}
}

func TestBackendDisruptionByRun_InvalidJobRunNames(t *testing.T) {
	s := &Server{bigQueryClient: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/runs/disruption?job_run_names=not-a-number", nil)
	w := httptest.NewRecorder()

	s.jsonBackendDisruptionByRun(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
