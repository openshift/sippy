package variantregistry

import (
	"context"
	_ "embed"
	"regexp"

	"cloud.google.com/go/bigquery"
	"github.com/hashicorp/go-version"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

// OCPVariantLoader generates a mapping of job names to their variant map for all known jobs.
type OCPVariantLoader struct {
	BigQueryClient *bigquery.Client
	VariantManager testidentification.VariantManager
}

type prowJobName struct {
	JobName string `bigquery:"prowjob_job_name"`
}

// LoadAllJobs queries all known jobs from the gce-devel "jobs" table (actually contains job runs).
// This effectively is every job that actually ran in the last several years.
func (v *OCPVariantLoader) LoadAllJobs() error {
	logrus.Info("Loading all known jobs")

	// TODO: pull presubmits for sippy as well

	// delete everything from the registry for now, we'll rebuild completely until sync logic is implemented.
	// TODO: Remove this, sync the changes we need to only, otherwise the apps will be working incorrectly while this process runs
	query := v.BigQueryClient.Query(`DELETE FROM openshift-ci-data-analysis.sippy.JobVariants WHERE true`)
	_, err := query.Read(context.TODO())
	if err != nil {
		logrus.WithError(err).Error("error clearing current registry job variants")
		return errors.Wrap(err, "error clearing current registry job variants")
	}
	logrus.Warn("deleted all current job variants in the registry")
	query = v.BigQueryClient.Query(`DELETE FROM openshift-ci-data-analysis.sippy.Jobs WHERE true`)
	_, err = query.Read(context.TODO())
	if err != nil {
		logrus.WithError(err).Error("error clearing current registry jobs")
		return errors.Wrap(err, "error clearing current registry jobs")
	}
	logrus.Warn("deleted all current jobs in the registry")

	// For the primary list of all job names, we will query everything that's run in the last 3 months:
	// TODO: for component readiness queries to work in the past, we may need far more than jobs that ran in 3 months
	// since start of 4.14 perhaps?
	query = v.BigQueryClient.Query(`SELECT prowjob_job_name, MAX(prowjob_completion), MAX(prowjob_url) FROM ` +
		"`ci_analysis_us.jobs` " +
		`WHERE (prowjob_job_name LIKE 'periodic-%%' OR prowjob_job_name LIKE 'release-%%' OR prowjob_job_name like 'aggregator-%%')
		GROUP BY prowjob_job_name`)
	it, err := query.Read(context.TODO())
	if err != nil {
		return errors.Wrap(err, "error querying primary list of all jobs")
	}

	// Using a set since sometimes bigquery has multiple copies of the same prow job
	//prowJobs := map[string]bool{}
	count := 0
	for {
		jn := prowJobName{}
		err := it.Next(&jn)
		if err == iterator.Done {
			break
		}
		if err != nil {
			logrus.WithError(err).Error("error parsing prowjob name from bigquery")
			return err
		}
		variants := v.GetVariantsForJob(jn.JobName)
		logrus.WithField("variants", variants).Debugf("calculated variants for %s", jn.JobName)

		count++
	}
	logrus.WithField("count", count).Info("loaded primary job list")

	// TODO: load the current registry job to variant mappings. join and then iterate building out go structure.
	// keep variants list sorted for comparisons.

	// build out a data structure mapping job name to variant key/value pairs:

	return nil
}

func (v *OCPVariantLoader) GetVariantsForJob(jobName string) map[string]string {
	variants := v.IdentifyVariants(jobName, "0.0")

	return variants
}

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
	etcdScaling     = regexp.MustCompile(`(?i)-etcd-scaling`)
	fipsRegex       = regexp.MustCompile(`(?i)-fips`)
	hypershiftRegex = regexp.MustCompile(`(?i)-hypershift`)
	upiRegex        = regexp.MustCompile(`(?i)-upi`)
	libvirtRegex    = regexp.MustCompile(`(?i)-libvirt`)
	metalRegex      = regexp.MustCompile(`(?i)-metal`)
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
	vsphereRegex = regexp.MustCompile(`(?i)-vsphere`)

	allOpenshiftVariants = sets.NewString(
		"alibaba",
		"amd64",
		"arm64",
		"assisted",
		"aws",
		"azure",
		"compact",
		"etcd-scaling",
		"fips",
		"gcp",
		"ha",
		"hypershift",
		"heterogeneous",
		"libvirt",
		"metal",
		"microshift",
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
		"sdn",
		"serial",
		"single-node",
		"techpreview",
		"upgrade",
		"upgrade-micro",
		"upgrade-minor",
		"vsphere",
	)

	allPlatforms = sets.NewString(
		"alibaba",
		"aws",
		"azure",
		"gcp",
		"libvirt",
		"metal-assisted",
		"metal",
		"openstack",
		"ovirt",
		"vsphere",
	)
)

const (
	VariantAggregation   = "Aggregation" // aggregated or none
	VariantArch          = "Arch"
	VariantControlPlane  = "ControlPlane"
	VariantFeatureSet    = "FeatureSet" // techpreview / standard
	VariantInstaller     = "Installer"  // ipi / upi / assisted
	VariantNetwork       = "Network"
	VariantNetworkAccess = "NetworkAccess" // disconnected / proxy / standard
	VariantNetworkStack  = "NetworkStack"  // ipv4 / ipv6 / dual
	VariantOwner         = "Owner"         // eng / osd
	VariantPlatform      = "Platform"
	VariantScheduler     = "Scheduler"    // realtime / standard
	VariantSecurityMode  = "SecurityMode" // fips / default
	VariantSuite         = "Suite"        // parallel / serial
	VariantTopology      = "Topology"     // ha / single-node / compact / external
	VariantUpgrade       = "Upgrade"
)

func (v OCPVariantLoader) IdentifyVariants(jobName, release string) map[string]string {
	variants := map[string]string{}

	/*
		defer func() {
			for _, variant := range variants {
				if !allOpenshiftVariants.Has(variant) {
					panic(fmt.Sprintf("coding error: missing variant: %q", variant))
				}
			}
		}()

	*/

	// No promote jobs in sippy db since 4.12, lets drop this variant.
	/*
		if promoteRegex.MatchString(jobName) {
			variants = append(variants, "promote")
			return variants
		}
	*/

	// If a job is an aggregation, it should only be bucketed in
	// `aggregated`. Pushing it into other variants causes unwanted
	// hysteresis for test and job pass rates. The jobs that compose
	// an aggregated job are already in Sippy.
	if aggregatedRegex.MatchString(jobName) {
		variants[VariantAggregation] = "aggregated"
		// TODO: sippy would stop here, but for the registry we probably want to keep processing,
		// and sippy will need to know to strip out other variants if it's an aggregated job
	}

	determinePlatform(variants, jobName)

	arch := determineArchitecture(jobName)
	if arch != "" {
		variants[VariantArch] = arch
	}

	// Network
	network := determineNetwork(jobName, release)
	if network != "" {
		variants[VariantNetwork] = network
	}

	if upgradeRegex.MatchString(jobName) {
		if upgradeMinorRegex.MatchString(jobName) {
			variants[VariantUpgrade] = "minor"
		} else {
			variants[VariantUpgrade] = "micro"
		}
		// TODO: add multi-upgrade
	} else {
		variants[VariantUpgrade] = "none"
	}

	// Topology
	// external == hypershift hosted
	if singleNodeRegex.MatchString(jobName) {
		variants[VariantTopology] = "single-node"
	} else if hypershiftRegex.MatchString(jobName) {
		variants[VariantTopology] = "hypershift" // or should this be external?
	} else if compactRegex.MatchString(jobName) {
		variants[VariantTopology] = "compact"
	} else {
		variants[VariantTopology] = "ha"
	}

	if hypershiftRegex.MatchString(jobName) {
		variants[VariantControlPlane] = "hypershift" // or should this be external?
	} else if microshiftRegex.MatchString(jobName) {
		variants[VariantControlPlane] = "microshift" // or should this be external?
	}

	// TODO: suite may not be the right word here
	if serialRegex.MatchString(jobName) {
		variants[VariantSuite] = "serial"
	} else if etcdScaling.MatchString(jobName) {
		variants[VariantSuite] = "etcd-scaling"
	} else {
		variants[VariantSuite] = "unknown" // parallel perhaps but lots of jobs aren't running out suites
	}

	if assistedRegex.MatchString(jobName) {
		variants[VariantInstaller] = "assisted"
	} else if upiRegex.MatchString(jobName) {
		variants[VariantInstaller] = "upi"
	} else {
		variants[VariantInstaller] = "ipi" // assume ipi by default
	}

	if osdRegex.MatchString(jobName) {
		variants[VariantOwner] = "osd"
	} else {
		variants[VariantOwner] = "eng"
	}

	if fipsRegex.MatchString(jobName) {
		variants[VariantSecurityMode] = "fips"
	} else {
		variants[VariantSecurityMode] = "default"
	}

	if techpreview.MatchString(jobName) {
		variants[VariantFeatureSet] = "techpreview"
	} else {
		variants[VariantFeatureSet] = "default"
	}

	if rtRegex.MatchString(jobName) {
		variants[VariantScheduler] = "realtime"
	} else {
		variants[VariantScheduler] = "default"
	}

	if proxyRegex.MatchString(jobName) {
		variants[VariantNetworkAccess] = "proxy"
	} else {
		variants[VariantNetworkAccess] = "default"
	}

	if len(variants) == 0 {
		log.WithField("job", jobName).Warn("unable to determine any variants for job")
		return map[string]string{}
	}

	return variants
}

func determinePlatform(variants map[string]string, jobName string) {
	platform := ""

	// Platforms
	if alibabaRegex.MatchString(jobName) {
		platform = "alibaba"
	} else if awsRegex.MatchString(jobName) {
		platform = "aws"
	} else if azureRegex.MatchString(jobName) {
		platform = "azure"
	} else if gcpRegex.MatchString(jobName) {
		platform = "gcp"
	} else if libvirtRegex.MatchString(jobName) {
		platform = "libvirt"
	} else if metalRegex.MatchString(jobName) {
		platform = "metal"
	} else if openstackRegex.MatchString(jobName) {
		platform = "openstack"
	} else if ovirtRegex.MatchString(jobName) {
		platform = "ovirt"
	} else if vsphereRegex.MatchString(jobName) {
		platform = "vsphere"
	}

	if platform == "" {
		logrus.WithField("jobName", jobName).Warn("unable to determine platform from job name")
	}
	variants[VariantPlatform] = platform
}

func determineArchitecture(jobName string) string {
	if arm64Regex.MatchString(jobName) {
		return "arm64"
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
