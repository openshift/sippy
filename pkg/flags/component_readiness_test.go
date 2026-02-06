package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionViewsConfiguration validates that the production config/views.yaml
// file is correctly configured and passes all validation rules.
//
// This test would have caught the regression tracking conflict that prevented
// COO dashboards from loading.
func TestProductionViewsConfiguration(t *testing.T) {
	// Test that the production views.yaml validates correctly
	crFlags := NewComponentReadinessFlags()
	crFlags.ComponentReadinessViewsFile = "../../config/views.yaml"

	views, err := crFlags.ParseViewsFile()
	require.NoError(t, err, "config/views.yaml failed validation - check for syntax errors or validation rule violations")
	require.Greater(t, len(views.ComponentReadiness), 0, "no views loaded from config/views.yaml")

	t.Logf("Successfully loaded %d views from config/views.yaml", len(views.ComponentReadiness))

}

// TestViewsValidationLogic tests the validation rules independently
func TestViewsValidationLogic(t *testing.T) {
	tests := []struct {
		name        string
		viewsFile   string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "production config should be valid",
			viewsFile:   "../../config/views.yaml",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crFlags := NewComponentReadinessFlags()
			crFlags.ComponentReadinessViewsFile = tt.viewsFile

			_, err := crFlags.ParseViewsFile()

			if tt.shouldError {
				require.Error(t, err, "expected validation to fail for %s", tt.viewsFile)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg,
						"error message should mention the validation constraint")
				}
			} else {
				require.NoError(t, err, "expected validation to pass for %s", tt.viewsFile)
			}
		})
	}
}
