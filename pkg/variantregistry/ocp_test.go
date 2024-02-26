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
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.16",
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
			job: "periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn-conformance",
			expected: map[string]string{
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.16",
				VariantArch:          "amd64",
				VariantInstaller:     "ipi", // Is this ok for hypershift?
				VariantFeatureSet:    "default",
				VariantPlatform:      "aws",
				VariantNetwork:       "ovn",
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "external",
				VariantSecurityMode:  "default",
				VariantSuite:         "unknown",
				VariantUpgrade:       "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn-single-node-serial",
			variantsFile: map[string]string{
				"Topology": "single", // should be ignored
			},
			expected: map[string]string{
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.16",
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
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.16",
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
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.16",
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
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.15",
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
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.16",
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
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.16",
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
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-no-network-plugin-no-variant-file",
			expected: map[string]string{
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.16",
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "",
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
		{
			job: "periodic-ci-openshift-release-master-nightly-4.11-e2e-no-network-plugin-no-variant-file",
			expected: map[string]string{
				VariantRelease:       "4.11",
				VariantFromRelease:   "4.11",
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "",
				VariantNetwork:       "sdn", // should default to sdn prior to 4.12
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "ha",
				VariantSecurityMode:  "default",
				VariantSuite:         "unknown",
				VariantUpgrade:       "none",
			},
		},
		{
			job: "release-openshift-origin-installer-e2e-aws-upgrade-4.13-to-4.14-to-4.15-to-4.16-ci",
			expected: map[string]string{
				VariantRelease:       "4.16",
				VariantFromRelease:   "4.13",
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "aws",
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
			job: "periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-from-stable-4.13-e2e-aws-sdn-upgrade",
			expected: map[string]string{
				VariantRelease:       "4.15",
				VariantFromRelease:   "4.13",
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantFeatureSet:    "default",
				VariantPlatform:      "aws",
				VariantNetwork:       "sdn",
				VariantNetworkAccess: "default",
				VariantOwner:         "eng",
				VariantScheduler:     "default",
				VariantTopology:      "ha",
				VariantSecurityMode:  "default",
				VariantSuite:         "unknown",
				VariantUpgrade:       "minor",
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