// this package is used to produce a reporting structure for rendering html pages.
// it also contains intermediate types used in the processing pipeline.
package v1

import (
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
)

// TestReport is a type that lives in service of producing the html rendering for sippy.
type TestReport struct {
	// release is the logical name used to identify the dashboard display name.  When using openshift, this corresponds
	// to actual releases like 4.6, 4.7, etc.  For non-openshift, it corresponds to --dashboard=<display-name>; the display-name
	// is the value of release.
	Release   string    `json:"release"`
	Timestamp time.Time `json:"timestamp"`

	// TopLevelIndicators is a curated list of metrics, that describe the overall health of the release independent of
	// individual jobs or variants.
	TopLevelIndicators TopLevelIndicators `json:"topLevelIndicators"`

	// ByVariant organizes jobs and tests by variant, sorted by job pass rate from low to high
	ByVariant []VariantResults `json:"byPlatform"`

	// ByTest organizes every test ordered by pass rate from low to high.
	ByTest []FailingTestResult `json:"byTest"`

	FailureGroups []JobRunResult `json:"failureGroups"`

	// ByJob are all the available job results by their job, sorted from low to high pass rate
	ByJob []JobResult `json:"byJob"`
	// FrequentJobResults are jobresults for jobs that run more than 1.5 times per day
	FrequentJobResults []JobResult `json:"frequentJobResults"`
	// InfrequentJobResults are jobresults for jobs that run less than 1.5 times per day
	InfrequentJobResults []JobResult `json:"infrequentJobResults"`

	// TopFailingTestsWithBug holds the top 50 failing tests that have bugs, sorted from low to high pass rate
	TopFailingTestsWithBug []FailingTestResult `json:"topFailingTestsWithBug"`
	// TopFailingTestsWithoutBug holds the top 50 failing tests that do not have bugs, sorted from low to high pass rate
	TopFailingTestsWithoutBug []FailingTestResult `json:"topFailingTestsWithoutBug"`
	// CuratedTests holds a list of tests that have been identified as, "needs watching".  sorted from low to high pass rate
	CuratedTests []FailingTestResult `json:"curatedTests"`

	// BugsByFailureCount lists the bugs by the most frequently failed
	// TODO add information about which FailingTestResult they reference and provide expansion links to tests and jobs
	BugsByFailureCount []bugsv1.Bug `json:"bugsByFailureCount"`

	// JobFailuresByBugzillaComponent are keyed by bugzilla components
	JobFailuresByBugzillaComponent map[string]SortedBugzillaComponentResult `json:"jobFailuresByBugzillaComponent"`

	// AnalysisWarnings is a free-form list of warnings to be displayed on sippy test reports
	AnalysisWarnings []string `json:"analysisWarnings"`
}

// TopLevelIndicators is a curated list of metrics, that describe the overall health of the release independent of
// individual jobs or variants.
type TopLevelIndicators struct {
	// Infrastructure goal is indicate when we fail before we start to install.  Because of other issue, this is slightly
	// broader, catching cases where we are not able to contact a kube-apiserver after the test run.  In theory, this
	// could include some cases of bootstrapping failure.  In practice, it doesn't appear to very often.
	// Low Infrastructure pass rates means we are doing a bad job of keeping the CI system itself up and running
	Infrastructure FailingTestResult
	// Install indicates how successful we are with installing onto clusters that have started
	// Low Install pass rates mean we are doing a bad job installing our product, often due to operators.  This should
	// limit new feature development, but in a pinch we could ship with low install rates.
	Install FailingTestResult
	// Upgrade indicates how successful we are with upgrading onto clusters that have already installed.
	// Low Upgrade pass rates means clusters are not able to upgrade.  This should stop us from shipping the product.
	Upgrade FailingTestResult
	// FinalOperatorState indicates how often test runs finish with every operator healthy
	FinalOperatorHealth FailingTestResult
	// Variants contains a metric for overall health. Success is the variant pass rate over 80%, flake over 60%, fail under
	// that. This excludes never-stable.
	Variant VariantHealth
}

// VariantHealth is used to report overall health of variants.
type VariantHealth struct {
	Success  int `json:"success"`
	Unstable int `json:"unstable"`
	Failed   int `json:"failed"`
}

// VariantResults
type VariantResults struct {
	VariantName                                       string  `json:"platformName"`
	JobRunSuccesses                                   int     `json:"jobRunSuccesses"`
	JobRunFailures                                    int     `json:"jobRunFailures"`
	JobRunKnownFailures                               int     `json:"jobRunKnownFailures"`
	JobRunInfrastructureFailures                      int     `json:"jobRunInfrastructureFailures"`
	JobRunPassPercentage                              float64 `json:"jobRunPassPercentage"`
	JobRunPassPercentageWithKnownFailures             float64 `json:"jobRunPassPercentageWithKnownFailures"`
	JobRunPassPercentageWithoutInfrastructureFailures float64 `json:"jobRunPassPercentageWithoutInfrastructureFailures"`

	// JobResults for all jobs that match this variant, ordered by lowest PassPercentage to highest
	JobResults []JobResult `json:"jobResults"`

	// TestResults holds entries for each test that is a part of this aggregation.  Each entry aggregates the results of all runs of a single test.  The array is sorted from lowest PassPercentage to highest PassPercentage
	AllTestResults []TestResult `json:"results"`
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

type JobRunResult struct {
	Job                string   `json:"job"`
	URL                string   `json:"url"`
	TestFailures       int      `json:"testFailures"`
	FailedTestNames    []string `json:"failedTestNames"`
	Failed             bool     `json:"failed"`
	HasUnknownFailures bool     `json:"hasUnknownFailures"`
	Succeeded          bool     `json:"succeeded"`
}

type JobStatus string

const (
	JobStatusSucceeded             JobStatus = "S"
	JobStatusRunning               JobStatus = "R"
	JobStatusInfrastructureFailure JobStatus = "N"
	JobStatusInstallFailure        JobStatus = "I"
	JobStatusUpgradeFailure        JobStatus = "U"
	JobStatusTestFailure           JobStatus = "F"
	JobStatusNoResults             JobStatus = "n"
	JobStatusUnknown               JobStatus = "f"
)

type BuildResult struct {
	Timestamp int       `json:"timestamp"`
	Result    JobStatus `json:"result"`
	URL       string    `json:"url"`
}

type JobResult struct {
	Name                                        string        `json:"name"`
	Variants                                    []string      `json:"variants"`
	Failures                                    int           `json:"failures"`
	KnownFailures                               int           `json:"knownFailures"`
	InfrastructureFailures                      int           `json:"infrastructureFailures"`
	Successes                                   int           `json:"successes"`
	PassPercentage                              float64       `json:"passPercentage"`
	PassPercentageWithKnownFailures             float64       `json:"passPercentageWithKnownFailures"`
	PassPercentageWithoutInfrastructureFailures float64       `json:"passPercentageWithoutInfrastructureFailures"`
	TestGridURL                                 string        `json:"testGridURL"`
	BuildResults                                []BuildResult `json:"buildResults"`

	BugList []bugsv1.Bug `json:"bugList"`
	// AssociatedBugList are bugs that match the test/job, but do not match the target release
	AssociatedBugList []bugsv1.Bug `json:"associatedBugList"`

	// TestResults holds entries for each test that is a part of this aggregation.  Each entry aggregates the results of all runs of a single test.  The array is sorted from lowest PassPercentage to highest PassPercentage
	TestResults []TestResult `json:"results"`
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
