package testidentification

import (
	"fmt"
	"regexp"

	"github.com/openshift/sippy/pkg/util/sets"

	"k8s.io/klog"
)

var (
	// platform regexes
	awsRegex       = regexp.MustCompile(`(?i)-aws-`)
	azureRegex     = regexp.MustCompile(`(?i)-azure-`)
	fipsRegex      = regexp.MustCompile(`(?i)-fips-`)
	metalRegex     = regexp.MustCompile(`(?i)-metal-`)
	metalIPIRegex  = regexp.MustCompile(`(?i)-metal-ipi`)
	gcpRegex       = regexp.MustCompile(`(?i)-gcp`)
	ocpRegex       = regexp.MustCompile(`(?i)-ocp-`)
	openstackRegex = regexp.MustCompile(`(?i)-openstack-`)
	originRegex    = regexp.MustCompile(`(?i)-origin-`)
	ovirtRegex     = regexp.MustCompile(`(?i)-ovirt-`)
	ovnRegex       = regexp.MustCompile(`(?i)-ovn-`)
	proxyRegex     = regexp.MustCompile(`(?i)-proxy`)
	ppc64leRegex   = regexp.MustCompile(`(?i)-ppc64le-`)
	rtRegex        = regexp.MustCompile(`(?i)-rt-`)
	s390xRegex     = regexp.MustCompile(`(?i)-s390x-`)
	serialRegex    = regexp.MustCompile(`(?i)-serial-`)
	upgradeRegex   = regexp.MustCompile(`(?i)-upgrade-`)
	vsphereRegex   = regexp.MustCompile(`(?i)-vsphere-`)

	AllPlatforms = sets.NewString(
		"ocp",
		"origin",
		"aws",
		"azure",
		"gcp",
		"openstack",
		"metal-ipi",
		"metal",
		"ovirt",
		"vsphere",
		"upgrade",
		"serial",
		"ovn",
		"fips",
		"ppc64le",
		"s390x",
		"rt",
		"proxy",
	)
)

func FindPlatform(name string) []string {
	platforms := []string{}
	if ocpRegex.MatchString(name) {
		platforms = append(platforms, "ocp")
	}
	if originRegex.MatchString(name) {
		platforms = append(platforms, "origin")
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
		platforms = append(platforms, "metal")
	}

	if ovirtRegex.MatchString(name) {
		platforms = append(platforms, "ovirt")
	}
	if vsphereRegex.MatchString(name) {
		platforms = append(platforms, "vsphere")
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
		platforms = append(platforms, "rt")
	}
	if proxyRegex.MatchString(name) {
		platforms = append(platforms, "proxy")
	}

	if len(platforms) == 0 {
		klog.V(2).Infof("unknown platform for job: %s\n", name)
		return []string{"unknown platform"}
	}

	for _, platform := range platforms {
		if !AllPlatforms.Has(platform) {
			panic(fmt.Sprintf("coding error: missing platform: %q", platform))
		}
	}
	return platforms
}
