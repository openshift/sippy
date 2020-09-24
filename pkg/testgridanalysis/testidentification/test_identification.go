package testidentification

import (
	"strings"

	"github.com/openshift/sippy/pkg/util/sets"
)

var customJobSetupContainers = sets.NewString(
	"e2e-metal-ipi-baremetalds-devscripts-setup",
	"e2e-aws-proxy-ipi-install-install",
	"e2e-aws-workers-rhel7-ipi-install-install",
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
