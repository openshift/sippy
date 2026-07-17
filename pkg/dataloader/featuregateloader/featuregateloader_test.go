package featuregateloader

import (
	"encoding/json"
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

func TestIsAllowedDownloadURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"raw.githubusercontent.com", "https://raw.githubusercontent.com/openshift/api/release-4.22/some/file.yaml", true},
		{"objects.githubusercontent.com", "https://objects.githubusercontent.com/some/blob", true},
		{"http scheme rejected", "http://raw.githubusercontent.com/openshift/api/release-4.22/some/file.yaml", false},
		{"github.com", "https://github.com/openshift/api/blob/main/file.yaml", false},
		{"arbitrary host", "https://evil.example.com/malicious", false},
		{"empty", "", false},
		{"invalid URL", "://not-a-url", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAllowedDownloadURL(tt.url))
		})
	}
}

func TestGitHubContentParsing(t *testing.T) {
	raw := `[
		{
			"name": "featureGate-4-10-Hypershift-Default.yaml",
			"download_url": "https://raw.githubusercontent.com/openshift/api/release-4.22/payload-manifests/featuregates/featureGate-4-10-Hypershift-Default.yaml",
			"type": "file"
		},
		{
			"name": "someDir",
			"download_url": null,
			"type": "dir"
		},
		{
			"name": "README.md",
			"download_url": "https://raw.githubusercontent.com/openshift/api/release-4.22/payload-manifests/featuregates/README.md",
			"type": "file"
		}
	]`

	var entries []githubContent
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	require.Len(t, entries, 3)

	assert.Equal(t, "featureGate-4-10-Hypershift-Default.yaml", entries[0].Name)
	assert.Equal(t, "file", entries[0].Type)
	assert.Contains(t, entries[0].DownloadURL, "raw.githubusercontent.com")

	assert.Equal(t, "dir", entries[1].Type)

	// Only the first entry should pass the filename check
	var featureGateFiles []githubContent
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		if _, _, valid := parseFeatureGateFilename(e.Name); valid {
			featureGateFiles = append(featureGateFiles, e)
		}
	}
	require.Len(t, featureGateFiles, 1)
	assert.Equal(t, "featureGate-4-10-Hypershift-Default.yaml", featureGateFiles[0].Name)
}
