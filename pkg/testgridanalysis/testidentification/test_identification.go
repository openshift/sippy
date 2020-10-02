package testidentification

import (
	"strings"

	"github.com/openshift/sippy/pkg/util/sets"
)

var customJobSetupContainers = sets.NewString(
	"e2e-aws-upgrade-ipi-install-install-stableinitial",
	"e2e-aws-proxy-ipi-install-install",
	"e2e-aws-workers-rhel7-ipi-install-install",
	"e2e-gcp-upgrade-ipi-install-install-stableinitial",
	"e2e-metal-ipi-baremetalds-devscripts-setup",
	"e2e-vsphere-ipi-install-vsphere",
	"e2e-vsphere-upi-upi-install-vsphere",
	"e2e-vsphere-upi-serial-upi-install-vsphere",
	"e2e-vsphere-serial-ipi-install-vsphere",
)

// not all setup containers are called setup.  This is heavily dependent on the actual UPI jobs, but they turn out to be different.
// When this needs updating,  it shows up as installs timing out in weird numbers
func IsSetupContainerEquivalent(testName string) bool {
	for setup := range customJobSetupContainers {
		if strings.Contains(testName, setup) {
			return true
		}
	}

	if strings.HasSuffix(testName, "container setup") {
		return true
	}

	return false
}

// curatedTestSubstrings is keyed by release.  This is a list of tests that are important enough to individually watch.
// Whoever is running or working on TRT gets freedom to choose 10-20 of these for whatever reason they need.  At the moment,
// we're chasing problems where pods are not running reliably and we have to track it down.
var curatedTestSubstrings = map[string][]string{
	"4.6": []string{
		"[Feature:SCC][Early] should not have pod creation failures during install",
		"infrastructure should work",
		"install should work",
		"Kubernetes APIs remain available",
		"OAuth APIs remain available",
		"OpenShift APIs remain available",
		"Pod Container Status should never report success for a pending container",
		"pods should never transition back to pending",
		"pods should successfully create sandboxes",
		"upgrade should work",
	},
}

func IsCuratedTest(release, testName string) bool {
	for _, substring := range curatedTestSubstrings[release] {
		if strings.Contains(testName, substring) {
			return true
		}
	}
	return false
}
