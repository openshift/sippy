package variantregistry

import (
	"testing"

	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/stretchr/testify/assert"
)

func TestVariantSyncer(t *testing.T) {
	variantSyncer := OCPVariantLoader{VariantManager: testidentification.NewOpenshiftVariantManager()}
	tests := []struct {
		job      string
		expected map[string]string
	}{
		{
			"periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp-ovn-fips",
			map[string]string{
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "gcp",
				VariantNetwork:       "ovn",
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "ha",
				VariantSecurityMode:  "fips",
				VariantSuite:         "unknown",
				VariantUpgrade:       "none",
			},
		},
		{
			"periodic-ci-openshift-release-master-nightly-4.16-e2e-vsphere-ovn-upi-serial",
			map[string]string{
				VariantArch:          "amd64",
				VariantInstaller:     "upi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "vsphere",
				VariantNetwork:       "ovn",
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "ha",
				VariantSecurityMode:  "default",
				VariantSuite:         "serial",
				VariantUpgrade:       "none",
			},
		},
		{
			"periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn-proxy",
			map[string]string{
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "aws",
				VariantNetwork:       "ovn",
				VariantNetworkAccess: "proxy",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "ha",
				VariantSecurityMode:  "default",
				VariantSuite:         "unknown",
				VariantUpgrade:       "none",
			},
		},
		{
			"periodic-ci-openshift-multiarch-master-nightly-4.16-upgrade-from-nightly-4.15-ocp-e2e-upgrade-gcp-ovn-heterogeneous",
			map[string]string{
				VariantArch:          "heterogeneous",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "gcp",
				VariantNetwork:       "ovn",
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "ha",
				VariantSecurityMode:  "default",
				VariantSuite:         "unknown",
				VariantUpgrade:       "minor",
			},
		},
		{
			"periodic-ci-openshift-release-master-nightly-4.16-e2e-metal-ipi-sdn-bm-upgrade",
			map[string]string{
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "metal",
				VariantNetwork:       "sdn",
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "ha",
				VariantSecurityMode:  "default",
				VariantSuite:         "unknown",
				VariantUpgrade:       "micro",
			},
		},
		{
			"periodic-ci-openshift-release-master-nightly-4.16-e2e-metal-ovn-assisted",
			map[string]string{
				VariantArch:          "amd64",
				VariantInstaller:     "assisted",
				VariantFeatureSet:    "default",
				VariantPlatform:      "metal",
				VariantNetwork:       "ovn",
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "ha",
				VariantSecurityMode:  "default",
				VariantSuite:         "unknown",
				VariantUpgrade:       "none",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.job, func(t *testing.T) {
			assert.Equal(t, test.expected, variantSyncer.GetVariantsForJob(test.job))
		})
	}
}
