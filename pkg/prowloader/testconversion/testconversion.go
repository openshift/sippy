package testconversion

import (
	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testidentification"
)

func ConvertProwJobRunToSyntheticTests(pj prow.ProwJob, tests []models.ProwJobRunTest, manager synthetictests.SyntheticTestManager) (*junit.TestSuite, v1.JobOverallResult) {
	jrr := testgridanalysisapi.RawJobRunResult{
		Job:       pj.Spec.Job,
		Failed:    pj.Status.State != prow.SuccessState,
		Succeeded: pj.Status.State == prow.SuccessState,
	}
	testsToRawJobRunResult(&jrr, tests)
	syntheticTests := manager.CreateSyntheticTests(&jrr)
	return syntheticTests, jrr.OverallResult
}

func testsToRawJobRunResult(jrr *testgridanalysisapi.RawJobRunResult, tests []models.ProwJobRunTest) {
	for _, test := range tests {
		switch testgridv1.TestStatus(test.Status) {
		case testgridv1.TestStatusSuccess, testgridv1.TestStatusFlake: // success, flake(failed one or more times but ultimately succeeded)
			switch {
			case testidentification.IsOverallTest(test.Test.Name):
				jrr.Succeeded = true
				// if the overall job succeeded, install is always considered successful, even for jobs
				// that don't have an explicitly defined install test.
				jrr.InstallStatus = testgridanalysisapi.Success
			case testidentification.IsOperatorHealthTest(test.Test.Name):
				jrr.FinalOperatorStates = append(jrr.FinalOperatorStates, testgridanalysisapi.OperatorState{
					Name:  testidentification.GetOperatorNameFromTest(test.Test.Name),
					State: testgridanalysisapi.Success,
				})
			case testidentification.IsInstallStepEquivalent(test.Test.Name):
				jrr.InstallStatus = testgridanalysisapi.Success
			case testidentification.IsUpgradeStartedTest(test.Test.Name):
				jrr.UpgradeStarted = true
			case testidentification.IsOperatorsUpgradedTest(test.Test.Name):
				jrr.UpgradeForOperatorsStatus = testgridanalysisapi.Success
			case testidentification.IsMachineConfigPoolsUpgradedTest(test.Test.Name):
				jrr.UpgradeForMachineConfigPoolsStatus = testgridanalysisapi.Success
			case testidentification.IsOpenShiftTest(test.Test.Name):
				// If there is a failed test, the aggregated value should stay "Failure"
				if jrr.OpenShiftTestsStatus == "" {
					jrr.OpenShiftTestsStatus = testgridanalysisapi.Success
				}
			}
		case testgridv1.TestStatusFailure:
			// only add the failing test and name if it has predictive value.  We excluded all the non-predictive ones above except for these
			// which we use to set various JobRunResult markers
			if !testidentification.IsOverallTest(test.Test.Name) {
				jrr.FailedTestNames = append(jrr.FailedTestNames, test.Test.Name)
				jrr.TestFailures++
			}

			// TODO: should we also add failures to jrr.TestResults so everything is in one place? Kill off FailedTestNames

			switch {
			case testidentification.IsOverallTest(test.Test.Name):
				jrr.Failed = true
			case testidentification.IsOperatorHealthTest(test.Test.Name):
				jrr.FinalOperatorStates = append(jrr.FinalOperatorStates, testgridanalysisapi.OperatorState{
					Name:  testidentification.GetOperatorNameFromTest(test.Test.Name),
					State: testgridanalysisapi.Failure,
				})
			case testidentification.IsInstallStepEquivalent(test.Test.Name):
				jrr.InstallStatus = testgridanalysisapi.Failure
			case testidentification.IsUpgradeStartedTest(test.Test.Name):
				jrr.UpgradeStarted = true // this is still true because we definitely started
			case testidentification.IsOperatorsUpgradedTest(test.Test.Name):
				jrr.UpgradeForOperatorsStatus = testgridanalysisapi.Failure
			case testidentification.IsMachineConfigPoolsUpgradedTest(test.Test.Name):
				jrr.UpgradeForMachineConfigPoolsStatus = testgridanalysisapi.Failure
			case testidentification.IsOpenShiftTest(test.Test.Name):
				jrr.OpenShiftTestsStatus = testgridanalysisapi.Failure
			}
		}
	}
}
