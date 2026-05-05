package db

import (
	"testing"
)

// TestDynamicSuitePatternMatching tests the pattern matching logic without requiring a database.
// Full integration testing of CheckForDynamicSuite with database operations should be done in e2e tests.
func TestDynamicSuitePatternMatching(t *testing.T) {
	tests := []struct {
		name          string
		suiteName     string
		shouldMatch   bool
		patternReason string
	}{
		{
			name:          "empty string",
			suiteName:     "",
			shouldMatch:   false,
			patternReason: "empty strings never match",
		},
		{
			name:          "lp-interop pattern with suffix",
			suiteName:     "lp-interop--my-tests",
			shouldMatch:   true,
			patternReason: "matches ^lp-interop-- pattern",
		},
		{
			name:          "lp-chaos pattern",
			suiteName:     "lp-chaos--test-suite",
			shouldMatch:   true,
			patternReason: "matches ^lp-chaos-- pattern",
		},
		{
			name:          "lp-ocp-compat pattern",
			suiteName:     "lp-ocp-compat--compatibility-tests",
			shouldMatch:   true,
			patternReason: "matches ^lp-ocp-compat-- pattern",
		},
		{
			name:          "no pattern match - single hyphen after lp-interop",
			suiteName:     "lp-interop-ACS--my-tests",
			shouldMatch:   false,
			patternReason: "pattern requires double hyphen immediately after lp-interop",
		},
		{
			name:          "no pattern match - single hyphen only",
			suiteName:     "lp-interop-Foo",
			shouldMatch:   false,
			patternReason: "pattern requires double hyphen (--)",
		},
		{
			name:          "no pattern match - suffix only",
			suiteName:     "CNV-lp-interop--extra-suffix",
			shouldMatch:   false,
			patternReason: "pattern must be at start",
		},
		{
			name:          "no pattern match - random suite",
			suiteName:     "random-suite",
			shouldMatch:   false,
			patternReason: "doesn't match any pattern",
		},
		{
			name:          "no pattern match - explicit suite",
			suiteName:     "openshift-tests",
			shouldMatch:   false,
			patternReason: "explicit suites don't match dynamic patterns",
		},
		{
			name:          "no pattern match - prefix added",
			suiteName:     "prefix-lp-interop--test",
			shouldMatch:   false,
			patternReason: "pattern requires lp-interop-- at start",
		},
		{
			name:          "no pattern match - only prefix no suffix",
			suiteName:     "lp-interop--",
			shouldMatch:   true,
			patternReason: "matches pattern even with empty suffix",
		},
		{
			name:          "no pattern match - missing double hyphen",
			suiteName:     "lp-interop",
			shouldMatch:   false,
			patternReason: "pattern requires double hyphen after lp-interop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the pattern matching logic by checking if any pattern matches
			matched := false
			for _, re := range testSuitePatterns {
				if tt.suiteName != "" && re.MatchString(tt.suiteName) {
					matched = true
					break
				}
			}

			if matched != tt.shouldMatch {
				t.Errorf("pattern matching for %q = %v, want %v (reason: %s)",
					tt.suiteName, matched, tt.shouldMatch, tt.patternReason)
			}
		})
	}
}
