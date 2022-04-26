package testidentification

import (
	"fmt"
	"regexp"

	"github.com/openshift/sippy/pkg/util/sets"
	"k8s.io/klog"
)

var (
	// variant regexes
	arm64Regex    = regexp.MustCompile(`(?i)-arm64`)
	assistedRegex = regexp.MustCompile(`(?i)-assisted`)
	awsRegex      = regexp.MustCompile(`(?i)-aws`)
	azureRegex    = regexp.MustCompile(`(?i)-azure`)
	compactRegex  = regexp.MustCompile(`(?i)-compact`)
	fipsRegex     = regexp.MustCompile(`(?i)-fips`)
	metalRegex    = regexp.MustCompile(`(?i)-metal`)
	// metal-assisted jobs do not have a trailing -version segment
	metalAssistedRegex = regexp.MustCompile(`(?i)-metal-assisted`)
	// metal-ipi jobs do not have a trailing -version segment
	metalIPIRegex = regexp.MustCompile(`(?i)-metal-ipi`)
	// 3.11 gcp jobs don't have a trailing -version segment
	gcpRegex       = regexp.MustCompile(`(?i)-gcp`)
	openstackRegex = regexp.MustCompile(`(?i)-openstack`)
	osdRegex       = regexp.MustCompile(`(?i)-osd`)
	ovirtRegex     = regexp.MustCompile(`(?i)-ovirt`)
	ovnRegex       = regexp.MustCompile(`(?i)-ovn`)
	// proxy jobs do not have a trailing -version segment
	proxyRegex   = regexp.MustCompile(`(?i)-proxy`)
	promoteRegex = regexp.MustCompile(`(?i)^promote-`)
	ppc64leRegex = regexp.MustCompile(`(?i)-ppc64le`)
	rtRegex      = regexp.MustCompile(`(?i)-rt`)
	s390xRegex   = regexp.MustCompile(`(?i)-s390x`)
	serialRegex  = regexp.MustCompile(`(?i)-serial`)
	techpreview  = regexp.MustCompile(`(?i)-techpreview`)
	upgradeRegex = regexp.MustCompile(`(?i)-upgrade`)
	// some vsphere jobs do not have a trailing -version segment
	vsphereRegex    = regexp.MustCompile(`(?i)-vsphere`)
	vsphereUPIRegex = regexp.MustCompile(`(?i)-vsphere-upi`)
	singleNodeRegex = regexp.MustCompile(`(?i)-single-node`)

	allOpenshiftVariants = sets.NewString(
		"arm64",
		"assisted",
		"aws",
		"azure",
		"compact",
		"fips",
		"gcp",
		"metal-assisted",
		"metal-upi",
		"metal-ipi",
		"never-stable",
		"openstack",
		"osd",
		"ovirt",
		"ovn",
		"ppc64le",
		"promote",
		"proxy",
		"realtime",
		"s390x",
		"serial",
		"techpreview",
		"upgrade",
		"vsphere-ipi",
		"vsphere-upi",
		"single-node",
	)

	// openshiftJobsNeverStableForVariants is a list of unproven new jobs or
	// jobs that are near permafail (i.e. < 40%) for an extended period of time.
	// They are excluded from "normal" variants and once they are passing above
	// 40% can "graduated" from never-stable.
	//
	// Jobs should have a linked BZ before being added to this list.
	//
	// These jobs are still listed as jobs in total and when individual
	// tests fail, they will still be listed with these jobs as causes.
	openshiftJobsNeverStableForVariants = sets.NewString(
		// Experimental jobs that are under active development
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-azurestack-csi",
		"periodic-ci-openshift-release-master-nightly-4.11-e2e-azurestack-csi",

		// These will fail until there's a stable 4.10 build
		"periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-aws-upgrade-infra",
		"periodic-ci-openshift-release-master-ci-4.11-upgrade-from-stable-4.10-e2e-azure-ovn-upgrade",
		"periodic-ci-openshift-release-master-nightly-4.11-upgrade-from-stable-4.10-e2e-metal-ipi-upgrade-ovn-ipv6",
		"periodic-ci-shiftstack-shiftstack-ci-main-periodic-4.11-upgrade-from-stable-4.10-e2e-openstack-upgrade",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2057502
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-telco5g",
		"periodic-ci-openshift-release-master-nightly-4.11-e2e-telco5g",

		// Single-node openshift jobs are currently failing conformance
		// tests fairly often, and the team is currently working on bringing
		// the pass percentage up.
		"periodic-ci-openshift-multiarch-master-nightly-4.10-ocp-e2e-aws-arm64-single-node",
		"periodic-ci-openshift-multiarch-master-nightly-4.11-ocp-e2e-aws-arm64-single-node",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-upgrade-single-node",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-azure-upgrade-single-node",
		"periodic-ci-openshift-release-master-ci-4.11-e2e-aws-upgrade-single-node",
		"periodic-ci-openshift-release-master-ci-4.11-e2e-azure-upgrade-single-node",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-single-node",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-single-node-serial",
		"periodic-ci-openshift-release-master-nightly-4.11-e2e-aws-single-node",
		"periodic-ci-openshift-release-master-nightly-4.11-e2e-aws-single-node-serial",

		// QE jobs, formerly named with "cucushift", tracked here: https://issues.redhat.com/browse/OCPQE-8577
		"periodic-ci-openshift-verification-tests-master-nightly-4.10-upgrade-from-stable-4.9-azure-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.10-e2e-baremetal-ipi",
		"periodic-ci-openshift-verification-tests-master-stable-4.10-e2e-azure-ipi",
		"periodic-ci-openshift-verification-tests-master-stable-4.10-upgrade-from-stable-4.9-azure-ipi",
		"periodic-ci-openshift-verification-tests-master-stable-4.10-e2e-baremetal-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.10-e2e-aws-ipi-destructive",
		"periodic-ci-openshift-verification-tests-master-stable-4.10-upgrade-from-stable-4.9-openstack-ipi",
		"periodic-ci-openshift-verification-tests-master-stable-4.10-e2e-aws-ipi-destructive",
		"periodic-ci-openshift-verification-tests-master-nightly-4.10-upgrade-from-stable-4.9-openstack-ipi",
		"periodic-ci-openshift-verification-tests-master-stable-4.10-upgrade-from-stable-4.9-baremetal-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.10-upgrade-from-stable-4.9-baremetal-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.10-e2e-azure-ipi",
		// And their 4.11 equivalents, tracked with new jira: https://issues.redhat.com/browse/OCPQE-9898
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-azure-ipi-proxy",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-aws-ipi-private",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-vsphere-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-aws-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-azure-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-vsphere-upi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-gcp-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-gcp-upi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-aws-ipi-proxy",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-e2e-vsphere-ipi-proxy",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-upgrade-from-stable-4.10-aws-ipi",
		"periodic-ci-openshift-verification-tests-master-nightly-4.11-upgrade-from-stable-4.10-azure-ipi",

		// Reported on slack to SD-CICD
		"release-openshift-ocp-osd-aws-nightly-4.10",
		"release-openshift-ocp-osd-gcp-nightly-4.10",
		"release-openshift-ocp-osd-aws-nightly-4.11",
		"release-openshift-ocp-osd-gcp-nightly-4.11",

		// These jobs are being moved to the step registry.
		// https://bugzilla.redhat.com/show_bug.cgi?id=2057582
		"release-openshift-ocp-installer-e2e-metal-4.10",
		"release-openshift-ocp-installer-e2e-metal-4.11",
		"release-openshift-ocp-installer-e2e-metal-compact-4.10",
		"release-openshift-ocp-installer-e2e-metal-compact-4.11",
		"release-openshift-ocp-installer-e2e-metal-serial-4.10",
		"release-openshift-ocp-installer-e2e-metal-serial-4.11",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2058266
		"periodic-ci-openshift-multiarch-master-nightly-4.10-ocp-e2e-aws-ovn-arm64",
		"periodic-ci-openshift-multiarch-master-nightly-4.10-ocp-e2e-compact-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.10-ocp-e2e-compact-remote-libvirt-s390x",
		"periodic-ci-openshift-multiarch-master-nightly-4.10-ocp-e2e-serial-aws-arm64",
		"periodic-ci-openshift-multiarch-master-nightly-4.10-upgrade-from-nightly-4.9-ocp-e2e-aws-arm64",
		"periodic-ci-openshift-multiarch-master-nightly-4.11-ocp-e2e-aws-ovn-arm64",
		"periodic-ci-openshift-multiarch-master-nightly-4.11-ocp-e2e-compact-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.11-ocp-e2e-compact-remote-libvirt-s390x",
		"periodic-ci-openshift-multiarch-master-nightly-4.11-ocp-e2e-serial-aws-arm64",
		"periodic-ci-openshift-multiarch-master-nightly-4.11-upgrade-from-nightly-4.10-ocp-e2e-aws-arm64",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-compact-remote-libvirt-s390x",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-image-ecosystem-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-image-ecosystem-remote-libvirt-s390x",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-upgrade-from-nightly-4.8-ocp-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-upgrade-from-nightly-4.8-ocp-remote-libvirt-s390x",

		// TODO: Add bug as these are investigated. These have been near 0% for more than two weeks.
		"periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-e2e-aws-ovn-upgrade-rollback",
		"periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-e2e-aws-upgrade-rollback",
		"periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-e2e-azure-ovn-upgrade",
		"periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-e2e-openstack-upgrade",
		"periodic-ci-openshift-release-master-ci-4.10-upgrade-from-stable-4.9-from-stable-4.8-e2e-aws-upgrade",
		"periodic-ci-openshift-release-master-ci-4.9-upgrade-from-stable-4.8-e2e-aws-uwm",
		"periodic-ci-openshift-release-master-ci-4.9-upgrade-from-stable-4.8-e2e-azure-ovn-upgrade",
		"periodic-ci-openshift-release-master-ci-4.9-upgrade-from-stable-4.8-e2e-gcp-ovn-upgrade",
		"periodic-ci-openshift-release-master-ci-4.9-upgrade-from-stable-4.8-e2e-openstack-upgrade",
		"periodic-ci-openshift-release-master-ci-4.9-upgrade-from-stable-4.8-from-stable-4.7-e2e-aws-upgrade",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-ovn-local-gateway",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-gcp-fips",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-gcp-libvirt-cert-rotation",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-metal-ipi-serial-compact",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-vsphere-proxy",
		"periodic-ci-openshift-release-master-nightly-4.10-upgrade-from-stable-4.8-e2e-aws-upgrade-paused",
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-aws-fips-serial",
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-aws-upgrade-rollback-oldest-supported",
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-gcp-libvirt-cert-rotation",
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-compact",
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-openstack-az",
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-openstack-fips",
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-openstack-proxy",

		"release-openshift-origin-installer-e2e-aws-disruptive-4.9",
		"release-openshift-origin-installer-e2e-aws-disruptive-4.10",
		"release-openshift-origin-installer-e2e-aws-disruptive-4.11",

		"release-openshift-origin-installer-e2e-aws-upgrade-4.6-to-4.7-to-4.8-to-4.9-ci",
		"release-openshift-origin-installer-e2e-aws-upgrade-4.7-to-4.8-to-4.9-to-4.10-ci",

		// https://bugzilla.redhatcom/show_bug.cgi?id=1979966
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-aws-workers-rhel7",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-workers-rhel7",
		"periodic-ci-openshift-release-master-nightly-4.11-e2e-aws-workers-rhel7",

		// https://bugzilla.redhat.com/show_bug.cgi?id=1936917
		"periodic-ci-openshift-release-master-ci-4.9-e2e-aws-calico",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-calico",
		"periodic-ci-openshift-release-master-ci-4.11-e2e-aws-calico",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2007580
		"periodic-ci-openshift-release-master-ci-4.10-e2e-azure-cilium",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-gcp-cilium",
		"periodic-ci-openshift-release-master-ci-4.11-e2e-azure-cilium",
		"periodic-ci-openshift-release-master-ci-4.11-e2e-gcp-cilium",
		"periodic-ci-openshift-release-master-ci-4.9-e2e-azure-cilium",
		"periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-cilium",

		// https://bugzilla.redhat.com/show_bug.cgi?id=1997345
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-compact-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-remote-libvirt-s390x",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2019376
		"periodic-ci-openshift-release-master-ci-4.9-e2e-aws-network-stress",
		"periodic-ci-openshift-release-master-ci-4.9-e2e-aws-ovn-network-stress",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-network-stress",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-ovn-network-stress",
		"periodic-ci-openshift-release-master-ci-4.11-e2e-aws-network-stress",
		"periodic-ci-openshift-release-master-ci-4.11-e2e-aws-ovn-network-stress",
	)
)

type openshiftVariants struct{}

func NewOpenshiftVariantManager() VariantManager {
	return openshiftVariants{}
}

func (openshiftVariants) AllVariants() sets.String {
	return allOpenshiftVariants
}

func (v openshiftVariants) IdentifyVariants(jobName string) []string {
	variants := []string{}

	defer func() {
		for _, variant := range variants {
			if !allOpenshiftVariants.Has(variant) {
				panic(fmt.Sprintf("coding error: missing variant: %q", variant))
			}
		}
	}()

	// If a job is in never-stable, it is excluded from other possible variant aggregations
	if v.IsJobNeverStable(jobName) {
		return []string{"never-stable"}
	}

	// Tech preview jobs are excluded from other possible variant aggregations
	if techpreview.MatchString(jobName) {
		return []string{"techpreview"}
	}

	// if it's a promotion job, it can't be a part of any other variant aggregation
	if promoteRegex.MatchString(jobName) {
		variants = append(variants, "promote")
		return variants
	}

	if arm64Regex.MatchString(jobName) {
		variants = append(variants, "arm64")
	}

	if assistedRegex.MatchString(jobName) {
		variants = append(variants, "assisted")
	}

	if awsRegex.MatchString(jobName) {
		variants = append(variants, "aws")
	}
	if azureRegex.MatchString(jobName) {
		variants = append(variants, "azure")
	}

	if compactRegex.MatchString(jobName) {
		variants = append(variants, "compact")
	}
	if gcpRegex.MatchString(jobName) {
		variants = append(variants, "gcp")
	}
	if openstackRegex.MatchString(jobName) {
		variants = append(variants, "openstack")
	}

	if osdRegex.MatchString(jobName) {
		variants = append(variants, "osd")
	}

	// Without support for negative lookbacks in the native
	// regexp library, it's easiest to differentiate these
	// three by seeing if it's metal-assisted or metal-ipi, and then fall through
	// to check if it's UPI metal.
	if metalAssistedRegex.MatchString(jobName) {
		variants = append(variants, "metal-assisted")
	} else if metalIPIRegex.MatchString(jobName) {
		variants = append(variants, "metal-ipi")
	} else if metalRegex.MatchString(jobName) {
		variants = append(variants, "metal-upi")
	}

	if ovirtRegex.MatchString(jobName) {
		variants = append(variants, "ovirt")
	}
	if vsphereUPIRegex.MatchString(jobName) {
		variants = append(variants, "vsphere-upi")
	} else if vsphereRegex.MatchString(jobName) {
		variants = append(variants, "vsphere-ipi")
	}

	if upgradeRegex.MatchString(jobName) {
		variants = append(variants, "upgrade")
	}
	if serialRegex.MatchString(jobName) {
		variants = append(variants, "serial")
	}
	if ovnRegex.MatchString(jobName) {
		variants = append(variants, "ovn")
	}
	if fipsRegex.MatchString(jobName) {
		variants = append(variants, "fips")
	}
	if ppc64leRegex.MatchString(jobName) {
		variants = append(variants, "ppc64le")
	}
	if s390xRegex.MatchString(jobName) {
		variants = append(variants, "s390x")
	}
	if rtRegex.MatchString(jobName) {
		variants = append(variants, "realtime")
	}
	if proxyRegex.MatchString(jobName) {
		variants = append(variants, "proxy")
	}
	if singleNodeRegex.MatchString(jobName) {
		variants = append(variants, "single-node")
	}

	if len(variants) == 0 {
		klog.V(2).Infof("unknown variant for job: %s\n", jobName)
		return []string{"unknown variant"}
	}

	return variants
}
func (openshiftVariants) IsJobNeverStable(jobName string) bool {
	return openshiftJobsNeverStableForVariants.Has(jobName)
}
