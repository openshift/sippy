package variantregistry

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
)

// OCPVariantLoader generates a mapping of job names to their variant map for all known jobs.
type OCPVariantLoader struct {
	BigQueryClient  *bigquery.Client
	bkt             *storage.BucketHandle
	bigQueryProject string
	bigQueryDataSet string
	bigQueryTable   string
}

func NewOCPVariantLoader(
	bigQueryClient *bigquery.Client,
	bigQueryProject string,
	bigQueryDataSet string,
	bigQueryTable string,
	gcsClient *storage.Client,
	gcsBucket string) *OCPVariantLoader {

	bkt := gcsClient.Bucket(gcsBucket)
	return &OCPVariantLoader{
		BigQueryClient:  bigQueryClient,
		bkt:             bkt,
		bigQueryProject: bigQueryProject,
		bigQueryDataSet: bigQueryDataSet,
		bigQueryTable:   bigQueryTable,
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
	log.Info("loading all known jobs from bigquery for variant classification")
	start := time.Now()

	// For the primary list of all job names, we will query everything that's run in the last 6 months. Because
	// we also try to pull cluster-data.json, we also join in a column for the prowjob_url of the most recent
	// successful run to attempt to get stable cluster-data. Without this, the jobs would bounce around variants
	// subtly when we hit runs without cluster-data.
	queryStr := `
WITH RecentSuccessfulJobs AS (
  SELECT 
    prowjob_job_name,
    MAX(prowjob_start) AS successful_start,
    MAX(prowjob_url) as prowjob_url,
  FROM DATASET
  WHERE prowjob_start > DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 180 DAY) AND
        prowjob_state = 'success' AND
        (prowjob_job_name LIKE 'periodic-ci-openshift-%%'
          OR prowjob_job_name LIKE 'periodic-ci-shiftstack-%%'
          OR prowjob_job_name LIKE 'release-%%'
          OR prowjob_job_name LIKE 'aggregator-%%'
          OR prowjob_job_name LIKE 'pull-ci-openshift-%%')
  GROUP BY prowjob_job_name
)
SELECT 
  j.prowjob_job_name,
  MAX(j.prowjob_start) AS prowjob_start,
  MAX(j.prowjob_build_id) AS prowjob_build_id,
  r.prowjob_url,
  r.successful_start,
FROM DATASET j
LEFT JOIN RecentSuccessfulJobs r
ON j.prowjob_job_name = r.prowjob_job_name 
WHERE j.prowjob_start > DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 180 DAY) AND
      ((j.prowjob_job_name LIKE 'periodic-ci-openshift-%%'
        OR j.prowjob_job_name LIKE 'periodic-ci-shiftstack-%%'
        OR j.prowjob_job_name LIKE 'release-%%'
        OR j.prowjob_job_name LIKE 'aggregator-%%')
      OR j.prowjob_job_name LIKE 'pull-ci-openshift-%%')
GROUP BY j.prowjob_job_name, r.prowjob_url, r.successful_start
ORDER BY j.prowjob_job_name;
`
	queryStr = strings.ReplaceAll(queryStr, "DATASET",
		fmt.Sprintf("%s.%s.%s", v.bigQueryProject, v.bigQueryDataSet, v.bigQueryTable))
	log.Infof("running query for recent jobs: \n%s", queryStr)

	query := v.BigQueryClient.Query(queryStr)
	it, err := query.Read(context.TODO())
	if err != nil {
		return nil, errors.Wrap(err, "error querying primary list of all jobs")
	}

	// TODO: fix release on presubmits

	expectedVariants := map[string]map[string]string{}

	count := 0
	for {
		// TODO: last run but not necessarily successful, this could be a problem for cluster-data file parsing causing
		// our churn. We can't flip the query to last success either as we wouldn't have variants for non-passing jobs at all.
		// Two queries? Use the successful one for cluster-data?
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
			for _, cm := range clusterMatches {
				// log with the file prefix for easy click/copy to browser:
				jLog.WithField("prowJobURL", jlr.URL.StringVal).Infof("Found cluster-data file: https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/%s", cm)
			}

			if len(clusterMatches) > 0 {
				clusterDataBytes, err := prowloader.GetClusterDataBytes(ctx, v.bkt, path, clusterMatches)
				if err != nil {
					jLog.WithError(err).Error("unable to read cluster data file, proceeding without")
				}
				clusterData, err = prowloader.ParseVariantDataFile(clusterDataBytes)
				if err != nil {
					jLog.WithError(err).Error("unable to parse cluster data file, proceeding without")
				} else {
					jLog.Infof("loaded cluster data: %+v", clusterData)
				}
			}
		}

		variants := v.CalculateVariantsForJob(jLog, jlr.JobName, clusterData)
		count++
		jLog.WithField("variants", variants).WithField("count", count).Info("calculated variants")
		expectedVariants[jlr.JobName] = variants
	}
	dur := time.Since(start)
	log.WithField("count", count).Infof("processed primary job list in %s", dur)

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
		if k == VariantNetworkStack {
			// Use ipv6 / ipv4 for consistency with a lot of pre-existing code.
			v = strings.ToLower(v)
		}
		if fileVariantsToIgnore[k] {
			continue
		}
		jnv, ok := variants[k]
		if !ok {
			// Job name did not return this variant, use the value from the file
			variants[k] = v
			continue
		} else if jnv != v {
			if v == "" {
				// If the cluster data file returned an empty value for a variant we calculated from the job
				// name, we just use the job name version. (i.e. FromRelease)
				continue
			}
			// Check and log mismatches between what we read from the file vs determined from job name:
			jLog = jLog.WithFields(logrus.Fields{
				"variant":  k,
				"fromJob":  jnv,
				"fromFile": v,
			})

			switch k {
			case VariantArch:
				// Job name identification wins for arch, heterogenous jobs can show cluster data with
				// amd64 as it's read from a single node.
				jLog.Infof("variant mismatch: using %s from job name", k)
				continue
			case VariantTopology:
				// Topology mismatches on Compact as the job cluster data reports ha.
				jLog.Infof("variant mismatch: using %s from job name", k)
				continue
			default:
				jLog.Infof("variant mismatch: using %s from job run variants file", k)
				variants[k] = v
			}
		}
	}

	return variants
}

var (
	aggregatedRegex = regexp.MustCompile(`(?i)aggregated-`)
	// We're not sure what these aggregator jobs are but they exist as of right now:
	aggregatorRegex = regexp.MustCompile(`(?i)aggregator-`)
	alibabaRegex    = regexp.MustCompile(`(?i)-alibaba`)
	arm64Regex      = regexp.MustCompile(`(?i)-arm64|-multi-a-a`)
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
	multiRegex   = regexp.MustCompile(`(?i)-heterogeneous|-multi-`)
	nutanixRegex = regexp.MustCompile(`(?i)-nutanix`)
	// 3.11 gcp jobs don't have a trailing -version segment
	gcpRegex       = regexp.MustCompile(`(?i)-gcp`)
	openstackRegex = regexp.MustCompile(`(?i)-openstack`)
	sdRegex        = regexp.MustCompile(`(?i)-osd|-rosa`)
	ovirtRegex     = regexp.MustCompile(`(?i)-ovirt`)
	ovnRegex       = regexp.MustCompile(`(?i)-ovn`)
	ipv6Regex      = regexp.MustCompile(`(?i)-ipv6`)
	dualStackRegex = regexp.MustCompile(`(?i)-dualstack`)
	perfScaleRegex = regexp.MustCompile(`(?i)-perfscale`)
	// proxy jobs do not have a trailing -version segment
	ppc64leRegex            = regexp.MustCompile(`(?i)-ppc64le|-multi-p-p`)
	proxyRegex              = regexp.MustCompile(`(?i)-proxy`)
	qeRegex                 = regexp.MustCompile(`(?i)-openshift-tests-private`)
	rosaRegex               = regexp.MustCompile(`(?i)-rosa`)
	cnfRegex                = regexp.MustCompile(`(?i)-telco5g`)
	rtRegex                 = regexp.MustCompile(`(?i)-rt`)
	s390xRegex              = regexp.MustCompile(`(?i)-s390x|-multi-z-z`)
	sdnRegex                = regexp.MustCompile(`(?i)-sdn`)
	serialRegex             = regexp.MustCompile(`(?i)-serial`)
	singleNodeRegex         = regexp.MustCompile(`(?i)-single-node`)
	techpreview             = regexp.MustCompile(`(?i)-techpreview`)
	upgradeMinorRegex       = regexp.MustCompile(`(?i)(-\d+\.\d+-.*-.*-\d+\.\d+)|(-\d+\.\d+-minor)`)
	upgradeOutOfChangeRegex = regexp.MustCompile(`(?i)-upgrade-out-of-change`)
	upgradeRegex            = regexp.MustCompile(`(?i)-upgrade`)
	// some vsphere jobs do not have a trailing -version segment
	vsphereRegex = regexp.MustCompile(`(?i)-vsphere`)
)

const (
	VariantAggregation      = "Aggregation" // aggregated or none
	VariantArch             = "Architecture"
	VariantFeatureSet       = "FeatureSet" // techpreview / standard
	VariantInstaller        = "Installer"  // ipi / upi / assisted
	VariantNetwork          = "Network"
	VariantNetworkAccess    = "NetworkAccess" // disconnected / proxy / standard
	VariantNetworkStack     = "NetworkStack"  // ipv4 / ipv6 / dual
	VariantOwner            = "Owner"         // eng / osd
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
	VariantDefaultValue     = "default"
)

func (v *OCPVariantLoader) IdentifyVariants(jLog logrus.FieldLogger, jobName string) map[string]string {
	variants := map[string]string{}

	if aggregatedRegex.MatchString(jobName) || aggregatorRegex.MatchString(jobName) {
		variants[VariantAggregation] = "aggregated"
	} else {
		variants[VariantAggregation] = "none"
	}

	release, fromRelease := extractReleases(jobName)
	releaseMajorMinor := strings.Split(release, ".")
	if release != "" {
		variants[VariantRelease] = release
		variants[VariantReleaseMajor] = releaseMajorMinor[0]
		variants[VariantReleaseMinor] = releaseMajorMinor[1]
	}
	fromReleaseMajorMinor := strings.Split(fromRelease, ".")
	if fromRelease != "" {
		variants[VariantFromRelease] = fromRelease
		variants[VariantFromReleaseMajor] = fromReleaseMajorMinor[0]
		variants[VariantFromReleaseMinor] = fromReleaseMajorMinor[1]
	}
	if upgradeRegex.MatchString(jobName) {
		switch {
		case upgradeOutOfChangeRegex.MatchString(jobName):
			variants[VariantUpgrade] = "micro-downgrade"
		case isMultiUpgrade(release, fromRelease):
			variants[VariantUpgrade] = "multi"
		case upgradeMinorRegex.MatchString(jobName):
			variants[VariantUpgrade] = "minor"
		default:
			variants[VariantUpgrade] = "micro"
		}
	} else {
		variants[VariantUpgrade] = "none"
		// Wipe out the FromRelease if it's not an upgrade job.
		delete(variants, VariantFromRelease)
		delete(variants, VariantFromReleaseMajor)
		delete(variants, VariantFromReleaseMinor)
	}

	// Platform
	determinePlatform(jLog, variants, jobName)

	// Installation
	install := determineInstallation(jobName)
	if install != "" {
		variants[VariantInstaller] = install
	}

	// Architecture
	arch := determineArchitecture(jobName)
	if arch != "" {
		variants[VariantArch] = arch
	}

	// Network
	network := determineNetwork(jLog, jobName, fromRelease)
	if network != "" {
		variants[VariantNetwork] = network
	}

	// Topology
	topology := determineTopology(jobName)
	if topology != "" {
		variants[VariantTopology] = topology
	}

	if dualStackRegex.MatchString(jobName) {
		variants[VariantNetworkStack] = "dual"
	} else if ipv6Regex.MatchString(jobName) {
		variants[VariantNetworkStack] = "ipv6"
	} else {
		variants[VariantNetworkStack] = "ipv4"
	}

	// TODO: suite may not be the right word here
	if serialRegex.MatchString(jobName) {
		variants[VariantSuite] = "serial"
	} else if etcdScaling.MatchString(jobName) {
		variants[VariantSuite] = "etcd-scaling"
	} else if strings.Contains(jobName, "conformance") {
		// jobs with "conformance" that don't explicitly mention serial are probably parallel
		variants[VariantSuite] = "parallel"
	} else {
		variants[VariantSuite] = "unknown" // parallel perhaps but lots of jobs aren't running out suites
	}

	if sdRegex.MatchString(jobName) {
		variants[VariantOwner] = "service-delivery"
	} else if qeRegex.MatchString(jobName) {
		variants[VariantOwner] = "qe"
	} else if cnfRegex.MatchString(jobName) {
		variants[VariantOwner] = "cnf"
	} else if perfScaleRegex.MatchString(jobName) {
		variants[VariantOwner] = "perfscale"
	} else {
		variants[VariantOwner] = "eng"
	}

	if fipsRegex.MatchString(jobName) {
		variants[VariantSecurityMode] = "fips"
	} else {
		variants[VariantSecurityMode] = VariantDefaultValue
	}

	if techpreview.MatchString(jobName) {
		variants[VariantFeatureSet] = "techpreview"
	} else {
		variants[VariantFeatureSet] = VariantDefaultValue
	}

	if rtRegex.MatchString(jobName) {
		variants[VariantScheduler] = "realtime"
	} else {
		variants[VariantScheduler] = VariantDefaultValue
	}

	if proxyRegex.MatchString(jobName) {
		variants[VariantNetworkAccess] = "proxy"
	} else {
		variants[VariantNetworkAccess] = VariantDefaultValue
	}

	if len(variants) == 0 {
		jLog.WithField("job", jobName).Warn("unable to determine any variants for job")
		return map[string]string{}
	}

	return variants
}

func determineTopology(jobName string) string {
	// Topology
	// external == hypershift hosted control plane
	if singleNodeRegex.MatchString(jobName) {
		return "single" // previously single-node
	} else if hypershiftRegex.MatchString(jobName) {
		return "external"
	} else if compactRegex.MatchString(jobName) {
		return "compact"
	} else if microshiftRegex.MatchString(jobName) { // No jobs for this in 4.15 - 4.16 that I can see.
		return "microshift"
	}

	return "ha"
}

func determineInstallation(jobName string) string {
	if assistedRegex.MatchString(jobName) {
		return "assisted"
	} else if hypershiftRegex.MatchString(jobName) {
		return "hypershift"
	} else if upiRegex.MatchString(jobName) {
		return "upi"
	} else if rosaRegex.MatchString(jobName) {
		return "rosa"
	}

	return "ipi" // assume ipi by default
}

// isMultiUpgrade checks if this is a multi-minor upgrade by examining the delta between the release minor
// and from release minor versions.
func isMultiUpgrade(release, fromRelease string) bool {
	if release == "" || fromRelease == "" {
		return false
	}

	releaseMajorMinor := strings.Split(release, ".")
	fromReleaseMajorMinor := strings.Split(fromRelease, ".")

	releaseMinor, err := strconv.Atoi(releaseMajorMinor[1])
	if err != nil {
		return false
	}
	fromReleaseMinor, err := strconv.Atoi(fromReleaseMajorMinor[1])
	if err != nil {
		return false
	}
	// If release minor minus from release minor is greater than 1, this is a multi-release upgrade job:
	if releaseMinor-fromReleaseMinor > 1 {
		return true
	}
	return false
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
	} else if nutanixRegex.MatchString(jobName) {
		platform = "nutanix"
	} else if openstackRegex.MatchString(jobName) {
		platform = "openstack"
	} else if ovirtRegex.MatchString(jobName) {
		platform = "ovirt"
	} else if rosaRegex.MatchString(jobName) {
		platform = "rosa"
	} else if vsphereRegex.MatchString(jobName) {
		platform = "vsphere"
	}

	if platform == "" {
		jLog.WithField("jobName", jobName).Warn("unable to determine platform from job name")
		return // do not set a platform if unknown
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
			jLog.Warning("could not determine network type")
			return ""
		} else if releaseVersion.GreaterThanOrEqual(ovnBecomesDefault) {
			return "ovn"
		} else {
			return "sdn"
		}
	}
}
