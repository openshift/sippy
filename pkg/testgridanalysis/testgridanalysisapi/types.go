// these types should not be used by anyone outside of testgridanalysis.
// long term, I think these types become internal under this package.
package testgridanalysisapi

import (
	"regexp"
)

// 1. TestGrid contains jobs
// 2. Jobs contain JobRuns.  Jobs have associated variants/platforms.
// 3. JobRuns contain Tests.

type RawData struct {
	// JobResults is a map keyed by job name to results for all runs of a job
	JobResults map[string]RawJobResult
}

type RawJobResult struct {
	JobName        string
	TestGridJobUrl string

	// JobRunResults is a map from individual job run URL to the results of that job run
	JobRunResults map[string]RawJobRunResult

	// TestResults is a map from test.Name to the aggregated results for each run of that test inside the job
	TestResults map[string]RawTestResult
}

// RawTestResult is an intermediate datatype that may not have complete or consistent data when interrogated.
// It holds data about an individual test that may have happened in may different jobs and job runs.
// It is used to build up a complete set of successes and failure, but until all the testgrid results have been checked, it will be incomplete
type RawTestResult struct {
	Name      string
	Successes int
	Failures  int
	Flakes    int
}

// RawJobRunResult is an intermediate datatype that may not have complete or consistent data when interrogated.
// It holds data for an individual run of a given job.
type RawJobRunResult struct {
	Job             string
	JobRunURL       string
	TestFailures    int
	FailedTestNames []string
	Failed          bool
	Succeeded       bool

	// SetupStatus can be "", "Success", "Failure"
	// Used to create synthentic tests.
	SetupStatus string
	// Used to create synthentic tests.
	SadOperators []OperatorState
	// Used to create synthentic tests.
	UpgradeOperators []OperatorState
}

type OperatorState struct {
	Name string
	// OperatorState can be "", "Success", "Failure"
	State string
}

const (
	OperatorUpgradePrefix = "Operator upgrade "

	InfrastructureTestName = `[sig-sippy] infrastructure should work`
	InstallTestName        = `[sig-sippy] install should work`
	InstallTimeoutTestName = `[sig-sippy] install should not timeout`
	UpgradeTestName        = `[sig-sippy] upgrade should work`

	Success = "Success"
	Failure = "Failure"
)

var (
	OperatorConditionsTestCaseName = regexp.MustCompile("operator (install|conditions) (?P<operator>.*)")
)
