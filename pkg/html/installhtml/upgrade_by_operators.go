package installhtml

import (
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

	"github.com/openshift/sippy/pkg/html/generichtml"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
)

func UpgradeOperatorTests(format ResponseFormat, curr, prev sippyprocessingv1.TestReport) string {
	dataForTestsByVariant := getDataForTestsByVariant(
		curr, prev,
		isUpgradeRelatedTest,
		func(testResult sippyprocessingv1.TestResult) bool {
			return testResult.Name == testgridanalysisapi.UpgradeTestName
		},
	)
	// compute variant columns before we add the special "All" column
	variantColumns := sets.StringKeySet(dataForTestsByVariant.aggregationToOverallTestResult).List()

	// we add an "All" column for all variants. Fill in the aggregate data for that key
	for _, testName := range sets.StringKeySet(dataForTestsByVariant.aggregateResultByTestName).List() {
		dataForTestsByVariant.testNameToVariantToTestResult[testName]["All"] = dataForTestsByVariant.aggregateResultByTestName[testName].toCurrPrevTestResult()
	}

	// fill in the data for the first row's "All" column
	var prevTestResult *sippyprocessingv1.TestResult
	if prevInstallTest := util.FindFailedTestResult(testgridanalysisapi.UpgradeTestName, prev.ByTest); prevInstallTest != nil {
		prevTestResult = &prevInstallTest.TestResultAcrossAllJobs
	}
	var currTestResult sippyprocessingv1.TestResult
	if currInstallTest := util.FindFailedTestResult(testgridanalysisapi.UpgradeTestName, curr.ByTest); currInstallTest != nil {
		currTestResult = currInstallTest.TestResultAcrossAllJobs
	}

	dataForTestsByVariant.aggregationToOverallTestResult["All"] = &currPrevTestResult{
		curr: currTestResult,
		prev: prevTestResult,
	}

	columnNames := append([]string{"All"}, variantColumns...)

	if format == "json" {
		return dataForTestsByVariant.getTableJSON("Upgrade Rates by Operator", "Upgrade Rates by Operator by Variant", columnNames, getOperatorFromTest)
	}

	return dataForTestsByVariant.getTableHTML("Upgrade Rates by Operator", "UpgradeRatesByOperator", "Upgrade Rates by Operator by Variant", columnNames, getOperatorFromTest)
}

func summaryUpgradeRelatedTests(curr, prev sippyprocessingv1.TestReport, numDays int, release string) string {
	// test name | bug | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=5 class="text-center">
				<a class="text-dark" id="UpgradeRelatedTests" href="#UpgradeRelatedTests">Upgrade Related Tests</a>
				<i class="fa fa-info-circle" title="Upgrade related tests, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a bug against the test."></i>
			</th>
		</tr>
		<tr>
			<th colspan=2/><th class="text-center">Latest %d Days</th><th/><th class="text-center">Previous 7 Days</th>
		</tr>
		<tr>
			<th>Test Name</th><th>File a Bug</th><th>Pass Rate</th><th/><th>Pass Rate</th>
		</tr>
	`, numDays)

	s += failingTestsRows(curr.ByTest, prev.ByTest, release, isUpgradeRelatedTest)

	s += "</table>"

	return s
}

func isUpgradeRelatedTest(testResult sippyprocessingv1.TestResult) bool {
	return testidentification.IsUpgradeRelatedTest(testResult.Name)
}

func neverMatch(testResult sippyprocessingv1.TestResult) bool {
	return false
}

func summaryUpgradeRelatedJobs(report, reportPrev sippyprocessingv1.TestReport, numDays int, release string) string {
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center">
				<a class="text-dark" id="UpgradeJobs" href="#UpgradeJobs">Upgrade Jobs</a>
				<i class="fa fa-info-circle" title="Passing rate for each job upgrade definition, sorted by passing percentage.  Jobs at the top of this list are unreliable or represent environments where the product is not stable and should be investigated.  The pass rate in parenthesis is the pass rate for jobs that started to run the installer and got at least the bootstrap kube-apiserver up and running."</i>
			</th>
		</tr>
		<tr>
			<th>Name</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, numDays)

	for _, currJobResult := range report.ByJob {
		if !strings.Contains(currJobResult.Name, "-upgrade-") {
			continue
		}
		prevJobResult := util.FindJobResultForJobName(currJobResult.Name, reportPrev.InfrequentJobResults)
		jobHTML := generichtml.NewJobResultRendererFromJobResult("by-infrequent-job-name", currJobResult, release).
			WithPreviousJobResult(prevJobResult).
			ToHTML()

		s += jobHTML
	}

	s += "</table>"
	return s
}
