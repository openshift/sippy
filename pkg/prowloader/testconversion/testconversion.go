package testconversion

import (
	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
)

func ConvertProwJobRunToSyntheticTests(pj prow.ProwJob, tests map[string]*models.ProwJobRunTest, manager synthetictests.SyntheticTestManager) (*junit.TestSuite, v1.JobOverallResult) {
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

func testsToRawJobRunResult(jrr *v1.RawJobRunResult, tests map[string]*models.ProwJobRunTest) {
	for name, test := range tests {
		switch testgridv1.TestStatus(test.Status) {
		case testgridv1.TestStatusSuccess, testgridv1.TestStatusFlake: // success, flake(failed one or more times but ultimately succeeded)
			switch {
			case testidentification.IsOverallTest(name):
				jrr.Succeeded = true
				// if the overall job succeeded, install is always considered successful, even for jobs
				// that don't have an explicitly defined install test.
				jrr.InstallStatus = testidentification.Success
			case testidentification.IsOperatorHealthTest(name):
				jrr.FinalOperatorStates = append(jrr.FinalOperatorStates, v1.OperatorState{
					Name:  testidentification.GetOperatorNameFromTest(name),
					State: testidentification.Success,
				})
			case testidentification.IsInstallStepEquivalent(name):
				jrr.InstallStatus = testidentification.Success
			case testidentification.IsUpgradeStartedTest(name):
				jrr.UpgradeStarted = true
			case testidentification.IsOperatorsUpgradedTest(name):
				jrr.UpgradeForOperatorsStatus = testidentification.Success
			case testidentification.IsMachineConfigPoolsUpgradedTest(name):
				jrr.UpgradeForMachineConfigPoolsStatus = testidentification.Success
			case testidentification.IsOpenShiftTest(name):
				// If there is a failed test, the aggregated value should stay "Failure"
				if jrr.OpenShiftTestsStatus == "" {
					jrr.OpenShiftTestsStatus = testidentification.Success
				}
			}
		case testgridv1.TestStatusFailure:
			// only add the failing test and name if it has predictive value.  We excluded all the non-predictive ones above except for these
			// which we use to set various JobRunResult markers
			if !testidentification.IsOverallTest(name) {
				jrr.FailedTestNames = append(jrr.FailedTestNames, name)
				jrr.TestFailures++
			}

			// TODO: should we also add failures to jrr.TestResults so everything is in one place? Kill off FailedTestNames

			switch {
			case testidentification.IsOverallTest(name):
				jrr.Failed = true
			case testidentification.IsOperatorHealthTest(name):
				jrr.FinalOperatorStates = append(jrr.FinalOperatorStates, v1.OperatorState{
					Name:  testidentification.GetOperatorNameFromTest(name),
					State: testidentification.Failure,
				})
			case testidentification.IsInstallStepEquivalent(name):
				jrr.InstallStatus = testidentification.Failure
			case testidentification.IsUpgradeStartedTest(name):
				jrr.UpgradeStarted = true // this is still true because we definitely started
			case testidentification.IsOperatorsUpgradedTest(name):
				jrr.UpgradeForOperatorsStatus = testidentification.Failure
			case testidentification.IsMachineConfigPoolsUpgradedTest(name):
				jrr.UpgradeForMachineConfigPoolsStatus = testidentification.Failure
			case testidentification.IsOpenShiftTest(name):
				jrr.OpenShiftTestsStatus = testidentification.Failure
			}
		}
	}
}
