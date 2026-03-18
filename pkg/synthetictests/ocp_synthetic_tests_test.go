package synthetictests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
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
				{Name: testidentification.InstallTestName, Status: v1.TestStatusSuccess},
				{Name: testidentification.FinalOperatorHealthTestName, Status: v1.TestStatusSuccess},
				{Name: "operator install openshift-apiserver", Status: v1.TestStatusSuccess},
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
				{Name: testidentification.FinalOperatorHealthTestName, Status: v1.TestStatusSuccess},
				{Name: "operator install openshift-apiserver", Status: v1.TestStatusSuccess},
			},
			expectedFailedTestNames: []string{
				testidentification.InstallTestName,
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

func TestJobRunStatusClassification(t *testing.T) {
	testCases := []struct {
		name           string
		jrr            v1.RawJobRunResult
		expectedResult v1.JobOverallResult
	}{
		{
			name: "tests failed → F",
			jrr: v1.RawJobRunResult{
				Failed:      true,
				TestsStatus: "Failure",
			},
			expectedResult: v1.JobTestFailure,
		},
		{
			name: "no test results → n",
			jrr: v1.RawJobRunResult{
				Failed: true,
			},
			expectedResult: v1.JobFailureBeforeSetup,
		},
		{
			name: "install succeeded but no test results → n",
			jrr: v1.RawJobRunResult{
				Failed:        true,
				InstallStatus: "Success",
				FinalOperatorStates: []v1.OperatorState{
					{Name: "openshift-apiserver", State: "Success"},
				},
			},
			expectedResult: v1.JobFailureBeforeSetup,
		},
		{
			name: "install failed + no operators → N",
			jrr: v1.RawJobRunResult{
				Failed:        true,
				InstallStatus: "Failure",
			},
			expectedResult: v1.JobInfrastructureFailure,
		},
		{
			name: "install failed + has operators → I",
			jrr: v1.RawJobRunResult{
				Failed:        true,
				InstallStatus: "Failure",
				FinalOperatorStates: []v1.OperatorState{
					{Name: "openshift-apiserver", State: "Failure"},
				},
			},
			expectedResult: v1.JobInstallFailure,
		},
		{
			name: "errored job → n",
			jrr: v1.RawJobRunResult{
				Failed:  true,
				Errored: true,
			},
			expectedResult: v1.JobFailureBeforeSetup,
		},
	}

	testMgr := NewOpenshiftSyntheticTestManager()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jrr := tc.jrr
			testMgr.CreateSyntheticTests(&jrr)
			assert.Equal(t, tc.expectedResult, jrr.OverallResult)
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
		TestsStatus: getStatusStr(testsSuccess),
		OverallResult:        overallJobResult,
		//Timestamp:            0,
	}
}
