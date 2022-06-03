// this package is used to produce a reporting structure for rendering html pages.
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
	JobNoResults             JobOverallResult = "n"
	JobUnknown               JobOverallResult = "f"
)

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
