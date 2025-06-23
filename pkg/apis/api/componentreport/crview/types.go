package crview

import "github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"

// View is a server side construct representing a predefined view over the component readiness data.
// Useful for defining the primary view of what we deem required for considering the release ready.
type View struct {
	Name            string                     `json:"name" yaml:"name"`
	BaseRelease     reqopts.RelativeRelease    `json:"base_release" yaml:"base_release"`
	SampleRelease   reqopts.RelativeRelease    `json:"sample_release" yaml:"sample_release"`
	TestIDOption    reqopts.TestIdentification `json:"test_id_options" yaml:"test_id_options"`
	VariantOptions  reqopts.Variants           `json:"variant_options" yaml:"variant_options"`
	AdvancedOptions reqopts.Advanced           `json:"advanced_options" yaml:"advanced_options"`

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
