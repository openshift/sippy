package testconversion

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/prow"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/types"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/stretchr/testify/assert"
)

func success(suite, test string) *types.TestCaseEntry {
	return &types.TestCaseEntry{SuiteName: suite, TestName: test, Status: int(v1.TestStatusSuccess)}
}

func failure(suite, test string) *types.TestCaseEntry {
	return &types.TestCaseEntry{SuiteName: suite, TestName: test, Status: int(v1.TestStatusFailure)}
}

func TestTestsToRawJobRunResult(t *testing.T) {
	tests := []struct {
		name     string
		tests    []*types.TestCaseEntry
		validate func(t *testing.T, jrr *v1.RawJobRunResult)
	}{
		{
			name:  "install step success sets InstallStatus",
			tests: []*types.TestCaseEntry{success("cluster install", "install should succeed: overall")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Success", jrr.InstallStatus)
			},
		},
		{
			name:  "install step failure sets InstallStatus",
			tests: []*types.TestCaseEntry{failure("cluster install", "install should succeed: overall")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.InstallStatus)
			},
		},
		{
			name:  "openshift test failure sets TestsStatus",
			tests: []*types.TestCaseEntry{failure("openshift-tests", "[sig-apps] example test")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.TestsStatus)
				assert.Equal(t, 1, jrr.TestFailures)
			},
		},
		{
			name:  "non-openshift test failure also sets TestsStatus",
			tests: []*types.TestCaseEntry{failure("aro-hcp-tests", "some integration test")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.TestsStatus)
				assert.Equal(t, 1, jrr.TestFailures)
			},
		},
		{
			name:  "non-openshift test success sets TestsStatus",
			tests: []*types.TestCaseEntry{success("aro-hcp-tests", "some integration test")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Success", jrr.TestsStatus)
			},
		},
		{
			name:  "prowjob-junit failures are skipped",
			tests: []*types.TestCaseEntry{failure("prowjob-junit", "some-step-name")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, 0, jrr.TestFailures)
				assert.Empty(t, jrr.FailedTestNames)
			},
		},
		{
			name:  "step graph failures are skipped",
			tests: []*types.TestCaseEntry{failure("step graph", "some-step")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, 0, jrr.TestFailures)
				assert.Empty(t, jrr.FailedTestNames)
			},
		},
		{
			name:  "normalized step graph failure contributes test signal",
			tests: []*types.TestCaseEntry{failure("step graph", "Run multi-stage step test0")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, testidentification.Failure, jrr.TestsStatus)
				assert.Equal(t, 1, jrr.TestFailures)
				assert.Equal(t, []string{"Run multi-stage step test0"}, jrr.FailedTestNames)
			},
		},
		{
			name:  "normalized step graph install failure sets InstallStatus",
			tests: []*types.TestCaseEntry{failure("step graph", "Run multi-stage step ipi-install-install-stableinitial")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, testidentification.Failure, jrr.InstallStatus)
				assert.Equal(t, 1, jrr.TestFailures)
				assert.Equal(t, []string{"Run multi-stage step ipi-install-install-stableinitial"}, jrr.FailedTestNames)
			},
		},
		{
			name:  "Run pipeline step failures are skipped",
			tests: []*types.TestCaseEntry{failure("aro-hcp-tests", "Run pipeline step provision")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, 0, jrr.TestFailures)
				assert.Empty(t, jrr.FailedTestNames)
			},
		},
		{
			name:  "overall test success sets Succeeded and InstallStatus",
			tests: []*types.TestCaseEntry{success("openshift-tests", "Overall")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.True(t, jrr.Succeeded)
				assert.Equal(t, "Success", jrr.InstallStatus)
			},
		},
		{
			name:  "overall test failure sets Failed",
			tests: []*types.TestCaseEntry{failure("openshift-tests", "Overall")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.True(t, jrr.Failed)
				// Overall failures should not be counted in TestFailures
				assert.Equal(t, 0, jrr.TestFailures)
			},
		},
		{
			name:  "upgrade started test sets UpgradeStarted",
			tests: []*types.TestCaseEntry{success("Cluster upgrade", "[sig-cluster-lifecycle] Cluster version operator acknowledges upgrade")},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.True(t, jrr.UpgradeStarted)
			},
		},
		{
			name: "install failure is sticky across suites",
			tests: []*types.TestCaseEntry{
				failure("openshift-tests", "install should succeed: overall"),
				success("openshift-tests-upgrade", "install should succeed: overall"),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.InstallStatus)
			},
		},
		{
			name: "upgrade operators failure is sticky across suites",
			tests: []*types.TestCaseEntry{
				failure("Cluster upgrade", "[sig-cluster-lifecycle] Cluster completes upgrade"),
				success("Cluster upgrade", "[sig-cluster-lifecycle] Cluster completes upgrade"),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.UpgradeForOperatorsStatus)
			},
		},
		{
			name: "machine config pools failure is sticky across suites",
			tests: []*types.TestCaseEntry{
				failure("Cluster upgrade", "[sig-mco] Machine config pools complete upgrade"),
				success("Cluster upgrade", "[sig-mco] Machine config pools complete upgrade"),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.UpgradeForMachineConfigPoolsStatus)
			},
		},
		{
			name: "only infra-only failures produces empty result",
			tests: []*types.TestCaseEntry{
				failure("prowjob-junit", "step1"),
				failure("step graph", "step2"),
				failure("suite", "Run pipeline step x"),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, 0, jrr.TestFailures)
				assert.Empty(t, jrr.FailedTestNames)
				assert.Empty(t, jrr.InstallStatus)
				assert.Empty(t, jrr.TestsStatus)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jrr := &v1.RawJobRunResult{}
			testsToRawJobRunResult(jrr, tc.tests)
			tc.validate(t, jrr)
		})
	}
}

// TestOpenShiftTestsShouldWorkAcrossJobStates guards the spot-check analyzer, which treats any passing
// run of the synthetic test as healthy: runs that never completed must not produce a pass.
func TestOpenShiftTestsShouldWorkAcrossJobStates(t *testing.T) {
	tests := []struct {
		name     string
		state    prow.ProwJobState
		tests    []*types.TestCaseEntry
		wantPass bool
		wantFail bool
	}{
		{
			name:     "successful job with passing tests",
			state:    prow.SuccessState,
			tests:    []*types.TestCaseEntry{success("openshift-tests", "[sig-apps] example test")},
			wantPass: true,
		},
		{
			name:  "successful job with only informing failures",
			state: prow.SuccessState,
			tests: []*types.TestCaseEntry{
				success("openshift-tests", "[sig-apps] example test"),
				failure("openshift-tests", "[sig-apps] informing test"),
			},
			wantPass: true,
		},
		{
			name:     "successful job without any test results",
			state:    prow.SuccessState,
			wantPass: true,
		},
		{
			name:     "failed job with failing tests",
			state:    prow.FailureState,
			tests:    []*types.TestCaseEntry{failure("openshift-tests", "[sig-apps] example test")},
			wantFail: true,
		},
		{
			name:  "failed job without test results",
			state: prow.FailureState,
		},
		{
			name:  "aborted job without test results",
			state: prow.AbortedState,
		},
		{
			name:  "errored job without test results",
			state: prow.ErrorState,
		},
	}

	manager := synthetictests.NewOpenshiftSyntheticTestManager()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := prow.ProwJob{Status: prow.ProwJobStatus{State: tc.state}}
			suite, _ := ConvertProwJobRunToSyntheticTests(job, tc.tests, manager)

			var gotPass, gotFail bool
			for _, c := range suite.TestCases {
				if c.Name != testidentification.OpenShiftTestsName {
					continue
				}
				if c.FailureOutput != nil {
					gotFail = true
				} else {
					gotPass = true
				}
			}
			assert.Equal(t, tc.wantPass, gotPass, "synthetic test pass")
			assert.Equal(t, tc.wantFail, gotFail, "synthetic test failure")
		})
	}
}
