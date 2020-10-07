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

func upgradeOperatorTests(curr, prev sippyprocessingv1.TestReport) string {
	dataForTestsByPlatform := getDataForTestsByPlatform(
		curr, prev,
		isUpgradeRelatedTest,
		func(testResult sippyprocessingv1.TestResult) bool {
			return testResult.Name == testgridanalysisapi.UpgradeTestName
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
	if installTest := util.FindFailedTestResult(testgridanalysisapi.UpgradeTestName, prev.ByTest); installTest != nil {
		prevTestResult = &installTest.TestResultAcrossAllJobs
	}
	dataForTestsByPlatform.aggregationToOverallTestResult["All"] = &currPrevTestResult{
		curr: util.FindFailedTestResult(testgridanalysisapi.UpgradeTestName, curr.ByTest).TestResultAcrossAllJobs,
		prev: prevTestResult,
	}

	columnNames := append([]string{"All"}, platformColumns...)

	return dataForTestsByPlatform.getTableHTML("Upgrade Rates by Operator", "UpgradeRatesByOperator", "Upgrade Rates by Operator by Platform", columnNames, getOperatorFromTest)
}

func summaryUpgradeRelatedTests(curr, prev sippyprocessingv1.TestReport, numDays int, release string) string {
	// test name | bug | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=5 class="text-center"><a class="text-dark" title="Upgrade related tests, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a bug against the test." id="UpgradeRelatedTests" href="#UpgradeRelatedTests">Upgrade Related Tests</a></th>
		</tr>
		<tr>
			<th colspan=2/><th class="text-center">Latest %d Days</th><th/><th class="text-center">Previous 7 Days</th>
		</tr>
		<tr>
			<th>Test Name</th><th>File a Bug</th><th>Pass Rate</th><th/><th>Pass Rate</th>
		</tr>
	`, numDays)

	s += failingTestsRows(curr.ByTest, prev.ByTest, release, isUpgradeRelatedTest)

	s = s + "</table>"

	return s
}

func isUpgradeRelatedTest(testResult sippyprocessingv1.TestResult) bool {
	if testidentification.IsUpgradeOperatorTest(testResult.Name) {
		return true
	}
	if strings.Contains(testResult.Name, testgridanalysisapi.UpgradeTestName) {
		return true
	}
	if strings.Contains(testResult.Name, `[sig-cluster-lifecycle] Cluster version operator acknowledges upgrade`) {
		return true
	}
	if strings.Contains(testResult.Name, `[sig-cluster-lifecycle] cluster upgrade should be fast`) {
		return true
	}
	if strings.Contains(testResult.Name, `APIs remain available`) {
		return true
	}

	return false

}

func summaryUpgradeRelatedJobs(report, reportPrev sippyprocessingv1.TestReport, numDays int, release string) string {
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Passing rate for each job upgrade definition, sorted by passing percentage.  Jobs at the top of this list are unreliable or represent environments where the product is not stable and should be investigated.  The pass rate in parenthesis is the pass rate for jobs that started to run the installer and got at least the bootstrap kube-apiserver up and running." id="UpgradeJobs" href="#UpgradeJobs">Upgrade Jobs</a></th>
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

	s = s + "</table>"
	return s
}
