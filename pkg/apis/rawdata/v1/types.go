// these types should not be used by anyone outside of sippy.
// long term, I think these types become internal to another package.
package v1

// 1. TestGrid contains jobs
// 2. Jobs contain JobRuns.  Jobs have associated variants/platforms.
// 3. JobRuns contain Tests.

// AggregateTestsResult is an intermediate datatype that may not have complete or consistent data when interrogated.
// It holds data about many different tests, not just one.
// It is used to build up a set of TestResults (details about individual tests).
type AggregateTestsResult struct {
	// TestResults is a map from test.Name to the aggregated results for each run of that test.
	RawTestResults map[string]RawTestResult `json:"results"`
}

// RawTestResult is an intermediate datatype that may not have complete or consistent data when interrogated.
// It holds data about an individual test that may have happened in may different jobs and job runs.
// It is used to build up a complete set of successes and failure, but until all the testgrid results have been checked, it will be incomplete
type RawTestResult struct {
	Name      string `json:"name"`
	Successes int    `json:"successes"`
	Failures  int    `json:"failures"`
	Flakes    int    `json:"flakes"`
}

// RawJobRunResult is an intermediate datatype that may not have complete or consistent data when interrogated.
// It holds data for an individual run of a given job.
type RawJobRunResult struct {
	Job             string   `json:"job"`
	Url             string   `json:"url"`
	TestGridJobUrl  string   `json:"testGridJobUrl"`
	TestFailures    int      `json:"testFailures"`
	FailedTestNames []string `json:"failedTestNames"`
	Failed          bool     `json:"failed"`
	Succeeded       bool     `json:"succeeded"`

	// SetupStatus can be "", "Success", "Failure"
	// Used to create synthentic tests.
	SetupStatus string `json:"setupStatus"`
	// Used to create synthentic tests.
	InstallOperators []OperatorState `json:"installOperators"`
	// Used to create synthentic tests.
	UpgradeOperators []OperatorState `json:"upgradeOperators"`
}

type OperatorState struct {
	Name string `json:"name"`
	// OperatorState can be "", "Success", "Failure"
	State string `json:"state"`
}

const (
	InfrastructureTestName = `[sig-sippy] infrastructure should work`
	InstallTestName        = `[sig-sippy] install should work`
	UpgradeTestName        = `[sig-sippy] upgrade should work`

	Success = "Success"
	Failure = "Failure"
)
