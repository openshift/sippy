package main

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAutomateJiraCommandJiraURLDefault(t *testing.T) {
	cmd := NewAutomateJiraCommand()
	flag := cmd.Flags().Lookup("jira-url")
	if flag == nil {
		t.Fatal("jira-url flag not registered")
	}
	if want := "https://redhat.atlassian.net/"; flag.DefValue != want {
		t.Errorf("jira-url default = %q, want %q", flag.DefValue, want)
	}
}

func TestAutomateJiraServiceAccountDoesNotRequireDryRun(t *testing.T) {
	t.Setenv("JIRA_CLIENT_ID", "")
	t.Setenv("JIRA_CLIENT_SECRET", "")
	cmd := NewAutomateJiraCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--jira-service-account"})
	// Without --dry-run, OAuth credentials are still validated before data clients.
	require.ErrorContains(t, cmd.Execute(), "client ID is required")
}

func TestAutomateJiraClientSelection(t *testing.T) {
	for _, tc := range []struct {
		name           string
		serviceAccount bool
		dryRun         bool
		wantError      string
	}{
		{"PAT remains default", false, false, ""},
		{"PAT dry run remains supported", false, true, ""},
		{"SA selects OAuth without dry run", true, false, "client ID is required"},
		{"SA does not fall back to PAT", true, true, "client ID is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JIRA_TOKEN", "existing-pat")
			t.Setenv("JIRA_CLIENT_ID", "")
			t.Setenv("JIRA_CLIENT_SECRET", "")
			t.Setenv("JIRA_READ_ONLY_CLIENT_ID", "")
			t.Setenv("JIRA_READ_ONLY_CLIENT_SECRET", "")
			t.Setenv("JIRA_CLOUD_ID", "")
			flags := NewAutomateJiraFlags()
			flags.JiraFlags.ServiceAccount, flags.DryRun = tc.serviceAccount, tc.dryRun
			client, err := flags.JiraFlags.GetJiraClient(context.Background())
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				assert.Nil(t, client)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, client)
		})
	}
}

func TestAutomateJiraServiceAccountIsOptIn(t *testing.T) {
	cmd := NewAutomateJiraCommand()
	flag := cmd.Flags().Lookup("jira-service-account")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
}
