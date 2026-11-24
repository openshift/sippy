package sippyserver

import (
	"net/http"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/util/param"
)

func (s *Server) jsonUpgradeReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := param.SafeRead(req, "release")

	api.PrintUpgradeJSONReportFromDB(w, req, s.db, release)
}

func (s *Server) jsonInstallReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := param.SafeRead(req, "release")

	api.PrintInstallJSONReportFromDB(w, s.db, release)
}
