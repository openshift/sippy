package main

import "testing"

// TestNewBackfillInfraFailuresCommand verifies the command wires up its flags
// with the expected defaults.
func TestNewBackfillInfraFailuresCommand(t *testing.T) {
	cmd := NewBackfillInfraFailuresCommand()

	if cmd.Use != "backfill-infra-failures" {
		t.Errorf("Use = %q, want %q", cmd.Use, "backfill-infra-failures")
	}
	if cmd.RunE == nil {
		t.Error("RunE must be set")
	}

	tests := []struct {
		flag        string
		wantDefault string
	}{
		{flag: "since", wantDefault: ""},
		{flag: "days", wantDefault: "90"},
		{flag: "dry-run", wantDefault: "false"},
		{flag: "batch-size", wantDefault: "100"},
	}
	for _, tc := range tests {
		t.Run(tc.flag, func(t *testing.T) {
			f := cmd.Flags().Lookup(tc.flag)
			if f == nil {
				t.Fatalf("flag %q not registered", tc.flag)
			}
			if f.DefValue != tc.wantDefault {
				t.Errorf("flag %q default = %q, want %q", tc.flag, f.DefValue, tc.wantDefault)
			}
		})
	}
}
