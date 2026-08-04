package crstatus

import (
	"math/big"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
)

// Package crstatus contains data-transfer types used between the data layer and
// the Component Readiness analysis pipeline. These types were originally in the
// bq package but are backend-agnostic — any data provider (BigQuery, PostgreSQL,
// mock) populates them identically.

// ReportTestStatus contains the mapping of all test keys (serialized with KeyWithVariants, variants + testID)
// It is an internal type used to pass data from the data provider to report generation,
// and does not get serialized as an API response.
type ReportTestStatus struct {
	// BaseStatus represents the stable basis for the comparison. Maps KeyWithVariants serialized as a string, to test status.
	BaseStatus map[string]TestStatus `json:"base_status"`

	// SampleStatus represents the sample for the comparison. Maps KeyWithVariants serialized as a string, to test status.
	SampleStatus map[string]TestStatus `json:"sample_status"`
	GeneratedAt  *time.Time            `json:"generated_at"`
}

// TestStatus is an internal type used to pass data from the data provider to the
// actual report generation. It is not serialized over the API.
type TestStatus struct {
	TestID       string            `json:"test_id"`
	TestName     string            `json:"test_name"`
	TestSuite    string            `json:"test_suite"`
	Component    string            `json:"component"`
	Capabilities []string          `json:"capabilities"`
	Variants     map[string]string `json:"variants"`
	crtest.Count
	LastFailure time.Time `json:"last_failure"`
}

// TestJobRunStatuses contains the rows returned from a test details query organized by base and sample.
// Status fields map prowjob name to per-test summaries for that job.
type TestJobRunStatuses struct {
	BaseStatus         map[string][]TestDetailsSummary `json:"base_status"`
	BaseOverrideStatus map[string][]TestDetailsSummary `json:"base_override_status"`
	SampleStatus       map[string][]TestDetailsSummary `json:"sample_status"`
	GeneratedAt        *time.Time                      `json:"generated_at"`
}

// TestDetailsSummary is a per-test, per-job summary with pre-computed counts
// and optional individual run details.
type TestDetailsSummary struct {
	TestKey         crtest.KeyWithVariants
	TestKeyStr      string
	ProwJob         string
	Stats           crtest.Stats
	JobRuns         []JobRunDetail `json:",omitempty"`
	JiraComponent   string
	JiraComponentID *big.Rat
	TestName        string
	Lifecycle       string
}

// JobRunDetail holds the per-run information for an individual prow job run.
type JobRunDetail struct {
	ProwJobRunID string
	ProwJobURL   string
	StartTime    time.Time
	crtest.Count
	JobLabels    []string `json:",omitempty"`
	JobSymptoms  []string `json:",omitempty"`
	TestFailures int
}

// TestJobRunRows are the per job run rows from a test details report
// indicating if the test passed or failed.
type TestJobRunRows struct {
	TestKey      crtest.KeyWithVariants `json:"test_key"`
	TestKeyStr   string                 `json:"-"` // transient field so we dont have to keep recalculating
	TestName     string                 `bigquery:"test_name"`
	ProwJob      string                 `bigquery:"prowjob_name"`
	ProwJobRunID string                 `bigquery:"prowjob_run_id"`
	ProwJobURL   string                 `bigquery:"prowjob_url"`
	StartTime    time.Time              `bigquery:"prowjob_start"`
	crtest.Count
	JiraComponent   string   `bigquery:"jira_component"`
	JiraComponentID *big.Rat `bigquery:"jira_component_id"`
	JobLabels       []string `bigquery:"-" json:"job_labels,omitempty"`
	JobSymptoms     []string `bigquery:"-" json:"job_symptoms,omitempty"`
	TestFailures    int      `bigquery:"-" json:"test_failures"`
	Lifecycle       string   `bigquery:"lifecycle"`
}

// JobVariant defines a variant and the possible values.
type JobVariant struct {
	VariantName   string   `bigquery:"variant_name"`
	VariantValues []string `bigquery:"variant_values"`
}

// Variant is a single key/value variant pair.
type Variant struct {
	Key   string `bigquery:"key" json:"key"`
	Value string `bigquery:"value" json:"value"`
}
