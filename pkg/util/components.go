package util

import (
	"fmt"

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

	OperatorToBugzillaComponent = map[string]string{}

	// SigToBugzillaComponent holds `sig-foo` (from '[sig-foo]' label in a test) as keys and maps them to the "most correct" BZ component
	SigToBugzillaComponent = map[string]string{}
)

func init() {
	Must(addOperatorMapping("authentication", "apiserver-auth"))
	Must(addOperatorMapping("cloud-credential", "Cloud Credential Operator"))
	Must(addOperatorMapping("cluster-autoscaler", "Cloud Compute"))
	Must(addOperatorMapping("config-operator", "config-operator"))
	Must(addOperatorMapping("console", "foo"))
	Must(addOperatorMapping("csi-snapshot-controller", "Storage"))
	Must(addOperatorMapping("dns", "DNS"))
	Must(addOperatorMapping("etcd", "Etcd"))
	Must(addOperatorMapping("ingress", "Routing"))
	Must(addOperatorMapping("image-registry", "Image Registry"))
	Must(addOperatorMapping("insights", "Insights Operator"))
	Must(addOperatorMapping("kube-apiserver", "kube-apiserver"))
	Must(addOperatorMapping("kube-controller-manager", "kube-controller-manager"))
	Must(addOperatorMapping("kube-scheduler", "kube-scheduler"))
	Must(addOperatorMapping("kube-storage-version-migrator", "kube-storage-version-migrator"))
	Must(addOperatorMapping("machine-api", "Cloud Compute"))
	Must(addOperatorMapping("machine-approver", "Cloud Compute"))
	Must(addOperatorMapping("machine-config", "Machine Config Operator"))
	Must(addOperatorMapping("marketplace", "OLM"))
	Must(addOperatorMapping("monitoring", "Monitoring"))
	Must(addOperatorMapping("network", "Networking"))
	Must(addOperatorMapping("node-tuning", "Node Tuning Operator"))
	Must(addOperatorMapping("openshift-apiserver", "openshift-apiserver"))
	Must(addOperatorMapping("openshift-controller-manager", "openshift-controller-manager"))
	Must(addOperatorMapping("openshift-samples", "Samples"))
	Must(addOperatorMapping("operator-lifecycle-manager", "OLM"))
	Must(addOperatorMapping("operator-lifecycle-manager-catalog", "OLM"))
	Must(addOperatorMapping("operator-lifecycle-manager-packageserver", "OLM"))
	Must(addOperatorMapping("service-ca", "service-ca"))
	Must(addOperatorMapping("storage", "Storage"))

	Must(addOperatorMapping("sig-cli", "oc"))
	Must(addOperatorMapping("sig-api-machinery", "kube-apiserver"))
	Must(addOperatorMapping("sig-apps", "kube-controller-manager"))
	Must(addOperatorMapping("sig-arch", "Unknown"))
	Must(addOperatorMapping("sig-auth", "apiserver-auth"))
	Must(addOperatorMapping("sig-builds", "Build"))
	Must(addOperatorMapping("sig-cli", "oc"))
	Must(addOperatorMapping("sig-cluster-lifecycle", "Unknown"))
	Must(addOperatorMapping("sig-devex", "Build"))
	Must(addOperatorMapping("sig-imageregistry", "Image Registry"))
	Must(addOperatorMapping("sig-network", "Networking"))
	Must(addOperatorMapping("sig-node", "Node"))
	Must(addOperatorMapping("sig-operator", "OLM"))
	Must(addOperatorMapping("sig-storage", "Storage"))
}

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

func addOperatorMapping(operator, bugzillaComponent string) error {
	if !ValidBugzillaComponents.Has(bugzillaComponent) {
		return fmt.Errorf("%q is not a valid bugzilla component")
	}
	OperatorToBugzillaComponent[operator] = bugzillaComponent
	return nil
}

func addSigMapping(sig, bugzillaComponent string) error {
	if !ValidBugzillaComponents.Has(bugzillaComponent) {
		return fmt.Errorf("%q is not a valid bugzilla component")
	}
	SigToBugzillaComponent[sig] = bugzillaComponent
	return nil
}
