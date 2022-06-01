package api

import (
	"net/http"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/html/installhtml"
)

// PrintUpgradeJSONReportFromDB reports on the success/fail of operator upgrades.
func PrintUpgradeJSONReportFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB, release string) {
	responseStr, err := installhtml.UpgradeOperatorTestsFromDB(dbc, release)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not generate upgrade report:" + err.Error()})
	}
	RespondWithJSON(http.StatusOK, w, responseStr)
}
