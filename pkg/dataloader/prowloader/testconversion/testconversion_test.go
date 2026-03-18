package testconversion

import (
	"testing"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/assert"
)

func successTest() *models.ProwJobRunTest {
	return &models.ProwJobRunTest{Status: int(v1.TestStatusSuccess)}
}

func failureTest() *models.ProwJobRunTest {
	return &models.ProwJobRunTest{Status: int(v1.TestStatusFailure)}
}

func TestTestsToRawJobRunResult(t *testing.T) {
	tests := []struct {
		name     string
		tests    map[string]*models.ProwJobRunTest
		validate func(t *testing.T, jrr *v1.RawJobRunResult)
	}{
		{
			name: "install step success sets InstallStatus",
			tests: map[string]*models.ProwJobRunTest{
				"cluster install.install should succeed: overall": successTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Success", jrr.InstallStatus)
			},
		},
		{
			name: "install step failure sets InstallStatus",
			tests: map[string]*models.ProwJobRunTest{
				"cluster install.install should succeed: overall": failureTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.InstallStatus)
			},
		},
		{
			name: "openshift test failure sets TestsStatus",
			tests: map[string]*models.ProwJobRunTest{
				"openshift-tests.[sig-apps] example test": failureTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.TestsStatus)
				assert.Equal(t, 1, jrr.TestFailures)
			},
		},
		{
			name: "non-openshift test failure also sets TestsStatus",
			tests: map[string]*models.ProwJobRunTest{
				"aro-hcp-tests.some integration test": failureTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Failure", jrr.TestsStatus)
				assert.Equal(t, 1, jrr.TestFailures)
			},
		},
		{
			name: "non-openshift test success sets TestsStatus",
			tests: map[string]*models.ProwJobRunTest{
				"aro-hcp-tests.some integration test": successTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, "Success", jrr.TestsStatus)
			},
		},
		{
			name: "prowjob-junit failures are skipped",
			tests: map[string]*models.ProwJobRunTest{
				"prowjob-junit.some-step-name": failureTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, 0, jrr.TestFailures)
				assert.Empty(t, jrr.FailedTestNames)
			},
		},
		{
			name: "step graph failures are skipped",
			tests: map[string]*models.ProwJobRunTest{
				"step graph.some-step": failureTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, 0, jrr.TestFailures)
				assert.Empty(t, jrr.FailedTestNames)
			},
		},
		{
			name: "Run pipeline step failures are skipped",
			tests: map[string]*models.ProwJobRunTest{
				"aro-hcp-tests.Run pipeline step provision": failureTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.Equal(t, 0, jrr.TestFailures)
				assert.Empty(t, jrr.FailedTestNames)
			},
		},
		{
			name: "overall test success sets Succeeded and InstallStatus",
			tests: map[string]*models.ProwJobRunTest{
				"openshift-tests.Overall": successTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.True(t, jrr.Succeeded)
				assert.Equal(t, "Success", jrr.InstallStatus)
			},
		},
		{
			name: "overall test failure sets Failed",
			tests: map[string]*models.ProwJobRunTest{
				"openshift-tests.Overall": failureTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.True(t, jrr.Failed)
				// Overall failures should not be counted in TestFailures
				assert.Equal(t, 0, jrr.TestFailures)
			},
		},
		{
			name: "upgrade started test sets UpgradeStarted",
			tests: map[string]*models.ProwJobRunTest{
				"Cluster upgrade.[sig-cluster-lifecycle] Cluster version operator acknowledges upgrade": successTest(),
			},
			validate: func(t *testing.T, jrr *v1.RawJobRunResult) {
				assert.True(t, jrr.UpgradeStarted)
			},
		},
		{
			name: "only infra-only failures produces empty result",
			tests: map[string]*models.ProwJobRunTest{
				"prowjob-junit.step1":       failureTest(),
				"step graph.step2":          failureTest(),
				"suite.Run pipeline step x": failureTest(),
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
