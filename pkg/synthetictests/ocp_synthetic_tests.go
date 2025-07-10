package synthetictests

import (
	"fmt"

	"github.com/openshift/sippy/pkg/apis/junit"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testidentification"
)

type openshiftSyntheticManager struct{}

func NewOpenshiftSyntheticTestManager() SyntheticTestManager {
	return openshiftSyntheticManager{}
}

// make a pass to fill in install, upgrade, and infra synthetic tests.
type syntheticTestResult struct {
	name string
	pass int
	fail int
}

//nolint:gocyclo
func (openshiftSyntheticManager) CreateSyntheticTests(jrr *sippyprocessingv1.RawJobRunResult) *junit.TestSuite {
	results := make([]*junit.TestCase, 0)

	syntheticTests := map[string]*syntheticTestResult{
		testidentification.InstallTestName:             &syntheticTestResult{name: testidentification.InstallTestName},
		testidentification.InstallTimeoutTestName:      &syntheticTestResult{name: testidentification.InstallTimeoutTestName},
		testidentification.InfrastructureTestName:      &syntheticTestResult{name: testidentification.InfrastructureTestName},
		testidentification.FinalOperatorHealthTestName: &syntheticTestResult{name: testidentification.FinalOperatorHealthTestName},
		testidentification.OpenShiftTestsName:          &syntheticTestResult{name: testidentification.OpenShiftTestsName},
	}
	// upgrades should only be indicated on jobs that run upgrades
	if jrr.UpgradeStarted {
		syntheticTests[testidentification.UpgradeTestName] = &syntheticTestResult{name: testidentification.UpgradeTestName}
	}

	hasFinalOperatorResults := len(jrr.FinalOperatorStates) > 0
	allOperatorsSuccessfulAtEndOfRun := true
	for _, operator := range jrr.FinalOperatorStates {
		if operator.State == testidentification.Failure {
			allOperatorsSuccessfulAtEndOfRun = false
			break
		}
	}
	installFailed := jrr.Failed && jrr.InstallStatus != testidentification.Success
	installSucceeded := jrr.Succeeded || jrr.InstallStatus == testidentification.Success

	switch {
	case !hasFinalOperatorResults:
	// without results, there is no run for the tests
	case allOperatorsSuccessfulAtEndOfRun:
		syntheticTests[testidentification.FinalOperatorHealthTestName].pass = 1
	default:
		syntheticTests[testidentification.FinalOperatorHealthTestName].fail = 1
	}

	// set overall installed status
	switch {
	case installSucceeded:
		syntheticTests[testidentification.InstallTestName].pass = 1
		// if the test succeeded, then the operator install tests should all be passes
		for _, operatorState := range jrr.FinalOperatorStates {
			testName := testidentification.OperatorInstallPrefix + operatorState.Name
			syntheticTests[testName] = &syntheticTestResult{
				name: testName,
				pass: 1,
			}
		}

	case !hasFinalOperatorResults:
		// if we don't have any operator results, then don't count this an install one way or the other.  This was an infra failure

	default:
		// the installation failed and we have some operator results, which means the install started. This is a failure
		syntheticTests[testidentification.InstallTestName].fail = 1

		// if the test failed, then the operator install tests should match the operator state
		for _, operatorState := range jrr.FinalOperatorStates {
			testName := testidentification.OperatorInstallPrefix + operatorState.Name
			syntheticTests[testName] = &syntheticTestResult{
				name: testName,
			}
			if operatorState.State == testidentification.Success {
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
		syntheticTests[testidentification.InstallTimeoutTestName].fail = 1

	default:
		syntheticTests[testidentification.InstallTimeoutTestName].pass = 1

	}

	// set the infra status
	switch {
	case installFailed && !hasFinalOperatorResults:
		// we only count failures as infra if we have no operator results.  If we got any operator working, then CI infra was working.
		syntheticTests[testidentification.InfrastructureTestName].fail = 1

	default:
		syntheticTests[testidentification.InfrastructureTestName].pass = 1
	}

	// set the update status
	switch {
	case installFailed:
		// do nothing
	case !jrr.UpgradeStarted:
	// do nothing

	default:
		if jrr.UpgradeForOperatorsStatus == testidentification.Success && jrr.UpgradeForMachineConfigPoolsStatus == testidentification.Success {
			syntheticTests[testidentification.UpgradeTestName].pass = 1
			// if the test succeeded, then the operator install tests should all be passes
			for _, operatorState := range jrr.FinalOperatorStates {
				testName := testidentification.SippyOperatorUpgradePrefix + operatorState.Name
				syntheticTests[testName] = &syntheticTestResult{
					name: testName,
					pass: 1,
				}
			}
		} else {
			syntheticTests[testidentification.UpgradeTestName].fail = 1
			// if the test failed, then the operator upgrade tests should match the operator state
			for _, operatorState := range jrr.FinalOperatorStates {
				testName := testidentification.SippyOperatorUpgradePrefix + operatorState.Name
				syntheticTests[testName] = &syntheticTestResult{
					name: testName,
				}
				if operatorState.State == testidentification.Success {
					syntheticTests[testName].pass = 1
				} else {
					syntheticTests[testName].fail = 1
				}
			}
		}
	}

	switch {
	case jrr.Failed && jrr.OpenShiftTestsStatus == testidentification.Failure:
		syntheticTests[testidentification.OpenShiftTestsName].fail = 1
	case jrr.OpenShiftTestsStatus == testidentification.Success:
		syntheticTests[testidentification.OpenShiftTestsName].pass = 1
	}

	for testName, result := range syntheticTests {
		// convert the result.pass or .fail to the status value we use for test results:
		if result.fail > 0 {
			jrr.TestFailures += result.fail
			jrr.FailedTestNames = append(jrr.FailedTestNames, testName)
		} else if result.pass > 0 {
			// Add successful test results as well.
			jrr.TestResults = append(jrr.TestResults, sippyprocessingv1.RawJobRunTestResult{
				Name:   testName,
				Status: sippyprocessingv1.TestStatusSuccess,
			})
		}

		// Create junits
		if result.pass > 0 {
			results = append(results, &junit.TestCase{
				Name: testName,
			})
		} else if result.fail > 0 {
			results = append(results, &junit.TestCase{
				Name: testName,
				FailureOutput: &junit.FailureOutput{
					Output: fmt.Sprintf("Synthetic test %q failed", testName),
				},
			})
		}
	}

	if jrr.InstallStatus == "" {
		jrr.InstallStatus = testidentification.Unknown
	}

	jrr.OverallResult = jobRunStatus(jrr)

	return &junit.TestSuite{
		Name:      testidentification.SippySuiteName,
		NumTests:  uint(len(results)),
		NumFailed: uint(jrr.TestFailures), // nolint:gosec
		TestCases: results,
	}
}

const failure string = "Failure"

func jobRunStatus(result *sippyprocessingv1.RawJobRunResult) sippyprocessingv1.JobOverallResult {
	if result.Succeeded {
		return sippyprocessingv1.JobSucceeded
	}
	if result.Aborted {
		return sippyprocessingv1.JobAborted
	}
	if result.Errored {
		return sippyprocessingv1.JobFailureBeforeSetup
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
		return sippyprocessingv1.JobFailureBeforeSetup
	}
	return sippyprocessingv1.JobUnknown
}
