package testgridconversion

import (
	"testing"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	tgv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"

	"github.com/stretchr/testify/assert"
)

const (
	job1Name    = "myjob1"
	job1RunURL1 = "http://prow.example.com/myjob1/98127398134"
)

func TestSyntheticSippyTestGeneration(t *testing.T) {
	testCases := []struct {
		name                string
		rawJobResults       testgridanalysisapi.RawJobResult
		expectedTestResults []testgridanalysisapi.RawJobRunTestResult
	}{
		{
			name: "successful install adds successful operator tests",
			rawJobResults: testgridanalysisapi.RawJobResult{
				JobName: job1Name,
				JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
					job1RunURL1: buildFakeRawJobRunResult(true, true, v1.JobSucceeded,
						[]testgridanalysisapi.OperatorState{
							{"openshift-apiserver", "Success"},
						},
					),
				},
				TestResults: map[string]testgridanalysisapi.RawTestResult{},
			},
			expectedTestResults: []testgridanalysisapi.RawJobRunTestResult{
				{testgridanalysisapi.InstallTestName, tgv1.TestStatusSuccess},
				{testgridanalysisapi.FinalOperatorHealthTestName, tgv1.TestStatusSuccess},
				{"operator install openshift-apiserver", tgv1.TestStatusSuccess},
			},
		},
		{
			name: "failed install adds successful operator tests",
			rawJobResults: testgridanalysisapi.RawJobResult{
				JobName: job1Name,
				JobRunResults: map[string]testgridanalysisapi.RawJobRunResult{
					job1RunURL1: buildFakeRawJobRunResult(false, false, v1.JobInstallFailure,
						[]testgridanalysisapi.OperatorState{
							{"openshift-apiserver", "Success"},
						},
					),
				},
				TestResults: map[string]testgridanalysisapi.RawTestResult{},
			},
			expectedTestResults: []testgridanalysisapi.RawJobRunTestResult{
				{testgridanalysisapi.InstallTestName, tgv1.TestStatusFailure},
				{testgridanalysisapi.FinalOperatorHealthTestName, tgv1.TestStatusSuccess},
				{"operator install openshift-apiserver", tgv1.TestStatusSuccess},
			},
		},
	}
	for _, tc := range testCases {
		testMgr := NewOpenshiftSyntheticTestManager()
		t.Run(tc.name, func(t *testing.T) {
			rjr := tc.rawJobResults
			testMgr.CreateSyntheticTests(testgridanalysisapi.RawData{JobResults: map[string]testgridanalysisapi.RawJobResult{job1Name: rjr}})
			assertJobRunTestResult(t, rjr, tc.expectedTestResults)

		})
	}
}

func assertJobRunTestResult(t *testing.T, rjr testgridanalysisapi.RawJobResult, expectedTestResults []testgridanalysisapi.RawJobRunTestResult) {
	for _, etr := range expectedTestResults {
		var found bool
		for _, tr := range rjr.JobRunResults[job1RunURL1].TestResults {
			if tr.Name == etr.Name {
				assert.Equal(t, etr.Status, tr.Status, "expected test found but with incorrect status")
				found = true
			}
		}
		assert.True(t, found, "expected test was not found: %s", etr.Name)
	}
}

func getStatusStr(success bool) string {
	if success {
		return "Success"
	}
	return "Failure"
}
func buildFakeRawJobRunResult(
	installSuccess bool,
	testsSuccess bool,
	overallJobResult v1.JobOverallResult,
	operatorStates []testgridanalysisapi.OperatorState,
) testgridanalysisapi.RawJobRunResult {
	return testgridanalysisapi.RawJobRunResult{
		Job:             job1Name,
		JobRunURL:       job1RunURL1,
		TestFailures:    0,
		FailedTestNames: []string{},
		TestResults: []testgridanalysisapi.RawJobRunTestResult{
			{},
		},
		Succeeded:           testsSuccess,
		Failed:              !testsSuccess,
		InstallStatus:       getStatusStr(installSuccess),
		FinalOperatorStates: operatorStates,
		/*
			UpgradeStarted:                     false,
			UpgradeForOperatorsStatus:          "",
			UpgradeForMachineConfigPoolsStatus: "",
		*/
		OpenShiftTestsStatus: getStatusStr(testsSuccess),
		OverallResult:        overallJobResult,
		//Timestamp:            0,
	}
}
