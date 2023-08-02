package testidentification

import (
	"reflect"
	"testing"

	"github.com/openshift/sippy/pkg/db/models"
)

func Test_openshiftVariants_IdentifyVariants(t *testing.T) {
	tests := []struct {
		name        string
		release     string
		clusterData models.ClusterData
		want        []string
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
		{
			name:        "periodic-ci-openshift-release-master-ci-e2e-aws-cluster-release",
			release:     "4.13",
			clusterData: models.ClusterData{Release: "4.13"},
			want:        []string{"aws", "amd64", "ovn", "ha"},
		},
		{
			name:        "periodic-ci-openshift-release-master-ci-e2e-aws-cluster-release-with-network",
			release:     "4.13",
			clusterData: models.ClusterData{Release: "4.13", Network: "sdn"},
			want:        []string{"aws", "amd64", "sdn", "ha"},
		},
		{
			name:        "periodic-ci-openshift-release-master-ci-e2e-aws-cluster-release-with-network-platform-override",
			release:     "4.13",
			clusterData: models.ClusterData{Release: "4.13", Network: "sdn", Platform: "azure"},
			want:        []string{"azure", "amd64", "sdn", "ha"},
		},
		{
			name:        "periodic-ci-openshift-release-master-ci-e2e-aws-cluster-release-with-network-platform-override-architecture",
			release:     "4.13",
			clusterData: models.ClusterData{Release: "4.13", Network: "sdn", Platform: "azure", Architecture: "arm64"},
			want:        []string{"azure", "arm64", "sdn", "ha"},
		},
		{
			name:        "periodic-ci-openshift-release-master-ci-e2e-aws-cluster-release-with-network-platform-override-architecture-topology",
			release:     "4.13",
			clusterData: models.ClusterData{Release: "4.13", Network: "sdn", Platform: "azure", Architecture: "arm64", Topology: "single-node"},
			want:        []string{"azure", "arm64", "sdn", "single-node"},
		},
		{
			name:        "periodic-ci-openshift-release-master-ci-e2e-aws-cluster-release-with-network-platform-override-architecture-topology-invalid",
			release:     "4.13",
			clusterData: models.ClusterData{Release: "4.13", Network: "sdn", Platform: "azure", Architecture: "arm64", Topology: "single"},
			want:        []string{"azure", "arm64", "sdn", "ha"},
		},
		{
			name: "periodic-ci-openshift-release-master-nightly-4.14-e2e-agent-ha-dualstack",
			want: []string{"amd64", "ha", "agent"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := openshiftVariants{}
			if got := v.IdentifyVariants(tt.name, tt.release, tt.clusterData); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IdentifyVariants() = %v, want %v", got, tt.want)
			}
		})
	}
}
