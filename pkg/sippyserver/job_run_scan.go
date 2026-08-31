package sippyserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/openshift/sippy/pkg/api"
	apijobrunscan "github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/sippyserver/workqueue/symptomre"
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

// Job run symptom re-evaluation handlers

func (s *Server) jsonReEvaluateJobRunSymptoms(w http.ResponseWriter, req *http.Request) {
	log.WithField("user", getUserForRequest(req)).Info("symptom re-evaluation POST")

	var body struct {
		ProwJobBuildIDs []string `json:"prow_job_build_ids"`
		DryRun          bool     `json:"dry_run"`
	}
	req.Body = http.MaxBytesReader(w, req.Body, 1<<20) // 1 MiB limit to prevent DoS
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields() // catch client errors faster
	if err := dec.Decode(&body); err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	deduped, err := apijobrunscan.ValidateReEvalRequest(body.ProwJobBuildIDs)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.symptomReSubmitter == nil {
		failureResponse(w, http.StatusServiceUnavailable, "async symptom re-evaluation is not configured")
		return
	}

	result, err := s.symptomReSubmitter.Submit(req.Context(), deduped, body.DryRun)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, "submitting re-evaluation batch: "+err.Error())
		return
	}

	api.RespondWithJSON(http.StatusAccepted, w, map[string]interface{}{
		"batch_id":  result.BatchID,
		"requested": result.Requested,
		"links": map[string]string{
			"status": api.GetBaseURL(req) + req.URL.Path + "/" + result.BatchID.String(),
		},
	})
}

func (s *Server) jsonGetReEvaluateBatchStatus(w http.ResponseWriter, req *http.Request) {
	batchIDStr := mux.Vars(req)["batch_id"]
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid batch_id: "+err.Error())
		return
	}

	if s.symptomReStatusQuerier == nil {
		failureResponse(w, http.StatusServiceUnavailable, "batch status query is not configured")
		return
	}

	resp, err := s.symptomReStatusQuerier.Query(batchID)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resp == nil {
		failureResponse(w, http.StatusNotFound, "batch not found")
		return
	}

	api.RespondWithJSON(http.StatusOK, w, resp)
}

func (s *Server) jsonCancelReEvaluateBatch(w http.ResponseWriter, req *http.Request) {
	batchIDStr := mux.Vars(req)["batch_id"]
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid batch_id: "+err.Error())
		return
	}

	if s.symptomReCanceller == nil {
		failureResponse(w, http.StatusServiceUnavailable, "batch cancellation is not configured")
		return
	}

	resp, err := s.symptomReCanceller.Cancel(req.Context(), batchID)
	if err != nil {
		if errors.Is(err, symptomre.ErrBatchTerminal) {
			failureResponse(w, http.StatusConflict, err.Error())
		} else {
			failureResponse(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if resp == nil {
		failureResponse(w, http.StatusNotFound, "batch not found")
		return
	}

	api.RespondWithJSON(http.StatusOK, w, resp)
}
