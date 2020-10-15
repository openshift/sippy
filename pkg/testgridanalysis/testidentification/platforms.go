package testidentification

import (
	"fmt"
	"regexp"

	"github.com/openshift/sippy/pkg/util/sets"
	"k8s.io/klog"
)

var (
	// platform regexes
	awsRegex   = regexp.MustCompile(`(?i)-aws-`)
	azureRegex = regexp.MustCompile(`(?i)-azure-`)
	fipsRegex  = regexp.MustCompile(`(?i)-fips-`)
	metalRegex = regexp.MustCompile(`(?i)-metal-`)
	// metal-ipi jobs do not have a trailing -version segment
	metalIPIRegex  = regexp.MustCompile(`(?i)-metal-ipi`)
	gcpRegex       = regexp.MustCompile(`(?i)-gcp-`)
	openstackRegex = regexp.MustCompile(`(?i)-openstack-`)
	ovirtRegex     = regexp.MustCompile(`(?i)-ovirt-`)
	ovnRegex       = regexp.MustCompile(`(?i)-ovn-`)
	// proxy jobs do not have a trailing -version segment
	proxyRegex   = regexp.MustCompile(`(?i)-proxy`)
	promoteRegex = regexp.MustCompile(`(?i)^promote-`)
	ppc64leRegex = regexp.MustCompile(`(?i)-ppc64le-`)
	rtRegex      = regexp.MustCompile(`(?i)-rt-`)
	s390xRegex   = regexp.MustCompile(`(?i)-s390x-`)
	serialRegex  = regexp.MustCompile(`(?i)-serial-`)
	upgradeRegex = regexp.MustCompile(`(?i)-upgrade-`)
	// some vsphere jobs do not have a trailing -version segment
	vsphereRegex    = regexp.MustCompile(`(?i)-vsphere`)
	vsphereUPIRegex = regexp.MustCompile(`(?i)-vsphere-upi`)

	AllPlatforms = sets.NewString(
		"aws",
		"azure",
		"fips",
		"gcp",
		"metal-upi",
		"metal-ipi",
		"never-stable",
		"openstack",
		"ovirt",
		"ovn",
		"ppc64le",
		"promote",
		"proxy",
		"realtime",
		"s390x",
		"serial",
		"upgrade",
		"vsphere-ipi",
		"vsphere-upi",
	)

	// jobsNeverStableForPlatforms is a list of jobs that have never been stable (not were stable and broke)
	// As we phase these jobs in, they should be excluded from "normal" variants.
	// These jobs are still listed as jobs in total and when individual tests fail, they will still be listed with these jobs as causes.
	jobsNeverStableForPlatforms = sets.NewString(
		"release-openshift-ocp-installer-e2e-ovirt-upgrade-4.5-stable-to-4.6-ci",
		"release-openshift-origin-installer-e2e-aws-upgrade-rollback-4.5-to-4.6", // this is manual for a networking change
		"release-openshift-origin-installer-e2e-aws-disruptive-4.6",              // doesn't recover cleanly.  There is a bug.
	)
)

func IsJobNeverStable(jobName string) bool {
	return jobsNeverStableForPlatforms.Has(jobName)
}

func FindPlatform(name string) []string {
	platforms := []string{}

	defer func() {
		for _, platform := range platforms {
			if !AllPlatforms.Has(platform) {
				panic(fmt.Sprintf("coding error: missing platform: %q", platform))
			}
		}
	}()

	if IsJobNeverStable(name) {
		return []string{"never-stable"}
	}

	// if it's a promotion job, it can't be a part of any other variant aggregation
	if promoteRegex.MatchString(name) {
		platforms = append(platforms, "promote")
		return platforms
	}

	if awsRegex.MatchString(name) {
		platforms = append(platforms, "aws")
	}
	if azureRegex.MatchString(name) {
		platforms = append(platforms, "azure")
	}
	if gcpRegex.MatchString(name) {
		platforms = append(platforms, "gcp")
	}
	if openstackRegex.MatchString(name) {
		platforms = append(platforms, "openstack")
	}

	// Without support for negative lookbacks in the native
	// regexp library, it's easiest to differentiate these
	// two by seeing if it's metal-ipi, and then fall through
	// to check if it's UPI metal.
	if metalIPIRegex.MatchString(name) {
		platforms = append(platforms, "metal-ipi")
	} else if metalRegex.MatchString(name) {
		platforms = append(platforms, "metal-upi")
	}

	if ovirtRegex.MatchString(name) {
		platforms = append(platforms, "ovirt")
	}
	if vsphereUPIRegex.MatchString(name) {
		platforms = append(platforms, "vsphere-upi")
	} else if vsphereRegex.MatchString(name) {
		platforms = append(platforms, "vsphere-ipi")
	}

	if upgradeRegex.MatchString(name) {
		platforms = append(platforms, "upgrade")
	}
	if serialRegex.MatchString(name) {
		platforms = append(platforms, "serial")
	}
	if ovnRegex.MatchString(name) {
		platforms = append(platforms, "ovn")
	}
	if fipsRegex.MatchString(name) {
		platforms = append(platforms, "fips")
	}
	if ppc64leRegex.MatchString(name) {
		platforms = append(platforms, "ppc64le")
	}
	if s390xRegex.MatchString(name) {
		platforms = append(platforms, "s390x")
	}
	if rtRegex.MatchString(name) {
		platforms = append(platforms, "realtime")
	}
	if proxyRegex.MatchString(name) {
		platforms = append(platforms, "proxy")
	}

	if len(platforms) == 0 {
		klog.V(2).Infof("unknown platform for job: %s\n", name)
		return []string{"unknown platform"}
	}

	return platforms
}
