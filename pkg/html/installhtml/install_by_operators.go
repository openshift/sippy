package installhtml

import (
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/util"

	"github.com/openshift/sippy/pkg/util/sets"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func installOperatorTests(curr, prev sippyprocessingv1.TestReport) string {
	dataForTestsByPlatform := getDataForTestsByPlatform(
		curr, prev,
		func(testResult sippyprocessingv1.TestResult) bool {
			return strings.HasPrefix(testResult.Name, testgridanalysisapi.OperatorInstallPrefix)
		},
		func(testResult sippyprocessingv1.TestResult) bool {
			return testResult.Name == testgridanalysisapi.InstallTestName
		},
	)
	// compute platform columns before we add the special "All" column
	platformColumns := sets.StringKeySet(dataForTestsByPlatform.aggregationToOverallTestResult).List()

	// we add an "All" column for all platforms. Fill in the aggregate data for that key
	for _, testName := range sets.StringKeySet(dataForTestsByPlatform.aggregateResultByTestName).List() {
		dataForTestsByPlatform.testNameToPlatformToTestResult[testName]["All"] = dataForTestsByPlatform.aggregateResultByTestName[testName].toCurrPrevTestResult()
	}

	// fill in the data for the first row's "All" column
	var prevTestResult *sippyprocessingv1.TestResult
	if installTest := util.FindFailedTestResult(testgridanalysisapi.InstallTestName, prev.ByTest); installTest != nil {
		prevTestResult = &installTest.TestResultAcrossAllJobs
	}
	dataForTestsByPlatform.aggregationToOverallTestResult["All"] = &currPrevTestResult{
		curr: util.FindFailedTestResult(testgridanalysisapi.InstallTestName, curr.ByTest).TestResultAcrossAllJobs,
		prev: prevTestResult,
	}

	columnNames := append([]string{"All"}, platformColumns...)

	return dataForTestsByPlatform.getTableHTML("Install Rates by Operator", "InstallRatesByOperator", "Install Rates by Operator by Platform", columnNames)
}

func summaryInstallRelatedTests(curr, prev sippyprocessingv1.TestReport, numDays int, release string) string {
	// test name | bug | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=5 class="text-center"><a class="text-dark" title="Install related tests, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a bug against the test." id="InstallRelatedTests" href="#InstallRelatedTests">Install Related Tests</a></th>
		</tr>
		<tr>
			<th colspan=2/><th class="text-center">Latest %d Days</th><th/><th class="text-center">Previous 7 Days</th>
		</tr>
		<tr>
			<th>Test Name</th><th>File a Bug</th><th>Pass Rate</th><th/><th>Pass Rate</th>
		</tr>
	`, numDays)

	s += failingTestsRows(curr.ByTest, prev.ByTest, release, isInstallRelatedTest)

	s = s + "</table>"

	return s
}

func isInstallRelatedTest(testResult sippyprocessingv1.TestResult) bool {
	if testgridanalysisapi.OperatorConditionsTestCaseName.MatchString(testResult.Name) {
		return true
	}
	if strings.Contains(testResult.Name, testgridanalysisapi.InstallTestName) {
		return true
	}
	if strings.Contains(testResult.Name, testgridanalysisapi.InstallTimeoutTestName) {
		return true
	}

	return false

}
