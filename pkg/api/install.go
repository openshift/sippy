package api

import (
	"net/http"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/html/installhtml"
)

// PrintInstallJSONReport renders a report showing the success/fail rates of operator installation.
func PrintInstallJSONReport(w http.ResponseWriter, req *http.Request, report, prevReport sippyprocessingv1.TestReport, numDays int, release string) {
	RespondWithJSON(http.StatusOK, w, installhtml.InstallOperatorTests("json", report, prevReport))
}

// PrintInstallJSONReportFromDB renders a report showing the success/fail rates of operator installation.
func PrintInstallJSONReportFromDB(w http.ResponseWriter, dbc *db.DB, release string) {
	jsonStr, err := installhtml.InstallOperatorTestsFromDB(dbc, release)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not generate install report:" + err.Error()})
	}
	RespondWithJSON(http.StatusOK, w, jsonStr)
}
