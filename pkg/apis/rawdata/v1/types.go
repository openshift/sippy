// these types should not be used by anyone outside of sippy.
// long term, I think these types become internal to another package.
package v1

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
