package componentreport

import (
	"math/big"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/util/sets"
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
//
// When returned in the API, it should include the concrete start/end calculated from relative
// for the point in time when the request was made. This is used in the UI to pre-populate the
// date picks to transition from view based to custom reporting.
type RequestRelativeReleaseOptions struct {
	RequestReleaseOptions `json:",inline" yaml:",inline"` //nolint:revive // inline is a known option
	RelativeStart         string                          `json:"relative_start,omitempty" yaml:"relative_start,omitempty"`
	RelativeEnd           string                          `json:"relative_end,omitempty" yaml:"relative_end,omitempty"`
}

type RequestTestIdentificationOptions struct {
	Component  string
	Capability string
	// TestID is a unique identification for the test defined in the DB.
	// It matches the test_id in the bigquery ci_analysis_us.junit table.
	TestID string
}

type RequestVariantOptions struct {
	ColumnGroupBy       sets.String         `json:"column_group_by" yaml:"column_group_by"`
	DBGroupBy           sets.String         `json:"db_group_by" yaml:"db_group_by"`
	IncludeVariants     map[string][]string `json:"include_variants" yaml:"include_variants"`
	CompareVariants     map[string][]string `json:"compare_variants,omitempty" yaml:"compare_variants,omitempty"`
	VariantCrossCompare []string            `json:"variant_cross_compare,omitempty" yaml:"variant_cross_compare,omitempty"`
	// RequestedVariants are used for filtering the test details view down to a specific set. Unused in the main component report.
	RequestedVariants map[string]string `json:"requested_variants,omitempty" yaml:"requested_variants,omitempty"`
}

// RequestOptions is a struct packaging all the options for a CR request.
// BaseOverrideRelease is the counterpart to RequestAdvancedOptions.IncludeMultiReleaseAnalysis
// When multi release analysis is enabled we 'fallback' to the release that has the highest
// threshold for indicating a regression.  If a release prior to the selected BaseRelease has a
// higher standard it will be set as the BaseOverrideRelease to be included in the TestDetails analysis
type RequestOptions struct {
	BaseRelease         RequestReleaseOptions
	BaseOverrideRelease RequestReleaseOptions
	SampleRelease       RequestReleaseOptions
	TestIDOption        RequestTestIdentificationOptions
	VariantOption       RequestVariantOptions
	AdvancedOption      RequestAdvancedOptions
	CacheOption         cache.RequestOptions
}

// View is a server side construct representing a predefined view over the component readiness data.
// Useful for defining the primary view of what we deem required for considering the release ready.
type View struct {
	Name            string                           `json:"name" yaml:"name"`
	BaseRelease     RequestRelativeReleaseOptions    `json:"base_release" yaml:"base_release"`
	SampleRelease   RequestRelativeReleaseOptions    `json:"sample_release" yaml:"sample_release"`
	TestIDOption    RequestTestIdentificationOptions `json:"test_id_options" yaml:"test_id_options"`
	VariantOptions  RequestVariantOptions            `json:"variant_options" yaml:"variant_options"`
	AdvancedOptions RequestAdvancedOptions           `json:"advanced_options" yaml:"advanced_options"`

	Metrics            ViewMetrics            `json:"metrics" yaml:"metrics"`
	RegressionTracking ViewRegressionTracking `json:"regression_tracking" yaml:"regression_tracking"`
	AutomateJira       AutomateJira           `json:"automate_jira" yaml:"automate_jira"`
}

type ViewMetrics struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type ViewRegressionTracking struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type AutomateJira struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type RequestAdvancedOptions struct {
	MinimumFailure              int  `json:"minimum_failure" yaml:"minimum_failure"`
	Confidence                  int  `json:"confidence" yaml:"confidence"`
	PityFactor                  int  `json:"pity_factor" yaml:"pity_factor"`
	PassRateRequiredNewTests    int  `json:"pass_rate_required_new_tests" yaml:"pass_rate_required_new_tests"`
	PassRateRequiredAllTests    int  `json:"pass_rate_required_all_tests" yaml:"pass_rate_required_all_tests"`
	IgnoreMissing               bool `json:"ignore_missing" yaml:"ignore_missing"`
	IgnoreDisruption            bool `json:"ignore_disruption" yaml:"ignore_disruption"`
	FlakeAsFailure              bool `json:"flake_as_failure" yaml:"flake_as_failure"`
	IncludeMultiReleaseAnalysis bool `json:"include_multi_release_analysis" yaml:"include_multi_release_analysis"`
}

// TestStatus is an internal type used to pass data bigquery onwards to the actual
// report generation. It is not serialized over the API.
type TestStatus struct {
	TestName     string    `json:"test_name"`
	TestSuite    string    `json:"test_suite"`
	Component    string    `json:"component"`
	Capabilities []string  `json:"capabilities"`
	Variants     []string  `json:"variants"`
	TotalCount   int       `json:"total_count"`
	SuccessCount int       `json:"success_count"`
	FlakeCount   int       `json:"flake_count"`
	LastFailure  time.Time `json:"last_failure"`
	// Release provides info on the release this test status was pulled from for base TestStatus.
	// If nil, assume the base/sample release from the request options. (used for ReleaseFallback)
	Release *Release `json:"release"`
}

func (ts TestStatus) GetTotalSuccessFailFlakeCounts() (int, int, int, int) {
	failures := ts.TotalCount - ts.SuccessCount - ts.FlakeCount
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
	Opened       *time.Time `json:"opened"`
	RegressionID int        `json:"regression_id"`

	// Links contains REST links for clients to follow for this specific triage. Most notably "self".
	// These are injected by the API and not stored in the DB.
	Links map[string]string `json:"links"`

	ReportTestStats
}

// Comparison is the type of comparison done for a test that has been marked red.
type Comparison string

const (
	PassRate    Comparison = "pass_rate"
	FisherExact Comparison = "fisher_exact"
)

// ReportTestStats is an overview struct for a particular regressed test's stats.
// (basis passes and pass rate, sample passes and pass rate, and fishers exact confidence)
// Important type returned by the API.
// TODO: compare with TestStatus we use internally, see if we can converge?
type ReportTestStats struct {
	// ReportStatus is an integer representing the severity of the regression.
	ReportStatus Status `json:"status"`

	// Comparison indicates what mode was used to check this tests results in the sample.
	Comparison Comparison `json:"comparison"`

	// Explanations are human-readable details of why this test was marked regressed.
	Explanations []string `json:"explanations"`

	SampleStats TestDetailsReleaseStats `json:"sample_stats"`

	// Optional fields depending on the Comparison mode

	// FisherExact indicates the confidence of a regression after applying Fisher's Exact Test.
	FisherExact *float64 `json:"fisher_exact,omitempty"`

	// BaseStats may not be present in the response, i.e. new tests regressed because of their pass rate.
	BaseStats *TestDetailsReleaseStats `json:"base_stats,omitempty"`

	// LastFailure is the last time the regressed test failed.
	LastFailure *time.Time `json:"last_failure"`
}

// IsTriaged returns true if this tests status is within the triaged regression range.
func (r ReportTestStats) IsTriaged() bool {
	return r.ReportStatus < MissingSample && r.ReportStatus > SignificantRegression
}

// TestDetailsAnalysis is a collection of stats for the report which could potentially carry
// multiple different analyses run.
type TestDetailsAnalysis struct {
	ReportTestStats
	JobStats         []TestDetailsJobStats `json:"job_stats,omitempty"`
	TriagedIncidents []TriagedIncident     `json:"incidents,omitempty"`
}

// ReportTestDetails is the top level API response for test details reports.
type ReportTestDetails struct {
	ReportTestIdentification
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
	TestDetailsTestStats
}

type TestDetailsTestStats struct {
	SuccessRate  float64 `json:"success_rate"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	FlakeCount   int     `json:"flake_count"`
}

type TestDetailsJobStats struct {
	// one of sample/base job name could be missing if jobs change between releases
	SampleJobName     string                   `json:"sample_job_name,omitempty"`
	BaseJobName       string                   `json:"base_job_name,omitempty"`
	SampleStats       TestDetailsTestStats     `json:"sample_stats"`
	BaseStats         TestDetailsTestStats     `json:"base_stats"`
	SampleJobRunStats []TestDetailsJobRunStats `json:"sample_job_run_stats,omitempty"`
	BaseJobRunStats   []TestDetailsJobRunStats `json:"base_job_run_stats,omitempty"`
	Significant       bool                     `json:"significant"`
}

type TestDetailsJobRunStats struct {
	JobURL   string `json:"job_url"`
	JobRunID string `json:"job_run_id"`
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
	ProwJobRunID    string   `bigquery:"prowjob_run_id"`
	ProwJobURL      string   `bigquery:"prowjob_url"`
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
	BaseStatus map[string][]JobRunTestStatusRow `json:"base_status"`
	// TODO: This could be a little cleaner if we did status.BaseStatuses plural and tied them to a release,
	// allowing the release fallback mechanism to stay a little cleaner. That would more clearly
	// keep middleware details out of the main codebase.
	BaseOverrideStatus map[string][]JobRunTestStatusRow `json:"base_override_status"`
	SampleStatus       map[string][]JobRunTestStatusRow `json:"sample_status"`
	GeneratedAt        *time.Time                       `json:"generated_at"`
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

func StringForStatus(s Status) string {
	switch s {
	case ExtremeRegression:
		return "Extreme"
	case SignificantRegression:
		return "Significant"
	case ExtremeTriagedRegression:
		return "ExtremeTriaged"
	case SignificantTriagedRegression:
		return "SignificantTriaged"
	case MissingSample:
		return "MissingSample"
	}
	return "Unknown"
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
	URL            string                 `bigquery:"url" json:"url"`
	StartTime      time.Time              `bigquery:"start_time" json:"start_time"`
	CompletionTime bigquery.NullTimestamp `bigquery:"completion_time" json:"completion_time"`
}

// TestWithVariantsKey connects the core unique db testID string to a set of variants.
// Used to serialize/deserialize as a map key when we pass test status around.
type TestWithVariantsKey struct {
	TestID string `json:"test_id"`

	// Proposed, need to serialize to use as map key
	Variants map[string]string `json:"variants"`
}
