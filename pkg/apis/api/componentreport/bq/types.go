package bq

import (
	"math/big"
	"time"

	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
)

// The bq package contains types filled in from BigQuery data or populated with other such types.
// Although many have json tags for serialization, these are for caching, not intended to be used directly in API responses.

// ReportTestStatus contains the mapping of all test keys (serialized with KeyWithVariants, variants + testID)
// It is also an internal type used to pass data from bigquery onwards to report generation, and does not get serialized
// as an API response.
type ReportTestStatus struct {
	// BaseStatus represents the stable basis for the comparison. Maps KeyWithVariants serialized as a string, to test status.
	BaseStatus map[string]TestStatus `json:"base_status"`

	// SampleSatus represents the sample for the comparison. Maps KeyWithVariants serialized as a string, to test status.
	SampleStatus map[string]TestStatus `json:"sample_status"`
	GeneratedAt  *time.Time            `json:"generated_at"`
}

// TestStatus is an internal type used to pass data bigquery onwards to the actual
// report generation. It is not serialized over the API.
type TestStatus struct {
	TestName     string   `json:"test_name"`
	TestSuite    string   `json:"test_suite"`
	Component    string   `json:"component"`
	Capabilities []string `json:"capabilities"`
	Variants     []string `json:"variants"`
	crtest.Count
	LastFailure time.Time `json:"last_failure"`
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

// TestJobRunRows are the per job run rows that come back from bigquery for a test details report
// indicating if the test passed or failed.
type TestJobRunRows struct {
	TestKey      crtest.KeyWithVariants `json:"test_key"`
	TestKeyStr   string                 `json:"-"` // transient field so we dont have to keep recalculating
	TestName     string                 `bigquery:"test_name"`
	ProwJob      string                 `bigquery:"prowjob_name"`
	ProwJobRunID string                 `bigquery:"prowjob_run_id"`
	ProwJobURL   string                 `bigquery:"prowjob_url"`
	StartTime    civil.DateTime         `bigquery:"prowjob_start"`
	crtest.Count
	JiraComponent   string   `bigquery:"jira_component"`
	JiraComponentID *big.Rat `bigquery:"jira_component_id"`
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
