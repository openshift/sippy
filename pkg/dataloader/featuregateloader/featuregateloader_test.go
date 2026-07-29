package featuregateloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{
			name:      "wrong prefix",
			filename:  "someOther-Hypershift-Default.yaml",
			wantValid: false,
		},
		{
			name:      "wrong suffix",
			filename:  "featureGate-Hypershift-Default.json",
			wantValid: false,
		},
		{
			name:      "no extension",
			filename:  "featureGate-something",
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

func TestConvertAPIToDB(t *testing.T) {
	fg := FeatureGate{
		Status: FeatureGateStatus{
			FeatureGates: []FeatureGateEntry{
				{
					Enabled: []Feature{
						{Name: "FeatureA"},
						{Name: "FeatureB"},
					},
					Disabled: []Feature{
						{Name: "FeatureC"},
					},
				},
			},
		},
	}

	result := convertAPIToDB(fg, "4.22", "Hypershift", "Default", "test.yaml")

	require.Len(t, result, 3)

	assert.Equal(t, "4.22", result[0].Release)
	assert.Equal(t, "Hypershift", result[0].Topology)
	assert.Equal(t, "Default", result[0].FeatureSet)
	assert.Equal(t, "FeatureA", result[0].FeatureGate)
	assert.Equal(t, "enabled", result[0].Status)

	assert.Equal(t, "FeatureB", result[1].FeatureGate)
	assert.Equal(t, "enabled", result[1].Status)

	assert.Equal(t, "FeatureC", result[2].FeatureGate)
	assert.Equal(t, "disabled", result[2].Status)
}

func TestConvertAPIToDBEmpty(t *testing.T) {
	fg := FeatureGate{
		Status: FeatureGateStatus{
			FeatureGates: []FeatureGateEntry{},
		},
	}
	result := convertAPIToDB(fg, "5.0", "SelfManagedHA", "TechPreviewNoUpgrade", "test.yaml")
	assert.Empty(t, result)
}
