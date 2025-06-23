package componentreport

import (
	"math/big"
	"time"

	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/db/models"
)

type Release struct {
	Release string
	End     *time.Time
	Start   *time.Time
}

//nolint:revive

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
	crtest.Identification
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
	crtest.Identification
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
	crtest.Stats
}

type TestDetailsJobStats struct {
	// one of sample/base job name could be missing if jobs change between releases
	SampleJobName     string                   `json:"sample_job_name,omitempty"`
	BaseJobName       string                   `json:"base_job_name,omitempty"`
	SampleStats       crtest.Stats             `json:"sample_stats"`
	BaseStats         crtest.Stats             `json:"base_stats"`
	SampleJobRunStats []TestDetailsJobRunStats `json:"sample_job_run_stats,omitempty"`
	BaseJobRunStats   []TestDetailsJobRunStats `json:"base_job_run_stats,omitempty"`
	Significant       bool                     `json:"significant"`
}

type TestDetailsJobRunStats struct {
	JobURL    string         `json:"job_url"`
	JobRunID  string         `json:"job_run_id"`
	StartTime civil.DateTime `json:"start_time"`
	// TestStats is the test stats from one particular job run.
	// For the majority of the tests, there is only one junit. But
	// there are cases multiple junits are generated for the same test.
	TestStats crtest.Stats `json:"test_stats"`
}

type ReportResponse []ReportRow
