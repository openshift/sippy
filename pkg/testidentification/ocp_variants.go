package testidentification

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/go-version"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util/sets"
)

// openshiftJobsNeverStable is a list of jobs that have permafailed
// (0%) for at least two weeks. They are excluded from "normal" variants. The list
// is generated programatically via scripts/update-neverstable.sh
//
//go:embed ocp_never_stable.txt
var openshiftJobsNeverStableRaw string
var openshiftJobsNeverStable = strings.Split(openshiftJobsNeverStableRaw, "\n")

var (
	// variant regexes - when adding a new one, please update both allOpenshiftVariants,
	// and allPlatforms as appropriate.
	aggregatedRegex = regexp.MustCompile(`(?i)aggregated-`)
	alibabaRegex    = regexp.MustCompile(`(?i)-alibaba`)
	arm64Regex      = regexp.MustCompile(`(?i)-arm64`)
	assistedRegex   = regexp.MustCompile(`(?i)-assisted`)
	awsRegex        = regexp.MustCompile(`(?i)-aws`)
	azureRegex      = regexp.MustCompile(`(?i)-azure`)
	compactRegex    = regexp.MustCompile(`(?i)-compact`)
	fipsRegex       = regexp.MustCompile(`(?i)-fips`)
	hypershiftRegex = regexp.MustCompile(`(?i)-hypershift`)
	libvirtRegex    = regexp.MustCompile(`(?i)-libvirt`)
	metalRegex      = regexp.MustCompile(`(?i)-metal`)
	// metal-assisted jobs do not have a trailing -version segment
	metalAssistedRegex = regexp.MustCompile(`(?i)-metal-assisted`)
	// metal-ipi jobs do not have a trailing -version segment
	metalIPIRegex   = regexp.MustCompile(`(?i)-metal-ipi`)
	microshiftRegex = regexp.MustCompile(`(?i)-microshift`)
	// Variant for Heterogeneous
	multiRegex = regexp.MustCompile(`(?i)-heterogeneous`)
	// 3.11 gcp jobs don't have a trailing -version segment
	gcpRegex       = regexp.MustCompile(`(?i)-gcp`)
	openstackRegex = regexp.MustCompile(`(?i)-openstack`)
	osdRegex       = regexp.MustCompile(`(?i)-osd`)
	ovirtRegex     = regexp.MustCompile(`(?i)-ovirt`)
	ovnRegex       = regexp.MustCompile(`(?i)-ovn`)
	// proxy jobs do not have a trailing -version segment
	powervsRegex      = regexp.MustCompile(`(?i)-ppc64le-powervs`)
	ppc64leRegex      = regexp.MustCompile(`(?i)-ppc64le`)
	promoteRegex      = regexp.MustCompile(`(?i)^promote-`)
	proxyRegex        = regexp.MustCompile(`(?i)-proxy`)
	rtRegex           = regexp.MustCompile(`(?i)-rt`)
	s390xRegex        = regexp.MustCompile(`(?i)-s390x`)
	sdnRegex          = regexp.MustCompile(`(?i)-sdn`)
	serialRegex       = regexp.MustCompile(`(?i)-serial`)
	singleNodeRegex   = regexp.MustCompile(`(?i)-single-node`)
	techpreview       = regexp.MustCompile(`(?i)-techpreview`)
	upgradeMinorRegex = regexp.MustCompile(`(?i)(-\d+\.\d+-.*-.*-\d+\.\d+)|(-\d+\.\d+-minor)`)
	upgradeRegex      = regexp.MustCompile(`(?i)-upgrade`)
	// some vsphere jobs do not have a trailing -version segment
	vsphereRegex    = regexp.MustCompile(`(?i)-vsphere`)
	vsphereUPIRegex = regexp.MustCompile(`(?i)-vsphere-upi`)

	allOpenshiftVariants = sets.NewString(
		"alibaba",
		"amd64",
		"arm64",
		"assisted",
		"aws",
		"azure",
		"compact",
		"fips",
		"gcp",
		"ha",
		"hypershift",
		"heterogeneous",
		"libvirt",
		"metal-assisted",
		"metal-ipi",
		"metal-upi",
		"microshift",
		"never-stable",
		"openstack",
		"osd",
		"ovirt",
		"ovn",
		"powervs",
		"ppc64le",
		"promote",
		"proxy",
		"realtime",
		"s390x",
		"sdn",
		"serial",
		"single-node",
		"techpreview",
		"upgrade",
		"upgrade-micro",
		"upgrade-minor",
		"vsphere-ipi",
		"vsphere-upi",
	)

	allPlatforms = sets.NewString(
		"alibaba",
		"aws",
		"azure",
		"gcp",
		"libvirt",
		"metal-assisted",
		"metal-ipi",
		"metal-upi",
		"openstack",
		"ovirt",
		"vsphere-ipi",
		"vsphere-upi",
	)
)

type openshiftVariants struct{}

func NewOpenshiftVariantManager() VariantManager {
	return openshiftVariants{}
}

func (openshiftVariants) AllVariants() sets.String {
	return allOpenshiftVariants
}
func (openshiftVariants) AllPlatforms() sets.String {
	return allPlatforms
}

func compareAndSelectVariant(jobNameVariant, clusterVariant, variantKey string) string {
	val := jobNameVariant

	if clusterVariant != "" {
		if val != "" && clusterVariant != val {
			log.Errorf("ClusterData %s: %s, does not match jobName %s: %s", variantKey, clusterVariant, variantKey, jobNameVariant)
		}
		// defer to the clusterVariant if it is a known openshift variant
		if allOpenshiftVariants.Has(clusterVariant) {
			val = clusterVariant
		}
	}

	return val
}

func (v openshiftVariants) IdentifyVariants(jobName, release string, jobVariants models.ClusterData) []string {
	variants := []string{}

	defer func() {
		for _, variant := range variants {
			if !allOpenshiftVariants.Has(variant) {
				panic(fmt.Sprintf("coding error: missing variant: %q", variant))
			}
		}
	}()

	// Terminal variants -- these are excluded from other possible variant aggregations
	if v.IsJobNeverStable(jobName) {
		return []string{"never-stable"}
	}
	if promoteRegex.MatchString(jobName) {
		variants = append(variants, "promote")
		return variants
	}

	// If a job is an aggregation, it should only be bucketed in
	// `aggregated`. Pushing it into other variants causes unwanted
	// hysteresis for test and job pass rates. The jobs that compose
	// an aggregated job are already in Sippy.
	if aggregatedRegex.MatchString(jobName) {
		return []string{"aggregated"}
	}

	// Platform
	platform := compareAndSelectVariant(determinePlatform(jobName, release), jobVariants.Platform, "Platform")
	if platform != "" {
		variants = append(variants, platform)
	}

	// Architectures
	arch := compareAndSelectVariant(determineArchitecture(jobName, release), jobVariants.Architecture, "Architecture")
	if arch != "" {
		variants = append(variants, arch)
	}

	// Network
	network := compareAndSelectVariant(determineNetwork(jobName, release), jobVariants.Network, "Network")
	if network != "" {
		variants = append(variants, network)
	}

	// Upgrade
	// TODO: consider adding jobType.FromRelease and jobType.Release comparisons for determining minor / micro, if desirable
	if upgradeRegex.MatchString(jobName) {
		variants = append(variants, "upgrade")
		if upgradeMinorRegex.MatchString(jobName) {
			variants = append(variants, "upgrade-minor")
		} else {
			variants = append(variants, "upgrade-micro")
		}
	}

	// Topology
	// external == hypershift hosted
	if singleNodeRegex.MatchString(jobName) {
		variants = append(variants, compareAndSelectVariant("single-node", jobVariants.Topology, "Topology"))
	} else {
		variants = append(variants, compareAndSelectVariant("ha", jobVariants.Topology, "Topology"))
	}

	// Other
	if microshiftRegex.MatchString(jobName) {
		variants = append(variants, "microshift")
	}
	if hypershiftRegex.MatchString(jobName) {
		variants = append(variants, "hypershift")
	}
	if serialRegex.MatchString(jobName) {
		variants = append(variants, "serial")
	}
	if assistedRegex.MatchString(jobName) {
		variants = append(variants, "assisted")
	}
	if compactRegex.MatchString(jobName) {
		variants = append(variants, "compact")
	}
	if osdRegex.MatchString(jobName) {
		variants = append(variants, "osd")
	}
	if fipsRegex.MatchString(jobName) {
		variants = append(variants, "fips")
	}
	if techpreview.MatchString(jobName) {
		variants = append(variants, "techpreview")
	}
	if rtRegex.MatchString(jobName) {
		variants = append(variants, "realtime")
	}
	if proxyRegex.MatchString(jobName) {
		variants = append(variants, "proxy")
	}

	if len(variants) == 0 {
		log.Infof("unknown variant for job: %s\n", jobName)
		return []string{"unknown variant"}
	}

	return variants
}

func determinePlatform(jobName, _ string) string {
	// Platforms
	if alibabaRegex.MatchString(jobName) {
		return "alibaba"
	} else if awsRegex.MatchString(jobName) {
		return "aws"
	} else if azureRegex.MatchString(jobName) {
		return "azure"
	} else if gcpRegex.MatchString(jobName) {
		return "gcp"
	} else if libvirtRegex.MatchString(jobName) {
		return "libvirt"
	} else if metalAssistedRegex.MatchString(jobName) || (metalRegex.MatchString(jobName) && singleNodeRegex.MatchString(jobName)) {
		// Without support for negative lookbacks in the native
		// regexp library, it's easiest to differentiate these
		// three by seeing if it's metal-assisted or metal-ipi, and then fall through
		// to check if it's UPI metal.
		return "metal-assisted"
	} else if metalIPIRegex.MatchString(jobName) {
		return "metal-ipi"
	} else if metalRegex.MatchString(jobName) {
		return "metal-upi"
	} else if openstackRegex.MatchString(jobName) {
		return "openstack"
	} else if ovirtRegex.MatchString(jobName) {
		return "ovirt"
	} else if vsphereUPIRegex.MatchString(jobName) {
		return "vsphere-upi"
	} else if vsphereRegex.MatchString(jobName) {
		return "vsphere-ipi"
	}

	return ""
}

func determineArchitecture(jobName, _ string) string {
	if arm64Regex.MatchString(jobName) {
		return "arm64"
	} else if powervsRegex.MatchString(jobName) {
		return "ppc64le"
	} else if ppc64leRegex.MatchString(jobName) {
		return "ppc64le"
	} else if s390xRegex.MatchString(jobName) {
		return "s390x"
	} else if multiRegex.MatchString(jobName) {
		return "heterogeneous"
	} else {
		return "amd64"
	}
}

func determineNetwork(jobName, release string) string {
	if ovnRegex.MatchString(jobName) {
		return "ovn"
	} else if sdnRegex.MatchString(jobName) {
		return "sdn"
	} else {
		// If no explicit version, guess based on release
		ovnBecomesDefault, _ := version.NewVersion("4.12")
		releaseVersion, err := version.NewVersion(release)
		if err != nil {
			log.Warningf("could not determine network type for %q", jobName)
			return ""
		} else if releaseVersion.GreaterThanOrEqual(ovnBecomesDefault) {
			return "ovn"
		} else {
			return "sdn"
		}
	}
}

func (openshiftVariants) IsJobNeverStable(jobName string) bool {
	for _, ns := range openshiftJobsNeverStable {
		if ns == jobName {
			return true
		}
	}

	return false
}
