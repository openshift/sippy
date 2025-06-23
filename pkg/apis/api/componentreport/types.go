package componentreport

import (
	"math/big"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/db/models"
)

type Release struct {
	Release string
	End     *time.Time
	Start   *time.Time
}

type ReleaseTestMap struct {
	Release
	Tests map[string]TestStatus
}

type FallbackReleases struct {
	Releases map[string]ReleaseTestMap
}

//nolint:revive

// TestStatus is an internal type used to pass data bigquery onwards to the actual
// report generation. It is not serialized over the API.
type TestStatus struct {
	TestName     string   `json:"test_name"`
	TestSuite    string   `json:"test_suite"`
	Component    string   `json:"component"`
	Capabilities []string `json:"capabilities"`
	Variants     []string `json:"variants"`
	crtest.TestCount
	LastFailure time.Time `json:"last_failure"`
}

func (ts TestStatus) GetTotalSuccessFailFlakeCounts() (int, int, int, int) {
	failures := ts.Failures()
	return ts.TotalCount, ts.SuccessCount, failures, ts.FlakeCount
}

// ReportTestStatus contains the mapping of all test keys (serialized with TestWithVariantsKey, variants + testID)
// It is also an internal type used to pass data from bigquery onwards to report generation, and does not get serialized
// as an API response.
type ReportTestStatus struct {
	// BaseStatus represents the stable basis for the comparison. Maps TestWithVariantsKey serialized as a string, to test status.
	BaseStatus map[string]TestStatus `json:"base_status"`

	// SampleSatus represents the sample for the comparison. Maps TestWithVariantsKey serialized as a string, to test status.
	SampleStatus map[string]TestStatus `json:"sample_status"`
	GeneratedAt  *time.Time            `json:"generated_at"`
}

type ComponentReport struct {
	Rows        []ReportRow `json:"rows,omitempty"`
	GeneratedAt *time.Time  `json:"generated_at"`
}

type ReportRow struct {
	crtest.RowIdentification
	Columns []ReportColumn `json:"columns,omitempty"`
}

type ReportColumn struct {
	crtest.ColumnIdentification
	Status         crtest.Status       `json:"status"`
	RegressedTests []ReportTestSummary `json:"regressed_tests,omitempty"`
}

type ReportTestSummary struct {
	// TODO: really feels like this could just be moved  ReportTestStats, eliminating the need for ReportTestSummary
	crtest.ReportTestIdentification
	ReportTestStats
}

// ReportTestStats is an overview struct for a particular regressed test's stats.
// (basis passes and pass rate, sample passes and pass rate, and fishers exact confidence)
// Important type returned by the API.
// TODO: compare with TestStatus we use internally, see if we can converge?
type ReportTestStats struct {
	// ReportStatus is an integer representing the severity of the regression.
	ReportStatus crtest.Status `json:"status"`

	// Comparison indicates what mode was used to check this tests results in the sample.
	Comparison crtest.Comparison `json:"comparison"`

	// Explanations are human-readable details of why this test was marked regressed.
	Explanations []string `json:"explanations"`

	SampleStats TestDetailsReleaseStats `json:"sample_stats"`

	// RequiredConfidence is the confidence required from Fishers to consider a regression.
	// Typically, it is as defined in the request options, but middleware may choose to adjust.
	// 95 = 95% confidence of a regression required.
	RequiredConfidence int `json:"-"`

	// PityAdjustment can be used to adjust the tolerance for failures for this particular test.
	PityAdjustment float64 `json:"-"`

	// RequiredPassRateAdjustment can be used to adjust the tolerance for failures for a new test.
	RequiredPassRateAdjustment float64 `json:"-"`

	// Optional fields depending on the Comparison mode

	// FisherExact indicates the confidence of a regression after applying Fisher's Exact Test.
	FisherExact *float64 `json:"fisher_exact,omitempty"`

	// BaseStats may not be present in the response, i.e. new tests regressed because of their pass rate.
	BaseStats *TestDetailsReleaseStats `json:"base_stats,omitempty"`

	// LastFailure is the last time the regressed test failed.
	LastFailure *time.Time `json:"last_failure"`

	// Regression is populated with data on when we first detected this regression. If unset it implies
	// the regression tracker has not yet run to find it, or you're using report params/a view without regression tracking.
	Regression *models.TestRegression `json:"regression,omitempty"`
}

// TestDetailsAnalysis is a collection of stats for the report which could potentially carry
// multiple different analyses run.
type TestDetailsAnalysis struct {
	ReportTestStats
	JobStats []TestDetailsJobStats `json:"job_stats,omitempty"`
}

// ReportTestDetails is the top level API response for test details reports.
type ReportTestDetails struct {
	crtest.ReportTestIdentification
	JiraComponent   string     `json:"jira_component"`
	JiraComponentID *big.Rat   `json:"jira_component_id"`
	TestName        string     `json:"test_name"`
	GeneratedAt     *time.Time `json:"generated_at"`

	// Analyses is a list of potentially multiple analysis run for this test.
	// Callers can assume that the first in the list is somewhat authoritative, and should
	// be displayed by default, but each analysis offers details and explanations on it's outcome
	// and can be used in some capacity.
	Analyses []TestDetailsAnalysis `json:"analyses"`
}

type TestDetailsReleaseStats struct {
	Release string `json:"release"`
	Start   *time.Time
	End     *time.Time
	crtest.TestDetailsTestStats
}

type TestDetailsJobStats struct {
	// one of sample/base job name could be missing if jobs change between releases
	SampleJobName     string                      `json:"sample_job_name,omitempty"`
	BaseJobName       string                      `json:"base_job_name,omitempty"`
	SampleStats       crtest.TestDetailsTestStats `json:"sample_stats"`
	BaseStats         crtest.TestDetailsTestStats `json:"base_stats"`
	SampleJobRunStats []TestDetailsJobRunStats    `json:"sample_job_run_stats,omitempty"`
	BaseJobRunStats   []TestDetailsJobRunStats    `json:"base_job_run_stats,omitempty"`
	Significant       bool                        `json:"significant"`
}

type TestDetailsJobRunStats struct {
	JobURL    string         `json:"job_url"`
	JobRunID  string         `json:"job_run_id"`
	StartTime civil.DateTime `json:"start_time"`
	// TestStats is the test stats from one particular job run.
	// For the majority of the tests, there is only one junit. But
	// there are cases multiple junits are generated for the same test.
	TestStats crtest.TestDetailsTestStats `json:"test_stats"`
}

// TestJobRunRows are the per job run rows that come back from bigquery for a test details report
// indicating if the test passed or failed.
// Fields are named count somewhat misleadingly as technically they're always 0 or 1 today.
type TestJobRunRows struct {
	TestKey      crtest.TestWithVariantsKey `json:"test_key"`
	TestKeyStr   string                     `json:"-"` // transient field so we dont have to keep recalculating
	TestName     string                     `bigquery:"test_name"`
	ProwJob      string                     `bigquery:"prowjob_name"`
	ProwJobRunID string                     `bigquery:"prowjob_run_id"`
	ProwJobURL   string                     `bigquery:"prowjob_url"`
	StartTime    civil.DateTime             `bigquery:"prowjob_start"`
	crtest.TestCount
	JiraComponent   string   `bigquery:"jira_component"`
	JiraComponentID *big.Rat `bigquery:"jira_component_id"`
}

// TestJobRunStatuses contains the rows returned from a test details query organized by base and sample,
// essentially the actual job runs and their status that was used to calculate this
// report.
// Status fields map prowjob name to each row result we received for that job.
type TestJobRunStatuses struct {
	BaseStatus map[string][]TestJobRunRows `json:"base_status"`
	// TODO: This could be a little cleaner if we did status.BaseStatuses plural and tied them to a release,
	// allowing the release fallback mechanism to stay a little cleaner. That would more clearly
	// keep middleware details out of the main codebase.
	BaseOverrideStatus map[string][]TestJobRunRows `json:"base_override_status"`
	SampleStatus       map[string][]TestJobRunRows `json:"sample_status"`
	GeneratedAt        *time.Time                  `json:"generated_at"`
}

type ReportResponse []ReportRow

type TestVariants struct {
	Network  []string `json:"network,omitempty"`
	Upgrade  []string `json:"upgrade,omitempty"`
	Arch     []string `json:"arch,omitempty"`
	Platform []string `json:"platform,omitempty"`
	Variant  []string `json:"variant,omitempty"`
}

// JobVariant defines a variant and the possible values
type JobVariant struct {
	VariantName   string   `bigquery:"variant_name"`
	VariantValues []string `bigquery:"variant_values"`
}

type Variant struct {
	Key   string `bigquery:"key" json:"key"`
	Value string `bigquery:"value" json:"value"`
}

// TODO: temporary for migration
type TestRegressionBigQuery struct {
	// Snapshot is the time at which the full set of regressions for all releases was inserted into the db.
	// When querying we use only those with the latest snapshot time.
	Snapshot     time.Time              `bigquery:"snapshot" json:"snapshot"`
	View         string                 `bigquery:"view" json:"view"`
	Release      string                 `bigquery:"release" json:"release"`
	TestID       string                 `bigquery:"test_id" json:"test_id"`
	TestName     string                 `bigquery:"test_name" json:"test_name"`
	RegressionID string                 `bigquery:"regression_id" json:"regression_id"`
	Opened       time.Time              `bigquery:"opened" json:"opened"`
	Closed       bigquery.NullTimestamp `bigquery:"closed" json:"closed"`
	Variants     []Variant              `bigquery:"variants" json:"variants"`
}
