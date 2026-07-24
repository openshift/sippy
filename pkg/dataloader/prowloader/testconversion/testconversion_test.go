package testconversion

import (
	"testing"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/types"
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
