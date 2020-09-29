package testreportconversion

import (
	"sort"
	"strings"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util/sets"
)

func generateAllJobFailuresByBugzillaComponent(
	rawJobResults map[string]testgridanalysisapi.RawJobResult,
	jobResults []sippyprocessingv1.JobResult,
) map[string]sippyprocessingv1.SortedBugzillaComponentResult {

	bzComponentToBZJobResults := map[string][]sippyprocessingv1.BugzillaJobResult{}
	for job, rawJobResult := range rawJobResults {
		for _, processedJobResult := range jobResults {
			if processedJobResult.Name != rawJobResult.JobName {
				continue
			}

			curr := generateJobFailuresByBugzillaComponent(job, rawJobResult.JobRunResults, processedJobResult.TestResults)
			// each job will be distinct, so we merely need to append
			for bzComponent, bzJobResult := range curr {
				bzComponentToBZJobResults[bzComponent] = append(bzComponentToBZJobResults[bzComponent], bzJobResult)
			}
			break
		}

	}

	sortedResults := map[string]sippyprocessingv1.SortedBugzillaComponentResult{}
	for bzComponent, jobResults := range bzComponentToBZJobResults {
		// sort from least passing to most passing
		// we expect these lists to be small, so the sort isn't awful
		sort.SliceStable(jobResults, func(i, j int) bool {
			return jobResults[i].FailPercentage > jobResults[j].FailPercentage
		})
		sortedResults[bzComponent] = sippyprocessingv1.SortedBugzillaComponentResult{
			Name:       bzComponent,
			JobsFailed: jobResults,
		}
	}

	return sortedResults
}

// returns bz component to bzJob
func generateJobFailuresByBugzillaComponent(
	jobName string,
	jobRuns map[string]testgridanalysisapi.RawJobRunResult,
	jobTestResults []sippyprocessingv1.TestResult,
) map[string]sippyprocessingv1.BugzillaJobResult {

	bzComponentToFailedJobRuns := map[string]sets.String{}
	bzToTestNameToTestResult := map[string]map[string]sippyprocessingv1.TestResult{}
	failedTestCount := 0
	foundTestCount := 0
	for _, rawJRR := range jobRuns {
		failedTestCount += len(rawJRR.FailedTestNames)
		for _, testName := range rawJRR.FailedTestNames {
			testResult, foundTest := getTestResultForJob(jobTestResults, testName)
			if !foundTest {
				continue
			}
			foundTestCount++

			bzComponents := getBugzillaComponentsFromTestResult(testResult)
			for _, bzComponent := range bzComponents {
				// set the failed runs so we know which jobs failed
				failedJobRuns, ok := bzComponentToFailedJobRuns[bzComponent]
				if !ok {
					failedJobRuns = sets.String{}
				}
				failedJobRuns.Insert(rawJRR.JobRunURL)
				bzComponentToFailedJobRuns[bzComponent] = failedJobRuns
				////////////////////////////////

				// set the filtered test result
				testNameToTestResult, ok := bzToTestNameToTestResult[bzComponent]
				if !ok {
					testNameToTestResult = map[string]sippyprocessingv1.TestResult{}
				}
				testNameToTestResult[testName] = getTestResultFilteredByComponent(testResult, bzComponent)
				bzToTestNameToTestResult[bzComponent] = testNameToTestResult
				////////////////////////////////
			}
		}
	}

	bzComponentToBZJobResult := map[string]sippyprocessingv1.BugzillaJobResult{}
	for bzComponent, failedJobRuns := range bzComponentToFailedJobRuns {
		totalRuns := len(jobRuns)
		numFailedJobRuns := len(failedJobRuns)
		failPercentage := float64(numFailedJobRuns*100) / float64(totalRuns)

		bzJobTestResult := []sippyprocessingv1.TestResult{}
		for _, testResult := range bzToTestNameToTestResult[bzComponent] {
			bzJobTestResult = append(bzJobTestResult, testResult)
		}
		// sort from least passing to most passing
		sort.SliceStable(bzJobTestResult, func(i, j int) bool {
			return bzJobTestResult[i].PassPercentage < bzJobTestResult[j].PassPercentage
		})

		bzComponentToBZJobResult[bzComponent] = sippyprocessingv1.BugzillaJobResult{
			JobName:               jobName,
			BugzillaComponent:     bzComponent,
			NumberOfJobRunsFailed: numFailedJobRuns,
			FailPercentage:        failPercentage,
			TotalRuns:             totalRuns,
			Failures:              bzJobTestResult,
		}
	}

	return bzComponentToBZJobResult
}

func getBugzillaComponentsFromTestResult(testResult sippyprocessingv1.TestResult) []string {
	bzComponents := sets.String{}
	bugList := testResult.BugList
	for _, bug := range bugList {
		bzComponents.Insert(bug.Component[0])
	}
	if len(bzComponents) > 0 {
		return bzComponents.List()
	}

	// If we didn't have a bug, use the test name itself to identify a likely victim/blame
	switch {
	case testgridanalysisapi.OperatorConditionsTestCaseName.MatchString(testResult.Name):
		matches := testgridanalysisapi.OperatorConditionsTestCaseName.FindStringSubmatch(testResult.Name)
		operatorIndex := testgridanalysisapi.OperatorConditionsTestCaseName.SubexpIndex("operator")
		operatorName := matches[operatorIndex]
		return []string{testidentification.GetBugzillaComponentForOperator(operatorName)}

	case strings.HasPrefix(testResult.Name, testgridanalysisapi.OperatorUpgradePrefix):
		operatorName := testResult.Name[len(testgridanalysisapi.OperatorUpgradePrefix):]
		return []string{testidentification.GetBugzillaComponentForOperator(operatorName)}

	default:
		return []string{testidentification.GetBugzillaComponentForSig(testidentification.FindSig(testResult.Name))}
	}

}

func getTestResultForJob(jobTestResults []sippyprocessingv1.TestResult, testName string) (sippyprocessingv1.TestResult, bool) {
	for _, testResult := range jobTestResults {
		if testResult.Name == testName {
			return testResult, true
		}
	}
	return sippyprocessingv1.TestResult{
		Name:           "if-seen-report-bug---" + testName,
		PassPercentage: 200.0,
	}, false
}

func getTestResultFilteredByComponent(testResult sippyprocessingv1.TestResult, bzComponent string) sippyprocessingv1.TestResult {
	ret := testResult
	ret.BugList = []bugsv1.Bug{}
	for i := range testResult.BugList {
		bug := testResult.BugList[i]
		if bug.Component[0] == bzComponent {
			ret.BugList = append(ret.BugList, bug)
		}
	}

	return ret
}
