package featuregateloader

type FeatureGate struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Spec       FeatureGateSpec   `yaml:"spec"`
	Status     FeatureGateStatus `yaml:"status"`
}

type FeatureGateSpec struct {
	FeatureSet string `yaml:"featureSet"`
}

type FeatureGateStatus struct {
	FeatureGates []FeatureGateEntry `yaml:"featureGates"`
}

type FeatureGateEntry struct {
	Disabled []Feature `yaml:"disabled"`
	Enabled  []Feature `yaml:"enabled"`
}

type Feature struct {
	Name string `yaml:"name"`
}
