package main

import (
	"testing"

	"github.com/openshift/sippy/pkg/flags"
	"github.com/stretchr/testify/require"
)

func TestComponentReadinessFlagsValidateDefaultDoesNotRequireGoogleCredentials(t *testing.T) {
	componentReadinessFlags := &ComponentReadinessFlags{
		DataProvider:     "default",
		GoogleCloudFlags: flags.NewGoogleCloudFlags(),
	}

	require.NoError(t, componentReadinessFlags.Validate())
}

func TestServerFlagsValidateDefaultDoesNotRequireGoogleCredentials(t *testing.T) {
	serverFlags := &ServerFlags{
		DataProvider:     "default",
		GoogleCloudFlags: flags.NewGoogleCloudFlags(),
	}

	require.NoError(t, serverFlags.Validate())
}
