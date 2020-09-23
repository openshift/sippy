package testidentification

import (
	"strings"
)

// not all setup containers are called setup.  This is heavily dependent on the actual UPI jobs, but they turn out to be different.
// When this needs updating,  it shows up as installs timing out in weird numbers
func IsSetupContainerEquivalent(testName string) bool {
	if strings.Contains(testName, "e2e-vsphere-upi-upi-install-vsphere") {
		return true
	}
	if strings.Contains(testName, "e2e-vsphere-upi-serial-upi-install-vsphere") {
		return true
	}
	if strings.Contains(testName, "e2e-vsphere-serial-ipi-install-vsphere") {
		return true
	}
	if strings.Contains(testName, "e2e-vsphere-ipi-install-vsphere") {
		return true
	}
	if strings.Contains(testName, "e2e-metal-ipi-baremetalds-devscripts-setup") {
		return true
	}
	if strings.HasSuffix(testName, "container setup") {
		return true
	}

	return false
}
