package api

import (
	"testing"
)

func TestUseNewInstallTest(t *testing.T) {
	tests := []struct {
		release  string
		expected bool
	}{
		{"4.17", true},
		{"4.11", true},
		{"4.10", false},
		{"4.8", false},
		{"3.11", false},
		{"5.0", true},
		// Non-numeric releases return false (use old synthetic names)
		{"rosa-stage", false},
		{"aro-stage", false},
		{"Presubmits", false},
	}

	for _, tt := range tests {
		t.Run(tt.release, func(t *testing.T) {
			if got := useNewInstallTest(tt.release); got != tt.expected {
				t.Errorf("useNewInstallTest(%q) = %v, want %v", tt.release, got, tt.expected)
			}
		})
	}
}
