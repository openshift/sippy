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

func TestAutomateJiraServiceAccountRequiresDryRun(t *testing.T) {
	cmd := NewAutomateJiraCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--jira-service-account"})
	// The guard must run before cache, BigQuery, and database initialization.
	require.ErrorContains(t, cmd.Execute(), "--jira-service-account requires --dry-run")
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
		{"SA cannot write", true, false, "--jira-service-account requires --dry-run"},
		{"SA does not fall back to PAT", true, true, "client ID is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JIRA_TOKEN", "existing-pat")
			t.Setenv("JIRA_READ_ONLY_CLIENT_ID", "")
			t.Setenv("JIRA_READ_ONLY_CLIENT_SECRET", "")
			t.Setenv("JIRA_CLOUD_ID", "")
			flags := NewAutomateJiraFlags()
			flags.JiraServiceAccount, flags.DryRun = tc.serviceAccount, tc.dryRun
			client, err := flags.getJiraClient(context.Background())
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
