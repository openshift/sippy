package variantregistry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/flags/configflags"
)

// TestQuay318RegexpLookup verifies that the quay-3.18 synthetic release matches
// the renamed periodics from openshift/release PR 85067 by regexp, not just the
// stale exact names, across both the manually maintained customizations overlay
// and the generated config it's hand-mirrored into.
func TestQuay318RegexpLookup(t *testing.T) {
	tests := []struct {
		jobName         string
		expectedRelease string
		expectedMatch   bool
	}{
		{
			jobName:         "periodic-ci-quay-quay-redhat-3.18-aws-ocp422-e2e-install-aws-s3-3-18-nightly-4-22",
			expectedRelease: "quay-3.18",
			expectedMatch:   true,
		},
		{
			jobName:         "periodic-ci-quay-quay-redhat-3.18-gcp-ocp422-e2e-install-gcp-gcs-3-18-nightly-4-22",
			expectedRelease: "quay-3.18",
			expectedMatch:   true,
		},
		{
			jobName:         "periodic-ci-quay-quay-redhat-3.18-aws-ocp422-e2e-install-aws-s3",
			expectedRelease: "quay-3.18",
			expectedMatch:   true,
		},
		{
			jobName:         "periodic-ci-quay-quay-redhat-3.18-aws-ocp422-e2e-install-aws-s3-nightly",
			expectedRelease: "quay-3.18",
			expectedMatch:   true,
		},
		{
			jobName:         "periodic-ci-quay-quay-redhat-3.18-aws-ocp422-e2e-install-aws-s3-stable",
			expectedRelease: "quay-3.18",
			expectedMatch:   true,
		},
		{
			jobName:         "periodic-ci-quay-quay-redhat-3.18-gcp-ocp422-e2e-install-gcp-gcs-nightly",
			expectedRelease: "quay-3.18",
			expectedMatch:   true,
		},
		{
			jobName:       "periodic-ci-quay-quay-tests-master-ocp-4.22-quay-lpGA-lp-ocp-compat-quay-e2e-tests-quay316-ocp422-aws-s3",
			expectedMatch: false,
		},
		{
			jobName:       "pull-ci-quay-quay-master-aws-s3",
			expectedMatch: false,
		},
		{
			jobName:       "periodic-ci-openshift-release-master-nightly-4.22-e2e-aws-ovn",
			expectedMatch: false,
		},
	}

	for _, configPath := range []string{
		"../../config/openshift-customizations.yaml",
		"../../config/openshift.yaml",
	} {
		t.Run(configPath, func(t *testing.T) {
			cfgFlags := &configflags.ConfigFlags{Path: configPath}
			cfg, err := cfgFlags.GetConfig()
			require.NoError(t, err)

			overrides, err := BuildSyntheticReleaseJobOverrides(cfg.Releases)
			require.NoError(t, err)

			for _, tt := range tests {
				t.Run(tt.jobName, func(t *testing.T) {
					release, ok := overrides.Lookup(tt.jobName)
					assert.Equal(t, tt.expectedMatch, ok)
					if tt.expectedMatch {
						assert.Equal(t, tt.expectedRelease, release)
					}
				})
			}
		})
	}
}
