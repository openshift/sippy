package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJiraCommandsServiceAccountIsOptIn(t *testing.T) {
	t.Setenv("JIRA_CLIENT_ID", "client")
	t.Setenv("JIRA_CLIENT_SECRET", "secret")
	t.Setenv("JIRA_CLOUD_ID", "11111111-2222-3333-4444-555555555555")
	for _, tc := range []struct {
		name    string
		command func() *cobra.Command
	}{
		{"automate-jira", NewAutomateJiraCommand},
		{"load", NewLoadCommand},
		{"serve", NewServeCommand},
		{"component-readiness", NewComponentReadinessCommand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.command()
			flag := cmd.Flags().Lookup("jira-service-account")
			require.NotNil(t, flag)
			assert.Equal(t, "false", flag.DefValue)
			require.NoError(t, cmd.Flags().Parse(nil))
			enabled, err := cmd.Flags().GetBool("jira-service-account")
			require.NoError(t, err)
			assert.False(t, enabled)
		})
	}
}
