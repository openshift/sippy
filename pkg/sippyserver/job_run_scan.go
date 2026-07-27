package sippyserver

import (
	"context"
	"encoding/json"
	"net/http"

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

// Job run symptom re-evaluation handlers

func (s *Server) jsonReEvaluateJobRunSymptoms(w http.ResponseWriter, req *http.Request) {
	user := getUserForRequest(req)
	log.WithField("user", user).Info("symptom re-evaluation POST")

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

	if s.bigQueryClient == nil || s.gcsClient == nil || s.gcsBucket == "" {
		failureResponse(w, http.StatusServiceUnavailable, "symptom re-evaluation requires BigQuery and GCS configuration")
		return
	}

	// Synchronous mode: preserve backward compatibility via ?sync=true
	syncMode := req.URL.Query().Get("sync") == "true"

	if syncMode {
		if err := apijobrunscan.ValidateReEvalRequest(body.ProwJobBuildIDs, apijobrunscan.MaxJobRunsSyncReq()); err != nil {
			failureResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		re := apijobrunscan.NewReEvaluator(s.bigQueryClient, s.gcsClient, s.gcsBucket, s.db, s.cache, s.jobartifactsManager, body.DryRun)
		results, err := re.ReEvaluateJobRuns(req.Context(), body.ProwJobBuildIDs)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp := apijobrunscan.ReEvaluationResponse{Results: results}
		apijobrunscan.InjectReEvalHATEOASLinks(&resp, api.GetBaseURL(req))
		api.RespondWithJSON(http.StatusOK, w, resp)
		return
	}

	// Async mode (default): create a task and return 202 Accepted
	if err := apijobrunscan.ValidateReEvalRequest(body.ProwJobBuildIDs, apijobrunscan.MaxJobRunsAsyncReq()); err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	task := s.reEvalTaskStore.Create(len(body.ProwJobBuildIDs))
	baseURL := api.GetBaseURL(req)

	// Launch background processing. Use a detached context so the goroutine
	// is not cancelled when the HTTP request completes.
	go func() {
		ctx := context.Background()
		s.reEvalTaskStore.SetRunning(task.ID)

		re := apijobrunscan.NewReEvaluator(s.bigQueryClient, s.gcsClient, s.gcsBucket, s.db, s.cache, s.jobartifactsManager, body.DryRun)
		symptoms, err := re.LoadActiveSymptoms()
		if err != nil {
			s.reEvalTaskStore.Complete(task.ID, err)
			return
		}

		for _, buildID := range body.ProwJobBuildIDs {
			result := re.ReEvaluateOne(ctx, buildID, symptoms)
			s.reEvalTaskStore.AppendResult(task.ID, result)
		}

		s.reEvalTaskStore.Complete(task.ID, nil)
	}()

	resp := apijobrunscan.ReEvalTaskResponse{
		ID:        task.ID,
		Status:    task.Status,
		Processed: task.Processed,
		Total:     task.Total,
		Results:   task.Results,
		CreatedAt: task.CreatedAt,
	}
	apijobrunscan.InjectTaskHATEOASLinks(&resp, baseURL)
	api.RespondWithJSON(http.StatusAccepted, w, resp)
}

// jsonGetReEvaluationTask returns the current state of an async re-evaluation task.
func (s *Server) jsonGetReEvaluationTask(w http.ResponseWriter, req *http.Request) {
	taskID := mux.Vars(req)["task_id"]

	task := s.reEvalTaskStore.Get(taskID)
	if task == nil {
		failureResponse(w, http.StatusNotFound, "task not found")
		return
	}

	resp := apijobrunscan.ReEvalTaskResponse{
		ID:          task.ID,
		Status:      task.Status,
		Processed:   task.Processed,
		Total:       task.Total,
		Results:     task.Results,
		CreatedAt:   task.CreatedAt,
		CompletedAt: task.CompletedAt,
		Error:       task.Error,
	}
	apijobrunscan.InjectTaskHATEOASLinks(&resp, api.GetBaseURL(req))
	api.RespondWithJSON(http.StatusOK, w, resp)
}
