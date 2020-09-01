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

	FailureGroups             []JobRunResult `json:"failureGroups"`
	JobPassRate               []JobResult    `json:"jobPassRate"`
	Timestamp                 time.Time      `json:"timestamp"`
	TopFailingTestsWithBug    []*TestResult  `json:"topFailingTestsWithBug"`
	TopFailingTestsWithoutBug []*TestResult  `json:"topFailingTestsWithoutBug"`
	BugsByFailureCount        []bugsv1.Bug   `json:"bugsByFailureCount"`
}

// SortedAggregateTestsResult
type SortedAggregateTestsResult struct {
	Successes          int          `json:"successes"`
	Failures           int          `json:"failures"`
	TestPassPercentage float64      `json:"testPassPercentage"`
	TestResults        []TestResult `json:"results"`
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
	TestGridJobUrl     string   `json:"testGridJobUrl"`
	TestFailures       int      `json:"testFailures"`
	FailedTestNames    []string `json:"failedTestNames"`
	Failed             bool     `json:"failed"`
	HasUnknownFailures bool     `json:"hasUnknownFailures"`
	Succeeded          bool     `json:"succeeded"`
}

type JobResult struct {
	Name                            string  `json:"name"`
	Platform                        string  `json:"platform"`
	Failures                        int     `json:"failures"`
	KnownFailures                   int     `json:"knownFailures"`
	Successes                       int     `json:"successes"`
	PassPercentage                  float64 `json:"PassPercentage"`
	PassPercentageWithKnownFailures float64 `json:"PassPercentageWithKnownFailures"`
	TestGridUrl                     string  `json:"TestGridUrl"`
}
