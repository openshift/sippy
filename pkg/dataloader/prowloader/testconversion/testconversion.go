package testconversion

import (
	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/types"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
)

func ConvertProwJobRunToSyntheticTests(pj prow.ProwJob, tests []*types.TestCaseEntry, manager synthetictests.SyntheticTestManager) (*junit.TestSuite, v1.JobOverallResult) {
	jrr := v1.RawJobRunResult{
		Job:       pj.Spec.Job,
		Errored:   pj.Status.State == prow.ErrorState,
		Failed:    pj.Status.State == prow.FailureState,
		Succeeded: pj.Status.State == prow.SuccessState,
		Aborted:   pj.Status.State == prow.AbortedState,
	}
	testsToRawJobRunResult(&jrr, tests)
	syntheticTests := manager.CreateSyntheticTests(&jrr)
	return syntheticTests, jrr.OverallResult
}

func testsToRawJobRunResult(jrr *v1.RawJobRunResult, tests []*types.TestCaseEntry) {
	for _, tc := range tests {
		if testidentification.IsNonSuiteTest(tc.SuiteName, tc.TestName) {
			continue
		}

		switch v1.TestStatus(tc.Status) {
		case v1.TestStatusSuccess, v1.TestStatusFlake: // success, flake(failed one or more times but ultimately succeeded)
			switch {
			case testidentification.IsOverallTest(tc.TestName):
				jrr.Succeeded = true
				// if the overall job succeeded, install is always considered successful, even for jobs
				// that don't have an explicitly defined install test.
				if jrr.InstallStatus != testidentification.Failure {
					jrr.InstallStatus = testidentification.Success
				}
			case testidentification.IsOperatorHealthTest(tc.TestName):
				jrr.FinalOperatorStates = append(jrr.FinalOperatorStates, v1.OperatorState{
					Name:  testidentification.GetOperatorNameFromTest(tc.TestName),
					State: testidentification.Success,
				})
			case testidentification.IsInstallStepEquivalent(tc.TestName):
				if jrr.InstallStatus != testidentification.Failure {
					jrr.InstallStatus = testidentification.Success
				}
			case testidentification.IsUpgradeStartedTest(tc.TestName):
				jrr.UpgradeStarted = true
			case testidentification.IsOperatorsUpgradedTest(tc.TestName):
				if jrr.UpgradeForOperatorsStatus != testidentification.Failure {
					jrr.UpgradeForOperatorsStatus = testidentification.Success
				}
			case testidentification.IsMachineConfigPoolsUpgradedTest(tc.TestName):
				if jrr.UpgradeForMachineConfigPoolsStatus != testidentification.Failure {
					jrr.UpgradeForMachineConfigPoolsStatus = testidentification.Success
				}
			default:
				// Any other non-special test contributes to overall test status
				if jrr.TestsStatus == "" {
					jrr.TestsStatus = testidentification.Success
				}
			}
		case v1.TestStatusFailure:
			// only add the failing test and name if it has predictive value.  We excluded all the non-predictive ones above except for these
			// which we use to set various JobRunResult markers
			if !testidentification.IsOverallTest(tc.TestName) {
				jrr.FailedTestNames = append(jrr.FailedTestNames, tc.TestName)
				jrr.TestFailures++
			}

			switch {
			case testidentification.IsOverallTest(tc.TestName):
				jrr.Failed = true
			case testidentification.IsOperatorHealthTest(tc.TestName):
				jrr.FinalOperatorStates = append(jrr.FinalOperatorStates, v1.OperatorState{
					Name:  testidentification.GetOperatorNameFromTest(tc.TestName),
					State: testidentification.Failure,
				})
			case testidentification.IsInstallStepEquivalent(tc.TestName):
				jrr.InstallStatus = testidentification.Failure
			case testidentification.IsUpgradeStartedTest(tc.TestName):
				jrr.UpgradeStarted = true // this is still true because we definitely started
			case testidentification.IsOperatorsUpgradedTest(tc.TestName):
				jrr.UpgradeForOperatorsStatus = testidentification.Failure
			case testidentification.IsMachineConfigPoolsUpgradedTest(tc.TestName):
				jrr.UpgradeForMachineConfigPoolsStatus = testidentification.Failure
			default:
				jrr.TestsStatus = testidentification.Failure
			}
		}
	}
}
