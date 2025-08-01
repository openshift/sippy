package bugs

import (
	"testing"

	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_FileBugAPI(t *testing.T) {
	t.Run("successful bug creation with all required fields", func(t *testing.T) {
		bugRequest := sippyserver.FileBugRequest{
			Summary:         "Test bug summary",
			Description:     "Test bug description with details",
			AffectsVersions: []string{"4.14", "4.15"},
			ComponentID:     "12345",
			Components:      []string{"Authentication"},
			Labels:          []string{"test-label"},
		}

		var bugResponse sippyserver.FileBugResponse
		err := util.SippyPost("/api/component_readiness/bugs", &bugRequest, &bugResponse)
		require.NoError(t, err)

		assert.True(t, bugResponse.Success)
		assert.True(t, bugResponse.DryRun, "should be dry run when no jira client is configured")
		assert.Equal(t, "OCPBUGS-1234", bugResponse.JiraKey)
		assert.Equal(t, "https://issues.redhat.com/browse/OCPBUGS-1234", bugResponse.JiraURL)
	})

	t.Run("successful bug creation with component ID", func(t *testing.T) {
		bugRequest := sippyserver.FileBugRequest{
			Summary:         "Test bug with component ID",
			Description:     "Test bug description",
			AffectsVersions: []string{"4.14"},
			ComponentID:     "12345",
		}

		var bugResponse sippyserver.FileBugResponse
		err := util.SippyPost("/api/component_readiness/bugs", &bugRequest, &bugResponse)
		require.NoError(t, err)

		assert.True(t, bugResponse.Success)
		assert.True(t, bugResponse.DryRun)
		assert.Equal(t, "OCPBUGS-1234", bugResponse.JiraKey)
	})

	t.Run("validation error - missing summary", func(t *testing.T) {
		bugRequest := sippyserver.FileBugRequest{
			Description:     "Test bug description",
			AffectsVersions: []string{"4.14"},
			Components:      []string{"Authentication"},
		}

		var bugResponse sippyserver.FileBugResponse
		err := util.SippyPost("/api/component_readiness/bugs", &bugRequest, &bugResponse)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Summary is required")
	})

	t.Run("validation error - missing description", func(t *testing.T) {
		bugRequest := sippyserver.FileBugRequest{
			Summary:         "Test bug summary",
			AffectsVersions: []string{"4.14"},
			Components:      []string{"Authentication"},
		}

		var bugResponse sippyserver.FileBugResponse
		err := util.SippyPost("/api/component_readiness/bugs", &bugRequest, &bugResponse)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Description is required")
	})

	t.Run("validation error - missing affects versions", func(t *testing.T) {
		bugRequest := sippyserver.FileBugRequest{
			Summary:     "Test bug summary",
			Description: "Test bug description",
			Components:  []string{"Authentication"},
		}

		var bugResponse sippyserver.FileBugResponse
		err := util.SippyPost("/api/component_readiness/bugs", &bugRequest, &bugResponse)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AffectsVersions is required")
	})

	t.Run("validation error - missing components and component ID", func(t *testing.T) {
		bugRequest := sippyserver.FileBugRequest{
			Summary:         "Test bug summary",
			Description:     "Test bug description",
			AffectsVersions: []string{"4.14"},
		}

		var bugResponse sippyserver.FileBugResponse
		err := util.SippyPost("/api/component_readiness/bugs", &bugRequest, &bugResponse)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "At least one Component is required")
	})

	t.Run("validation error - multiple validation errors combined", func(t *testing.T) {
		bugRequest := sippyserver.FileBugRequest{
			// Summary is missing
			// Description is missing
			// AffectsVersions is missing
			// Components and ComponentID are missing
		}

		var bugResponse sippyserver.FileBugResponse
		err := util.SippyPost("/api/component_readiness/bugs", &bugRequest, &bugResponse)
		require.Error(t, err)

		errorMsg := err.Error()
		assert.Contains(t, errorMsg, "Summary is required")
		assert.Contains(t, errorMsg, "Description is required")
		assert.Contains(t, errorMsg, "AffectsVersions is required")
		assert.Contains(t, errorMsg, "At least one Component is required")
	})
}
