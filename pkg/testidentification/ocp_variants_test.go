package testidentification

import (
	"reflect"
	"testing"
)

func Test_openshiftVariants_IdentifyVariants(t *testing.T) {
	tests := []struct {
		name    string
		release string
		want    []string
	}{
		{
			name:    "periodic-ci-openshift-hypershift-main-periodics-conformance-aws-ovn-4-12",
			release: "4.12",
			want:    []string{"aws", "amd64", "ovn", "ha", "hypershift"},
		},
		{
			name:    "periodic-ci-openshift-release-master-nightly-4.12-e2e-metal-ovn-single-node-live-iso",
			release: "4.12",
			want:    []string{"metal-assisted", "amd64", "ovn", "single-node"},
		},
		{
			name:    "periodic-ci-openshift-release-master-ci-4.12-e2e-aws-ovn",
			release: "4.12",
			want:    []string{"aws", "amd64", "ovn", "ha"},
		},
		{
			name:    "periodic-ci-openshift-release-master-ci-4.12-e2e-aws",
			release: "4.12",
			want:    []string{"aws", "amd64", "ovn", "ha"},
		},
		{
			name:    "periodic-ci-openshift-release-master-ci-4.11-e2e-aws-ovn",
			release: "4.11",
			want:    []string{"aws", "amd64", "ovn", "ha"},
		},
		{
			name:    "periodic-ci-openshift-release-master-ci-4.12-e2e-aws",
			release: "4.11",
			want:    []string{"aws", "amd64", "sdn", "ha"},
		},
		{
			name:    "periodic-ci-openshift-release-master-ci-e2e-aws",
			release: "invalid",
			want:    []string{"aws", "amd64", "ha"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := openshiftVariants{}
			if got := v.IdentifyVariants(tt.name, tt.release); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IdentifyVariants() = %v, want %v", got, tt.want)
			}
		})
	}
}
