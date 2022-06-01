package sippyserver

import (
	"net/http"

	"github.com/openshift/sippy/pkg/api"
)

func (s *Server) jsonUpgradeReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")

	api.PrintUpgradeJSONReportFromDB(w, req, s.db, release)
}

func (s *Server) jsonInstallReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")

	api.PrintInstallJSONReportFromDB(w, s.db,
		release,
	)
}
