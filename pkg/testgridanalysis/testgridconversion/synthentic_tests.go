package testgridconversion

import (
	"strings"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

// createSyntheticTests takes the JobRunResult information and produces some pre-analysis by interpreting different types of failures
// and potentially producing synthentic test results and aggregations to better inform sippy.
// This needs to be called after all the JobDetails have been processed.
func createSyntheticTests(rawJobResults testgridanalysisapi.RawData) {
	// make a pass to fill in install, upgrade, and infra synthentic tests.
	type synthenticTestResult struct {
		name string
		pass int
		fail int
	}

	for jobName, jobResults := range rawJobResults.JobResults {
		for jrrKey, jrr := range jobResults.JobRunResults {
			isUpgrade := strings.Contains(jrr.Job, "upgrade")

			syntheticTests := map[string]*synthenticTestResult{
				testgridanalysisapi.InstallTestName:        &synthenticTestResult{name: testgridanalysisapi.InstallTestName},
				testgridanalysisapi.UpgradeTestName:        &synthenticTestResult{name: testgridanalysisapi.UpgradeTestName},
				testgridanalysisapi.InfrastructureTestName: &synthenticTestResult{name: testgridanalysisapi.InfrastructureTestName},
			}

			installFailed := false
			for _, operator := range jrr.InstallOperators {
				if operator.State == testgridanalysisapi.Failure {
					installFailed = true
					break
				}
			}
			upgradeFailed := false
			for _, operator := range jrr.UpgradeOperators {
				if operator.State == testgridanalysisapi.Failure {
					upgradeFailed = true
					break
				}
			}
			setupFailed := jrr.SetupStatus != testgridanalysisapi.Success

			if installFailed {
				jrr.TestFailures++
				jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.InstallTestName)
				syntheticTests[testgridanalysisapi.InstallTestName].fail = 1
			} else {
				if !setupFailed { // this will be an undercount, but we only want to count installs that actually worked.
					syntheticTests[testgridanalysisapi.InstallTestName].pass = 1
				}
			}
			if setupFailed && len(jrr.InstallOperators) == 0 { // we only want to count it as an infra issue if the install did not start
				jrr.TestFailures++
				jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.InfrastructureTestName)
				syntheticTests[testgridanalysisapi.InfrastructureTestName].fail = 1
			} else {
				syntheticTests[testgridanalysisapi.InfrastructureTestName].pass = 1
			}
			if isUpgrade && !setupFailed && !installFailed { // only record upgrade status if we were able to attempt the upgrade
				if upgradeFailed || len(jrr.UpgradeOperators) == 0 {
					jrr.TestFailures++
					jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.UpgradeTestName)
					syntheticTests[testgridanalysisapi.UpgradeTestName].fail = 1
				} else {
					syntheticTests[testgridanalysisapi.UpgradeTestName].pass = 1
				}
			}

			for testName, result := range syntheticTests {
				addTestResult(jobResults.TestResults, testName, result.pass, result.fail, 0)
			}

			jobResults.JobRunResults[jrrKey] = jrr
		}

		rawJobResults.JobResults[jobName] = jobResults
	}
}
