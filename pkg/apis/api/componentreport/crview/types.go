package crview

import "github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"

// View is a server side construct representing a predefined view over the component readiness data.
// Useful for defining the primary view of what we deem required for considering the release ready.
type View struct {
	Name            string                     `json:"name" yaml:"name"`
	BaseRelease     reqopts.RelativeRelease    `json:"base_release" yaml:"base_release"`
	SampleRelease   reqopts.RelativeRelease    `json:"sample_release" yaml:"sample_release"`
	TestIDOption    reqopts.TestIdentification `json:"test_id_options" yaml:"test_id_options"`
	TestFilters     reqopts.TestFilters        `json:"test_filters" yaml:"test_filters"`
	VariantOptions  reqopts.Variants           `json:"variant_options" yaml:"variant_options"`
	AdvancedOptions reqopts.Advanced           `json:"advanced_options" yaml:"advanced_options"`

	// SpotCheckJobSamples defines sample windows for spot-check job analysis, keyed by tier name
	// (e.g. "spotcheck-30d"). Each entry specifies a time window and variant filters (ANDed)
	// used to select which jobs to query. If empty, spot-check analysis is disabled for this view.
	SpotCheckJobSamples map[string]SpotCheckJobSample `json:"spot_check_job_samples,omitempty" yaml:"spot_check_job_samples,omitempty"`

	Metrics            Metrics            `json:"metrics" yaml:"metrics"`
	RegressionTracking RegressionTracking `json:"regression_tracking" yaml:"regression_tracking"`
	AutomateJira       AutomateJira       `json:"automate_jira" yaml:"automate_jira"`
	PrimeCache         PrimeCache         `json:"prime_cache" yaml:"prime_cache"`
}

type Metrics struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type RegressionTracking struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type PrimeCache struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}
type AutomateJira struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// SpotCheckJobSample defines a spot-check sample window and variant filters for a tier.
// The release is always inherited from SampleRelease.
type SpotCheckJobSample struct {
	RelativeStart   string              `json:"relative_start,omitempty" yaml:"relative_start,omitempty"`
	RelativeEnd     string              `json:"relative_end,omitempty" yaml:"relative_end,omitempty"`
	IncludeVariants map[string][]string `json:"include_variants,omitempty" yaml:"include_variants,omitempty"`
}
