package reqopts

import (
	"time"

	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/util/sets"
)

// These component report types respresent options that the user has requested,
// which need to be serialized as part of caching the report results.

// RequestOptions is a struct packaging all the options for a CR request.
// BaseOverrideRelease is the counterpart to RequestAdvancedOptions.IncludeMultiReleaseAnalysis
// When multi release analysis is enabled we 'fallback' to the release that has the highest
// threshold for indicating a regression.  If a release prior to the selected BaseRelease has a
// higher standard it will be set as the BaseOverrideRelease to be included in the TestDetails analysis
type RequestOptions struct {
	BaseRelease    RequestReleaseOptions
	SampleRelease  RequestReleaseOptions
	VariantOption  RequestVariantOptions
	AdvancedOption RequestAdvancedOptions
	CacheOption    cache.RequestOptions
	// TODO: phase our once multi TestIDOptions is fully implemented
	TestIDOptions []RequestTestIdentificationOptions
}

// PullRequestOptions specifies a specific pull request to use as the
// basis or (more often) sample for the report.
type PullRequestOptions struct {
	Org      string
	Repo     string
	PRNumber string
}

// PayloadOptions specifies a specific payload tag to use as the
// sample for the report. This is only used for sample, not basis.
type PayloadOptions struct {
	Tag string
}

type RequestReleaseOptions struct {
	Release            string              `json:"release" yaml:"release"`
	PullRequestOptions *PullRequestOptions `json:"pull_request_options,omitempty" yaml:"pull_request_options,omitempty"`
	PayloadOptions     *PayloadOptions     `json:"payload_options,omitempty" yaml:"payload_options,omitempty"`
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
	RequestReleaseOptions `json:",inline" yaml:",inline"` //nolint:revive
	// inline is a known option
	RelativeStart string `json:"relative_start,omitempty" yaml:"relative_start,omitempty"`
	RelativeEnd   string `json:"relative_end,omitempty" yaml:"relative_end,omitempty"`
}

// RequestTestIdentificationOptions handles options used in the test details report when we focus in
// on a specific test and variants combo, typically because it is or was regressed.
type RequestTestIdentificationOptions struct {
	Component  string `json:"component,omitempty" yaml:"component,omitempty"`
	Capability string `json:"capability,omitempty" yaml:"capability,omitempty"`
	// TestID is a unique identification for the test defined in the DB.
	// It matches the test_id in the bigquery ci_analysis_us.junit table.
	TestID string `json:"test_id,omitempty" yaml:"test_id,omitempty"`
	// RequestedVariants are used for filtering the test details view down to a specific set.
	RequestedVariants map[string]string `json:"requested_variants,omitempty" yaml:"requested_variants,omitempty"`
	// BaseOverrideRelease is used when we're requesting a test details report for both the base release, and a fallback override that had a better pass rate.
	BaseOverrideRelease string `json:"base_override_release,omitempty" yaml:"base_override_release,omitempty"`
}

func AnyAreBaseOverrides(opts []RequestTestIdentificationOptions) bool {
	for _, tid := range opts {
		if tid.BaseOverrideRelease != "" {
			return true
		}
	}
	return false
}

type RequestVariantOptions struct {
	ColumnGroupBy       sets.String         `json:"column_group_by" yaml:"column_group_by"`
	DBGroupBy           sets.String         `json:"db_group_by" yaml:"db_group_by"`
	IncludeVariants     map[string][]string `json:"include_variants" yaml:"include_variants"`
	CompareVariants     map[string][]string `json:"compare_variants,omitempty" yaml:"compare_variants,omitempty"`
	VariantCrossCompare []string            `json:"variant_cross_compare,omitempty" yaml:"variant_cross_compare,omitempty"`
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
