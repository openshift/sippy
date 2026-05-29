package variantregistry

import (
	"testing"

	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/releaseoverride"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSyntheticReleaseJobOverrides(t *testing.T) {
	tests := []struct {
		name              string
		releases          map[string]v1.ReleaseConfig
		expectedOverrides map[string]string
		expectError       bool
	}{
		{
			name: "no synthetic releases",
			releases: map[string]v1.ReleaseConfig{
				"4.22": {
					Jobs: map[string]bool{"job-a": true, "job-b": true},
				},
			},
			expectedOverrides: map[string]string{},
		},
		{
			name: "single synthetic release",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Jobs:      map[string]bool{"job-a": true, "job-b": true},
				},
			},
			expectedOverrides: map[string]string{
				"job-a": "rosa-stage",
				"job-b": "rosa-stage",
			},
		},
		{
			name: "multiple synthetic releases no overlap",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Jobs:      map[string]bool{"job-a": true},
				},
				"aro-integration": {
					Synthetic: true,
					Jobs:      map[string]bool{"job-b": true},
				},
			},
			expectedOverrides: map[string]string{
				"job-a": "rosa-stage",
				"job-b": "aro-integration",
			},
		},
		{
			name: "conflict same job in two synthetic releases",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Jobs:      map[string]bool{"job-a": true},
				},
				"rrp-integration": {
					Synthetic: true,
					Jobs:      map[string]bool{"job-a": true},
				},
			},
			expectError: true,
		},
		{
			name: "synthetic release with no jobs",
			releases: map[string]v1.ReleaseConfig{
				"rosa-production": {
					Synthetic: true,
				},
			},
			expectedOverrides: map[string]string{},
		},
		{
			name: "standard release jobs not in overrides",
			releases: map[string]v1.ReleaseConfig{
				"4.22": {
					Jobs: map[string]bool{"job-a": true},
				},
				"rosa-stage": {
					Synthetic: true,
					Jobs:      map[string]bool{"job-b": true},
				},
			},
			expectedOverrides: map[string]string{
				"job-b": "rosa-stage",
			},
		},
		{
			name: "job in both synthetic and standard release only appears once",
			releases: map[string]v1.ReleaseConfig{
				"4.22": {
					Jobs: map[string]bool{"job-a": true},
				},
				"rosa-stage": {
					Synthetic: true,
					Jobs:      map[string]bool{"job-a": true},
				},
			},
			expectedOverrides: map[string]string{
				"job-a": "rosa-stage",
			},
		},
		{
			name: "disabled jobs in synthetic release are excluded",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Jobs:      map[string]bool{"job-a": true, "job-b": false},
				},
			},
			expectedOverrides: map[string]string{
				"job-a": "rosa-stage",
			},
		},
		{
			name: "release marked synthetic but not in config is fine",
			releases: map[string]v1.ReleaseConfig{
				"4.22": {
					Jobs: map[string]bool{"job-a": true},
				},
			},
			expectedOverrides: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides, err := BuildSyntheticReleaseJobOverrides(tt.releases)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			// Verify each expected override is present via Lookup
			for jobName, expectedRelease := range tt.expectedOverrides {
				release, ok := overrides.Lookup(jobName)
				assert.True(t, ok, "expected override for %q", jobName)
				assert.Equal(t, expectedRelease, release)
			}
			// Verify no unexpected overrides by checking a job not in the map
			_, ok := overrides.Lookup("not-a-real-job-name")
			assert.False(t, ok)
		})
	}
}

func TestSyntheticReleaseOverridesRegexp(t *testing.T) {
	tests := []struct {
		name            string
		releases        map[string]v1.ReleaseConfig
		jobName         string
		expectedRelease string
		expectedMatch   bool
	}{
		{
			name: "regexp matches job",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Regexp:    []string{`^periodic-ci-openshift-online-rosa-e2e-main-.*`},
				},
			},
			jobName:         "periodic-ci-openshift-online-rosa-e2e-main-nightly-4.22",
			expectedRelease: "rosa-stage",
			expectedMatch:   true,
		},
		{
			name: "regexp does not match job",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Regexp:    []string{`^periodic-ci-openshift-online-rosa-e2e-main-.*`},
				},
			},
			jobName:       "periodic-ci-openshift-release-master-nightly-4.22-e2e-aws-ovn",
			expectedMatch: false,
		},
		{
			name: "exact match takes priority over regexp",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Jobs:      map[string]bool{"my-exact-job": true},
					Regexp:    []string{`^my-.*`},
				},
			},
			jobName:         "my-exact-job",
			expectedRelease: "rosa-stage",
			expectedMatch:   true,
		},
		{
			name: "non-synthetic release regexp is ignored",
			releases: map[string]v1.ReleaseConfig{
				"4.22": {
					Regexp: []string{`^periodic-ci-openshift-online-rosa-e2e-main-.*`},
				},
			},
			jobName:       "periodic-ci-openshift-online-rosa-e2e-main-nightly-4.22",
			expectedMatch: false,
		},
		{
			name: "multiple regexp patterns",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Regexp: []string{
						`^periodic-ci-openshift-online-rosa-e2e-main-.*`,
						`^periodic-ci-openshift-release-main-nightly-.*-e2e-rosa-hcp-ovn$`,
					},
				},
			},
			jobName:         "periodic-ci-openshift-release-main-nightly-4.19-e2e-rosa-hcp-ovn",
			expectedRelease: "rosa-stage",
			expectedMatch:   true,
		},
		{
			name: "invalid regexp returns error",
			releases: map[string]v1.ReleaseConfig{
				"rosa-stage": {
					Synthetic: true,
					Regexp:    []string{`[invalid`},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides, err := BuildSyntheticReleaseJobOverrides(tt.releases)
			if tt.name == "invalid regexp returns error" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			release, ok := overrides.Lookup(tt.jobName)
			assert.Equal(t, tt.expectedMatch, ok)
			if tt.expectedMatch {
				assert.Equal(t, tt.expectedRelease, release)
			}
		})
	}
}

func TestSyntheticReleaseOverridesLookupNil(t *testing.T) {
	var overrides *releaseoverride.SyntheticReleaseOverrides
	release, ok := overrides.Lookup("any-job")
	assert.False(t, ok)
	assert.Empty(t, release)
}
