package sippyserver

import (
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
	log.Infof("label POST made by user: %s", user)
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
	log.Infof("label PUT made by user: %s", user)
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
	log.Infof("label DELETE made by user: %s", user)
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
	log.Infof("symptom POST made by user: %s", user)
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
	log.Infof("symptom PUT made by user: %s", user)
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
	log.Infof("symptom DELETE made by user: %s", user)
	if err := apijobrunscan.DeleteSymptom(s.db.DB, id, user); err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
