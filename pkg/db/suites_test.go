package db

import "testing"

func TestIsTestSuiteImportable(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"", false},
		{"openshift-tests", true},
		{"CNV-lp-interop", true},
		{"ACS-lp-interop", true},
		{"lp-interop-ACS--my-tests", true},
		{"lp-interop-Foo", true},
		{"CNV-lp-interop-extra-suffix", false},
		{"random-suite", false},
		{"-lp-interop", false},
		{"lp-interop", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTestSuiteImportable(tt.name); got != tt.want {
				t.Errorf("IsTestSuiteImportable(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
