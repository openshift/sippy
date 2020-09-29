package testgridconversion

import (
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util/sets"
)

// createSyntheticTests takes the JobRunResult information and produces some pre-analysis by interpreting different types of failures
// and potentially producing synthentic test results and aggregations to better inform sippy.
// This needs to be called after all the JobDetails have been processed.
// returns warnings found in the data. Not failures to process it.
func createSyntheticTests(rawJobResults testgridanalysisapi.RawData) []string {
	warnings := []string{}

	// make a pass to fill in install, upgrade, and infra synthentic tests.
	type synthenticTestResult struct {
		name string
		pass int
		fail int
	}

	for jobName, jobResults := range rawJobResults.JobResults {
		numRunsWithoutSetup := 0
		for jrrKey, jrr := range jobResults.JobRunResults {
			if jrr.SetupStatus == "" {
				numRunsWithoutSetup++
			}
			isUpgrade := strings.Contains(jrr.Job, "upgrade")

			syntheticTests := map[string]*synthenticTestResult{
				testgridanalysisapi.InstallTestName:        &synthenticTestResult{name: testgridanalysisapi.InstallTestName},
				testgridanalysisapi.InstallTimeoutTestName: &synthenticTestResult{name: testgridanalysisapi.InstallTestName},
				testgridanalysisapi.UpgradeTestName:        &synthenticTestResult{name: testgridanalysisapi.UpgradeTestName},
				testgridanalysisapi.InfrastructureTestName: &synthenticTestResult{name: testgridanalysisapi.InfrastructureTestName},
			}

			hasSomeOperatorResults := len(jrr.SadOperators) > 0
			allOperatorsSuccessfulAtEndOfRun := true
			for _, operator := range jrr.SadOperators {
				if operator.State == testgridanalysisapi.Failure {
					allOperatorsSuccessfulAtEndOfRun = false
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
			setupSucceeded := jrr.SetupStatus == testgridanalysisapi.Success

			// set overall installed status
			switch {
			case setupSucceeded:
				// if setup succeeded, we are guaranteed that installation succeeded.
				syntheticTests[testgridanalysisapi.InstallTestName].pass = 1

			case !hasSomeOperatorResults:
				// if we don't have any operator results, then don't count this an install one way or the other.  This was an infra failure

			default:
				// the setup failed and we have some operator results, which means the install started. This is a failure
				jrr.TestFailures++
				jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.InstallTestName)
				syntheticTests[testgridanalysisapi.InstallTestName].fail = 1

				// TODO if the setupSucceeds, but we have some failing operators reporting failing at the end, then we should consider
				//  marking all the operator tests themselves as flaking, but not failing because the install worked.

			}

			// set overall install timeout status
			switch {
			case !setupSucceeded && hasSomeOperatorResults && allOperatorsSuccessfulAtEndOfRun:
				// the setup failed and yet all operators were successful in the end.  This means we had a weird problem.  Probably a timeout failure.
				jrr.TestFailures++
				jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.InstallTimeoutTestName)
				syntheticTests[testgridanalysisapi.InstallTimeoutTestName].fail = 1

			default:
				syntheticTests[testgridanalysisapi.InstallTimeoutTestName].pass = 1

			}

			// set the infra status
			switch {
			case setupFailed && !hasSomeOperatorResults:
				// we only count failures as infra if we have no operator results.  If we got any operator working, then CI infra was working.
				jrr.TestFailures++
				jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.InfrastructureTestName)
				syntheticTests[testgridanalysisapi.InfrastructureTestName].fail = 1

			default:
				syntheticTests[testgridanalysisapi.InfrastructureTestName].pass = 1
			}

			// set the update status
			switch {
			case setupFailed:
				// do nothing
			case !isUpgrade:
			// do nothing

			case len(jrr.UpgradeOperators) == 0 || upgradeFailed:
				jrr.TestFailures++
				jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.UpgradeTestName)
				syntheticTests[testgridanalysisapi.UpgradeTestName].fail = 1

			default:
				syntheticTests[testgridanalysisapi.UpgradeTestName].pass = 1
			}

			for testName, result := range syntheticTests {
				addTestResult(jobResults.TestResults, testName, result.pass, result.fail, 0)
			}

			jobResults.JobRunResults[jrrKey] = jrr
		}
		if float64(numRunsWithoutSetup)/float64(len(jobResults.JobRunResults)+1)*100 > 50 {
			if !jobsWithKnownBadSetupContainer.Has(jobName) {
				warnings = append(warnings, fmt.Sprintf("%q is missing a test setup job to indicate successful installs", jobName))
			}
		}

		rawJobResults.JobResults[jobName] = jobResults
	}
	return warnings
}

// this a list of jobs that either do not install the product (bug) or have never had a passing install.
// both should be fixed over time, but this reduces noise as we ratchet down.
var jobsWithKnownBadSetupContainer = sets.NewString(
	"promote-release-openshift-machine-os-content-e2e-aws-4.6-s390x",
	"promote-release-openshift-machine-os-content-e2e-aws-4.6-ppc64le",
	"release-openshift-origin-installer-e2e-aws-upgrade-rollback-4.5-to-4.6",
)
