package testidentification

import (
	"strings"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util/sets"
)

var customJobSetupContainers = sets.NewString(
	"e2e-44-stable-to-45-ci-ipi-install-install-stableinitial",
	"e2e-aws-upgrade-ipi-install-install-stableinitial",
	"e2e-aws-upgrade-rollback-ipi-install-install-stableinitial",
	"e2e-aws-proxy-ipi-install-install",
	"e2e-aws-workers-rhel7-ipi-install-install",
	"e2e-azure-upgrade-ipi-conf-azure",
	"e2e-gcp-upgrade-ipi-install-install-stableinitial",
	"e2e-metal-ipi-baremetalds-devscripts-setup",
	"e2e-metal-ipi-ovn-ipv6-baremetalds-devscripts-setup",
	"e2e-metal-ipi-ovn-dualstack-baremetalds-devscripts-setup",
	"e2e-vsphere-ipi-install-vsphere",
	"e2e-vsphere-upi-upi-install-vsphere",
	"e2e-vsphere-upi-serial-upi-install-vsphere",
	"e2e-vsphere-serial-ipi-install-vsphere",
	"e2e-metal-assisted-baremetalds-assisted-setup",
	"e2e-metal-assisted-onprem-baremetalds-assisted-setup",
	"e2e-metal-ipi-virtualmedia-baremetalds-devscripts-setup",
	"install-stableinitial container test",
	"install-install container test",
)

// TODO We should instead try to detect whether we fail in a pre-step to determine whether setup succeeded
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
		"Cluster completes upgrade",
	},
}

var (
	cvoAcknowledgesUpgrade = "[sig-cluster-lifecycle] Cluster version operator acknowledges upgrade"
	operatorsUpgraded      = "[sig-cluster-lifecycle] Cluster completes upgrade"
	machineConfigsUpgraded = "[sig-mco] Machine config pools complete upgrade"
)

func IsCuratedTest(release, testName string) bool {
	for _, substring := range curatedTestSubstrings[release] {
		if strings.Contains(testName, substring) {
			return true
		}
	}
	return false
}

func IsOldInstallOperatorTest(testName string) bool {
	return testgridanalysisapi.OperatorConditionsTestCaseName.MatchString(testName)
}

func GetOperatorFromInstallTest(testName string) string {
	if !IsOldInstallOperatorTest(testName) {
		return "NOT-AN-INSTALL-TEST-" + testName
	}
	matches := testgridanalysisapi.OperatorConditionsTestCaseName.FindStringSubmatch(testName)
	operatorIndex := testgridanalysisapi.OperatorConditionsTestCaseName.SubexpIndex("operator")
	return matches[operatorIndex]
}

func IsOldUpgradeOperatorTest(testName string) bool {
	return strings.HasPrefix(testName, testgridanalysisapi.OperatorUpgradePrefix)
}

func IsOperatorHealthTest(testName string) bool {
	if strings.HasPrefix(testName, testgridanalysisapi.OperatorUpgradePrefix) {
		return true
	}
	if testgridanalysisapi.OperatorConditionsTestCaseName.MatchString(testName) {
		return true
	}
	if strings.HasPrefix(testName, testgridanalysisapi.OperatorFinalHealthPrefix) {
		return true
	}
	return false
}

func IsUpgradeStartedTest(testName string) bool {
	return testName == cvoAcknowledgesUpgrade
}

func IsOperatorsUpgradedTest(testName string) bool {
	return testName == operatorsUpgraded
}

func IsMachineConfigPoolsUpgradedTest(testName string) bool {
	return testName == machineConfigsUpgraded
}

func GetOperatorFromUpgradeTest(testName string) string {
	if !IsOldUpgradeOperatorTest(testName) {
		return "NOT-AN-UPGRADE-TEST-" + testName
	}
	return testName[len(testgridanalysisapi.OperatorUpgradePrefix):]
}

func GetOperatorNameFromTest(testName string) string {
	if IsOldUpgradeOperatorTest(testName) {
		return GetOperatorFromUpgradeTest(testName)
	}
	if IsOldInstallOperatorTest(testName) {
		return GetOperatorFromInstallTest(testName)
	}
	if strings.HasPrefix(testName, testgridanalysisapi.OperatorFinalHealthPrefix) {
		return testName[len(testgridanalysisapi.OperatorFinalHealthPrefix):]
	}
	return ""
}

// IsUpgradeRelatedTest is a filter function for identifying tests that are valuable to track for upgrade diagnosis.
func IsUpgradeRelatedTest(testName string) bool {
	if IsOldUpgradeOperatorTest(testName) {
		return true
	}
	if strings.Contains(testName, testgridanalysisapi.UpgradeTestName) {
		return true
	}
	if IsUpgradeStartedTest(testName) {
		// indicates that the CVO updated the clusterversion.status to indicate that it started work on a new payload
		return true
	}
	if IsOperatorsUpgradedTest(testName) {
		// indicates every cluster operator upgraded successfully.  This does not include machine config pools
		return true
	}
	if strings.Contains(testName, `[sig-cluster-lifecycle] cluster upgrade should be fast`) {
		// indicates that every cluster operator upgraded withing X minutes (currently 75 as of today)
		return true
	}
	if IsMachineConfigPoolsUpgradedTest(testName) {
		// indicates that all the machines restarted with new rhcos
		return true
	}
	if strings.Contains(testName, `APIs remain available`) {
		return true
	}

	return false

}
