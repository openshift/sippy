package api

import (
	"net/http"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/installhtml"
)

// PrintUpgradeJSONReport reports on the success/fail of operator upgrades.
func PrintUpgradeJSONReport(w http.ResponseWriter, req *http.Request, report, prevReport sippyprocessingv1.TestReport, numDays int, release string) {
	RespondWithJSON(http.StatusOK, w, installhtml.UpgradeOperatorTests(installhtml.JSON, report, prevReport))
}
