package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/openshift/sippy/pkg/api"
	apijobrunscan "github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	log "github.com/sirupsen/logrus"
)

// Job run labels CRUD handlers

func (s *Server) jsonListLabels(w http.ResponseWriter, req *http.Request) {
	labels, err := apijobrunscan.ListLabels(s.db, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, labels)
}

func (s *Server) jsonGetLabel(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	label, err := apijobrunscan.GetLabel(s.db, id, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if label == nil {
		failureResponse(w, http.StatusNotFound, "label not found")
		return
	}
	api.RespondWithJSON(http.StatusOK, w, label)
}

func (s *Server) jsonCreateLabel(w http.ResponseWriter, req *http.Request) {
	user := getUserForRequest(req)
	log.WithField("user", user).Info("label POST")
	var label jobrunscan.Label
	if err := json.NewDecoder(req.Body).Decode(&label); err != nil {
		log.WithError(err).Error("error parsing new label")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	label, err := apijobrunscan.CreateLabel(s.db.DB, label, user, req)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusCreated, w, label)
}

func (s *Server) jsonUpdateLabel(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]

	user := getUserForRequest(req)
	log.WithField("user", user).Info("label PUT")
	var label jobrunscan.Label
	if err := json.NewDecoder(req.Body).Decode(&label); err != nil {
		log.WithError(err).Error("error parsing label update")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if id != label.ID {
		failureResponse(w, http.StatusBadRequest, "resource label ID does not match URL")
		return
	}
	label, err := apijobrunscan.UpdateLabel(s.db.DB, label, user, req)
	if err != nil {
		log.WithError(err).Error("error updating label")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, label)
}

func (s *Server) jsonDeleteLabel(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]

	user := getUserForRequest(req)
	log.WithField("user", user).Info("label DELETE")
	if err := apijobrunscan.DeleteLabel(s.db.DB, id, user); err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Job run symptoms CRUD handlers

func (s *Server) jsonListSymptoms(w http.ResponseWriter, req *http.Request) {
	symptoms, err := apijobrunscan.ListSymptoms(s.db, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, symptoms)
}

func (s *Server) jsonGetSymptom(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["id"]

	symptom, err := apijobrunscan.GetSymptom(s.db, id, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if symptom == nil {
		failureResponse(w, http.StatusNotFound, "symptom not found")
		return
	}
	api.RespondWithJSON(http.StatusOK, w, symptom)
}

func (s *Server) jsonCreateSymptom(w http.ResponseWriter, req *http.Request) {
	user := getUserForRequest(req)
	log.WithField("user", user).Info("symptom POST")
	var symptom jobrunscan.Symptom
	if err := json.NewDecoder(req.Body).Decode(&symptom); err != nil {
		log.WithError(err).Error("error parsing new symptom")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	symptom, err := apijobrunscan.CreateSymptom(s.db.DB, symptom, user, req)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusCreated, w, symptom)
}

func (s *Server) jsonUpdateSymptom(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]

	user := getUserForRequest(req)
	log.WithField("user", user).Info("symptom PUT")
	var symptom jobrunscan.Symptom
	if err := json.NewDecoder(req.Body).Decode(&symptom); err != nil {
		log.WithError(err).Error("error parsing symptom update")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if id != symptom.ID {
		failureResponse(w, http.StatusBadRequest, "resource symptom ID does not match URL")
		return
	}
	symptom, err := apijobrunscan.UpdateSymptom(s.db.DB, symptom, user, req)
	if err != nil {
		log.WithError(err).Error("error updating symptom")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, symptom)
}

func (s *Server) jsonDeleteSymptom(w http.ResponseWriter, req *http.Request) {
	id := mux.Vars(req)["id"]

	user := getUserForRequest(req)
	log.WithField("user", user).Info("symptom DELETE")
	if err := apijobrunscan.DeleteSymptom(s.db.DB, id, user); err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Job run symptom re-evaluation handler

func (s *Server) jsonReEvaluateJobRunSymptoms(w http.ResponseWriter, req *http.Request) {
	log.WithField("user", getUserForRequest(req)).Info("symptom re-evaluation POST")

	var body struct {
		ProwJobBuildIDs []string `json:"prow_job_build_ids"`
		DryRun          bool     `json:"dry_run"`
	}
	req.Body = http.MaxBytesReader(w, req.Body, 1<<20) // 1 MiB limit to prevent DoS
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if s.bigQueryClient == nil || s.gcsClient == nil || s.gcsBucket == "" {
		failureResponse(w, http.StatusServiceUnavailable, "symptom re-evaluation requires BigQuery and GCS configuration")
		return
	}

	if body.DryRun {
		s.reEvaluateSynchronous(w, req, body.ProwJobBuildIDs)
		return
	}
	s.reEvaluateAsync(w, req, body.ProwJobBuildIDs)
}

func (s *Server) reEvaluateSynchronous(w http.ResponseWriter, req *http.Request, buildIDs []string) {
	if err := apijobrunscan.ValidateReEvalRequest(buildIDs); err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	re := apijobrunscan.NewReEvaluator(s.bigQueryClient, s.gcsClient, s.gcsBucket, s.db, s.cache, s.jobartifactsManager, true)
	results, err := re.ReEvaluateJobRuns(req.Context(), buildIDs)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := apijobrunscan.ReEvaluationResponse{Results: results}
	apijobrunscan.InjectReEvalHATEOASLinks(&resp, api.GetBaseURL(req))
	api.RespondWithJSON(http.StatusOK, w, resp)
}

func (s *Server) reEvaluateAsync(w http.ResponseWriter, req *http.Request, buildIDs []string) {
	if s.workqueueSubmitter == nil {
		failureResponse(w, http.StatusServiceUnavailable, "async re-evaluation is not configured")
		return
	}

	if len(buildIDs) == 0 {
		failureResponse(w, http.StatusBadRequest, "prow_job_build_ids is required")
		return
	}
	if len(buildIDs) > apijobrunscan.MaxJobRunsPerBatch {
		failureResponse(w, http.StatusBadRequest, fmt.Sprintf("maximum %d job runs per batch", apijobrunscan.MaxJobRunsPerBatch))
		return
	}

	re := apijobrunscan.NewReEvaluator(s.bigQueryClient, s.gcsClient, s.gcsBucket, s.db, s.cache, s.jobartifactsManager, false)
	symptomHash, err := re.RefreshSymptomCache()
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, "failed to load symptoms: "+err.Error())
		return
	}

	insertParams, itemKeys := apijobrunscan.BuildInsertParams(buildIDs, symptomHash)
	result, err := s.workqueueSubmitter.Submit(req.Context(), apijobrunscan.ReevaluateJobKind, insertParams, itemKeys)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, "failed to submit batch: "+err.Error())
		return
	}

	baseURL := api.GetBaseURL(req)
	api.RespondWithJSON(http.StatusAccepted, w, map[string]interface{}{
		"batch_id":  result.BatchID,
		"requested": result.Requested,
		"enqueued":  result.Enqueued,
		"deduped":   result.Deduped,
		"links": map[string]string{
			"status": baseURL + "/api/jobs/runs/reevaluate/" + result.BatchID.String(),
		},
	})
}

func (s *Server) jsonReEvaluateBatchStatus(w http.ResponseWriter, req *http.Request) {
	if s.workqueueStatusQuerier == nil {
		failureResponse(w, http.StatusServiceUnavailable, "batch status is not configured")
		return
	}

	batchIDStr := mux.Vars(req)["batch_id"]
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid batch_id: "+err.Error())
		return
	}

	status, err := s.workqueueStatusQuerier.Query(req.Context(), batchID)
	if err != nil {
		if strings.Contains(err.Error(), "record not found") {
			failureResponse(w, http.StatusNotFound, "batch not found")
			return
		}
		failureResponse(w, http.StatusInternalServerError, "failed to query batch status: "+err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, status)
}
