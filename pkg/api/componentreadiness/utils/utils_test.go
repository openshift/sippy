package utils

import (
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTestDetailsURL(t *testing.T) {
	t.Run("basic URL generation without view", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:       123,
			View:     "nonexistent-view",
			Release:  "4.19",
			TestID:   "openshift-tests:abc123",
			TestName: "test-example",
			Variants: pq.StringArray{"Architecture:amd64", "Platform:aws", "Network:ovn"},
		}

		// Empty views and releases - should use fallback defaults
		url, err := GenerateTestDetailsURL(regression, "https://sippy.example.com", nil, nil, 0)
		require.NoError(t, err)
		assert.NotEmpty(t, url)

		// Verify the URL contains expected components
		assert.Contains(t, url, "https://sippy.example.com/api/component_readiness/test_details")
		assert.Contains(t, url, "testId=openshift-tests%3Aabc123")
		assert.Contains(t, url, "baseRelease=4.19")
		assert.Contains(t, url, "sampleRelease=4.19")
		assert.Contains(t, url, "testBasisRelease=4.19")
		assert.Contains(t, url, "Architecture=amd64")
		assert.Contains(t, url, "Platform=aws")
		assert.Contains(t, url, "Network=ovn")
		assert.Contains(t, url, "environment=Architecture%3Aamd64+Platform%3Aaws+Network%3Aovn")
	})

	t.Run("empty base URL", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:       123,
			Release:  "4.19",
			TestID:   "test-id",
			Variants: pq.StringArray{"Platform:aws"},
		}

		url, err := GenerateTestDetailsURL(regression, "", nil, nil, 0)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(url, "/api/component_readiness/test_details"))
	})

	t.Run("nil regression", func(t *testing.T) {
		_, err := GenerateTestDetailsURL(nil, "https://example.com", nil, nil, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "regression cannot be nil")
	})

	t.Run("invalid base URL", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:      123,
			Release: "4.19",
			TestID:  "test-id",
		}

		_, err := GenerateTestDetailsURL(regression, "://invalid-url", nil, nil, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse base URL")
	})

	t.Run("variants with malformed entries", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:       123,
			Release:  "4.19",
			TestID:   "test-id",
			Variants: pq.StringArray{"Architecture:amd64", "InvalidVariant", "Platform:aws"},
		}

		url, err := GenerateTestDetailsURL(regression, "", nil, nil, 0)
		require.NoError(t, err)

		// Should still work, just ignoring malformed variants
		assert.Contains(t, url, "Architecture=amd64")
		assert.Contains(t, url, "Platform=aws")
		assert.NotContains(t, url, "InvalidVariant")
	})

	t.Run("URL generation with view fallback", func(t *testing.T) {
		regression := &models.TestRegression{
			ID:       123,
			View:     "nonexistent-view", // View that doesn't exist, should fallback to defaults
			Release:  "4.19",
			TestID:   "openshift-tests:abc123",
			TestName: "test-example",
			Variants: pq.StringArray{"Architecture:amd64", "Platform:aws"},
		}

		// Create a test view with a different name
		views := []crtype.View{
			{
				Name: "different-view",
				AdvancedOptions: crtype.RequestAdvancedOptions{
					Confidence: 99,
				},
			},
		}

		url, err := GenerateTestDetailsURL(regression, "https://sippy.example.com", views, nil, time.Hour)
		require.NoError(t, err)
		assert.NotEmpty(t, url)

		// Should use fallback defaults since view doesn't match
		assert.Contains(t, url, "https://sippy.example.com/api/component_readiness/test_details")
		assert.Contains(t, url, "testId=openshift-tests%3Aabc123")
		assert.Contains(t, url, "baseRelease=4.19")
		assert.Contains(t, url, "sampleRelease=4.19")
		assert.Contains(t, url, "confidence=95") // Default value, not 99 from the view
		assert.Contains(t, url, "minFail=3")
		assert.Contains(t, url, "pity=5")
		assert.Contains(t, url, "Architecture=amd64")
		assert.Contains(t, url, "Platform=aws")
	})
}
