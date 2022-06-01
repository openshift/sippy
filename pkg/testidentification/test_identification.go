package testidentification

import (
	"regexp"
	"strings"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util/sets"
)

var customJobInstallNames = sets.NewString(
	"aws-ipi-ipi-install-install-stableinitial",
	"azure-upi-upi-install-azure",
	"e2e-44-stable-to-45-ci-ipi-install-install-stableinitial",
	"e2e-aws-hypershift-ipi-install",
	"e2e-aws-proxy-ipi-install-install",
	"e2e-aws-upgrade-ipi-install-install-stableinitial",
	"e2e-aws-upgrade-rollback-ipi-install-install-stableinitial",
	"e2e-aws-workers-rhel7-ipi-install-install",
	"e2e-azure-cucushift-upi-upi-install-azure",
	"e2e-azure-upgrade-ipi-conf-azure",
	"e2e-azure-upi-upi-install-azure",
	"e2e-azurestack-csi-upi-install-azurestack",
	"e2e-baremetal-cucushift-ipi-baremetalds-devscripts-setup",
	"e2e-gcp-cucushift-upi-upi-install-gcp",
	"e2e-gcp-libvirt-cert-rotation-openshift-e2e-gcp-libvirt-cert-rotation-setup",
	"e2e-gcp-upgrade-ipi-install-install-stableinitial",
	"e2e-gcp-upi-upi-install-gcp",
	"e2e-metal-assisted-baremetalds-assisted-setup",
	"e2e-metal-assisted-ipv6-baremetalds-assisted-setup",
	"e2e-metal-assisted-onprem-baremetalds-assisted-setup",
	"e2e-metal-ipi-baremetalds-devscripts-setup",
	"e2e-metal-ipi-compact-baremetalds-devscripts-setup",
	"e2e-metal-ipi-ovn-dualstack-baremetalds-devscripts-setup",
	"e2e-metal-ipi-ovn-ipv6-baremetalds-devscripts-setup",
	"e2e-metal-ipi-serial-compact-baremetalds-devscripts-setup",
	"e2e-metal-ipi-serial-ipv4-baremetalds-devscripts-setup",
	"e2e-metal-ipi-serial-ovn-ipv6-baremetalds-devscripts-setup",
	"e2e-metal-ipi-serial-ovn-dualstack-baremetalds-devscripts-setup",
	"e2e-metal-ipi-serial-virtualmedia-baremetalds-devscripts-setup",
	"e2e-metal-ipi-virtualmedia-baremetalds-devscripts-setup",
	"e2e-metal-ipi-ovn-dualstack-local-gateway-baremetalds-devscripts-setup",
	"e2e-metal-ipi-upgrade-baremetalds-devscripts-setup container test",
	"e2e-metal-ipi-upgrade-ovn-ipv6-baremetalds-devscripts-setup",
	"e2e-metal-ipi-virtualmedia-baremetalds-devscripts-setup ",
	"e2e-metal-single-node-live-iso-baremetalds-sno-setup",
	"e2e-openshift-proxy-ipi-install-install",
	"e2e-openstack-upgrade-ipi-install",
	"e2e-ovirt-ipi-install-install container test",
	"e2e-telco5g-telco-bastion-setup",
	"e2e-vsphere-cucushift-upi-upi-install-vsphere",
	"e2e-vsphere-ipi-install-vsphere",
	"e2e-vsphere-serial-ipi-install-vsphere",
	"e2e-vsphere-upi-serial-upi-install-vsphere",
	"e2e-vsphere-upi-upi-install-vsphere",
	"hypershift-launch-wait-for-nodes",
	"install-install container test",
	"install-stableinitial container test",
	"ipi-install-libvirt-install",
	"ocp-installer-remote-libvirt-ppc64le-ipi-install-libvirt-install",
	"ocp-installer-remote-libvirt-s390x-ipi-install-libvirt-install",
	"upgrade-verification-tests-azure-upi-upi-install-azure",
	"upgrade-verification-tests-baremetal-ipi-baremetalds-devscripts-setup",
	"upgrade-verification-tests-gcp-upi-upi-install-gcp",
	"upgrade-verification-tests-vsphere-upi-upi-install-vsphere",
	"vsphere-upi-upi-install-vsphere",
)

// TODO We should instead try to detect whether we fail in a pre-step to determine whether install succeeded
// Install steps have different names in different jobs. This is heavily dependent on the actual UPI jobs, but they turn out to be different.
// When this needs updating,  it shows up as installs timing out in weird numbers
func IsInstallStepEquivalent(testName string) bool {
	for installName := range customJobInstallNames {
		if strings.Contains(testName, installName) {
			return true
		}
	}

	if strings.HasSuffix(testName, "container setup") {
		return true
	}

	//  kube uses this to mean the installation worked.  It's not perfectly analogous, but it's close.
	if testName == "Up" {
		return true
	}
	if strings.HasSuffix(testName, "create-cluster") {
		return true
	}

	return false
}

// curatedTestSubstrings is keyed by release.  This is a list of tests that are important enough to individually watch.
// Whoever is running or working on TRT gets freedom to choose 10-20 of these for whatever reason they need.  At the moment,
// we're chasing problems where pods are not running reliably and we have to track it down.
var curatedTestSubstrings = map[string][]string{
	"4.11": []string{
		"Kubernetes APIs remain available",
		"OAuth APIs remain available",
		"OpenShift APIs remain available",
		"Cluster frontend ingress remain available",
	},
	"4.10": []string{
		"Kubernetes APIs remain available",
		"OAuth APIs remain available",
		"OpenShift APIs remain available",
		"Cluster frontend ingress remain available",
	},
	"4.9": []string{
		"Kubernetes APIs remain available",
		"OAuth APIs remain available",
		"OpenShift APIs remain available",
		"Cluster frontend ingress remain available",
	},
}

var (
	cvoAcknowledgesUpgradeRegex = regexp.MustCompile(`^(Cluster upgrade\.)?\[sig-cluster-lifecycle\] Cluster version operator acknowledges upgrade$`)
	CVOAcknowledgesUpgradeTest  = "[sig-cluster-lifecycle] Cluster version operator acknowledges upgrade"
	operatorsUpgradedRegex      = regexp.MustCompile(`^(Cluster upgrade\.)?\[sig-cluster-lifecycle\] Cluster completes upgrade$`)
	OperatorsUpgradedTest       = "[sig-cluster-lifecycle] Cluster completes upgrade"
	machineConfigsUpgradedRegex = regexp.MustCompile(`^(Cluster upgrade\.)?\[sig-mco\] Machine config pools complete upgrade$`)
	MachineConfigsUpgradedRegex = "[sig-mco] Machine config pools complete upgrade"
	openshiftTestsRegex         = regexp.MustCompile(`(?:^openshift-tests\.|\[Suite:openshift|\[k8s\.io\]|\[sig-|\[bz-)`)
	UpgradeFastTest             = "[sig-cluster-lifecycle] cluster upgrade should be fast"
	APIsRemainAvailTest         = "APIs remain available"
	ignoreTestRegex             = regexp.MustCompile(`^$|Run multi-stage test|operator.Import the release payload|operator.Import a release payload|operator.Run template|operator.Build image|Monitor cluster while tests execute|Overall|job.initialize|\[sig-arch\]\[Feature:ClusterUpgrade\] Cluster should remain functional during upgrade`)
)

func IsCuratedTest(bugzillaRelease, testName string) bool {
	for _, substring := range curatedTestSubstrings[bugzillaRelease] {
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
	return cvoAcknowledgesUpgradeRegex.MatchString(testName)
}

func IsOperatorsUpgradedTest(testName string) bool {
	return operatorsUpgradedRegex.MatchString(testName)
}

func IsMachineConfigPoolsUpgradedTest(testName string) bool {
	return machineConfigsUpgradedRegex.MatchString(testName)
}

func IsOpenShiftTest(testName string) bool {
	return openshiftTestsRegex.MatchString(testName)
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
	if strings.Contains(testName, UpgradeFastTest) {
		// indicates that every cluster operator upgraded within X minutes (currently 75 as of today)
		return true
	}
	if IsMachineConfigPoolsUpgradedTest(testName) {
		// indicates that all the machines restarted with new rhcos
		return true
	}
	if strings.Contains(testName, APIsRemainAvailTest) {
		return true
	}

	return false

}

// IsInstallRelatedTest is a filter function for identifying tests that are valuable to track for upgrade diagnosis.
func IsInstallRelatedTest(testName string) bool {
	if testgridanalysisapi.OperatorConditionsTestCaseName.MatchString(testName) {
		return true
	}
	if strings.Contains(testName, testgridanalysisapi.InstallTestName) {
		return true
	}
	if strings.Contains(testName, testgridanalysisapi.InstallTimeoutTestName) {
		return true
	}
	// this shows the stages of install like infrastructure, configuration, bootstrap
	if strings.Contains(testName, testgridanalysisapi.InstallTestNamePrefix) {
		return true
	}
	if strings.HasPrefix(testName, testgridanalysisapi.OperatorInstallPrefix) {
		return true
	}

	return false
}

// IsIgnoredTest is used to strip out tests that don't have predictive or diagnostic value.  We don't want to show these in our data.
func IsIgnoredTest(testName string) bool {
	return ignoreTestRegex.MatchString(testName)
}
