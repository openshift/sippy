package synthetictests

import (
	"fmt"

	"github.com/openshift/sippy/pkg/apis/junit"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	v1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
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

func (openshiftSyntheticManager) CreateSyntheticTests(jrr *testgridanalysisapi.RawJobRunResult) *junit.TestSuite {
	results := make([]*junit.TestCase, 0)

	syntheticTests := map[string]*syntheticTestResult{
		testgridanalysisapi.SippySuiteName + "." + testgridanalysisapi.InstallTestName:             &syntheticTestResult{name: testgridanalysisapi.InstallTestName},
		testgridanalysisapi.SippySuiteName + "." + testgridanalysisapi.InstallTimeoutTestName:      &syntheticTestResult{name: testgridanalysisapi.InstallTimeoutTestName},
		testgridanalysisapi.SippySuiteName + "." + testgridanalysisapi.InfrastructureTestName:      &syntheticTestResult{name: testgridanalysisapi.InfrastructureTestName},
		testgridanalysisapi.SippySuiteName + "." + testgridanalysisapi.FinalOperatorHealthTestName: &syntheticTestResult{name: testgridanalysisapi.FinalOperatorHealthTestName},
		testgridanalysisapi.SippySuiteName + "." + testgridanalysisapi.OpenShiftTestsName:          &syntheticTestResult{name: testgridanalysisapi.OpenShiftTestsName},
	}
	// upgrades should only be indicated on jobs that run upgrades
	if jrr.UpgradeStarted {
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.UpgradeTestName] = &syntheticTestResult{name: testgridanalysisapi.UpgradeTestName}
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
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.FinalOperatorHealthTestName].pass = 1
	default:
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.FinalOperatorHealthTestName].fail = 1
	}

	// set overall installed status
	switch {
	case installSucceeded:
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.InstallTestName].pass = 1
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
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.InstallTestName].fail = 1

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
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.InstallTimeoutTestName].fail = 1

	default:
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.InstallTimeoutTestName].pass = 1

	}

	// set the infra status
	switch {
	case installFailed && !hasFinalOperatorResults:
		// we only count failures as infra if we have no operator results.  If we got any operator working, then CI infra was working.
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.InfrastructureTestName].fail = 1

	default:
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.InfrastructureTestName].pass = 1
	}

	// set the update status
	switch {
	case installFailed:
		// do nothing
	case !jrr.UpgradeStarted:
	// do nothing

	default:
		if jrr.UpgradeForOperatorsStatus == testgridanalysisapi.Success && jrr.UpgradeForMachineConfigPoolsStatus == testgridanalysisapi.Success {
			syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.UpgradeTestName].pass = 1
			// if the test succeeded, then the operator install tests should all be passes
			for _, operatorState := range jrr.FinalOperatorStates {
				testName := testgridanalysisapi.SippyOperatorUpgradePrefix + operatorState.Name
				syntheticTests[testName] = &syntheticTestResult{
					name: testName,
					pass: 1,
				}
			}
		} else {
			syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.UpgradeTestName].fail = 1
			// if the test failed, then the operator upgrade tests should match the operator state
			for _, operatorState := range jrr.FinalOperatorStates {
				testName := testgridanalysisapi.SippyOperatorUpgradePrefix + operatorState.Name
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
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.OpenShiftTestsName].fail = 1
	case jrr.OpenShiftTestsStatus == testgridanalysisapi.Success:
		syntheticTests[testgridanalysisapi.SippySuiteName+"."+testgridanalysisapi.OpenShiftTestsName].pass = 1
	}

	for testName, result := range syntheticTests {
		// convert the result.pass or .fail to the status value we use for test results:
		if result.fail > 0 {
			jrr.TestFailures += result.fail
			jrr.FailedTestNames = append(jrr.FailedTestNames, testName)
		} else if result.pass > 0 {
			// Add successful test results as well.
			jrr.TestResults = append(jrr.TestResults, testgridanalysisapi.RawJobRunTestResult{
				Name:   testName,
				Status: v1.TestStatusSuccess,
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
		jrr.InstallStatus = testgridanalysisapi.Unknown
	}

	jrr.OverallResult = jobRunStatus(jrr)

	return &junit.TestSuite{
		Name:      testgridanalysisapi.SippySuiteName,
		NumTests:  uint(len(results)),
		NumFailed: uint(jrr.TestFailures),
		TestCases: results,
	}
}

const failure string = "Failure"

func jobRunStatus(result *testgridanalysisapi.RawJobRunResult) sippyprocessingv1.JobOverallResult {
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
