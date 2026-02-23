package featuregateloader

import (
	"testing"
)

func TestParseFeatureGateFilename(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		wantTopo    string
		wantFeatSet string
		wantValid   bool
	}{
		{
			name:        "old format Hypershift Default",
			filename:    "featureGate-Hypershift-Default.yaml",
			wantTopo:    "Hypershift",
			wantFeatSet: "Default",
			wantValid:   true,
		},
		{
			name:        "old format SelfManagedHA TechPreviewNoUpgrade",
			filename:    "featureGate-SelfManagedHA-TechPreviewNoUpgrade.yaml",
			wantTopo:    "SelfManagedHA",
			wantFeatSet: "TechPreviewNoUpgrade",
			wantValid:   true,
		},
		{
			name:        "versioned format Hypershift Default",
			filename:    "featureGate-4-10-Hypershift-Default.yaml",
			wantTopo:    "Hypershift",
			wantFeatSet: "Default",
			wantValid:   true,
		},
		{
			name:        "versioned format SelfManagedHA DevPreviewNoUpgrade",
			filename:    "featureGate-4-10-SelfManagedHA-DevPreviewNoUpgrade.yaml",
			wantTopo:    "SelfManagedHA",
			wantFeatSet: "DevPreviewNoUpgrade",
			wantValid:   true,
		},
		{
			name:        "versioned format Hypershift OKD",
			filename:    "featureGate-4-10-Hypershift-OKD.yaml",
			wantTopo:    "Hypershift",
			wantFeatSet: "OKD",
			wantValid:   true,
		},
		{
			name:      "too few parts",
			filename:  "featureGate-Default.yaml",
			wantValid: false,
		},
		{
			name:      "just prefix",
			filename:  "featureGate.yaml",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTopo, gotFeatSet, gotValid := parseFeatureGateFilename(tt.filename)
			if gotValid != tt.wantValid {
				t.Fatalf("valid = %v, want %v", gotValid, tt.wantValid)
			}
			if !gotValid {
				return
			}
			if gotTopo != tt.wantTopo {
				t.Errorf("topology = %q, want %q", gotTopo, tt.wantTopo)
			}
			if gotFeatSet != tt.wantFeatSet {
				t.Errorf("featureSet = %q, want %q", gotFeatSet, tt.wantFeatSet)
			}
		})
	}
}
