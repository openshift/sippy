package installhtml

import (
	"encoding/json"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/testidentification"

	"github.com/openshift/sippy/pkg/util/sets"
)

// UpgradeOperatorTestsFromDB returns json for the table of all upgrade related tests and their pass rates overall and per variant.
func UpgradeOperatorTestsFromDB(dbc *db.DB, release string) (string, error) {
	testSubstrings := []string{
		testidentification.OperatorUpgradePrefix, // "old" upgrade test, TODO: would prefer prefix matching for this
		testidentification.UpgradeTestName,       // TODO: would prefer exact matching
		testidentification.CVOAcknowledgesUpgradeTest,
		testidentification.OperatorsUpgradedTest,
		testidentification.MachineConfigsUpgradedRegex,
		testidentification.UpgradeFastTest,
		testidentification.APIsRemainAvailTest,
	}

	testReports, err := query.TestReportsByVariant(dbc, release, testSubstrings)
	if err != nil {
		return "", err
	}

	variantColumns := sets.NewString()

	// Map testname -> variant|All -> Test report
	tests := make(map[string]map[string]api.Test)

	for _, tr := range testReports {
		log.Infof("Found test %s for variant %s", tr.Name, tr.Variant)
		variantColumns.Insert(tr.Variant)

		if _, ok := tests[tr.Name]; !ok {
			tests[tr.Name] = map[string]api.Test{}
		}
		tests[tr.Name][tr.Variant] = tr
	}

	// Add in the All column for each test:
	for testName := range tests {
		allReport, err := query.TestReportExcludeVariants(dbc, release, testName, []string{})
		if err != nil {
			return "", err
		}
		tests[testName]["All"] = allReport
	}

	// Build up a set of column names, every variant we encounter as well as an "All":
	columnNames := append([]string{"All"}, variantColumns.List()...)
	summary := map[string]interface{}{
		"title":        "Upgrade Rates by Operator",
		"description":  "Upgrade Rates by Operator by Variant",
		"column_names": columnNames,
		"tests":        tests,
	}
	result, err := json.Marshal(summary)
	if err != nil {
		panic(err)
	}

	return string(result), nil
}
