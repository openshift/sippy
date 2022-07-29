package synthetictests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	tgv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testidentification"
)

const (
	job1Name    = "myjob1"
	job1RunURL1 = "http://prow.example.com/myjob1/98127398134"
)

func TestSyntheticSippyTestGeneration(t *testing.T) {
	testCases := []struct {
		name                    string
		rawJobResults           v1.RawJobResult
		expectedTestResults     []v1.RawJobRunTestResult
		expectedFailedTestNames []string
	}{
		{
			name: "successful install adds successful operator tests",
			rawJobResults: v1.RawJobResult{
				JobName: job1Name,
				JobRunResults: map[string]*v1.RawJobRunResult{
					job1RunURL1: buildFakeRawJobRunResult(true, true, v1.JobSucceeded,
						[]v1.OperatorState{
							{Name: "openshift-apiserver", State: "Success"},
						},
					),
				},
				TestResults: map[string]v1.RawTestResult{},
			},
			expectedTestResults: []v1.RawJobRunTestResult{
				{Name: testidentification.InstallTestName, Status: tgv1.TestStatusSuccess},
				{Name: testidentification.FinalOperatorHealthTestName, Status: tgv1.TestStatusSuccess},
				{Name: "operator install openshift-apiserver", Status: tgv1.TestStatusSuccess},
			},
		},
		{
			name: "failed install adds successful operator tests",
			rawJobResults: v1.RawJobResult{
				JobName: job1Name,
				JobRunResults: map[string]*v1.RawJobRunResult{
					job1RunURL1: buildFakeRawJobRunResult(false, false, v1.JobInstallFailure,
						[]v1.OperatorState{
							{Name: "openshift-apiserver", State: "Success"},
						},
					),
				},
				TestResults: map[string]v1.RawTestResult{},
			},
			expectedTestResults: []v1.RawJobRunTestResult{
				{Name: testidentification.FinalOperatorHealthTestName, Status: tgv1.TestStatusSuccess},
				{Name: "operator install openshift-apiserver", Status: tgv1.TestStatusSuccess},
			},
			expectedFailedTestNames: []string{
				testidentification.SippySuiteName + "." + testidentification.InstallTestName,
			},
		},
	}
	for _, tc := range testCases {
		testMgr := NewOpenshiftSyntheticTestManager()
		t.Run(tc.name, func(t *testing.T) {
			rjr := tc.rawJobResults
			for _, jrr := range rjr.JobRunResults {
				testMgr.CreateSyntheticTests(jrr)
			}
			assertJobRunTestResult(t, rjr, tc.expectedTestResults)
			assertFailedTestNames(t, rjr, tc.expectedFailedTestNames)

		})
	}
}

func assertJobRunTestResult(t *testing.T, rjr v1.RawJobResult, expectedTestResults []v1.RawJobRunTestResult) {
	for _, etr := range expectedTestResults {
		var found bool
		for _, tr := range rjr.JobRunResults[job1RunURL1].TestResults {
			t.Logf("test: %s", tr.Name)
			if tr.Name == etr.Name {
				assert.Equal(t, etr.Status, tr.Status, "expected test found but with incorrect status")
				found = true
			}
		}
		assert.True(t, found, "expected test was not found: %s", etr.Name)
	}
}

func assertFailedTestNames(t *testing.T, rjr v1.RawJobResult, expectedFailedTestNames []string) {
	for _, tn := range expectedFailedTestNames {
		var found bool
		for _, tr := range rjr.JobRunResults[job1RunURL1].FailedTestNames {
			t.Logf("test: %s", tn)
			if tr == tn {
				found = true
			}
		}
		assert.True(t, found, "expected failed test was not found: %s", tn)
	}
}

// revive:disable:flag-parameter
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
	operatorStates []v1.OperatorState,
) *v1.RawJobRunResult {
	return &v1.RawJobRunResult{
		Job:             job1Name,
		JobRunURL:       job1RunURL1,
		TestFailures:    0,
		FailedTestNames: []string{},
		TestResults: []v1.RawJobRunTestResult{
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
