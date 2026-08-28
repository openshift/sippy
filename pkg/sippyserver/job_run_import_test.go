package sippyserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/dataloader/prowloader"
)

type fakeJobRunImporter struct {
	result *prowloader.SingleRunImportResult
	err    error
	calls  int
	body   prowloader.SingleRunImportRequest
}

func (f *fakeJobRunImporter) Import(_ context.Context, body prowloader.SingleRunImportRequest) (*prowloader.SingleRunImportResult, error) {
	f.calls++
	f.body = body
	return f.result, f.err
}

func importRequest(t *testing.T, importer jobRunImporter, body, user string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{jobRunImporter: importer}
	req := httptest.NewRequest(http.MethodPost, "https://sippy.example/api/jobs/runs/import", strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-Forwarded-User", user)
	}
	w := httptest.NewRecorder()
	s.jsonImportJobRun(w, req)
	return w
}

func TestJobRunImportRequestAndResponses(t *testing.T) {
	validBody := `{"prow_job_run_id":"123","bucket":"test-platform-results","job_prefix":"logs/job/123"}`
	t.Run("created", func(t *testing.T) {
		fake := &fakeJobRunImporter{result: &prowloader.SingleRunImportResult{
			ProwJobRunID: "123", Status: "imported", Bucket: "test-platform-results", JobPrefix: "logs/job/123",
		}}
		w := importRequest(t, fake, validBody, "engineer")
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "123", fake.body.ProwJobRunID)
		var response prowloader.SingleRunImportResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "https://sippy.example/api/jobs/runs/import", response.Links["self"])
	})
	t.Run("duplicate", func(t *testing.T) {
		fake := &fakeJobRunImporter{result: &prowloader.SingleRunImportResult{Status: "already_imported"}}
		w := importRequest(t, fake, validBody, "engineer")
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestJobRunImportRejectsMissingIdentityBeforeImporter(t *testing.T) {
	for _, user := range []string{"", "   "} {
		fake := &fakeJobRunImporter{}
		w := importRequest(t, fake, `{}`, user)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Zero(t, fake.calls)
	}
}

func TestJobRunImportStrictJSON(t *testing.T) {
	tests := []struct {
		name, body string
	}{
		{"removed legacy location field is not public", `{"prow_job_run_id":"123","bucket":"test-platform-results","job_prefix":"logs/job/123","finished` + `_path":"logs/job/123/finished.json"}`},
		{"unknown field", `{"prow_job_run_id":"123","bucket":"test-platform-results","job_prefix":"logs/job/123","surprise":true}`},
		{"trailing JSON", `{"prow_job_run_id":"123","bucket":"test-platform-results","job_prefix":"logs/job/123"} {}`},
		{"malformed JSON", `{"prow_job_run_id":`},
		{"oversized body", `{"prow_job_run_id":"` + strings.Repeat("1", singleRunImportBodyLimit) + `"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeJobRunImporter{}
			w := importRequest(t, fake, tc.body, "engineer")
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Zero(t, fake.calls)
		})
	}
}

func TestJobRunImportAllowsTrailingWhitespace(t *testing.T) {
	fake := &fakeJobRunImporter{result: &prowloader.SingleRunImportResult{Status: "imported"}}
	w := importRequest(t, fake, `{"prow_job_run_id":"123","bucket":"test-platform-results","job_prefix":"logs/job/123"}`+" \n\t", "engineer")
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, 1, fake.calls)
}

func TestJobRunImportErrorStatusMapping(t *testing.T) {
	tests := []struct {
		kind prowloader.SingleRunImportErrorKind
		code int
	}{
		{prowloader.SingleRunInvalidRequest, 400},
		{prowloader.SingleRunUnauthenticated, 401},
		{prowloader.SingleRunNotFound, 404},
		{prowloader.SingleRunInvalidProwJob, 422},
		{prowloader.SingleRunArtifactFailure, 502},
		{prowloader.SingleRunDependencyFailure, 503},
		{prowloader.SingleRunPersistenceFailure, 500},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			fake := &fakeJobRunImporter{err: &prowloader.SingleRunImportError{Kind: tc.kind, Err: errors.New("raw detail")}}
			w := importRequest(t, fake, `{"prow_job_run_id":"123","bucket":"test-platform-results","job_prefix":"logs/job/123"}`, "engineer")
			assert.Equal(t, tc.code, w.Code)
			assert.Contains(t, w.Body.String(), "raw detail")
		})
	}
}

func TestJobRunImportUnavailableWhenImporterMissing(t *testing.T) {
	w := importRequest(t, nil, `{}`, "engineer")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
