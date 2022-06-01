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
