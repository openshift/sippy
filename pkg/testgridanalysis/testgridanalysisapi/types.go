// these types should not be used by anyone outside of testgridanalysis.
// long term, I think these types become internal under this package.
package testgridanalysisapi

import (
	"regexp"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
)

// 1. TestGrid contains jobs
// 2. Jobs contain JobRuns.  Jobs have associated variants.
// 3. JobRuns contain Tests.

type RawData struct {
	// JobResults is a map keyed by job name to results for all runs of a job
	JobResults map[string]RawJobResult
}

type RawJobResult struct {
	JobName        string
	TestGridJobURL string

	Query       string
	ChangeLists []string

	// JobRunResults is a map from individual job run URL to the results of that job run
	JobRunResults map[string]RawJobRunResult

	// TestResults is a map from test.Name to the aggregated results for each run of that test inside the job
	TestResults map[string]RawTestResult
}

// RawTestResult is an intermediate datatype that may not have complete or consistent data when interrogated.
// It holds data about an individual test that may have happened in many different jobs and job runs.
// It is used to build up a complete set of successes and failure, but until all the testgrid results have been checked, it will be incomplete
type RawTestResult struct {
	Name       string
	Timestamps []int
	Successes  int
	Failures   int
	Flakes     int
}

// RawJobRunTestResult represents an execution of a test in a job run, and whether it was success, failure, or a flake.
type RawJobRunTestResult struct {
	Name   string
	Status testgridv1.TestStatus
}

// RawJobRunResult is an intermediate datatype that may not have complete or consistent data when interrogated.
// It holds data for an individual run of a given job.
type RawJobRunResult struct {
	Job             string
	JobRunURL       string
	TestFailures    int
	FailedTestNames []string
	TestResults     []RawJobRunTestResult
	Failed          bool
	Succeeded       bool

	// SetupStatus can be "", "Success", "Failure"
	// Used to create synthetic tests.
	SetupStatus         string
	FinalOperatorStates []OperatorState

	// UpgradeStarted is true if the test attempted to start an upgrade based on the CVO succeeding (or failing) to acknowledge a request
	UpgradeStarted bool
	// Success, Failure, or ""
	UpgradeForOperatorsStatus string
	// Success, Failure, or ""
	UpgradeForMachineConfigPoolsStatus string

	// OpenShiftTestsStatus can be "", "Success", "Failure"
	OpenShiftTestsStatus string

	// Overall result
	OverallResult v1.JobOverallResult

	// Timestamp
	Timestamp int
}

type OperatorState struct {
	Name string
	// OperatorState can be "", "Success", "Failure"
	State string
}

const (
	OperatorUpgradePrefix       = "Operator upgrade "
	OperatorFinalHealthPrefix   = "operator conditions "
	FinalOperatorHealthTestName = `[sig-sippy] tests should finish with healthy operators`

	InfrastructureTestName = `[sig-sippy] infrastructure should work`
	InstallTestName        = `[sig-sippy] install should work`
	InstallTimeoutTestName = `[sig-sippy] install should not timeout`
	UpgradeTestName        = `[sig-sippy] upgrade should work`
	OpenShiftTestsName     = `[sig-sippy] openshift-tests should work`

	Success = "Success"
	Failure = "Failure"
	Unknown = "Unknown"
)

var (
	// TODO: add [sig-sippy] here as well so we can more clearly identify and substring search
	OperatorInstallPrefix          = "operator install "
	OperatorConditionsTestCaseName = regexp.MustCompile("operator install (?P<operator>.*)")
)
