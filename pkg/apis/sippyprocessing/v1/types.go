// this package is used to produce a reporting structure for rendering html pages.
// it also contains intermediate types used in the processing pipeline.
package v1

import (
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
)

// TestReport is a type that lives in service of producing the html rendering for sippy.
type TestReport struct {
	Release                   string                               `json:"release"`
	All                       map[string]SortedAggregateTestResult `json:"all"`
	ByPlatform                map[string]SortedAggregateTestResult `json:"byPlatform`
	ByJob                     map[string]SortedAggregateTestResult `json:"byJob`
	BySig                     map[string]SortedAggregateTestResult `json:"bySig`
	FailureGroups             []JobRunResult                       `json:"failureGroups"`
	JobPassRate               []JobResult                          `json:"jobPassRate"`
	Timestamp                 time.Time                            `json:"timestamp"`
	TopFailingTestsWithBug    []*TestResult                        `json:"topFailingTestsWithBug"`
	TopFailingTestsWithoutBug []*TestResult                        `json:"topFailingTestsWithoutBug"`
	BugsByFailureCount        []bugsv1.Bug                         `json:"bugsByFailureCount"`
}

type SortedAggregateTestResult struct {
	Successes          int          `json:"successes"`
	Failures           int          `json:"failures"`
	TestPassPercentage float64      `json:"testPassPercentage"`
	TestResults        []TestResult `json:"results"`
}

type AggregateTestResult struct {
	Successes          int                   `json:"successes"`
	Failures           int                   `json:"failures"`
	TestPassPercentage float64               `json:"testPassPercentage"`
	TestResults        map[string]TestResult `json:"results"`
}

type TestResult struct {
	Name           string       `json:"name"`
	Successes      int          `json:"successes"`
	Failures       int          `json:"failures"`
	Flakes         int          `json:"flakes"`
	PassPercentage float64      `json:"passPercentage"`
	BugList        []bugsv1.Bug `json:"BugList"`
	SearchLink     string       `json:"searchLink"`
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

	// SetupStatus can be "", "Success", "Failure"
	SetupStatus      string          `json:"setupStatus"`
	InstallOperators []OperatorState `json:"installOperators"`
	UpgradeOperators []OperatorState `json:"upgradeOperators"`
}

const (
	InfrastructureTestName = `[sig-sippy] infrastructure should work`
	InstallTestName        = `[sig-sippy] install should work`
	UpgradeTestName        = `[sig-sippy] upgrade should work`

	Success = "Success"
	Failure = "Failure"
)

type OperatorState struct {
	Name string `json:"name"`
	// OperatorState can be "", "Success", "Failure"
	State string `json:"state"`
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
