// this package is used to produce a reporting structure for rendering html pages.
// it also contains intermediate types used in the processing pipeline.
package v1

import (
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
)

// TestReport is a type that lives in service of producing the html rendering for sippy.
type TestReport struct {
	Release    string                                `json:"release"`
	All        map[string]SortedAggregateTestsResult `json:"all"`
	ByPlatform map[string]SortedAggregateTestsResult `json:"byPlatform`
	ByJob      map[string]SortedAggregateTestsResult `json:"byJob`
	BySig      map[string]SortedAggregateTestsResult `json:"bySig`

	FailureGroups []JobRunResult `json:"failureGroups"`

	// JobResults are jobresults for jobs that run more than 1.5 times per day
	JobResults []JobResult `json:"jobResults"`
	// InfrequentJobResults are jobresults for jobs that run less than 1.5 times per day
	InfrequentJobResults []JobResult `json:"infrequentJobResults"`

	Timestamp                 time.Time     `json:"timestamp"`
	TopFailingTestsWithBug    []*TestResult `json:"topFailingTestsWithBug"`
	TopFailingTestsWithoutBug []*TestResult `json:"topFailingTestsWithoutBug"`
	BugsByFailureCount        []bugsv1.Bug  `json:"bugsByFailureCount"`

	// JobFailuresByBugzillaComponent are keyed by bugzilla components
	JobFailuresByBugzillaComponent map[string]SortedBugzillaComponentResult `json:"jobFailuresByBugzillaComponent"`

	// AnalysisWarnings is a free-form list of warnings to be displayed on sippy test reports
	AnalysisWarnings []string `json:"analysisWarnings"`
}

// SortedAggregateTestsResult
type SortedAggregateTestsResult struct {
	Successes          int     `json:"successes"`
	Failures           int     `json:"failures"`
	TestPassPercentage float64 `json:"testPassPercentage"`
	// TestResults holds entries for each test that is a part of this aggregation.  Each entry aggregates the results of all runs of a single test.  The array is sorted from lowest PassPercentage to highest PassPercentage
	TestResults []TestResult `json:"results"`
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
	// TODO Inside a platform, only bugs matching the platform are present.
	BugList []bugsv1.Bug `json:"bugList"`
	// TODO fix search link to properly take into account release, job, and platform.
	SearchLink string `json:"searchLink"`
}

type JobRunResult struct {
	Job                string   `json:"job"`
	Url                string   `json:"url"`
	TestFailures       int      `json:"testFailures"`
	FailedTestNames    []string `json:"failedTestNames"`
	Failed             bool     `json:"failed"`
	HasUnknownFailures bool     `json:"hasUnknownFailures"`
	Succeeded          bool     `json:"succeeded"`
}

type JobResult struct {
	Name                            string  `json:"name"`
	Platform                        string  `json:"platform"`
	KnownFailures                   int     `json:"knownFailures"`
	PassPercentageWithKnownFailures float64 `json:"passPercentageWithKnownFailures"`
	TestGridURL                     string  `json:"TestGridURL"`

	SortedAggregateTestsResult `json:",inline"`
}

type SortedBugzillaComponentResult struct {
	Name string `json:"name"`

	JobsFailed []BugzillaJobResult `json:"jobsFailed"`
}

// BugzillaJobResult is a summary of bugzilla component/job tuple.
type BugzillaJobResult struct {
	JobName           string `json:"jobName"`
	BugzillaComponent string `json:"bugzillaComponent"`

	// NumberOfJobRunsFailed is the number of job runs that had failures caused by this bugzilla component
	NumberOfJobRunsFailed int `json:"numberOfJobRunsFailed"`
	// This one is phrased as a failure percentage because we don't know a success percentage since we don't know how many times it actually ran
	// we only know how many times its tests failed and how often the job ran.  This is more useful for some types of analysis anyway: "how often
	// does a sig cause a job to fail".
	FailPercentage float64 `json:"failPercentage"`
	// TotalRuns is the number of runs this Job has run total.
	TotalRuns int `json:"totalRuns"`

	// Failures are a full list of the failures caused by this BZ component in the given job.
	Failures []TestResult `json:"failures"`
}
