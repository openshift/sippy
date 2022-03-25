package api

import (
	"net/http"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/html/installhtml"
)

// PrintUpgradeJSONReport reports on the success/fail of operator upgrades.
func PrintUpgradeJSONReport(w http.ResponseWriter, req *http.Request, report, prevReport sippyprocessingv1.TestReport, numDays int, release string) {
	RespondWithJSON(http.StatusOK, w, installhtml.UpgradeOperatorTests(installhtml.JSON, report, prevReport))
}

// PrintUpgradeJSONReportFromDB reports on the success/fail of operator upgrades.
func PrintUpgradeJSONReportFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB, release string) {
	responseStr, err := installhtml.UpgradeOperatorTestsFromDB(dbc, release)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not generate upgrade report:" + err.Error()})
	}
	RespondWithJSON(http.StatusOK, w, responseStr)
}
