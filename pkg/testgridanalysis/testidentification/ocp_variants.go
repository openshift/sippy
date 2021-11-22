package testidentification

import (
	"fmt"
	"regexp"

	"github.com/openshift/sippy/pkg/util/sets"
	"k8s.io/klog"
)

var (
	// variant regexes
	arm64Regex   = regexp.MustCompile(`(?i)-arm64`)
	awsRegex     = regexp.MustCompile(`(?i)-aws`)
	azureRegex   = regexp.MustCompile(`(?i)-azure`)
	compactRegex = regexp.MustCompile(`(?i)-compact`)
	fipsRegex    = regexp.MustCompile(`(?i)-fips`)
	metalRegex   = regexp.MustCompile(`(?i)-metal`)
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
		"periodic-ci-openshift-release-master-ci-4.9-e2e-aws-hypershift",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-hypershift",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-azurestack-csi",

		// Single-node openshift jobs are currently failing conformance
		// tests fairly often, and the team is currently working on bringing
		// the pass percentage up.
		"periodic-ci-openshift-multiarch-master-nightly-4.10-ocp-e2e-aws-arm64-single-node",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-upgrade-single-node",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-azure-upgrade-single-node",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-single-node",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-single-node-serial",

		// TODO: Figure out where to report problems for these
		"periodic-ci-openshift-verification-tests-master-ocp-4.10-e2e-aws-cucushift-ipi",
		"periodic-ci-openshift-verification-tests-master-ocp-4.10-e2e-gcp-cucushift-ipi",
		"periodic-ci-openshift-verification-tests-master-ocp-4.10-e2e-baremetal-cucushift-ipi",
		"periodic-ci-openshift-verification-tests-master-ocp-4.10-e2e-baremetal-cucushift-ipi",
		"periodic-ci-openshift-verification-tests-master-ocp-4.10-e2e-openstack-cucushift-ipi",

		// Reported on slack to SD-CICD
		"release-openshift-ocp-osd-aws-nightly-4.10",
		"release-openshift-ocp-osd-gcp-nightly-4.10",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2019375
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-openstack-proxy",

		// TODO: Add bug, currently being investigated
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-metal-ipi-compact",
		"release-openshift-ocp-installer-e2e-metal-compact-4.10",

		// https://bugzilla.redhat.com/show_bug.cgi?id=1979966
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-aws-workers-rhel7",
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-workers-rhel7",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2003646
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-aws-workers-rhel8",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2008201
		"periodic-ci-openshift-release-master-nightly-4.10-e2e-openstack-az",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2007692
		"release-openshift-origin-installer-old-rhcos-e2e-aws-4.9",
		"release-openshift-origin-installer-old-rhcos-e2e-aws-4.10",

		// https://bugzilla.redhat.com/show_bug.cgi?id=1936917
		"periodic-ci-openshift-release-master-ci-4.9-e2e-aws-calico",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-calico",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2007580
		"periodic-ci-openshift-release-master-ci-4.9-e2e-azure-cilium",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-azure-cilium",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2006947
		"periodic-ci-openshift-release-master-nightly-4.9-e2e-aws-proxy",

		// https://bugzilla.redhat.com/show_bug.cgi?id=1997345
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-compact-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-remote-libvirt-ppc64le",
		"periodic-ci-openshift-multiarch-master-nightly-4.9-ocp-e2e-remote-libvirt-s390x",

		// https://bugzilla.redhat.com/show_bug.cgi?id=1989100
		"periodic-ci-openshift-release-master-ci-4.9-e2e-openstack-ovn",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-openstack-ovn",

		// https://bugzilla.redhat.com/show_bug.cgi?id=2019376
		"periodic-ci-openshift-release-master-ci-4.9-e2e-aws-network-stress",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-network-stress",
		"periodic-ci-openshift-release-master-ci-4.9-e2e-aws-ovn-network-stress",
		"periodic-ci-openshift-release-master-ci-4.10-e2e-aws-ovn-network-stress",
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
