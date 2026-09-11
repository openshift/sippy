package api

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/testidentification"
)

// PrintUpgradeJSONReportFromDB reports on the success/fail of operator upgrades.
func PrintUpgradeJSONReportFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB, release string) {
	lifecycleTests := lifecycleTestsForRelease(release)

	variantColumns, tests, err := VariantTestsReport(dbc, release, v1.CurrentReport,
		lifecycleTests.UpgradeExactNames, lifecycleTests.UpgradePrefixes, lifecycleTests.UpgradeSubstrings, testidentification.DefaultExcludedVariants)
	if err != nil {
		log.WithError(err).Error("could not generate upgrade report")
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Could not generate install report: " + err.Error()})
		return
	}

	// Build up a set of column names, every variant we encounter as well as an "All":
	summary := map[string]interface{}{
		"title":        "Upgrade Rates by Operator",
		"description":  "Upgrade Rates by Operator by Variant",
		"column_names": sets.List(variantColumns),
		"tests":        tests,
	}
	if links := lifecycleLinks(release, lifecycleTests); links != nil {
		summary["links"] = links
	}

	result, err := json.Marshal(summary)
	if err != nil {
		log.WithError(err).Error("could not generate install report")
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Could not generate install report: " + err.Error()})
		return
	}

	jsonStr := string(result)
	RespondWithJSON(http.StatusOK, w, jsonStr)
}
