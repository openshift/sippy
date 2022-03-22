package installhtml

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
	"k8s.io/klog"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func InstallOperatorTests(format ResponseFormat, curr, prev sippyprocessingv1.TestReport) string {
	dataForTestsByVariant := getDataForTestsByVariant(
		curr, prev,
		func(testResult sippyprocessingv1.TestResult) bool {
			return strings.HasPrefix(testResult.Name, testgridanalysisapi.OperatorInstallPrefix)
		},
		func(testResult sippyprocessingv1.TestResult) bool {
			return testResult.Name == testgridanalysisapi.InstallTestName
		},
	)

	// compute variants columns before we add the special "All" column
	variantColumns := sets.StringKeySet(dataForTestsByVariant.aggregationToOverallTestResult).List()

	// we add an "All" column for all variants. Fill in the aggregate data for that key
	for _, testName := range sets.StringKeySet(dataForTestsByVariant.aggregateResultByTestName).List() {
		dataForTestsByVariant.testNameToVariantToTestResult[testName]["All"] = dataForTestsByVariant.aggregateResultByTestName[testName].toCurrPrevTestResult()
	}

	// fill in the data for the first row's "All" column
	var prevTestResult *sippyprocessingv1.TestResult
	if installTest := util.FindFailedTestResult(testgridanalysisapi.InstallTestName, prev.ByTest); installTest != nil {
		prevTestResult = &installTest.TestResultAcrossAllJobs
	}
	dataForTestsByVariant.aggregationToOverallTestResult["All"] = &currPrevTestResult{
		curr: util.FindFailedTestResult(testgridanalysisapi.InstallTestName, curr.ByTest).TestResultAcrossAllJobs,
		prev: prevTestResult,
	}

	columnNames := append([]string{"All"}, variantColumns...)

	if format == JSON {
		return dataForTestsByVariant.getTableJSON("Install Rates by Operator", "Install Rates by Operator by Variant", columnNames, getOperatorFromTest)
	}

	return dataForTestsByVariant.getTableHTML("Install Rates by Operator", "InstallRatesByOperator", "Install Rates by Operator by Variant", columnNames, getOperatorFromTest)
}

func InstallOperatorTestsFromDB(dbc *db.DB, release string) (string, error) {
	// Using substring search here is a little funky, we'd prefer prefix matching for the operator tests.
	// For the overall test, the exact match on the InstallTestName const which includes [sig-sippy] isn't working,
	// so we have to use a simpler substring.
	testSubstrings := []string{
		testgridanalysisapi.OperatorInstallPrefix, // TODO: would prefer prefix matching for this
		"install should work",                     // TODO: would prefer exact matching on the full InstallTestName const
	}

	testReports, err := query.TestReportsByVariant(dbc, release, testSubstrings)
	if err != nil {
		return "", err
	}

	variantColumns := sets.NewString()
	// Map operatorName -> variant -> Test report
	tests := make(map[string]map[string]api.Test)
	// Map variant -> Test report for the "Overall" install success
	overallResults := make(map[string]api.Test)

	for _, tr := range testReports {
		variantColumns.Insert(tr.Variant)

		switch {
		case tr.Name == testgridanalysisapi.InstallTestName:
			klog.Infof("Found overall install test %s for variant %s", tr.Name, tr.Variant)
			// Build out the "Overall" entry in the "tests" section. Sippy adds a synthetic overall test for
			// "install should work", which we use to determine the overall install success rate across all operators.
			overallResults[tr.Variant] = tr
		case strings.HasPrefix(tr.Name, testgridanalysisapi.OperatorInstallPrefix):
			// Add an entry to the "tests" section for each operator by using the "operator install" tests and extracting
			// the name of the operator from the test name.
			operatorName := tr.Name
			if ret := testidentification.GetOperatorNameFromTest(tr.Name); len(ret) > 0 {
				operatorName = ret
			}
			if _, ok := tests[operatorName]; !ok {
				tests[operatorName] = map[string]api.Test{}
			}
			klog.Infof("Found operator install test %s for variant %s", operatorName, tr.Variant)
			tests[operatorName][tr.Variant] = tr
		default:
			// Our substring searching can pickup a couple other tests incorrectly right now.
			klog.Infof("Ignoring test %s for variant %s", tr.Name, tr.Variant)
		}
	}
	tests["Overall"] = overallResults

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

func isInstallRelatedTest(testResult sippyprocessingv1.TestResult) bool {
	return testidentification.IsInstallRelatedTest(testResult.Name)
}

func summaryInstallRelatedTests(curr, prev sippyprocessingv1.TestReport, numDays int, release string) string {
	// test name | bug | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=5 class="text-center">
				<a class="text-dark" id="InstallRelatedTests" href="#InstallRelatedTests">Install Related Tests</a>
				<i class="fa fa-info-circle" title="Install related tests, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a bug against the test."</i>
			</th>
		</tr>
		<tr>
			<th colspan=2/><th class="text-center">Latest %d Days</th><th/><th class="text-center">Previous 7 Days</th>
		</tr>
		<tr>
			<th>Test Name</th><th>File a Bug</th><th>Pass Rate</th><th/><th>Pass Rate</th>
		</tr>
	`, numDays)

	s += failingTestsRows(curr.ByTest, prev.ByTest, release, isInstallRelatedTest)

	s += "</table>"

	return s
}
