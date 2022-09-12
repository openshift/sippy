package api

import (
	"encoding/json"
	"net/http"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
	log "github.com/sirupsen/logrus"
)

// PrintUpgradeJSONReportFromDB reports on the success/fail of operator upgrades.
func PrintUpgradeJSONReportFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB, release string) {

	exactTestNames := sets.NewString(
		testidentification.UpgradeTestName,
	)
	testPrefixes := sets.NewString(
		testidentification.OperatorUpgradePrefix, // "old" upgrade test
	)
	// Some of these are substring matches due to suites being included in the test name but not in sippy code.
	testSubStrings := sets.NewString(
		testidentification.OperatorsUpgradedTest,
		testidentification.APIsRemainAvailTest,
		testidentification.MachineConfigsUpgradedTest,
		testidentification.CVOAcknowledgesUpgradeTest,
	)

	variantColumns, tests, err := VariantTestsReport(dbc, release, v1.CurrentReport,
		exactTestNames, testPrefixes, testSubStrings)
	if err != nil {
		log.WithError(err).Error("could not generate upgrade report")
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Could not generate install report: " + err.Error()})
		return
	}

	// Build up a set of column names, every variant we encounter as well as an "All":
	summary := map[string]interface{}{
		"title":        "Upgrade Rates by Operator",
		"description":  "Upgrade Rates by Operator by Variant",
		"column_names": variantColumns.List(),
		"tests":        tests,
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
