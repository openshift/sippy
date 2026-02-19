package api

import (
	"testing"
	"time"
)

func TestRecentTestFailureGetFieldType(t *testing.T) {
	r := RecentTestFailure{}
	cases := []struct {
		field    string
		expected ColumnType
	}{
		{"test_name", ColumnTypeString},
		{"suite_name", ColumnTypeString},
		{"jira_component", ColumnTypeString},
		{"test_id", ColumnTypeNumerical},
		{"failure_count", ColumnTypeNumerical},
		{"first_failure", ColumnTypeNumerical},
		{"last_failure", ColumnTypeNumerical},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			got := r.GetFieldType(tc.field)
			if got != tc.expected {
				t.Errorf("GetFieldType(%q) = %d, want %d", tc.field, got, tc.expected)
			}
		})
	}
}

func TestRecentTestFailureGetStringValue(t *testing.T) {
	r := RecentTestFailure{
		TestName:      "my-test",
		SuiteName:     "my-suite",
		JiraComponent: "Networking",
	}
	cases := []struct {
		field    string
		expected string
	}{
		{"test_name", "my-test"},
		{"suite_name", "my-suite"},
		{"jira_component", "Networking"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			got, err := r.GetStringValue(tc.field)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("GetStringValue(%q) = %q, want %q", tc.field, got, tc.expected)
			}
		})
	}

	_, err := r.GetStringValue("unknown_field")
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestRecentTestFailureGetNumericalValue(t *testing.T) {
	now := time.Now()
	r := RecentTestFailure{
		TestID:       42,
		FailureCount: 7,
		FirstFailure: now,
		LastFailure:  now,
	}
	cases := []struct {
		field    string
		expected float64
	}{
		{"test_id", 42},
		{"failure_count", 7},
		{"first_failure", float64(now.Unix())},
		{"last_failure", float64(now.Unix())},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			got, err := r.GetNumericalValue(tc.field)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("GetNumericalValue(%q) = %f, want %f", tc.field, got, tc.expected)
			}
		})
	}

	_, err := r.GetNumericalValue("unknown_field")
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestRecentTestFailureGetArrayValue(t *testing.T) {
	r := RecentTestFailure{}
	_, err := r.GetArrayValue("anything")
	if err == nil {
		t.Error("expected error, got nil")
	}
}
