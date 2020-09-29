package testidentification

import (
	"fmt"
	"regexp"

	"github.com/openshift/sippy/pkg/util/sets"
)

var (
	ValidBugzillaComponents = sets.NewString(
		"apiserver-auth",
		"assisted-installer",
		"Bare Metal Hardware Provisioning",
		"Build",
		"Cloud Compute",
		"Cloud Credential Operator",
		"Cluster Loader",
		"Cluster Version Operator",
		"CNF Platform Validation",
		"Compliance Operator",
		"config-operator",
		"Console Kubevirt Plugin",
		"Console Metal3 Plugin",
		"Console Storage Plugin",
		"Containers",
		"crc",
		"Dev Console",
		"DNS",
		"Documentation",
		"Etcd",
		"Federation",
		"File Integrity Operator",
		"Fuse",
		"Hawkular",
		"ibm-roks-toolkit",
		"Image",
		"Image Registry",
		"Insights Operator",
		"Installer",
		"ISV Operators",
		"Jenkins",
		"kata-containers",
		"kube-apiserver",
		"kube-controller-manager",
		"kube-scheduler",
		"kube-storage-version-migrator",
		"Logging",
		"Machine Config Operator",
		"Management Console",
		"Metering Operator",
		"Migration Tooling",
		"Monitoring",
		"Multi-Arch",
		"Multi-cluster-management",
		"Networking",
		"Node",
		"Node Feature Discovery Operator",
		"Node Tuning Operator",
		"oauth-apiserver",
		"oauth-proxy",
		"oc",
		"OLM",
		"openshift-apiserver",
		"openshift-controller-manager",
		"Operator SDK",
		"Performance Addon Operator",
		"Reference Architecture",
		"Registry Console",
		"Release",
		"RHCOS",
		"RHMI Monitoring",
		"Routing",
		"Samples",
		"Security",
		"Service Broker",
		"Service Catalog",
		"service-ca",
		"Special Resources Operator",
		"Storage",
		"Templates",
		"Test Infrastructure",
		"Unknown",
		"Windows Containers",
	)

	operatorToBugzillaComponent = map[string]string{}

	// sigToBugzillaComponent holds `sig-foo` (from '[sig-foo]' label in a test) as keys and maps them to the "most correct" BZ component
	sigToBugzillaComponent = map[string]string{}

	sigRegex *regexp.Regexp = regexp.MustCompile(`\[(sig-.*?)\]`)
)

func init() {
	must(addOperatorMapping("authentication", "apiserver-auth"))
	must(addOperatorMapping("cloud-credential", "Cloud Credential Operator"))
	must(addOperatorMapping("cluster-autoscaler", "Cloud Compute"))
	must(addOperatorMapping("config-operator", "config-operator"))
	must(addOperatorMapping("console", "Management Console"))
	must(addOperatorMapping("csi-snapshot-controller", "Storage"))
	must(addOperatorMapping("dns", "DNS"))
	must(addOperatorMapping("etcd", "Etcd"))
	must(addOperatorMapping("ingress", "Routing"))
	must(addOperatorMapping("image-registry", "Image Registry"))
	must(addOperatorMapping("insights", "Insights Operator"))
	must(addOperatorMapping("kube-apiserver", "kube-apiserver"))
	must(addOperatorMapping("kube-controller-manager", "kube-controller-manager"))
	must(addOperatorMapping("kube-scheduler", "kube-scheduler"))
	must(addOperatorMapping("kube-storage-version-migrator", "kube-storage-version-migrator"))
	must(addOperatorMapping("machine-api", "Cloud Compute"))
	must(addOperatorMapping("machine-approver", "Cloud Compute"))
	must(addOperatorMapping("machine-config", "Machine Config Operator"))
	must(addOperatorMapping("marketplace", "OLM"))
	must(addOperatorMapping("monitoring", "Monitoring"))
	must(addOperatorMapping("network", "Networking"))
	must(addOperatorMapping("node-tuning", "Node Tuning Operator"))
	must(addOperatorMapping("openshift-apiserver", "openshift-apiserver"))
	must(addOperatorMapping("openshift-controller-manager", "openshift-controller-manager"))
	must(addOperatorMapping("openshift-samples", "Samples"))
	must(addOperatorMapping("operator-lifecycle-manager", "OLM"))
	must(addOperatorMapping("operator-lifecycle-manager-catalog", "OLM"))
	must(addOperatorMapping("operator-lifecycle-manager-packageserver", "OLM"))
	must(addOperatorMapping("service-ca", "service-ca"))
	must(addOperatorMapping("storage", "Storage"))

	must(addSigMapping("sig-cli", "oc"))
	must(addSigMapping("sig-api-machinery", "kube-apiserver"))
	must(addSigMapping("sig-apps", "kube-controller-manager"))
	must(addSigMapping("sig-arch", "Unknown"))
	must(addSigMapping("sig-auth", "apiserver-auth"))
	must(addSigMapping("sig-builds", "Build"))
	must(addSigMapping("sig-cli", "oc"))
	must(addSigMapping("sig-cluster-lifecycle", "Unknown"))
	must(addSigMapping("sig-devex", "Build"))
	must(addSigMapping("sig-imageregistry", "Image Registry"))
	must(addSigMapping("sig-network", "Networking"))
	must(addSigMapping("sig-node", "Node"))
	must(addSigMapping("sig-operator", "OLM"))
	must(addSigMapping("sig-storage", "Storage"))
	must(addSigMapping("sig-unknown", "Unknown"))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func GetBugzillaComponentForOperator(operator string) string {
	ret, ok := operatorToBugzillaComponent[operator]
	if !ok {
		return "Unknown"
	}
	return ret
}

func GetBugzillaComponentForSig(sig string) string {
	ret, ok := operatorToBugzillaComponent[sig]
	if !ok {
		return "Unknown"
	}
	return ret
}

func addOperatorMapping(operator, bugzillaComponent string) error {
	if !ValidBugzillaComponents.Has(bugzillaComponent) {
		return fmt.Errorf("%q is not a valid bugzilla component", bugzillaComponent)
	}
	operatorToBugzillaComponent[operator] = bugzillaComponent
	return nil
}

func addSigMapping(sig, bugzillaComponent string) error {
	if !ValidBugzillaComponents.Has(bugzillaComponent) {
		return fmt.Errorf("%q is not a valid bugzilla component", bugzillaComponent)
	}
	sigToBugzillaComponent[sig] = bugzillaComponent
	return nil
}

// find associated sig from test name
func FindSig(name string) string {
	match := sigRegex.FindStringSubmatch(name)
	if len(match) > 1 {
		return match[1]
	}
	return "sig-unknown"
}
