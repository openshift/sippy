package api

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/testidentification"
)

// PrintInstallJSONReportFromDB renders a report showing the success/fail rates of operator installation.
func PrintInstallJSONReportFromDB(w http.ResponseWriter, dbc *db.DB, release string) {
	excludedVariants := testidentification.DefaultExcludedVariants
	excludedVariants = append(excludedVariants, "upgrade-minor")
	exactTestNames := sets.New[string]()
	testPrefixes := sets.New(testidentification.OperatorInstallPrefix)
	if useNewInstallTest(release) {
		testPrefixes.Insert(testidentification.InstallTestNamePrefix)
	} else {
		exactTestNames = exactTestNames.Insert(testidentification.InstallTestName)
	}

	variantColumns, tests, err := VariantTestsReport(dbc, release, v1.CurrentReport,
		exactTestNames, testPrefixes, sets.New[string](), excludedVariants)
	if err != nil {
		log.WithError(err).Error("could not generate install report")
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Could not generate install report: " + err.Error()})
		return
	}

	// Build up a set of column names, every variant we encounter as well as an "All":
	summary := map[string]interface{}{
		"title":        "Install Rates by Operator",
		"description":  "Install Rates by Operator by Variant",
		"column_names": sets.List(variantColumns),
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

// VariantTestsReport returns a set of all variant columns plus "All", and a map of testName to variant column to test results for that variant.
// Caller can provide exact test names to match, test name prefixes, or test substrings.
func VariantTestsReport(dbc *db.DB, release string, reportType v1.ReportType, testNames, testPrefixes, testSubStrings sets.Set[string], excludedVariants []string) (sets.Set[string], map[string]map[string]apitype.Test, error) {
	nameMatches := query.TestNameMatches{
		ExactNames: sets.List(testNames),
		Prefixes:   sets.List(testPrefixes),
		Substrings: sets.List(testSubStrings),
	}

	testReports, err := query.TestReportsByVariant(dbc, release, reportType, nameMatches, excludedVariants, true)
	if err != nil {
		return sets.New[string](), map[string]map[string]apitype.Test{}, err
	}

	variantColumns := sets.New[string]()
	tests := make(map[string]map[string]apitype.Test)

	for _, tr := range testReports {
		variantColumns.Insert(tr.Variant)
		if _, ok := tests[tr.Name]; !ok {
			tests[tr.Name] = map[string]apitype.Test{}
		}
		tests[tr.Name][tr.Variant] = tr
	}

	return variantColumns, tests, nil
}
