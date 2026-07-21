package featuregatepromotion

// PromotionStatus represents the overall promotion readiness of a feature gate.
type PromotionStatus struct {
	FeatureGate      string            `json:"feature_gate"`
	Release          string            `json:"release"`
	Sufficient       bool              `json:"sufficient"`
	ResultsByVariant []VariantResult   `json:"results_by_variant"`
	Warnings         []string          `json:"warnings"`
	Errors           []string          `json:"errors"`
	Links            map[string]string `json:"links,omitempty"`
}

// VariantResult represents the promotion readiness for a single variant combination.
type VariantResult struct {
	Variants    map[string]string `json:"variants"`
	Optional    bool              `json:"optional"`
	Sufficient  bool              `json:"sufficient"`
	TestResults []TestResult      `json:"test_results"`
	Warnings    []string          `json:"warnings,omitempty"`
	Errors      []string          `json:"errors,omitempty"`
}

// TestResult represents the test run statistics for a single test on a single variant.
type TestResult struct {
	TestName       string            `json:"test_name"`
	TotalRuns      int               `json:"total_runs"`
	SuccessfulRuns int               `json:"successful_runs"`
	FailedRuns     int               `json:"failed_runs"`
	FlakedRuns     int               `json:"flaked_runs"`
	PassPercent    float32           `json:"pass_percent"`
	Sufficient     bool              `json:"sufficient"`
	Links          map[string]string `json:"links,omitempty"`
}

// JobVariant defines a required combination of platform variants to check for promotion.
type JobVariant struct {
	Cloud        string
	Architecture string
	Topology     string
	NetworkStack string
	OS           string
	JobTiers     string
	Optional     bool
}

// testQueryRow represents a single row returned by the promotion readiness query.
type testQueryRow struct {
	TestName          string
	Platform          string
	Architecture      string
	Topology          string
	NetworkStack      string
	OS                string
	JobTier           string
	CurrentRuns       int
	CurrentSuccesses  int
	CurrentFailures   int
	CurrentFlakes     int
	PreviousRuns      int
	PreviousSuccesses int
	PreviousFailures  int
	PreviousFlakes    int
}
