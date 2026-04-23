package v1

type SippyConfig struct {
	Prow                     ProwConfig               `yaml:"prow"`
	Releases                 map[string]ReleaseConfig `yaml:"releases"`
	ComponentReadinessConfig ComponentReadinessConfig `yaml:"componentReadiness"`
}

type ProwConfig struct {
	// URL to the prowjob.js endpoint of the prow instance. This endpoint contains
	// a JSON file with all the ProwJob resources from the prow cluster.
	URL string `yaml:"url"`
}

type ReleaseConfig struct {
	// Jobs is a set of jobs that should be considered part of the release.
	Jobs map[string]bool `yaml:"jobs,omitempty"`

	// Regexp is a list of regular expressions that match a job to a release.
	Regexp []string `yaml:"regexp,omitempty"`

	// BlockingJobs is the list of blocking payload jobs
	BlockingJobs []string `yaml:"blockingJobs,omitempty"`

	// InformingJobs is the list of informing payload jobs
	InformingJobs []string `yaml:"informingJobs,omitempty"`

	// Overview configures the release overview page display behavior.
	Overview *OverviewConfig `yaml:"overview,omitempty"`
}

// OverviewConfig controls how the release overview page is rendered.
type OverviewConfig struct {
	// MultiVersionInstallTests queries both old-style synthetic and new-style
	// install test names, keeping whichever has more data. Useful for releases
	// that span multiple OCP versions.
	MultiVersionInstallTests bool `yaml:"multiVersionInstallTests,omitempty" json:"multi_version_install_tests,omitempty"`

	// RecentFailuresPeriod overrides the default "current" window for new test
	// failure detection (default: "24h").
	RecentFailuresPeriod string `yaml:"recentFailuresPeriod,omitempty" json:"recent_failures_period,omitempty"`

	// RecentFailuresPreviousPeriod overrides the default "previous" window for
	// new test failure comparison (default: "72h").
	RecentFailuresPreviousPeriod string `yaml:"recentFailuresPreviousPeriod,omitempty" json:"recent_failures_previous_period,omitempty"`

	// TopFailingTestsPeriod, when set, adds a "Top Failing Tests" section
	// showing all test failures within this window (e.g. "168h").
	TopFailingTestsPeriod string `yaml:"topFailingTestsPeriod,omitempty" json:"top_failing_tests_period,omitempty"`
}

type ComponentReadinessConfig struct {
	// VariantJunitTableOverrides allow pulling results from additional
	VariantJunitTableOverrides []VariantJunitTableOverride `yaml:"variantJunitTableOverrides,omitempty"`
}

// VariantJunitTableOverride is used to pull in junit results from a different table, if the given variant
// is included in your query. (i.e. rarely run jobs support)
type VariantJunitTableOverride struct {
	VariantName  string `yaml:"variantName"`
	VariantValue string `yaml:"variantValue"`
	TableName    string `yaml:"tableName"`
	// RelativeStart is used to allow the rarely run functionality to ignore the report start date, and instead use
	// a much longer one for rarely run jobs. In practice, it will be based off the report end date.
	// As with views, this is specified as a string of the form end-90d.
	RelativeStart string `yaml:"relativeStart,omitempty"`
}
