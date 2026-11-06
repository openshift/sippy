package main

import (
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/flags"
	"github.com/stretchr/testify/require"
)

func TestComponentReadinessFlagsValidateDefaultDoesNotRequireGoogleCredentials(t *testing.T) {
	componentReadinessFlags := &ComponentReadinessFlags{
		DataProvider:            "default",
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
	}

	require.NoError(t, componentReadinessFlags.Validate())
}

func TestServerFlagsValidateDefaultDoesNotRequireGoogleCredentials(t *testing.T) {
	serverFlags := &ServerFlags{
		DataProvider:            "default",
		GoogleCloudFlags:        flags.NewGoogleCloudFlags(),
		ComponentReadinessFlags: flags.NewComponentReadinessFlags(),
	}

	require.NoError(t, serverFlags.Validate())
}

// Both commands must reject an invalid rounding factor through their Validate methods.
func TestCommandFlagsValidateTimeRounding(t *testing.T) {
	commands := []struct {
		name     string
		validate func(cr *flags.ComponentReadinessFlags) error
	}{
		{
			name: "component-readiness",
			validate: func(cr *flags.ComponentReadinessFlags) error {
				return (&ComponentReadinessFlags{DataProvider: "default", GoogleCloudFlags: flags.NewGoogleCloudFlags(), ComponentReadinessFlags: cr}).Validate()
			},
		},
		{
			name: "serve",
			validate: func(cr *flags.ComponentReadinessFlags) error {
				return (&ServerFlags{DataProvider: "default", GoogleCloudFlags: flags.NewGoogleCloudFlags(), ComponentReadinessFlags: cr}).Validate()
			},
		},
	}
	factors := []struct {
		name    string
		factor  time.Duration
		wantErr bool
	}{
		{name: "default factor", factor: flags.NewCRTimeRoundingFlags().Factor},
		{name: "48h factor", factor: 48 * time.Hour, wantErr: true},
		{name: "zero factor", factor: 0, wantErr: true},
	}
	for _, cmd := range commands {
		for _, tt := range factors {
			t.Run(cmd.name+"/"+tt.name, func(t *testing.T) {
				cr := flags.NewComponentReadinessFlags()
				cr.TimeRounding.Factor = tt.factor
				err := cmd.validate(cr)
				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}
