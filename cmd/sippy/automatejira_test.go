package main

import "testing"

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
