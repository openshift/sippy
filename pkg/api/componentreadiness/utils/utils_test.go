package utils

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTestDetailsURL(t *testing.T) {
	// Define releases with GA dates for all tests
	ga419 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ga420 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	releases := []v1.Release{
		{
			Release: "4.19",
			GADate:  &ga419,
		},
		{
			Release: "4.20",
			GADate:  &ga420,
		},
	}

	// Define a common view to use across all tests
	testView := crview.View{
		Name: "test-view",
		BaseRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.19",
			},
			RelativeStart: "ga-30d",
			RelativeEnd:   "ga",
		},
		SampleRelease: reqopts.RelativeRelease{
			Release: reqopts.Release{
				Name: "4.20",
			},
			RelativeStart: "ga-7d",
			RelativeEnd:   "ga",
		},
		AdvancedOptions: reqopts.Advanced{
			MinimumFailure:              3,
			Confidence:                  95,
			PityFactor:                  5,
			IncludeMultiReleaseAnalysis: true,
		},
	}

	// Helper function to get release options from view
	getBaseReleaseOpts := func() reqopts.Release {
		opts, err := GetViewReleaseOptions(releases, "basis", testView.BaseRelease, 0)
		require.NoError(t, err)
		return opts
	}
	getSampleReleaseOpts := func() reqopts.Release {
		opts, err := GetViewReleaseOptions(releases, "sample", testView.SampleRelease, 0)
		require.NoError(t, err)
		return opts
	}

	t.Run("empty base URL", func(t *testing.T) {
		url, err := GenerateTestDetailsURL(
			"test-id",
			"",
			getBaseReleaseOpts(),
			getSampleReleaseOpts(),
			testView.AdvancedOptions,
			testView.VariantOptions,
			"",
			"",
			[]string{"Platform:aws"},
			"",
		)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(url, "/api/component_readiness/test_details"))
	})

	t.Run("empty test ID", func(t *testing.T) {
		_, err := GenerateTestDetailsURL(
			"",
			"https://example.com",
			getBaseReleaseOpts(),
			getSampleReleaseOpts(),
			testView.AdvancedOptions,
			testView.VariantOptions,
			"",
			"",
			[]string{},
			"",
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "testID cannot be empty")
	})

	t.Run("variants with malformed entries", func(t *testing.T) {
		url, err := GenerateTestDetailsURL(
			"test-id",
			"",
			getBaseReleaseOpts(),
			getSampleReleaseOpts(),
			testView.AdvancedOptions,
			testView.VariantOptions,
			"",
			"",
			[]string{"Architecture:amd64", "InvalidVariant", "Platform:aws"},
			"",
		)
		require.NoError(t, err)

		// Should still work, just ignoring malformed variants
		assert.Contains(t, url, "Architecture=amd64")
		assert.Contains(t, url, "Platform=aws")
		assert.NotContains(t, url, "InvalidVariant")
	})

	t.Run("environment parameter is sorted", func(t *testing.T) {
		url, err := GenerateTestDetailsURL(
			"test-id",
			"",
			getBaseReleaseOpts(),
			getSampleReleaseOpts(),
			testView.AdvancedOptions,
			testView.VariantOptions,
			"",
			"",
			// Use variants in non-alphabetical order to test sorting
			[]string{"Topology:ha", "Architecture:amd64", "Platform:aws", "Network:ovn"},
			"",
		)
		require.NoError(t, err)

		// Environment should be sorted alphabetically regardless of input order
		assert.Contains(t, url, "environment=Architecture%3Aamd64+Network%3Aovn+Platform%3Aaws+Topology%3Aha")
	})

	t.Run("URL generation with view", func(t *testing.T) {
		url, err := GenerateTestDetailsURL(
			"openshift-tests:abc123",
			"https://sippy.example.com",
			getBaseReleaseOpts(),
			getSampleReleaseOpts(),
			testView.AdvancedOptions,
			testView.VariantOptions,
			"component-example",
			"capability-example",
			[]string{"Architecture:amd64", "Platform:aws"},
			"",
		)
		require.NoError(t, err)
		assert.NotEmpty(t, url)

		// Should use view configuration
		assert.Contains(t, url, "https://sippy.example.com/api/component_readiness/test_details")
		assert.Contains(t, url, "testId=openshift-tests%3Aabc123")
		assert.Contains(t, url, "baseRelease=4.19")
		assert.Contains(t, url, "sampleRelease=4.20")
		assert.Contains(t, url, "confidence=95")
		assert.Contains(t, url, "minFail=3")
		assert.Contains(t, url, "pity=5")
		assert.Contains(t, url, "includeMultiReleaseAnalysis=true")
		assert.Contains(t, url, "Architecture=amd64")
		assert.Contains(t, url, "Platform=aws")
		assert.Contains(t, url, "component=component-example")
		assert.Contains(t, url, "capability=capability-example")
		assert.NotContains(t, url, "testBasisRelease")
	})

	t.Run("URL generation with release fallback", func(t *testing.T) {
		url, err := GenerateTestDetailsURL(
			"openshift-tests:abc123",
			"https://sippy.example.com",
			getBaseReleaseOpts(),
			getSampleReleaseOpts(),
			testView.AdvancedOptions,
			testView.VariantOptions,
			"",
			"",
			[]string{"Architecture:amd64", "Platform:aws"},
			"4.17", // Different from view's base release
		)
		require.NoError(t, err)
		assert.NotEmpty(t, url)

		assert.Contains(t, url, "https://sippy.example.com/api/component_readiness/test_details")
		assert.Contains(t, url, "testId=openshift-tests%3Aabc123")
		assert.Contains(t, url, "baseRelease=4.19")
		assert.Contains(t, url, "sampleRelease=4.20")
		assert.Contains(t, url, "testBasisRelease=4.17")
		assert.Contains(t, url, "confidence=95")
		assert.Contains(t, url, "minFail=3")
		assert.Contains(t, url, "pity=5")
		assert.Contains(t, url, "includeMultiReleaseAnalysis=true")
		assert.Contains(t, url, "Architecture=amd64")
		assert.Contains(t, url, "Platform=aws")
	})

	t.Run("URL generation with real-world regression data", func(t *testing.T) {
		// Add a more comprehensive view for this test
		realWorldView := crview.View{
			Name: "4.20-main",
			BaseRelease: reqopts.RelativeRelease{
				Release: reqopts.Release{
					Name: "4.19",
				},
				RelativeStart: "ga-30d",
				RelativeEnd:   "ga",
			},
			SampleRelease: reqopts.RelativeRelease{
				Release: reqopts.Release{
					Name: "4.20",
				},
				RelativeStart: "ga-7d",
				RelativeEnd:   "ga",
			},
			AdvancedOptions: reqopts.Advanced{
				MinimumFailure:              3,
				Confidence:                  95,
				PityFactor:                  5,
				IncludeMultiReleaseAnalysis: true,
				PassRateRequiredNewTests:    95,
				PassRateRequiredAllTests:    0,
				FlakeAsFailure:              false,
				IgnoreDisruption:            true,
				IgnoreMissing:               false,
			},
		}

		// Get release options from the real-world view
		baseReleaseOpts, err := GetViewReleaseOptions(releases, "basis", realWorldView.BaseRelease, time.Hour)
		require.NoError(t, err)
		sampleReleaseOpts, err := GetViewReleaseOptions(releases, "sample", realWorldView.SampleRelease, time.Hour)
		require.NoError(t, err)

		url, err := GenerateTestDetailsURL(
			"openshift-tests:9f3fb60052539c29ab66564689f616ce",
			"https://sippy-auth.dptools.openshift.org",
			baseReleaseOpts,
			sampleReleaseOpts,
			realWorldView.AdvancedOptions,
			realWorldView.VariantOptions,
			"",
			"",
			[]string{
				"Installer:ipi",
				"Network:ovn",
				"Platform:vsphere",
				"Suite:serial",
				"Topology:ha",
				"Upgrade:none",
				"Architecture:amd64",
				"FeatureSet:default",
			},
			"4.18",
		)
		require.NoError(t, err)
		assert.NotEmpty(t, url)
		fmt.Println(url)

		// Verify the URL contains the expected components
		assert.Contains(t, url, "https://sippy-auth.dptools.openshift.org/api/component_readiness/test_details")

		// Test identification parameters
		assert.Contains(t, url, "testId=openshift-tests%3A9f3fb60052539c29ab66564689f616ce")
		assert.Contains(t, url, "baseRelease=4.19")
		assert.Contains(t, url, "sampleRelease=4.20")
		assert.Contains(t, url, "testBasisRelease=4.18")
		assert.Contains(t, url, "confidence=95")
		assert.Contains(t, url, "minFail=3")
		assert.Contains(t, url, "pity=5")
		assert.Contains(t, url, "includeMultiReleaseAnalysis=true")
		assert.Contains(t, url, "passRateNewTests=95")
		assert.Contains(t, url, "passRateAllTests=0")
		assert.Contains(t, url, "ignoreDisruption=true")
		assert.Contains(t, url, "flakeAsFailure=false")

		// Verify all variants are included in the environment parameter
		assert.Contains(t, url, "environment=Architecture%3Aamd64+FeatureSet%3Adefault+Installer%3Aipi+Network%3Aovn+Platform%3Avsphere+Suite%3Aserial+Topology%3Aha+Upgrade%3Anone")

		// Verify individual variant parameters
		assert.Contains(t, url, "Architecture=amd64")
		assert.Contains(t, url, "FeatureSet=default")
		assert.Contains(t, url, "Installer=ipi")
		assert.Contains(t, url, "Network=ovn")
		assert.Contains(t, url, "Platform=vsphere")
		assert.Contains(t, url, "Suite=serial")
		assert.Contains(t, url, "Topology=ha")
		assert.Contains(t, url, "Upgrade=none")
	})

	t.Run("includeVariant parameters are sorted", func(t *testing.T) {
		// Create a view with includeVariants in non-alphabetical order
		viewWithVariants := crview.View{
			Name: "test-view-with-variants",
			BaseRelease: reqopts.RelativeRelease{
				Release: reqopts.Release{
					Name: "4.19",
				},
				RelativeStart: "ga-30d",
				RelativeEnd:   "ga",
			},
			SampleRelease: reqopts.RelativeRelease{
				Release: reqopts.Release{
					Name: "4.20",
				},
				RelativeStart: "ga-7d",
				RelativeEnd:   "ga",
			},
			VariantOptions: reqopts.Variants{
				IncludeVariants: map[string][]string{
					// Keys and values in non-alphabetical order to test sorting
					"Platform":     {"gcp", "aws", "azure"},
					"Architecture": {"amd64"},
					"Network":      {"ovn", "sdn"},
				},
			},
			AdvancedOptions: reqopts.Advanced{
				MinimumFailure:              3,
				Confidence:                  95,
				PityFactor:                  5,
				IncludeMultiReleaseAnalysis: true,
			},
		}

		// Get release options from the view with variants
		baseReleaseOpts, err := GetViewReleaseOptions(releases, "basis", viewWithVariants.BaseRelease, 0)
		require.NoError(t, err)
		sampleReleaseOpts, err := GetViewReleaseOptions(releases, "sample", viewWithVariants.SampleRelease, 0)
		require.NoError(t, err)

		url, err := GenerateTestDetailsURL(
			"test-id",
			"https://example.com",
			baseReleaseOpts,
			sampleReleaseOpts,
			viewWithVariants.AdvancedOptions,
			viewWithVariants.VariantOptions,
			"",
			"",
			[]string{},
			"",
		)
		require.NoError(t, err)

		// Verify that includeVariant parameters are sorted by key and value
		// Architecture should come first (alphabetically), then Network, then Platform
		// Within Platform, values should be sorted: aws, azure, gcp
		assert.Contains(t, url, "includeVariant=Architecture%3Aamd64")
		assert.Contains(t, url, "includeVariant=Network%3Aovn")
		assert.Contains(t, url, "includeVariant=Network%3Asdn")
		assert.Contains(t, url, "includeVariant=Platform%3Aaws")
		assert.Contains(t, url, "includeVariant=Platform%3Aazure")
		assert.Contains(t, url, "includeVariant=Platform%3Agcp")

		// Check that the order is correct by looking at the raw query
		// The URL encoding makes this a bit tricky, but we can check the pattern
		assert.Regexp(t, `includeVariant=Architecture.*includeVariant=Network.*includeVariant=Platform`, url)
	})

	t.Run("variant cross-compare parameters are included", func(t *testing.T) {
		// Create a view with variant cross-compare settings
		viewWithCrossCompare := crview.View{
			Name: "test-view-with-cross-compare",
			BaseRelease: reqopts.RelativeRelease{
				Release: reqopts.Release{
					Name: "4.19",
				},
				RelativeStart: "ga-30d",
				RelativeEnd:   "ga",
			},
			SampleRelease: reqopts.RelativeRelease{
				Release: reqopts.Release{
					Name: "4.20",
				},
				RelativeStart: "ga-7d",
				RelativeEnd:   "ga",
			},
			VariantOptions: reqopts.Variants{
				IncludeVariants: map[string][]string{
					"Architecture": {"amd64"},
					"Platform":     {"aws"},
					"Topology":     {"ha"},
				},
				VariantCrossCompare: []string{"Architecture", "Topology"},
				CompareVariants: map[string][]string{
					"Architecture": {"s390x", "ppc64le"},
					"Topology":     {"single"},
				},
			},
			AdvancedOptions: reqopts.Advanced{
				MinimumFailure: 3,
				Confidence:     95,
				PityFactor:     5,
			},
		}

		baseReleaseOpts, err := GetViewReleaseOptions(releases, "basis", viewWithCrossCompare.BaseRelease, 0)
		require.NoError(t, err)
		sampleReleaseOpts, err := GetViewReleaseOptions(releases, "sample", viewWithCrossCompare.SampleRelease, 0)
		require.NoError(t, err)

		url, err := GenerateTestDetailsURL(
			"test-id-123",
			"https://example.com",
			baseReleaseOpts,
			sampleReleaseOpts,
			viewWithCrossCompare.AdvancedOptions,
			viewWithCrossCompare.VariantOptions,
			"",
			"",
			[]string{"Platform:aws"},
			"",
		)
		require.NoError(t, err)

		// Verify includeVariant parameters are present
		assert.Contains(t, url, "includeVariant=Architecture%3Aamd64")
		assert.Contains(t, url, "includeVariant=Platform%3Aaws")
		assert.Contains(t, url, "includeVariant=Topology%3Aha")

		// Verify variantCrossCompare parameters are present and sorted
		assert.Contains(t, url, "variantCrossCompare=Architecture")
		assert.Contains(t, url, "variantCrossCompare=Topology")

		// Verify compareVariant parameters are present and sorted
		assert.Contains(t, url, "compareVariant=Architecture%3Appc64le")
		assert.Contains(t, url, "compareVariant=Architecture%3As390x")
		assert.Contains(t, url, "compareVariant=Topology%3Asingle")

		// Verify the order is correct (Architecture before Topology)
		assert.Regexp(t, `variantCrossCompare=Architecture.*variantCrossCompare=Topology`, url)
		assert.Regexp(t, `compareVariant=Architecture.*compareVariant=Topology`, url)
	})

	t.Run("URL generation with PR options", func(t *testing.T) {
		// Create a sample release with PR options
		sampleReleaseWithPR := reqopts.Release{
			Name:  "4.20",
			Start: time.Date(2025, 5, 25, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			PullRequestOptions: &reqopts.PullRequest{
				Org:      "openshift",
				Repo:     "origin",
				PRNumber: "12345",
			},
		}

		url, err := GenerateTestDetailsURL(
			"test-id",
			"https://sippy.example.com",
			getBaseReleaseOpts(),
			sampleReleaseWithPR,
			testView.AdvancedOptions,
			testView.VariantOptions,
			"",
			"",
			[]string{"Platform:aws"},
			"",
		)
		require.NoError(t, err)
		assert.NotEmpty(t, url)

		// Verify PR parameters are included
		assert.Contains(t, url, "samplePROrg=openshift")
		assert.Contains(t, url, "samplePRRepo=origin")
		assert.Contains(t, url, "samplePRNumber=12345")
	})

	t.Run("URL generation with Payload options", func(t *testing.T) {
		// Create a sample release with Payload options
		sampleReleaseWithPayload := reqopts.Release{
			Name:  "4.20",
			Start: time.Date(2025, 5, 25, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			PayloadOptions: &reqopts.Payload{
				Tags: []string{"tag1", "tag2"},
			},
		}

		url, err := GenerateTestDetailsURL(
			"test-id",
			"https://sippy.example.com",
			getBaseReleaseOpts(),
			sampleReleaseWithPayload,
			testView.AdvancedOptions,
			testView.VariantOptions,
			"",
			"",
			[]string{"Platform:aws"},
			"",
		)
		require.NoError(t, err)
		assert.NotEmpty(t, url)

		// Verify Payload parameters are included
		assert.Contains(t, url, "samplePayloadTag=tag1")
		assert.Contains(t, url, "samplePayloadTag=tag2")
	})

}
