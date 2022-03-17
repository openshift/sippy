package testgridconversion

import (
	"fmt"
	"regexp"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	v1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util/sets"
)

type openshiftSyntheticManager struct{}

func NewOpenshiftSyntheticTestManager() SyntheticTestManager {
	return openshiftSyntheticManager{}
}

// CreateSyntheticTests takes the JobRunResult information and produces some pre-analysis by interpreting different types of failures
// and potentially producing synthetic test results and aggregations to better inform sippy.
// This needs to be called after all the JobDetails have been processed.
// returns warnings found in the data. Not failures to process it.
//nolint:gocyclo // TODO: Break this function up, see: https://github.com/fzipp/gocyclo
func (openshiftSyntheticManager) CreateSyntheticTests(rawJobResults testgridanalysisapi.RawData) []string {
	warnings := []string{}

	// make a pass to fill in install, upgrade, and infra synthetic tests.
	type syntheticTestResult struct {
		name string
		pass int
		fail int
	}

	for jobName, jobResults := range rawJobResults.JobResults {
		numsWithoutInstallIndicator := 0
		for jrrKey, jrr := range jobResults.JobRunResults {
			if jrr.InstallStatus == "" {
				numsWithoutInstallIndicator++
			}

			syntheticTests := map[string]*syntheticTestResult{
				testgridanalysisapi.InstallTestName:             &syntheticTestResult{name: testgridanalysisapi.InstallTestName},
				testgridanalysisapi.InstallTimeoutTestName:      &syntheticTestResult{name: testgridanalysisapi.InstallTimeoutTestName},
				testgridanalysisapi.InfrastructureTestName:      &syntheticTestResult{name: testgridanalysisapi.InfrastructureTestName},
				testgridanalysisapi.FinalOperatorHealthTestName: &syntheticTestResult{name: testgridanalysisapi.FinalOperatorHealthTestName},
				testgridanalysisapi.OpenShiftTestsName:          &syntheticTestResult{name: testgridanalysisapi.OpenShiftTestsName},
			}
			// upgrades should only be indicated on jobs that run upgrades
			if jrr.UpgradeStarted {
				syntheticTests[testgridanalysisapi.UpgradeTestName] = &syntheticTestResult{name: testgridanalysisapi.UpgradeTestName}
			}

			hasFinalOperatorResults := len(jrr.FinalOperatorStates) > 0
			allOperatorsSuccessfulAtEndOfRun := true
			for _, operator := range jrr.FinalOperatorStates {
				if operator.State == testgridanalysisapi.Failure {
					allOperatorsSuccessfulAtEndOfRun = false
					break
				}
			}
			installFailed := jrr.Failed && jrr.InstallStatus != testgridanalysisapi.Success
			installSucceeded := jrr.Succeeded || jrr.InstallStatus == testgridanalysisapi.Success

			switch {
			case !hasFinalOperatorResults:
			// without results, there is no run for the tests
			case allOperatorsSuccessfulAtEndOfRun:
				syntheticTests[testgridanalysisapi.FinalOperatorHealthTestName].pass = 1
			default:
				syntheticTests[testgridanalysisapi.FinalOperatorHealthTestName].fail = 1
			}

			// set overall installed status
			switch {
			case installSucceeded:
				syntheticTests[testgridanalysisapi.InstallTestName].pass = 1
				// if the test succeeded, then the operator install tests should all be passes
				for _, operatorState := range jrr.FinalOperatorStates {
					testName := "sippy." + testgridanalysisapi.OperatorInstallPrefix + operatorState.Name
					syntheticTests[testName] = &syntheticTestResult{
						name: testName,
						pass: 1,
					}
				}

			case !hasFinalOperatorResults:
				// if we don't have any operator results, then don't count this an install one way or the other.  This was an infra failure

			default:
				// the installation failed and we have some operator results, which means the install started. This is a failure
				syntheticTests[testgridanalysisapi.InstallTestName].fail = 1

				// if the test failed, then the operator install tests should match the operator state
				for _, operatorState := range jrr.FinalOperatorStates {
					testName := "sippy." + testgridanalysisapi.OperatorInstallPrefix + operatorState.Name
					syntheticTests[testName] = &syntheticTestResult{
						name: testName,
					}
					if operatorState.State == testgridanalysisapi.Success {
						syntheticTests[testName].pass = 1
					} else {
						syntheticTests[testName].fail = 1
					}
				}
			}

			// set overall install timeout status
			switch {
			case !installSucceeded && hasFinalOperatorResults && allOperatorsSuccessfulAtEndOfRun:
				// the install failed and yet all operators were successful in the end.  This means we had a weird problem.  Probably a timeout failure.
				syntheticTests[testgridanalysisapi.InstallTimeoutTestName].fail = 1

			default:
				syntheticTests[testgridanalysisapi.InstallTimeoutTestName].pass = 1

			}

			// set the infra status
			switch {
			case matchJobRegexList(jobName, jobRegexesWithKnownInstallIssues):
				// do nothing.  If we don't have an install test, we have no way of determining infrastructure

			case installFailed && !hasFinalOperatorResults:
				// we only count failures as infra if we have no operator results.  If we got any operator working, then CI infra was working.
				syntheticTests[testgridanalysisapi.InfrastructureTestName].fail = 1

			default:
				syntheticTests[testgridanalysisapi.InfrastructureTestName].pass = 1
			}

			// set the update status
			switch {
			case installFailed:
				// do nothing
			case !jrr.UpgradeStarted:
			// do nothing

			default:
				if jrr.UpgradeForOperatorsStatus == testgridanalysisapi.Success && jrr.UpgradeForMachineConfigPoolsStatus == testgridanalysisapi.Success {
					syntheticTests[testgridanalysisapi.UpgradeTestName].pass = 1
					// if the test succeeded, then the operator install tests should all be passes
					for _, operatorState := range jrr.FinalOperatorStates {
						testName := testgridanalysisapi.OperatorUpgradePrefix + operatorState.Name
						syntheticTests[testName] = &syntheticTestResult{
							name: testName,
							pass: 1,
						}
					}
				} else {
					syntheticTests[testgridanalysisapi.UpgradeTestName].fail = 1
					// if the test failed, then the operator upgrade tests should match the operator state
					for _, operatorState := range jrr.FinalOperatorStates {
						testName := testgridanalysisapi.OperatorUpgradePrefix + operatorState.Name
						syntheticTests[testName] = &syntheticTestResult{
							name: testName,
						}
						if operatorState.State == testgridanalysisapi.Success {
							syntheticTests[testName].pass = 1
						} else {
							syntheticTests[testName].fail = 1
						}
					}
				}
			}

			switch {
			case jrr.Failed && jrr.OpenShiftTestsStatus == testgridanalysisapi.Failure:
				syntheticTests[testgridanalysisapi.OpenShiftTestsName].fail = 1
			case jrr.OpenShiftTestsStatus == testgridanalysisapi.Success:
				syntheticTests[testgridanalysisapi.OpenShiftTestsName].pass = 1
			}

			for testName, result := range syntheticTests {
				// convert the result.pass or .fail to the status value we use for test results:
				testResultStatus := v1.TestStatusSuccess // assume success to start with
				if result.fail > 0 {
					jrr.TestFailures += result.fail
					jrr.FailedTestNames = append(jrr.FailedTestNames, testName)
					testResultStatus = v1.TestStatusFailure
				}
				// Inject successful test results as well.
				jrr.TestResults = append(jrr.TestResults, testgridanalysisapi.RawJobRunTestResult{
					Name:   testName,
					Status: testResultStatus,
				})
				addTestResult(jobResults.TestResults, nil, testName, result.pass, result.fail, 0)
			}

			if jrr.InstallStatus == "" && matchJobRegexList(jobName, jobRegexesWithKnownInstallIssues) {
				jrr.InstallStatus = testgridanalysisapi.Unknown
			}

			jrr.OverallResult = jobRunStatus(jrr)
			jobResults.JobRunResults[jrrKey] = jrr
		}

		if numsWithoutInstallIndicator > 0 && numsWithoutInstallIndicator == len(jobResults.JobRunResults) {
			if !matchJobRegexList(jobName, jobRegexesWithKnownInstallIssues) {
				warnings = append(warnings, fmt.Sprintf("%q is missing a test install job to indicate successful installs", jobName))
			}
		}

		rawJobResults.JobResults[jobName] = jobResults
	}
	return warnings
}

const failure string = "Failure"

func jobRunStatus(result testgridanalysisapi.RawJobRunResult) sippyprocessingv1.JobOverallResult {
	if result.Succeeded {
		return sippyprocessingv1.JobSucceeded
	}

	if !result.Failed {
		return sippyprocessingv1.JobRunning
	}

	if result.InstallStatus == failure {
		if len(result.FinalOperatorStates) == 0 {
			return sippyprocessingv1.JobInfrastructureFailure
		}
		return sippyprocessingv1.JobInstallFailure
	}
	if result.UpgradeStarted && (result.UpgradeForOperatorsStatus == failure || result.UpgradeForMachineConfigPoolsStatus == failure) {
		return sippyprocessingv1.JobUpgradeFailure
	}
	if result.OpenShiftTestsStatus == failure {
		return sippyprocessingv1.JobTestFailure
	}
	if result.InstallStatus == "" {
		return sippyprocessingv1.JobNoResults
	}
	return sippyprocessingv1.JobUnknown
}

// this a list of job name regexes that either do not install the product (bug) or have
// never had a passing install. both should be fixed over time, but this reduces noise as we ratchet down.
var jobRegexesWithKnownInstallIssues = sets.NewString(
	`promote-release-openshift-machine-os-content-e2e-aws-4\.[0-9].*`,
	"periodic-ci-openshift-multiarch-master-nightly-4.8-ocp-installer-remote-libvirt-ppc64le",
	"periodic-ci-openshift-origin-release-3.11-e2e-gcp",
	"periodic-ci-openshift-release-master-nightly-4.7-e2e-powervs",
	"periodic-ci-openshift-release-master-nightly-4.10-credentials-request-freeze",
	"release-openshift-ocp-osd",
)

func matchJobRegexList(jobName string, regexList sets.String) bool {
	for expression := range regexList {
		result, _ := regexp.MatchString(expression, jobName)
		if result {
			return true
		}
	}
	return false
}
