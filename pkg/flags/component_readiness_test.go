package flags

import (
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/spf13/pflag"
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

func TestValidateSpotCheckSamples(t *testing.T) {
	sample := func(tiers ...string) crview.SpotCheckJobSample {
		variants := map[string][]string{}
		if tiers != nil {
			variants["JobTier"] = tiers
		}
		return crview.SpotCheckJobSample{RelativeStart: "now-7d", RelativeEnd: "now", IncludeVariants: variants}
	}
	tests := []struct {
		name     string
		samples  map[string]crview.SpotCheckJobSample
		errorMsg string
	}{
		{name: "no samples"},
		{name: "sample selects its own tier", samples: map[string]crview.SpotCheckJobSample{"spotcheck-7d": sample("spotcheck-7d")}},
		{
			name:     "sample without a tier",
			samples:  map[string]crview.SpotCheckJobSample{"spotcheck-7d": sample()},
			errorMsg: "must select exactly JobTier spotcheck-7d",
		},
		{
			name:     "sample with an empty tier",
			samples:  map[string]crview.SpotCheckJobSample{"spotcheck-7d": sample([]string{}...)},
			errorMsg: "must select exactly JobTier spotcheck-7d",
		},
		{
			name:     "sample selecting a different tier",
			samples:  map[string]crview.SpotCheckJobSample{"spotcheck-7d": sample("spotcheck-30d")},
			errorMsg: "must select exactly JobTier spotcheck-7d",
		},
		{
			name:     "sample selecting extra tiers",
			samples:  map[string]crview.SpotCheckJobSample{"spotcheck-7d": sample("spotcheck-7d", "standard")},
			errorMsg: "must select exactly JobTier spotcheck-7d",
		},
		{
			name:     "sample name without the spot-check prefix",
			samples:  map[string]crview.SpotCheckJobSample{"weekly": sample("weekly")},
			errorMsg: `must be named with the "spotcheck-" prefix`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSpotCheckSamples(crview.View{Name: "test-view", SpotCheckJobSamples: tt.samples})
			if tt.errorMsg == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// A factor over 24h collapsed the 4.22 sample window to start == end at
// 2026-11-04T00:30Z, so the report counted failures the details query could not see.
func TestCRTimeRoundingFlagsValidate(t *testing.T) {
	tests := []struct {
		name    string
		factor  time.Duration
		wantErr bool
	}{
		{name: "default 12h", factor: defaultCRTimeRoundingFactor},
		{name: "maximum 24h", factor: 24 * time.Hour},
		{name: "small 1h", factor: time.Hour},
		{name: "96h collapses the sample window", factor: 96 * time.Hour, wantErr: true},
		{name: "just over 24h", factor: 24*time.Hour + time.Second, wantErr: true},
		{name: "zero", factor: 0, wantErr: true},
		{name: "negative", factor: -time.Hour, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewCRTimeRoundingFlags()
			f.Factor = tt.factor
			err := f.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// The zero value of ComponentReadinessFlags must stay usable: binding applies the rounding
// defaults under the names 'sippy serve' and seed-data share, and parsed values override them.
func TestComponentReadinessFlagsZeroValueBindFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantFactor time.Duration
		wantOffset time.Duration
	}{
		{name: "defaults", wantFactor: defaultCRTimeRoundingFactor, wantOffset: defaultCRTimeRoundingOffset},
		{
			name:       "overrides",
			args:       []string{"--component-readiness-time-rounding-factor=6h", "--component-readiness-time-rounding-offset=5h"},
			wantFactor: 6 * time.Hour,
			wantOffset: 5 * time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f ComponentReadinessFlags
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			require.NotPanics(t, func() { f.BindFlags(fs) })
			require.NoError(t, fs.Parse(tt.args))
			assert.Equal(t, tt.wantFactor, f.TimeRounding.Factor)
			assert.Equal(t, tt.wantOffset, f.TimeRounding.Offset)
			assert.NoError(t, f.Validate())
		})
	}
}
