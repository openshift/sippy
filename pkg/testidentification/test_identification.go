package testidentification

import (
	"regexp"
	"strings"

	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	// OperatorUpgradePrefix is used to detect tests in the junit which signal an operator's upgrade status.
	// TODO: what writes these initially?
	OperatorUpgradePrefix = "Operator upgrade "

	// SippyOperatorUpgradePrefix is the test name sippy prefix adds to signal operator upgrade success. It added based on
	// the above OperatorUpgradePrefix.
	// TODO: why do we add a test when there already is one? It looks like it may have been to converge a legacy test name under the same name as a new test name.
	SippyOperatorUpgradePrefix = "[sig-sippy] operator upgrade "

	// OperatorFinalHealthPrefix is used to detect tests in the junit which signal an operator's install status.
	OperatorFinalHealthPrefix = "Operator results.operator conditions "

	FinalOperatorHealthTestName = "[sig-sippy] tests should finish with healthy operators"

	SippySuiteName         = "sippy"
	InfrastructureTestName = `[sig-sippy] infrastructure should work`
	InstallTestName        = `[sig-sippy] install should work`
	InstallTimeoutTestName = `[sig-sippy] install should not timeout`
	UpgradeTestName        = `[sig-sippy] upgrade should work`
	OpenShiftTestsName     = `[sig-sippy] openshift-tests should work`

	InstallTestNamePrefix     = `install should succeed: `
	InstallConfigTestName     = `install should succeed: configuration`
	InstallBootstrapTestName  = `install should succeed: cluster bootstrap`
	InstallOtherTestName      = `install should succeed: other`
	NewInfrastructureTestName = `install should succeed: infrastructure`
	NewInstallTestName        = `install should succeed: overall`

	Success = "Success"
	Failure = "Failure"
	Unknown = "Unknown"
)

var (
	// DefaultExcludedVariants is used to exclude particular variants in reporting
	DefaultExcludedVariants = []string{"aggregated", "never-stable"}

	// TODO: add [sig-sippy] here as well so we can more clearly identify and substring search
	// OperatorInstallPrefix is used when sippy adds synthetic tests to report if each operator installed correct.
	OperatorInstallPrefix = "operator install "

	// TODO: is this even used anymore?
	OperatorConditionsTestCaseName = regexp.MustCompile(`Operator results.*operator install (?P<operator>.*)`)
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
	if strings.Contains(testName, NewInstallTestName) {
		return true
	}
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

var (
	cvoAcknowledgesUpgradeRegex = regexp.MustCompile(`^(Cluster upgrade\.)?\[sig-cluster-lifecycle\] Cluster version operator acknowledges upgrade$`)
	CVOAcknowledgesUpgradeTest  = "[sig-cluster-lifecycle] Cluster version operator acknowledges upgrade"
	operatorsUpgradedRegex      = regexp.MustCompile(`^(Cluster upgrade\.)?\[sig-cluster-lifecycle\] Cluster completes upgrade$`)
	OperatorsUpgradedTest       = "[sig-cluster-lifecycle] Cluster completes upgrade"
	machineConfigsUpgradedRegex = regexp.MustCompile(`^(Cluster upgrade\.)?\[sig-mco\] Machine config pools complete upgrade$`)
	MachineConfigsUpgradedTest  = "[sig-mco] Machine config pools complete upgrade"
	openshiftTestsRegex         = regexp.MustCompile(`(?:^openshift-tests\.|\[Suite:openshift|\[k8s\.io\]|\[sig-|\[bz-)`)
	APIsRemainAvailTest         = "APIs remain available"
	ignoreTestRegex             = regexp.MustCompile(`^$|Run multi-stage test|operator.Import the release payload|operator.Import a release payload|operator.Run template|operator.Build image|Monitor cluster while tests execute|Overall|job.initialize|\[sig-arch\]\[Feature:ClusterUpgrade\] Cluster should remain functional during upgrade`)
)

func IsOldInstallOperatorTest(testName string) bool {
	return OperatorConditionsTestCaseName.MatchString(testName)
}

func GetOperatorFromInstallTest(testName string) string {
	if !IsOldInstallOperatorTest(testName) {
		return "NOT-AN-INSTALL-TEST-" + testName
	}
	matches := OperatorConditionsTestCaseName.FindStringSubmatch(testName)
	operatorIndex := OperatorConditionsTestCaseName.SubexpIndex("operator")
	return matches[operatorIndex]
}

func IsOldUpgradeOperatorTest(testName string) bool {
	return strings.HasPrefix(testName, OperatorUpgradePrefix)
}

func IsOperatorHealthTest(testName string) bool {
	if strings.HasPrefix(testName, OperatorUpgradePrefix) {
		return true
	}
	if OperatorConditionsTestCaseName.MatchString(testName) {
		return true
	}
	if strings.HasPrefix(testName, OperatorFinalHealthPrefix) {
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
	return testName[len(OperatorUpgradePrefix):]
}

func GetOperatorNameFromTest(testName string) string {
	if IsOldUpgradeOperatorTest(testName) {
		return GetOperatorFromUpgradeTest(testName)
	}
	if IsOldInstallOperatorTest(testName) {
		return GetOperatorFromInstallTest(testName)
	}
	if strings.HasPrefix(testName, OperatorFinalHealthPrefix) {
		return testName[len(OperatorFinalHealthPrefix):]
	}
	return ""
}

// IsIgnoredTest is used to strip out tests that don't have predictive or diagnostic value.  We don't want to show these in our data.
func IsIgnoredTest(testName string) bool {
	return ignoreTestRegex.MatchString(testName)
}

// IsOverallTest returns true if the given test name qualifies as the "Overall" test. On Oct 4 2021
// the test name changed from "Overall" to "[jobName|testGridTabName].Overall", and for now we need to support both.
func IsOverallTest(testName string) bool {
	return testName == "Overall" || strings.HasSuffix(testName, ".Overall")
}
