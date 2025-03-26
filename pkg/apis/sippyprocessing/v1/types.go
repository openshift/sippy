// Package v1 is used to produce a reporting structure for rendering html pages.
// it also contains intermediate types used in the processing pipeline.
package v1

import (
	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
)

type ReportType string

const (
	CurrentReport  ReportType = "current"
	TwoDayReport   ReportType = "twoDay"
	PreviousReport ReportType = "previous"
)

// Statistics is a type that contains statistical summaries.
type Statistics struct {
	Mean              float64   `json:"mean"`
	StandardDeviation float64   `json:"standard_deviation"`
	Histogram         []int     `json:"histogram"`
	Quartiles         []float64 `json:"quartiles"`
	P95               float64   `json:"p95"`
}

// VariantHealth is used to report overall health of variants.
type VariantHealth struct {
	Success  int `json:"success"`
	Unstable int `json:"unstable"`
	Failed   int `json:"failed"`
}

type FailingTestResult struct {
	TestName string `json:"testName"`

	// TestResultAcrossAllJobs contains the testResult aggregated across all jobs.  Each entry aggregates the results of all runs of a single test.  The array is sorted from lowest PassPercentage to highest PassPercentage
	TestResultAcrossAllJobs TestResult `json:"results"`

	// JobResults for all jobs that failed on this test ordered by the pass percentage of the test on a given job
	JobResults []FailingTestJobResult `json:"jobResults"`
}

// FailingTestJobResult is a job summary for the number of runs failed by this
type FailingTestJobResult struct {
	Name           string  `json:"name"`
	TestFailures   int     `json:"testFailures"`
	TestSuccesses  int     `json:"testSuccesses"`
	PassPercentage float64 `json:"passPercentage"`
	TestGridURL    string  `json:"testGridURL"`
}

// TestResult is a reporting type, not an intermediate type.  It represents the complete view of a given test.  It should
// always have complete data, not partial data.
type TestResult struct {
	Name           string  `json:"name"`
	Successes      int     `json:"successes"`
	Failures       int     `json:"failures"`
	Flakes         int     `json:"flakes"`
	PassPercentage float64 `json:"passPercentage"`
	// BugList shows all applicable bugs for the context.
	// Inside of a release, only bugs matching the release are present.
	// TODO Inside a particular job, only bugs matching the job are present.
	// TODO Inside a variant, only bugs matching the variant are present.
	BugList []bugsv1.Bug `json:"bugList"`
	// AssociatedBugList are bugs that match the test/job, but do not match the target release
	AssociatedBugList []bugsv1.Bug `json:"associatedBugList"`
}

type JobOverallResult string

const (
	JobSucceeded             JobOverallResult = "S"
	JobRunning               JobOverallResult = "R"
	JobInfrastructureFailure JobOverallResult = "N"
	JobInstallFailure        JobOverallResult = "I"
	JobUpgradeFailure        JobOverallResult = "U"
	JobTestFailure           JobOverallResult = "F"
	JobFailureBeforeSetup    JobOverallResult = "n"
	JobAborted               JobOverallResult = "A"
	JobUnknown               JobOverallResult = "f"
)

func (r *JobOverallResult) String() string {
	switch *r {
	case "S":
		return "Succeeded"
	case "R":
		return "Running"
	case "N":
		return "Infrastructure failure"
	case "I":
		return "Install failure"
	case "U":
		return "Upgrade failure"
	case "F":
		return "Test failures"
	case "n":
		return "CI Infrastructure failure"
	case "A":
		return "Aborted"
	default:
		return "Unknown"
	}

}

// JobRunResult represents a single invocation of a prow job and it's status, as well as any failed tests.
type JobRunResult struct {
	ProwID          uint     `json:"prowID"`
	Job             string   `json:"job"`
	URL             string   `json:"url"`
	TestFailures    int      `json:"testFailures"`
	FailedTestNames []string `json:"failedTestNames"`
	Failed          bool     `json:"failed"`
	// InfrastructureFailure is true if the job run failed, for reasons which appear to be related to test/CI infra.
	InfrastructureFailure bool `json:"infrastructureFailure"`
	// KnownFailure is true if the job run failed, but we found a bug that is likely related already filed.
	KnownFailure bool `json:"knownFailure"`
	Succeeded    bool `json:"succeeded"`
	// Timestamp is milliseconds since epoch when this job was run.
	Timestamp     int              `json:"timestamp"`
	OverallResult JobOverallResult `json:"result"`
}

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
	JobRunResults map[string]*RawJobRunResult

	// TestResults is a map from test.Name to the aggregated results for each run of that test inside the job
	// TODO: rename to indicate this is aggregated across all job runs. The name currently is identical to a field
	// on each JobRunResult.
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
	Status TestStatus
}

// RawJobRunResult is an intermediate datatype that may not have complete or consistent data when interrogated.
// It holds data for an individual run of a given job.
type RawJobRunResult struct {
	Job             string
	JobRunURL       string
	TestFailures    int
	FailedTestNames []string // TODO: drop this and favor TestResults going forward, it has caused bugs.
	TestResults     []RawJobRunTestResult
	Failed          bool
	Errored         bool
	Succeeded       bool
	Aborted         bool

	// InstallStatus can be "", "Success", "Failure"
	// Used to create synthetic tests.
	InstallStatus       string
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
	OverallResult JobOverallResult

	// Timestamp
	Timestamp int
}

type OperatorState struct {
	Name string
	// OperatorState can be "", "Success", "Failure"
	State string
}

// TestStatus corresponds to the values used by TestGrid, for historical reasons.
type TestStatus int

const (
	TestStatusAbsent  TestStatus = 0
	TestStatusSuccess TestStatus = 1
	TestStatusRunning TestStatus = 4
	TestStatusFailure TestStatus = 12
	TestStatusFlake   TestStatus = 13
)
