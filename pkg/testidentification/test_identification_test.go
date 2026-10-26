package testidentification

import "testing"

func TestIsIgnoredTest(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{
			name: "",
			want: true,
		},
		{
			name: "Run multi-stage test e2e-agnostic-cmd - e2e-agnostic-cmd-ipi-install-install container test",
			want: true,
		},
		{
			name: "Add storage is applicable for all workloads daemonsets create a daemonsets resource and adds storage to it",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIgnoredTest(tt.name); got != tt.want {
				t.Errorf("IsIgnoredTest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNonSuiteTest(t *testing.T) {
	tests := []struct {
		name      string
		suiteName string
		testName  string
		want      bool
	}{
		{
			name:      "normalized multi-stage step graph test is included",
			suiteName: "step graph",
			testName:  "Run multi-stage step ipi-install-install-stableinitial",
			want:      false,
		},
		{
			name:      "unnormalized step graph test is excluded",
			suiteName: "step graph",
			testName:  "some-step",
			want:      true,
		},
		{
			name:      "legacy multi-stage test is excluded",
			suiteName: "step graph",
			testName:  "Run multi-stage test e2e-aws - pod container test",
			want:      true,
		},
		{
			name:      "pipeline step is excluded",
			suiteName: "step graph",
			testName:  "Run pipeline step provision",
			want:      true,
		},
		{
			name:      "prowjob junit remains excluded",
			suiteName: "prowjob-junit",
			testName:  "Run multi-stage step test",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNonSuiteTest(tt.suiteName, tt.testName); got != tt.want {
				t.Errorf("IsNonSuiteTest(%q, %q) = %v, want %v", tt.suiteName, tt.testName, got, tt.want)
			}
		})
	}
}
