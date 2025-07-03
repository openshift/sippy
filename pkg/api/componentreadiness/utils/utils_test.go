package utils

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db/models"
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
	views := []crview.View{testView}

	t.Run("empty base URL", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:       123,
			View:     "test-view",
			Release:  "4.20",
			TestID:   "test-id",
			Variants: pq.StringArray{"Platform:aws"},
		}

		url, err := GenerateTestDetailsURL(regression, "", views, releases, 0)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(url, "/api/component_readiness/test_details"))
	})

	t.Run("nil regression", func(t *testing.T) {
		_, err := GenerateTestDetailsURL(nil, "https://example.com", views, releases, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regression cannot be nil")
	})

	t.Run("variants with malformed entries", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:       123,
			View:     "test-view",
			Release:  "4.20",
			TestID:   "test-id",
			Variants: pq.StringArray{"Architecture:amd64", "InvalidVariant", "Platform:aws"},
		}

		url, err := GenerateTestDetailsURL(regression, "", views, releases, 0)
		require.NoError(t, err)

		// Should still work, just ignoring malformed variants
		assert.Contains(t, url, "Architecture=amd64")
		assert.Contains(t, url, "Platform=aws")
		assert.NotContains(t, url, "InvalidVariant")
	})

	t.Run("environment parameter is sorted", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:      123,
			View:    "test-view",
			Release: "4.20",
			TestID:  "test-id",
			// Use variants in non-alphabetical order to test sorting
			Variants: pq.StringArray{"Topology:ha", "Architecture:amd64", "Platform:aws", "Network:ovn"},
		}

		url, err := GenerateTestDetailsURL(regression, "", views, releases, 0)
		require.NoError(t, err)

		// Environment should be sorted alphabetically regardless of input order
		assert.Contains(t, url, "environment=Architecture%3Aamd64+Network%3Aovn+Platform%3Aaws+Topology%3Aha")
	})

	t.Run("URL generation with view", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:          123,
			View:        "test-view",
			Release:     "4.20",
			BaseRelease: "4.19",
			TestID:      "openshift-tests:abc123",
			TestName:    "test-example",
			Component:   "component-example",
			Capability:  "capability-example",
			Variants:    pq.StringArray{"Architecture:amd64", "Platform:aws"},
		}

		url, err := GenerateTestDetailsURL(regression, "https://sippy.example.com", views, releases, time.Hour)
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
		regression := &models.TestRegression{
			ID:          123,
			View:        "test-view",
			Release:     "4.20",
			BaseRelease: "4.17", // Different from view's base release
			TestID:      "openshift-tests:abc123",
			TestName:    "test-example",
			Variants:    pq.StringArray{"Architecture:amd64", "Platform:aws"},
		}

		url, err := GenerateTestDetailsURL(regression, "https://sippy.example.com", views, releases, time.Hour)
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
		// Real-world regression data
		regression := &models.TestRegression{
			ID:       1948,
			View:     "4.20-main",
			Release:  "4.20",
			TestID:   "openshift-tests:9f3fb60052539c29ab66564689f616ce",
			TestName: "[sig-cluster-lifecycle][Feature:Machines][Serial] Managed cluster should grow and decrease when scaling different machineSets simultaneously [Timeout:30m][apigroup:machine.openshift.io] [Suite:openshift/conformance/serial]",
			Variants: pq.StringArray{
				"Installer:ipi",
				"Network:ovn",
				"Platform:vsphere",
				"Suite:serial",
				"Topology:ha",
				"Upgrade:none",
				"Architecture:amd64",
				"FeatureSet:default",
			},
			BaseRelease: "4.18",
		}

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
		testViews := []crview.View{realWorldView}

		url, err := GenerateTestDetailsURL(regression, "https://sippy-auth.dptools.openshift.org", testViews, releases, time.Hour)
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
		viewsWithVariants := []crview.View{viewWithVariants}

		regression := &models.TestRegression{
			ID:      123,
			View:    "test-view-with-variants",
			Release: "4.20",
			TestID:  "test-id",
		}

		url, err := GenerateTestDetailsURL(regression, "https://example.com", viewsWithVariants, releases, 0)
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

}
