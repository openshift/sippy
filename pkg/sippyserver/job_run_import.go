package sippyserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
)

const singleRunImportBodyLimit = 1 << 20

type jobRunImporter interface {
	Import(context.Context, prowloader.SingleRunImportRequest) (*prowloader.SingleRunImportResult, error)
}

func (s *Server) jsonImportJobRun(w http.ResponseWriter, req *http.Request) {
	if strings.TrimSpace(getUserForRequest(req)) == "" {
		failureResponse(w, http.StatusUnauthorized, "authenticated forwarded user is required")
		return
	}
	if s.jobRunImporter == nil {
		failureResponse(w, http.StatusServiceUnavailable, "single-run import dependencies are not configured")
		return
	}

	req.Body = http.MaxBytesReader(w, req.Body, singleRunImportBodyLimit)
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	var body prowloader.SingleRunImportRequest
	if err := decoder.Decode(&body); err != nil {
		failureResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		failureResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid trailing request content: %v", err))
		return
	}

	result, err := s.jobRunImporter.Import(req.Context(), body)
	if err != nil {
		failureResponse(w, singleRunImportStatus(err), err.Error())
		return
	}
	if result.Links == nil {
		result.Links = map[string]string{}
	}
	result.Links["self"] = api.GetBaseURL(req) + "/api/jobs/runs/import"
	code := http.StatusCreated
	if result.Status == "already_imported" {
		code = http.StatusOK
	}
	api.RespondWithJSON(code, w, result)
}

func singleRunImportStatus(err error) int {
	var importErr *prowloader.SingleRunImportError
	if !errors.As(err, &importErr) {
		return http.StatusInternalServerError
	}
	switch importErr.Kind {
	case prowloader.SingleRunInvalidRequest:
		return http.StatusBadRequest
	case prowloader.SingleRunUnauthenticated:
		return http.StatusUnauthorized
	case prowloader.SingleRunNotFound:
		return http.StatusNotFound
	case prowloader.SingleRunInvalidProwJob:
		return http.StatusUnprocessableEntity
	case prowloader.SingleRunArtifactFailure:
		return http.StatusBadGateway
	case prowloader.SingleRunDependencyFailure:
		return http.StatusServiceUnavailable
	case prowloader.SingleRunPersistenceFailure:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
