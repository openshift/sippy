package installhtml

import (
	"encoding/json"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

func InstallOperatorTestsFromDB(dbc *db.DB, release string) (string, error) {
	// Using substring search here is a little funky, we'd prefer prefix matching for the operator tests.
	// For the overall test, the exact match on the InstallTestName const which includes [sig-sippy] isn't working,
	// so we have to use a simpler substring.
	testSubstrings := []string{
		testidentification.OperatorInstallPrefix, // TODO: would prefer prefix matching for this
		testidentification.InstallTestName,       // TODO: would prefer exact matching on the full InstallTestName const
		testidentification.InstallTestNamePrefix, // TODO: would prefer prefix matching for this
	}

	testReports, err := query.TestReportsByVariant(dbc, release, testSubstrings)
	if err != nil {
		return "", err
	}

	variantColumns := sets.NewString()
	// Map operatorName -> variant -> Test report
	tests := make(map[string]map[string]api.Test)

	for _, tr := range testReports {

		switch {
		case tr.Name == testidentification.InstallTestName ||
			strings.HasPrefix(tr.Name, testidentification.OperatorInstallPrefix) ||
			strings.HasPrefix(tr.Name, testidentification.InstallTestNamePrefix):
			log.Infof("Found install test %s for variant %s", tr.Name, tr.Variant)
			variantColumns.Insert(tr.Variant)
			if _, ok := tests[tr.Name]; !ok {
				tests[tr.Name] = map[string]api.Test{}
			}
			tests[tr.Name][tr.Variant] = tr
		default:
			// Our substring searching can pickup a couple other tests incorrectly right now.
			log.Infof("Ignoring test %s for variant %s", tr.Name, tr.Variant)
		}
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
		"title":        "Install Rates by Operator",
		"description":  "Install Rates by Operator by Variant",
		"column_names": columnNames,
		"tests":        tests,
	}
	result, err := json.Marshal(summary)
	if err != nil {
		panic(err)
	}

	return string(result), nil
}
