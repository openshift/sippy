package componentreport

import (
	"math/big"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/util/sets"
)

// PullRequestOptions specifies a specific pull request to use as the
// basis or (more often) sample for the report.
type PullRequestOptions struct {
	Org      string
	Repo     string
	PRNumber string
}

type RequestReleaseOptions struct {
	Release            string              `json:"release" yaml:"release"`
	PullRequestOptions *PullRequestOptions `json:"pull_request_options,omitempty" yaml:"pull_request_options,omitempty"`
	Start              time.Time           `json:"start,omitempty" yaml:"start,omitempty"`
	End                time.Time           `json:"end,omitempty" yaml:"end,omitempty"`
}

// RequestRelativeReleaseOptions is an unfortunate necessity for views where we do not have
// a fixed time, rather a relative time to now/ga. It is translated to the above normal struct before use.
type RequestRelativeReleaseOptions struct {
	Release            string              `json:"release" yaml:"release"`
	PullRequestOptions *PullRequestOptions `json:"pull_request_options,omitempty" yaml:"pull_request_options,omitempty"`
	RelativeStart      string              `json:"start,omitempty" yaml:"start,omitempty"`
	RelativeEnd        string              `json:"end,omitempty" yaml:"end,omitempty"`
}

type RequestTestIdentificationOptions struct {
	Component  string
	Capability string
	// TestID is a unique identification for the test defined in the DB.
	// It matches the test_id in the bigquery ci_analysis_us.junit table.
	TestID string
}

type RequestVariantOptions struct {
	ColumnGroupBy     sets.String         `json:"column_group_by" yaml:"column_group_by"`
	DBGroupBy         sets.String         `json:"db_group_by" yaml:"db_group_by"`
	IncludeVariants   map[string][]string `json:"include_variants" yaml:"include_variants"`
	RequestedVariants map[string]string   `json:"requested_variants" yaml:"requested_variants"`
}

// RequestOptions is a struct packaging all the options for a CR request.
type RequestOptions struct {
	BaseRelease    RequestReleaseOptions
	SampleRelease  RequestReleaseOptions
	TestIDOption   RequestTestIdentificationOptions
	VariantOption  RequestVariantOptions
	AdvancedOption RequestAdvancedOptions
	CacheOption    cache.RequestOptions
}

// View is a server side construct representing a predefined view over the data.
// Useful for defining the primary view of what we deem required for considering the release ready.
type View struct {
	Name            string                        `json:"name" yaml:"name"`
	BaseRelease     RequestRelativeReleaseOptions `json:"base_release" yaml:"base_release"`
	SampleRelease   RequestRelativeReleaseOptions `json:"sample_release" yaml:"sample_release"`
	VariantOptions  RequestVariantOptions         `json:"variant_options" yaml:"variant_options"`
	AdvancedOptions RequestAdvancedOptions        `json:"advanced_options" yaml:"advanced_options"`
}

type RequestAdvancedOptions struct {
	MinimumFailure   int  `json:"minimum_failure" yaml:"minimum_failure"`
	Confidence       int  `json:"confidence" yaml:"confidence"`
	PityFactor       int  `json:"pity_factor" yaml:"pity_factor"`
	IgnoreMissing    bool `json:"ignore_missing" yaml:"ignore_missing"`
	IgnoreDisruption bool `json:"ignore_disruption" yaml:"ignore_disruption"`
}

type TestStatus struct {
	TestName     string   `json:"test_name"`
	TestSuite    string   `json:"test_suite"`
	Component    string   `json:"component"`
	Capabilities []string `json:"capabilities"`
	Variants     []string `json:"variants"`
	TotalCount   int      `json:"total_count"`
	SuccessCount int      `json:"success_count"`
	FlakeCount   int      `json:"flake_count"`
}

type ReportTestStatus struct {
	// BaseStatus represents the stable basis for the comparison. Maps TestIdentification serialized as a string, to test status.
	BaseStatus map[string]TestStatus `json:"base_status"`

	// SampleSatus represents the sample for the comparison. Maps TestIdentification serialized as a string, to test status.
	SampleStatus map[string]TestStatus `json:"sample_status"`
	GeneratedAt  *time.Time            `json:"generated_at"`
}

// TestIdentification TODO: we need to get Network/Upgrade/Arch/Platform/FlatVariants off this struct as the actual variants will be dynamic.
// However making it a map will likely break anything using this struct as a map key.
// We may need to serialize it to a predictable string? Serialize as JSON string perhaps? Will fields be predictably ordered? Seems like go maps are always alphabetical.
type TestIdentification struct {
	TestID string `json:"test_id"`

	// Proposed, need to serialize to use as map key
	Variants map[string]string `json:"variants"`
}

type ComponentReport struct {
	Rows        []ReportRow `json:"rows,omitempty"`
	GeneratedAt *time.Time  `json:"generated_at"`
}

type ReportRow struct {
	RowIdentification
	Columns []ReportColumn `json:"columns,omitempty"`
}

type RowIdentification struct {
	Component  string `json:"component"`
	Capability string `json:"capability,omitempty"`
	TestName   string `json:"test_name,omitempty"`
	TestSuite  string `json:"test_suite,omitempty"`
	TestID     string `json:"test_id,omitempty"`
}

type ReportColumn struct {
	ColumnIdentification
	Status           Status                  `json:"status"`
	RegressedTests   []ReportTestSummary     `json:"regressed_tests,omitempty"`
	TriagedIncidents []TriageIncidentSummary `json:"triaged_incidents,omitempty"`
}

type ColumnID string

type ColumnIdentification struct {
	Variants map[string]string `json:"variants"`
}

type Status int

type ReportTestIdentification struct {
	RowIdentification
	ColumnIdentification
}

type ReportTestSummary struct {
	ReportTestIdentification

	// Opened will be set to the time we first recorded this test went regressed.
	// TODO: This is largely a hack right now, the sippy metrics loop sets this as soon as it notices
	// the regression with it's *default view* query. However we always include it in the response (if that test
	// is regressed per the query params used). Eventually we should only include these details if the default view
	// is being used, without overriding the start/end dates.
	Opened *time.Time `json:"opened"`

	ReportTestStats
}

// ReportTestStats is an overview struct for a particular regressed test's stats.
// (basis passes and pass rate, sample passes and pass rate, and fishers exact confidence)
type ReportTestStats struct {
	// Status is an integer representing the severity of the regression.
	ReportStatus Status                  `json:"status"`
	FisherExact  float64                 `json:"fisher_exact"`
	SampleStats  TestDetailsReleaseStats `json:"sample_stats"`
	BaseStats    TestDetailsReleaseStats `json:"base_stats"`
}

type ReportTestDetails struct {
	ReportTestIdentification
	ReportTestStats
	JiraComponent   string                `json:"jira_component"`
	JiraComponentID *big.Rat              `json:"jira_component_id"`
	JobStats        []TestDetailsJobStats `json:"job_stats,omitempty"`
	GeneratedAt     *time.Time            `json:"generated_at"`
}

type TestDetailsReleaseStats struct {
	Release string `json:"release"`
	TestDetailsTestStats
}

type TestDetailsTestStats struct {
	SuccessRate  float64 `json:"success_rate"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	FlakeCount   int     `json:"flake_count"`
}

type TestDetailsJobStats struct {
	JobName           string                   `json:"job_name"`
	SampleStats       TestDetailsTestStats     `json:"sample_stats"`
	BaseStats         TestDetailsTestStats     `json:"base_stats"`
	SampleJobRunStats []TestDetailsJobRunStats `json:"sample_job_run_stats,omitempty"`
	BaseJobRunStats   []TestDetailsJobRunStats `json:"base_job_run_stats,omitempty"`
	Significant       bool                     `json:"significant"`
}

type TestDetailsJobRunStats struct {
	JobURL string `json:"job_url"`
	// TestStats is the test stats from one particular job run.
	// For the majority of the tests, there is only one junit. But
	// there are cases multiple junits are generated for the same test.
	TestStats TestDetailsTestStats `json:"test_stats"`
}

type JobRunTestStatus struct {
	Component    string
	Capabilities []string
	Network      string
	Upgrade      string
	Arch         string
	Platform     string
	Variants     []string
	TotalCount   int
	SuccessCount int
	FlakeCount   int
}

type JobRunTestIdentification struct {
	TestName string
	TestID   string
	FilePath string
}

type JobRunTestStatusRow struct {
	ProwJob         string   `bigquery:"prowjob_name"`
	TestID          string   `bigquery:"test_id"`
	TestName        string   `bigquery:"test_name"`
	FilePath        string   `bigquery:"file_path"`
	TotalCount      int      `bigquery:"total_count"`
	SuccessCount    int      `bigquery:"success_count"`
	FlakeCount      int      `bigquery:"flake_count"`
	JiraComponent   string   `bigquery:"jira_component"`
	JiraComponentID *big.Rat `bigquery:"jira_component_id"`
}

type JobRunTestReportStatus struct {
	BaseStatus   map[string][]JobRunTestStatusRow `json:"base_status"`
	SampleStatus map[string][]JobRunTestStatusRow `json:"sample_status"`
	GeneratedAt  *time.Time                       `json:"generated_at"`
}

const (
	// ExtremeRegression shows regression with >15% pass rate change
	ExtremeRegression Status = -5
	// SignificantRegression shows significant regression
	SignificantRegression Status = -4
	// ExtremeTriagedRegression shows an ExtremeRegression that clears when Triaged incidents are factored in
	ExtremeTriagedRegression Status = -3
	// SignificantTriagedRegression shows a SignificantRegression that clears when Triaged incidents are factored in
	SignificantTriagedRegression Status = -2
	// MissingSample indicates sample data missing
	MissingSample Status = -1
	// NotSignificant indicates no significant difference
	NotSignificant Status = 0
	// MissingBasis indicates basis data missing
	MissingBasis Status = 1
	// MissingBasisAndSample indicates basis and sample data missing
	MissingBasisAndSample Status = 2
	// SignificantImprovement indicates improved sample rate
	SignificantImprovement Status = 3
)

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

// JobVariants contains all variants supported in the system.
type JobVariants struct {
	Variants map[string][]string `json:"variants,omitempty"`
}

type TriageIncidentSummary struct {
	ReportTestSummary
	TriagedIncidents []TriagedIncident `json:"incidents"`
}

// Variant is currently only used with TriagedIncidents
type Variant struct {
	Key   string `bigquery:"key" json:"key"`
	Value string `bigquery:"value" json:"value"`
}

// TestRegression is used for rows in the test_regressions table and is used to track when we detect test
// regressions opening and closing.
type TestRegression struct {
	Release      string                 `bigquery:"release" json:"release"`
	TestID       string                 `bigquery:"test_id" json:"test_id"`
	TestName     string                 `bigquery:"test_name" json:"test_name"`
	RegressionID string                 `bigquery:"regression_id" json:"regression_id"`
	Opened       time.Time              `bigquery:"opened" json:"opened"`
	Closed       bigquery.NullTimestamp `bigquery:"closed" json:"closed"`
	Variants     []Variant              `bigquery:"variants" json:"variants"`
}

type TriagedIncident struct {
	Release string `bigquery:"release" json:"release"`
	TestID  string `bigquery:"test_id" json:"test_id"`
	// TODO: should this be joined in instead of recording? test_name can change for a given test_id
	TestName        string                       `bigquery:"test_name" json:"test_name"`
	IncidentID      string                       `bigquery:"incident_id" json:"incident_id"`
	IncidentGroupID string                       `bigquery:"incident_group_id" json:"incident_group_id"`
	ModifiedTime    time.Time                    `bigquery:"modified_time" json:"modified_time"`
	Variants        []Variant                    `bigquery:"variants" json:"variants"`
	Issue           TriagedIncidentIssue         `bigquery:"issue" json:"issue"`
	JobRuns         []TriageJobRun               `bigquery:"job_runs" json:"job_runs"`
	Attributions    []TriagedIncidentAttribution `bigquery:"attributions" json:"attributions"`
}

type TriagedIncidentIssue struct {
	Type           string                 `bigquery:"type" json:"type"`
	Description    bigquery.NullString    `bigquery:"description" json:"description"`
	URL            bigquery.NullString    `bigquery:"url" json:"url"`
	StartDate      time.Time              `bigquery:"start_date" json:"start_date"`
	ResolutionDate bigquery.NullTimestamp `bigquery:"resolution_date" json:"resolution_date"`
}

type TriagedIncidentAttribution struct {
	ID         string    `bigquery:"id" json:"id"`
	UpdateTime time.Time `bigquery:"update_time" json:"update_time"`
}

type TriageJobRun struct {
	URL           string                 `bigquery:"url" json:"url"`
	StartTime     time.Time              `bigquery:"start_time" json:"start_time"`
	CompletedTime bigquery.NullTimestamp `bigquery:"completed_time" json:"completed_time"`
}