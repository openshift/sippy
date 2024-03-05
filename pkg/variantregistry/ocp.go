package variantregistry

import (
	"context"
	_ "embed"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/hashicorp/go-version"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

// OCPVariantLoader generates a mapping of job names to their variant map for all known jobs.
type OCPVariantLoader struct {
	BigQueryClient *bigquery.Client
	VariantManager testidentification.VariantManager
	bkt            *storage.BucketHandle
}

func NewOCPVariantLoader(
	bigQueryClient *bigquery.Client,
	gcsClient *storage.Client,
	gcsBucket string) *OCPVariantLoader {

	bkt := gcsClient.Bucket(gcsBucket)
	return &OCPVariantLoader{
		BigQueryClient: bigQueryClient,
		bkt:            bkt,
	}

}

type prowJobLastRun struct {
	JobName  string              `bigquery:"prowjob_job_name"`
	JobRunID string              `bigquery:"prowjob_build_id"`
	URL      bigquery.NullString `bigquery:"prowjob_url"`
}

// LoadExpectedJobVariants queries all known jobs from the gce-devel "jobs" table (actually contains job runs).
// This effectively is every job that actually ran in the last several years.
func (v *OCPVariantLoader) LoadExpectedJobVariants(ctx context.Context) (map[string]map[string]string, error) {
	log := logrus.WithField("func", "LoadExpectedJobVariants")
	log.Info("Loading all known jobs")
	start := time.Now()

	// TODO: pull presubmits for sippy as well

	// For the primary list of all job names, we will query everything that's run in the last 3 months:
	// TODO: for component readiness queries to work in the past, we may need far more than jobs that ran in 3 months
	// since start of 4.14 perhaps?
	query := v.BigQueryClient.Query(`SELECT prowjob_job_name, MAX(prowjob_url) AS prowjob_url, MAX(prowjob_build_id) AS prowjob_build_id FROM ` +
		"`ci_analysis_us.jobs` " +
		`WHERE (prowjob_job_name LIKE 'periodic-%%' OR prowjob_job_name LIKE 'release-%%' OR prowjob_job_name like 'aggregator-%%')
		GROUP BY prowjob_job_name
		LIMIT 20`)
	// TODO: ^^ remove limit
	it, err := query.Read(context.TODO())
	if err != nil {
		return nil, errors.Wrap(err, "error querying primary list of all jobs")
	}

	expectedVariants := map[string]map[string]string{}

	// Using a set since sometimes bigquery has multiple copies of the same prow job
	//prowJobs := map[string]bool{}
	count := 0
	for {
		jlr := prowJobLastRun{}
		err := it.Next(&jlr)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing prowjob name from bigquery")
			return nil, err
		}
		clusterData := map[string]string{}
		jLog := log.WithField("job", jlr.JobName)
		if jlr.URL.Valid {
			path, err := prowloader.GetGCSPathForProwJobURL(jLog, jlr.URL.StringVal)
			if err != nil {
				jLog.WithError(err).WithField("prowJobURL", jlr.URL).Error("error getting GCS path for prow job URL")
				return nil, err
			}
			gcsJobRun := gcs.NewGCSJobRun(v.bkt, path)
			allMatches := gcsJobRun.FindAllMatches([]*regexp.Regexp{gcs.GetDefaultClusterDataFile()})
			var clusterMatches []string
			if len(allMatches) > 0 {
				clusterMatches = allMatches[0]
			}
			jLog.WithField("prowJobURL", jlr.URL.StringVal).Debugf("Found %d cluster-data files: %s", len(clusterMatches), clusterMatches)

			if len(clusterMatches) > 0 {
				clusterDataBytes, err := prowloader.GetClusterDataBytes(ctx, v.bkt, path, clusterMatches)
				if err != nil {
					jLog.WithError(err).Error("unable to read cluster data file, proceeding without")
				}
				clusterData, err = prowloader.ParseVariantDataFile(clusterDataBytes)
				if err != nil {
					jLog.WithError(err).Error("unable to parse cluster data file, proceeding without")
				} else {
					jLog.Debugf("loaded cluster data: %+v", clusterData)
					// TODO: do something with it
				}
			}
		}

		variants := v.CalculateVariantsForJob(jLog, jlr.JobName, clusterData)
		count++
		jLog.WithField("variants", variants).WithField("count", count).Info("calculated variants")
		expectedVariants[jlr.JobName] = variants
	}
	dur := time.Now().Sub(start)
	log.WithField("count", count).Infof("processed primary job list in %s", dur)

	// TODO: load the current registry job to variant mappings. join and then iterate building out go structure.
	// keep variants list sorted for comparisons.

	// build out a data structure mapping job name to variant key/value pairs:

	return expectedVariants, nil
}

// fileVariantsToIgnore are values in the cluster-data.json that vary by run, and are not consistent for the job itself.
// These are unsuited for variants.
var fileVariantsToIgnore = map[string]bool{
	"CloudRegion":        true,
	"CloudZone":          true,
	"MasterNodesUpdated": true,
}

func (v *OCPVariantLoader) CalculateVariantsForJob(jLog logrus.FieldLogger, jobName string, variantFile map[string]string) map[string]string {

	// Calculate variants based on job name:
	variants := v.IdentifyVariants(jLog, jobName)

	// Carefully merge in the values read from cluster-data.json or any arbitrary variants data file
	// containing a map. Some properties will be ignored as they are job RUN specific, not job specific.
	// Others we need to carefully decide who wins in the event is a mismatch.
	for k, v := range variantFile {
		if fileVariantsToIgnore[k] {
			continue
		}
		jnv, ok := variants[k]
		if !ok {
			// Job name did not return this variant, use the value from the file
			variants[k] = v
			continue
		} else if jnv != v {
			// Check and log mismatches between what we read from the file vs determined from job name:
			jLog.WithFields(logrus.Fields{
				"variant":  k,
				"fromJob":  jnv,
				"fromFile": v,
			}).Warnf("variant mismatch")

			switch k {
			case VariantArch:
				// Job name identification wins for arch, heterogenous jobs can show cluster data with
				// amd64 as it's read from a single node.
				jLog.Warnf("using %s from job name", k)
				continue
			default:
				jLog.Warnf("using %s from job run variants file", k)
				variants[k] = v
			}
		}
	}

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
	VariantArch          = "Architecture"
	VariantFeatureSet    = "FeatureSet" // techpreview / standard
	VariantInstaller     = "Installer"  // ipi / upi / assisted
	VariantNetwork       = "Network"
	VariantNetworkAccess = "NetworkAccess" // disconnected / proxy / standard
	// TODO
	VariantNetworkStack     = "NetworkStack" // ipv4 / ipv6 / dual
	VariantOwner            = "Owner"        // eng / osd
	VariantPlatform         = "Platform"
	VariantScheduler        = "Scheduler"    // realtime / standard
	VariantSecurityMode     = "SecurityMode" // fips / default
	VariantSuite            = "Suite"        // parallel / serial
	VariantTopology         = "Topology"     // ha / single / compact / external
	VariantUpgrade          = "Upgrade"
	VariantRelease          = "Release"
	VariantReleaseMinor     = "ReleaseMinor"
	VariantReleaseMajor     = "ReleaseMajor"
	VariantFromRelease      = "FromRelease"
	VariantFromReleaseMinor = "FromReleaseMinor"
	VariantFromReleaseMajor = "FromReleaseMajor"
)

func (v *OCPVariantLoader) IdentifyVariants(jLog logrus.FieldLogger, jobName string) map[string]string {
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

	release, fromRelease := extractReleases(jobName)
	variants[VariantRelease] = release
	variants[VariantFromRelease] = fromRelease
	if release != "" {
		majMin := strings.Split(release, ".")
		variants[VariantReleaseMajor] = majMin[0]
		variants[VariantReleaseMinor] = majMin[1]
	}
	if fromRelease != "" {
		majMin := strings.Split(fromRelease, ".")
		variants[VariantFromReleaseMajor] = majMin[0]
		variants[VariantFromReleaseMinor] = majMin[1]
	}

	determinePlatform(jLog, variants, jobName)

	arch := determineArchitecture(jobName)
	if arch != "" {
		variants[VariantArch] = arch
	}

	// Network
	network := determineNetwork(jLog, jobName, fromRelease)
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
		variants[VariantTopology] = "single" // previously single-node
	} else if hypershiftRegex.MatchString(jobName) {
		variants[VariantTopology] = "external"
	} else if compactRegex.MatchString(jobName) {
		variants[VariantTopology] = "compact"
	} else if microshiftRegex.MatchString(jobName) { // No jobs for this in 4.15 - 4.16 that I can see.
		variants[VariantTopology] = "microshift"
	} else {
		variants[VariantTopology] = "ha"
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
		jLog.WithField("job", jobName).Warn("unable to determine any variants for job")
		return map[string]string{}
	}

	return variants
}

func determinePlatform(jLog logrus.FieldLogger, variants map[string]string, jobName string) {
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
		jLog.WithField("jobName", jobName).Warn("unable to determine platform from job name")
	}
	variants[VariantPlatform] = platform
}

func extractReleases(jobName string) (release, fromRelease string) {
	re := regexp.MustCompile(`\d+\.\d+`)
	matches := re.FindAllString(jobName, -1)

	if len(matches) > 0 {
		minRelease := matches[0]
		maxRelease := matches[0]

		for _, match := range matches {
			matchNum, _ := strconv.ParseFloat(match, 64)
			minNum, _ := strconv.ParseFloat(minRelease, 64)
			maxNum, _ := strconv.ParseFloat(maxRelease, 64)

			if matchNum < minNum {
				minRelease = match
			}

			if matchNum > maxNum {
				maxRelease = match
			}
		}

		release = maxRelease
		fromRelease = minRelease
	}

	return release, fromRelease
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

func determineNetwork(jLog logrus.FieldLogger, jobName, release string) string {
	if ovnRegex.MatchString(jobName) {
		return "ovn"
	} else if sdnRegex.MatchString(jobName) {
		return "sdn"
	} else {
		// If no explicit version, guess based on release
		ovnBecomesDefault, _ := version.NewVersion("4.12")
		releaseVersion, err := version.NewVersion(release)
		if err != nil {
			jLog.Warningf("could not determine network type for %q", jobName)
			return ""
		} else if releaseVersion.GreaterThanOrEqual(ovnBecomesDefault) {
			return "ovn"
		} else {
			return "sdn"
		}
	}
}
