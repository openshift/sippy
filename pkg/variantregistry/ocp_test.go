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
				VariantRelease:      "4.16",
				VariantReleaseMajor: "4",
				VariantReleaseMinor: "16",
				VariantArch:         "amd64",
				VariantInstaller:    "ipi",
				VariantPlatform:     "gcp",
				VariantNetwork:      "ovn",
				VariantNetworkStack: "ipv4",
				VariantOwner:        "eng",
				VariantTopology:     "ha",
				VariantSecurityMode: "fips",
				VariantUpgrade:      "none",
				"Foo":               "bar",
			},
		},
		{
			job: "periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn-conformance",
			expected: map[string]string{
				VariantRelease:      "4.16",
				VariantReleaseMajor: "4",
				VariantReleaseMinor: "16",
				VariantArch:         "amd64",
				VariantInstaller:    "hypershift", // hypershift uses it's own installer
				VariantPlatform:     "aws",
				VariantNetwork:      "ovn",
				VariantNetworkStack: "ipv4",
				VariantOwner:        "eng",
				VariantTopology:     "external",
				VariantUpgrade:      "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn-single-node-serial",
			variantsFile: map[string]string{
				"Topology": "single", // should be ignored
			},
			expected: map[string]string{
				VariantRelease:      "4.16",
				VariantReleaseMajor: "4",
				VariantReleaseMinor: "16",
				VariantArch:         "amd64",
				VariantInstaller:    "ipi",
				VariantPlatform:     "aws",
				VariantNetwork:      "ovn",
				VariantNetworkStack: "ipv4",
				VariantOwner:        "eng",
				VariantTopology:     "single",
				VariantSuite:        "serial",
				VariantUpgrade:      "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-vsphere-ovn-upi-serial",
			expected: map[string]string{
				VariantRelease:      "4.16",
				VariantReleaseMajor: "4",
				VariantReleaseMinor: "16",
				VariantArch:         "amd64",
				VariantInstaller:    "upi",
				VariantPlatform:     "vsphere",
				VariantNetwork:      "ovn",
				VariantNetworkStack: "ipv4",
				VariantOwner:        "eng",
				VariantTopology:     "ha",
				VariantSuite:        "serial",
				VariantUpgrade:      "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn-proxy",
			expected: map[string]string{
				VariantRelease:       "4.16",
				VariantReleaseMajor:  "4",
				VariantReleaseMinor:  "16",
				VariantArch:          "amd64",
				VariantInstaller:     "ipi",
				VariantPlatform:      "aws",
				VariantNetwork:       "ovn",
				VariantNetworkAccess: "proxy",
				VariantNetworkStack:  "ipv4",
				VariantOwner:         "eng",
				VariantTopology:      "ha",
				VariantUpgrade:       "none",
			},
		},
		{
			job: "periodic-ci-openshift-multiarch-master-nightly-4.16-upgrade-from-nightly-4.15-ocp-e2e-upgrade-gcp-ovn-heterogeneous",
			variantsFile: map[string]string{
				"Architecture": "amd64", // should be overruled by the job parsing.
			},
			expected: map[string]string{
				VariantRelease:          "4.16",
				VariantFromRelease:      "4.15",
				VariantReleaseMajor:     "4",
				VariantReleaseMinor:     "16",
				VariantFromReleaseMajor: "4",
				VariantFromReleaseMinor: "15",
				VariantArch:             "heterogeneous",
				VariantInstaller:        "ipi",
				VariantPlatform:         "gcp",
				VariantNetwork:          "ovn",
				VariantNetworkStack:     "ipv4",
				VariantOwner:            "eng",
				VariantTopology:         "ha",
				VariantUpgrade:          "minor",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-metal-ipi-sdn-bm-upgrade",
			expected: map[string]string{
				VariantRelease:          "4.16",
				VariantFromRelease:      "4.16",
				VariantReleaseMajor:     "4",
				VariantReleaseMinor:     "16",
				VariantFromReleaseMajor: "4",
				VariantFromReleaseMinor: "16",
				VariantArch:             "amd64",
				VariantInstaller:        "ipi",
				VariantPlatform:         "metal",
				VariantNetwork:          "sdn",
				VariantNetworkStack:     "ipv4",
				VariantOwner:            "eng",
				VariantTopology:         "ha",
				VariantUpgrade:          "micro",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-metal-ovn-assisted",
			expected: map[string]string{
				VariantRelease:      "4.16",
				VariantReleaseMajor: "4",
				VariantReleaseMinor: "16",
				VariantArch:         "amd64",
				VariantInstaller:    "assisted",
				VariantPlatform:     "metal",
				VariantNetwork:      "ovn",
				VariantNetworkStack: "ipv4",
				VariantOwner:        "eng",
				VariantTopology:     "ha",
				VariantUpgrade:      "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-no-network-plugin-no-variant-file",
			expected: map[string]string{
				VariantRelease:      "4.16",
				VariantReleaseMajor: "4",
				VariantReleaseMinor: "16",
				VariantArch:         "amd64",
				VariantInstaller:    "ipi",
				VariantNetwork:      "ovn",
				VariantNetworkStack: "ipv4",
				VariantOwner:        "eng",
				VariantTopology:     "ha",
				VariantUpgrade:      "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.11-e2e-no-network-plugin-no-variant-file",
			expected: map[string]string{
				VariantRelease:      "4.11",
				VariantReleaseMajor: "4",
				VariantReleaseMinor: "11",
				VariantArch:         "amd64",
				VariantInstaller:    "ipi",
				VariantNetwork:      "sdn", // should default to sdn prior to 4.12
				VariantNetworkStack: "ipv4",
				VariantOwner:        "eng",
				VariantTopology:     "ha",
				VariantUpgrade:      "none",
			},
		},
		{
			job: "release-openshift-origin-installer-e2e-aws-upgrade-4.13-to-4.14-to-4.15-to-4.16-ci",
			expected: map[string]string{
				VariantRelease:          "4.16",
				VariantFromRelease:      "4.13",
				VariantReleaseMajor:     "4",
				VariantReleaseMinor:     "16",
				VariantFromReleaseMajor: "4",
				VariantFromReleaseMinor: "13",
				VariantArch:             "amd64",
				VariantInstaller:        "ipi",
				VariantPlatform:         "aws",
				VariantNetwork:          "ovn",
				VariantNetworkStack:     "ipv4",
				VariantOwner:            "eng",
				VariantTopology:         "ha",
				VariantUpgrade:          "multi",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-from-stable-4.13-e2e-aws-sdn-upgrade",
			expected: map[string]string{
				VariantRelease:          "4.15",
				VariantFromRelease:      "4.13",
				VariantReleaseMajor:     "4",
				VariantReleaseMinor:     "15",
				VariantFromReleaseMajor: "4",
				VariantFromReleaseMinor: "13",
				VariantArch:             "amd64",
				VariantInstaller:        "ipi",
				VariantPlatform:         "aws",
				VariantNetwork:          "sdn",
				VariantNetworkStack:     "ipv4",
				VariantOwner:            "eng",
				VariantTopology:         "ha",
				VariantUpgrade:          "multi",
			},
		},
		{
			job: "periodic-ci-openshift-with-no-release-info",
			expected: map[string]string{
				VariantArch:         "amd64",
				VariantInstaller:    "ipi",
				VariantNetworkStack: "ipv4",
				VariantOwner:        "eng",
				VariantTopology:     "ha",
				VariantUpgrade:      "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-e2e-metal-ipi-ovn-dualstack",
			expected: map[string]string{
				VariantRelease:      "4.16",
				VariantReleaseMajor: "4",
				VariantReleaseMinor: "16",
				VariantArch:         "amd64",
				VariantInstaller:    "ipi",
				VariantPlatform:     "metal",
				VariantNetwork:      "ovn",
				VariantNetworkStack: "dual",
				VariantOwner:        "eng",
				VariantTopology:     "ha",
				VariantUpgrade:      "none",
			},
		},
		{
			job: "periodic-ci-openshift-release-master-nightly-4.16-upgrade-from-stable-4.15-e2e-metal-ipi-upgrade-ovn-ipv6",
			expected: map[string]string{
				VariantRelease:          "4.16",
				VariantFromRelease:      "4.15",
				VariantReleaseMajor:     "4",
				VariantReleaseMinor:     "16",
				VariantFromReleaseMajor: "4",
				VariantFromReleaseMinor: "15",
				VariantArch:             "amd64",
				VariantInstaller:        "ipi",
				VariantPlatform:         "metal",
				VariantNetwork:          "ovn",
				VariantNetworkStack:     "ipv6",
				VariantOwner:            "eng",
				VariantTopology:         "ha",
				VariantUpgrade:          "minor",
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
