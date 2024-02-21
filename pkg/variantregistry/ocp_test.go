package variantregistry

import (
	"testing"

	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestVariantSyncer(t *testing.T) {
	variantSyncer := OCPVariantLoader{VariantManager: testidentification.NewOpenshiftVariantManager()}
	tests := []struct {
		job          string
		variantsFile map[string]string
		expected     map[string]string
	}{
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp-ovn-fips",
			variantsFile: map[string]string{
				"Foo":         "bar",          // should be added
				"CloudRegion": "us-central-1", // should be ignored
			},
			expected: map[string]string{
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
				"Foo":                "bar",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn-single-node-serial",
			variantsFile: map[string]string{
				"Topology": "single", // should be ignored
			},
			expected: map[string]string{
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "aws",
				VariantNetwork:       "ovn",
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "single",
				VariantSecurityMode:  "default",
				VariantSuite:         "serial",
				VariantUpgrade:       "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-vsphere-ovn-upi-serial",
			expected: map[string]string{
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
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn-proxy",
			expected: map[string]string{
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
			job: "periodic-ci-openshift-multiarch-master-nightly-4.16-upgrade-from-nightly-4.15-ocp-e2e-upgrade-gcp-ovn-heterogeneous",
			variantsFile: map[string]string{
				"Architecture": "amd64", // should be overruled by the job parsing.
			},
			expected: map[string]string{
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
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-metal-ipi-sdn-bm-upgrade",
			expected: map[string]string{
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
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-metal-ovn-assisted",
			expected: map[string]string{
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
			assert.Equal(t, test.expected,
				variantSyncer.CalculateVariantsForJob(
					logrus.WithField("source", "TestVariantSyncer"),
					test.job,
					test.variantsFile))
		})
	}
}
