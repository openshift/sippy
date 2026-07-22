package variantregistry

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/hashicorp/go-version"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/releaseoverride"
	"github.com/openshift/sippy/pkg/util"
)

// OCPVariantLoader generates a mapping of job names to their variant map for all known jobs.
type OCPVariantLoader struct {
	BigQueryClient               *bigquery.Client
	bqOpContext                  bqlabel.OperationalContext
	config                       *v1.SippyConfig
	views                        []crview.View
	syntheticReleaseJobOverrides *releaseoverride.SyntheticReleaseOverrides
	bigQueryProject              string
	bigQueryDataSet              string
	bigQueryTable                string
	gcsClient                    *storage.Client
}

func NewOCPVariantLoader(
	bigQueryClient *bigquery.Client,
	opCtx bqlabel.OperationalContext,
	bigQueryProject, bigQueryDataSet, bigQueryTable string,
	gcsClient *storage.Client,
	config *v1.SippyConfig,
	views []crview.View,
	syntheticReleaseJobOverrides *releaseoverride.SyntheticReleaseOverrides,
) *OCPVariantLoader {
	return &OCPVariantLoader{
		BigQueryClient:               bigQueryClient,
		bqOpContext:                  opCtx,
		gcsClient:                    gcsClient,
		config:                       config,
		views:                        views,
		syntheticReleaseJobOverrides: syntheticReleaseJobOverrides,
		bigQueryProject:              bigQueryProject,
		bigQueryDataSet:              bigQueryDataSet,
		bigQueryTable:                bigQueryTable,
	}
}

type prowJobLastRun struct {
	JobName   string              `bigquery:"prowjob_job_name"`
	JobRunID  string              `bigquery:"prowjob_build_id"`
	GCSBucket bigquery.NullString `bigquery:"gcs_bucket"`
	URL       bigquery.NullString `bigquery:"prowjob_url"`
}

// applyQueryLabels applies query labels manually since this loader does not use the client that would do it for us.
func (v *OCPVariantLoader) applyQueryLabels(queryLabel bqlabel.QueryValue, q *bigquery.Query) {
	bqlabel.Context{
		OperationalContext: v.bqOpContext,
		RequestContext:     bqlabel.RequestContext{Query: queryLabel},
	}.ApplyLabels(q)
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
    ARRAY_AGG(gcs_bucket ORDER BY CASE WHEN gcs_bucket != '' AND prowjob_url != '' THEN prowjob_start ELSE NULL END DESC LIMIT 1)[SAFE_OFFSET(0)] AS gcs_bucket,
    ARRAY_AGG(prowjob_url ORDER BY CASE WHEN gcs_bucket != '' AND prowjob_url != '' THEN prowjob_start ELSE NULL END DESC LIMIT 1)[SAFE_OFFSET(0)] AS prowjob_url
  FROM DATASET
  WHERE prowjob_start > DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 180 DAY) AND
        prowjob_state = 'success' AND
        (prowjob_job_name LIKE 'periodic-ci-openshift-%%'
          OR prowjob_job_name LIKE 'periodic-ci-shiftstack-%%'
          OR prowjob_job_name LIKE 'periodic-ci-redhat-chaos-prow-scripts-main-cr-%%'
          OR prowjob_job_name LIKE 'periodic-ci-Azure-ARO-HCP-%%'
          OR prowjob_job_name LIKE 'release-%%'
          OR prowjob_job_name LIKE 'aggregator-%%'
          OR prowjob_job_name LIKE 'periodic-ci-%%-lp-interop-%%'
          OR prowjob_job_name LIKE 'periodic-ci-%%-lp-chaos-%%'
          OR prowjob_job_name LIKE 'periodic-ci-%%-lp-ocp-compat-%%'
          OR prowjob_job_name LIKE 'periodic-ci-%%-quay-cr-%%'
          OR prowjob_job_name LIKE 'pull-ci-openshift-%%')
  GROUP BY prowjob_job_name
)
SELECT 
  j.prowjob_job_name,
  MAX(j.prowjob_start) AS prowjob_start,
  MAX(j.prowjob_build_id) AS prowjob_build_id,
  r.prowjob_url,
  ANY_VALUE(r.gcs_bucket) AS gcs_bucket,
  r.successful_start,
FROM DATASET j
LEFT JOIN RecentSuccessfulJobs r
ON j.prowjob_job_name = r.prowjob_job_name 
WHERE j.prowjob_start > DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 180 DAY) AND
      ((j.prowjob_job_name LIKE 'periodic-ci-openshift-%%'
        OR j.prowjob_job_name LIKE 'periodic-ci-shiftstack-%%'
        OR j.prowjob_job_name LIKE 'periodic-ci-redhat-chaos-prow-scripts-main-cr-%%'
        OR j.prowjob_job_name LIKE 'periodic-ci-Azure-ARO-HCP-%%'
        OR j.prowjob_job_name LIKE 'release-%%'
		OR j.prowjob_job_name LIKE 'periodic-ci-%%-lp-interop-%%'
		OR j.prowjob_job_name LIKE 'periodic-ci-%%-lp-chaos-%%'
		OR j.prowjob_job_name LIKE 'periodic-ci-%%-lp-ocp-compat-%%'
        OR j.prowjob_job_name LIKE 'periodic-ci-%%-quay-cr-%%'
        OR j.prowjob_job_name LIKE 'aggregator-%%')
      OR j.prowjob_job_name LIKE 'pull-ci-openshift-%%')
GROUP BY j.prowjob_job_name, r.prowjob_url, r.successful_start
ORDER BY j.prowjob_job_name;
`
	queryStr = strings.ReplaceAll(queryStr, "DATASET",
		fmt.Sprintf("%s.%s.%s", v.bigQueryProject, v.bigQueryDataSet, v.bigQueryTable))
	log.Infof("running query for recent jobs: \n%s", queryStr)

	query := v.BigQueryClient.Query(queryStr)
	v.applyQueryLabels(bqlabel.VariantRegistryLoadExpectedVariants, query)
	it, err := query.Read(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error querying primary list of all jobs")
	}

	var prowJobLastRuns []*prowJobLastRun

	for {
		// TODO: last run but not necessarily successful, this could be a problem for cluster-data file parsing causing
		// our churn. We can't flip the query to last success either as we wouldn't have variants for non-passing jobs at all.
		// Two queries? Use the successful one for cluster-data?
		jlr := new(prowJobLastRun)
		err := it.Next(jlr)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing prowjob name from bigquery")
			return nil, err
		}

		if isIgnoredJob(jlr.JobName) {
			continue
		}

		prowJobLastRuns = append(prowJobLastRuns, jlr)
	}

	var (
		wg              sync.WaitGroup
		parallelism     = 20
		jobCh           = make(chan *prowJobLastRun)
		count           atomic.Int64
		variantsByJobMu sync.Mutex
		variantsByJob   = make(map[string]map[string]string)
	)

	// Producer
	go func() {
		defer close(jobCh)
		for _, jlr := range prowJobLastRuns {
			select {
			case <-ctx.Done():
				return // Exit when context is cancelled
			case jobCh <- jlr:
			}
		}
	}()

	// Consumer
	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return // Exit when context is cancelled
				case jlr, ok := <-jobCh:
					if !ok {
						return // Channel was closed
					}
					clusterData := map[string]string{}
					jLog := log.WithField("job", jlr.JobName)
					if jlr.URL.Valid && jlr.GCSBucket.Valid {
						path, err := prowloader.GetGCSPathForProwJobURL(jLog, jlr.URL.StringVal)
						if err != nil {
							jLog.WithError(err).WithField("prowJobURL", jlr.URL).Error("error getting GCS path for prow job URL")
							continue
						}
						bkt := v.gcsClient.Bucket(jlr.GCSBucket.StringVal)
						gcsJobRun := gcs.NewGCSJobRun(bkt, path)
						clusterMatches, err := gcsJobRun.FindAllMatches(ctx, gcs.GlobClusterData)
						if err != nil {
							jLog.WithError(err).Error("error finding cluster data file, proceeding without")
							clusterMatches = nil
						}
						for _, cm := range clusterMatches {
							// log with the file prefix for easy click/copy to browser:
							jLog.WithField("prowJobURL", jlr.URL.StringVal).Infof("Found cluster-data file: https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/%s/%s", jlr.GCSBucket.StringVal, cm)
						}

						if len(clusterMatches) > 0 {
							clusterDataBytes, err := prowloader.GetClusterDataBytes(ctx, bkt, path, clusterMatches)
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
					} else {
						jLog.WithField("gcs_bucket", jlr.GCSBucket).WithField("url", jlr.URL.StringVal).Error("job had no gcs bucket or prow job url, proceeding without")
					}

					variants := v.CalculateVariantsForJob(jLog, jlr.JobName, clusterData)
					variantsByJobMu.Lock()
					variantsByJob[jlr.JobName] = variants
					variantsByJobMu.Unlock()
					count.Add(1)
					jLog.WithField("variants", variants).WithField("count", count.Load()).Info("calculated variants")
				}
			}
		}()
	}

	wg.Wait()
	dur := time.Since(start)
	log.WithField("count", count.Load()).Infof("processed primary job list in %s", dur)

	var errs []string
	for jobName, variants := range variantsByJob {
		if err := validateComponentCapabilityVariants(jobName, variants); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return nil, errors.New("variant registry validation failed:\n" + strings.Join(errs, "\n"))
	}

	return variantsByJob, nil
}

// validateComponentCapabilityVariants returns an error if a job has a spotcheck JobTier without both
// Component and Capability defined.
func validateComponentCapabilityVariants(jobName string, variants map[string]string) error {
	if strings.HasPrefix(variants[VariantJobTier], "spotcheck-") {
		if variants[VariantComponent] == "" || variants[VariantCapability] == "" {
			return fmt.Errorf("job %q has JobTier=%s but is missing Component or Capability", jobName, variants[VariantJobTier])
		}
	}
	return nil
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
			case VariantPlatform:
				// ROSA is identified as AWS, but we want to keep it in a separate bucket
				if jnv == "rosa" {
					continue
				}
				// OSD GCP is identified as GCP, but we want to keep it in a separate bucket
				if jnv == "osd-gcp" {
					continue
				}
				variants[k] = v
			case VariantArch:
				// Job name identification wins for arch, heterogenous jobs can show cluster data with
				// amd64 as it's read from a single node.
				jLog.Infof("variant mismatch: using %s from job name", k)
				continue
			case VariantTopology:
				// Topology mismatches on Compact as the job cluster data reports ha.
				jLog.Infof("variant mismatch: using %s from job name", k)
				continue
			case VariantNetworkStack:
				// Discovered in https://issues.redhat.com/browse/TRT-1777
				// 4.13+ gained cluster-data.json but it was not able to detect dualstack, so
				// jobs in this range were categorized as ipv4 mistakenly.
				// For 4.21+, cluster-data.json network stack detection is reliable, so use it.
				releaseVersion := releaseVersionFromVariants(jLog, variants)
				releaseLabel := "unknown"
				if releaseVersion != nil {
					releaseLabel = releaseVersion.String()
				}
				clusterDataReliable, _ := version.NewVersion("4.21")
				if releaseVersion != nil && releaseVersion.GreaterThanOrEqual(clusterDataReliable) {
					jLog.Infof("variant mismatch: using %s from cluster-data (release %s >= 4.21)", k, releaseLabel)
					variants[k] = v
				} else {
					jLog.Infof("variant mismatch: using %s from job name (release %s < 4.21)", k, releaseLabel)
				}
				continue
			default:
				jLog.Infof("variant mismatch: using %s from job run variants file", k)
				variants[k] = v
			}
		}
	}

	v.adjustJobTierBasedOnView(jLog, jobName, variants)

	return variants
}

// adjustJobTierBasedOnView checks if a job with a component readiness tier (blocking/standard/informing)
// has variant values that are filtered out by the release-main view. If so, it downgrades the tier
// to candidate since the job would not appear in component readiness anyway.
func (v *OCPVariantLoader) adjustJobTierBasedOnView(jLog logrus.FieldLogger, jobName string, variants map[string]string) {
	release := variants[VariantRelease]
	if release == "" {
		return
	}

	viewName := release + "-main"
	var view *crview.View
	for i := range v.views {
		if v.views[i].Name == viewName {
			view = &v.views[i]
			break
		}
	}
	if view == nil {
		return
	}

	// Check if the job's tier is one that is included in the view. If the view
	// doesn't specify JobTier at all, all tiers are included and we skip adjustment.
	// If the tier is already not included (e.g. candidate, hidden), no point adjusting.
	tier := variants[VariantJobTier]
	allowedTiers := view.VariantOptions.IncludeVariants[VariantJobTier]
	if len(allowedTiers) == 0 {
		return
	}
	tierIncluded := false
	for _, at := range allowedTiers {
		if at == tier {
			tierIncluded = true
			break
		}
	}
	if !tierIncluded {
		return
	}

	for variantName, allowedValues := range view.VariantOptions.IncludeVariants {
		if len(allowedValues) == 0 {
			continue
		}
		jobValue, ok := variants[variantName]
		if !ok {
			continue
		}
		found := false
		for _, av := range allowedValues {
			if av == jobValue {
				found = true
				break
			}
		}
		if !found {
			jLog.Warnf("job %s has %s tier but variant %s=%s is not included in view %s (allowed: %v), adjusting to candidate",
				jobName, tier, variantName, jobValue, viewName, allowedValues)
			variants[VariantJobTier] = "candidate"
			return
		}
	}
}

var (
	upgradeMajorMinorRegex  = regexp.MustCompile(`(?i)(-\d+\.\d+-.*-.*-\d+\.\d+)|(-\d+\.\d+-(minor|major))`)
	upgradeOutOfChangeRegex = regexp.MustCompile(`(?i)-upgrade-out-of-change`)
	upgradeRegex            = regexp.MustCompile(`(?i)-upgrade`)

	presubmitRegex = regexp.MustCompile(`^pull-ci-(openshift|operator-framework).*-(master|main).*-e2e-.*`)
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
	VariantProcedure        = "Procedure"    // for jobs that do a specific procedure on the cluster (etcd scaling, cpu partitioning, etc.), and then optionally run conformance
	VariantJobTier          = "JobTier"      // specifies rare, blocking, informing, standard jobs
	VariantTopology         = "Topology"     // ha / single / compact / external
	VariantUpgrade          = "Upgrade"
	VariantContainerRuntime = "ContainerRuntime" // runc / crun
	VariantCGroupMode       = "CGroupMode"       // v2 / v1
	VariantRelease          = "Release"
	VariantReleaseMinor     = "ReleaseMinor"
	VariantReleaseMajor     = "ReleaseMajor"
	VariantFromRelease      = "FromRelease"
	VariantFromReleaseMinor = "FromReleaseMinor"
	VariantFromReleaseMajor = "FromReleaseMajor"
	VariantLayeredProduct   = "LayeredProduct"
	VariantOS               = "OS"
	VariantComponent        = "Component"  // jobs with an owning component, used to flag regressions if a tailored job is not passing
	VariantCapability       = "Capability" // jobs with an owning capability, used to flag regressions if a tailored job is not passing
	VariantDefaultValue     = "default"
	VariantNoValue          = "none"
)

func (v *OCPVariantLoader) IdentifyVariants(jLog logrus.FieldLogger, jobName string) map[string]string {
	variants := map[string]string{}

	for _, setter := range []func(jLog logrus.FieldLogger, variants map[string]string, jobName string){
		v.setRelease, // Keep release first, other setters may look up release info in variants map
		setAggregation,
		setPlatform,
		setInstaller,
		setArchitecture,
		setNetwork,
		setTopology,
		setNetworkStack,
		setSuite,
		setOwner,
		setSecurityMode,
		setFeatureSet,
		setScheduler,
		setNetworkAccess,
		setCGroupMode,
		setLayeredProduct,
		setContainerRuntime,
		setProcedure,
		setOS,
		setComponentAndCapability,
		v.setJobTier, // Keep this near last, it relies on other variants like owner
	} {
		setter(jLog, variants, jobName)
	}

	if len(variants) == 0 {
		jLog.WithField("job", jobName).Warn("unable to determine any variants for job")
		return map[string]string{}
	}

	return variants
}

func setAggregation(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	aggregationPatterns := []struct {
		substring   string
		aggregation string
	}{
		{"aggregated-", "aggregated"},
		{"aggregator-", "aggregated"},
	}

	variants[VariantAggregation] = VariantNoValue
	for _, entry := range aggregationPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantAggregation] = entry.aggregation
			return
		}
	}
}

func setOwner(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	ownerPatterns := []struct {
		substring string
		owner     string
	}{
		{"-osd", "service-delivery"},
		{"-rosa", "service-delivery"},
		{"-openshift-online", "service-delivery"},
		{"-telco5g", "cnf"},
		{"-perfscale", "perfscale"},
		{"-chaos-", "chaos"},
		{"-azure-aro-hcp", "aro"},
		{"-qe", "qe"}, // Keep this one below perfscale
		{"-openshift-tests-private", "qe"},
		{"-openshift-verification-tests", "qe"},
		{"-openshift-distributed-tracing", "qe"},
		{"-oadp-", "oadp"},
		{"-lp-chaos-", "mpict"},   // MPEX Integrity Engineering Chaos Team
		{"-lp-interop-", "mpiit"}, // MPEX Integrity Engineering Interop Team
		{"-lp-ocp-compat-", "lp"}, // Layered Product Teams
	}

	for _, entry := range ownerPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantOwner] = entry.owner
			return
		}
	}

	variants[VariantOwner] = "eng"
}

func setSuite(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	suitePatterns := []struct {
		substring string
		suite     string
	}{
		{"-serial", "serial"},
		{"-etcd-scaling", "etcd-scaling"},
		{"conformance", "parallel"}, // Jobs with "conformance" but no explicit serial are probably parallel
		{"-e2e-external-", "parallel"},
	}

	for _, entry := range suitePatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantSuite] = entry.suite
			return
		}
	}

	variants[VariantSuite] = "unknown" // Default case for jobs not running suites
}

func setSecurityMode(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	securityPatterns := []struct {
		substring string
		mode      string
	}{
		{"-fips", "fips"},
	}

	variants[VariantSecurityMode] = VariantDefaultValue
	for _, entry := range securityPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantSecurityMode] = entry.mode
			return
		}
	}
}

func setFeatureSet(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	featurePatterns := []struct {
		substring string
		feature   string
	}{
		{"-techpreview", "techpreview"},
		{"-tp-", "techpreview"},
	}

	variants[VariantFeatureSet] = VariantDefaultValue
	for _, entry := range featurePatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantFeatureSet] = entry.feature
			return
		}
	}
}

func setScheduler(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	schedulerPatterns := []struct {
		substring string
		scheduler string
	}{
		{"-rt", "realtime"},
	}

	variants[VariantScheduler] = VariantDefaultValue
	for _, entry := range schedulerPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantScheduler] = entry.scheduler
			return
		}
	}
}

func setNetworkAccess(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	networkPatterns := []struct {
		substring string
		access    string
	}{
		{"-proxy", "proxy"},
		{"-metal-ipi-ovn-ipv6", "disconnected"},

		// NAT Instance is a temporary testing variant to analyze the
		// impacts of a cost reduction strategy in ephemeral test accounts.
		// https://github.com/openshift/ci-tools/pull/4534 .
		{"-nat-instance", "nat-instance"},
	}

	variants[VariantNetworkAccess] = VariantDefaultValue
	for _, entry := range networkPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantNetworkAccess] = entry.access
			return
		}
	}
}

func setNetworkStack(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	networkPatterns := []struct {
		substring string
		stack     string
	}{
		{"-dualstack", "dual"},
		{"-ipv6", "ipv6"},
	}

	for _, entry := range networkPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantNetworkStack] = entry.stack
			return
		}
	}

	variants[VariantNetworkStack] = "ipv4"
}

func (v *OCPVariantLoader) setRelease(logger logrus.FieldLogger, variants map[string]string, jobName string) {
	// Presubmits on main branch use the Presubmits pseudo-release
	if presubmitRegex.MatchString(jobName) {
		variants[VariantRelease] = models.ReleasePresubmits
		return
	}

	// Prefer core release from sippy config -- only if the job name references the release. Too many jobs
	// are attached to "master" and move between releases.
	for releaseName, releaseConfig := range v.config.Releases {
		if _, ok := releaseConfig.Jobs[jobName]; ok && strings.Contains(jobName, releaseName) {
			variants[VariantRelease] = releaseName
		}
	}

	releasesInJobName := extractReleases(jobName)
	if count := len(releasesInJobName); count > 0 {
		release := releasesInJobName[count-1]
		variants[VariantRelease] = release.Original()
		variants[VariantReleaseMajor] = strconv.Itoa(release.Segments()[0])
		variants[VariantReleaseMinor] = strconv.Itoa(release.Segments()[1])

		fromRelease := releasesInJobName[0]
		variants[VariantFromRelease] = fromRelease.Original()
		variants[VariantFromReleaseMajor] = strconv.Itoa(fromRelease.Segments()[0])
		variants[VariantFromReleaseMinor] = strconv.Itoa(fromRelease.Segments()[1])
	}

	// for jobs that look like upgrades, determine upgrade variant
	if upgradeRegex.MatchString(jobName) {
		switch {
		case upgradeOutOfChangeRegex.MatchString(jobName):
			variants[VariantUpgrade] = "micro-downgrade"
		case upgradeMajorMinorRegex.MatchString(jobName):
			variants[VariantUpgrade] = upgradeVariant(logger, releasesInJobName, jobName)
		default:
			variants[VariantUpgrade] = "micro"
		}
	} else {
		variants[VariantUpgrade] = VariantNoValue
		// Wipe out the FromRelease if it's not an upgrade job.
		delete(variants, VariantFromRelease)
		delete(variants, VariantFromReleaseMajor)
		delete(variants, VariantFromReleaseMinor)
	}

	// Synthetic release claims take priority over other release logic.
	if release, ok := v.syntheticReleaseJobOverrides.Lookup(jobName); ok {
		variants[VariantRelease] = release
	}
}

// componentCapabilityEntry defines a job's component/capability ownership and its
// optional job tier override. Both setComponentAndCapability and setJobTier
// reference this shared table so additions stay in sync.
type componentCapabilityEntry struct {
	substrings []string
	component  string
	capability string
	jobTier    string // if non-empty, setJobTier uses this instead of the default tier logic
}

// componentCapabilityPatterns is the single source of truth for job component/capability
// ownership. Be sure to use real Component names from OCPBUGS.
var componentCapabilityPatterns = []componentCapabilityEntry{
	{[]string{"-cpu-partitioning"}, "Node / Kubelet", "CPU Partitioning", "spotcheck-30d"},
	{[]string{"-etcd-scaling"}, "Etcd", "Scaling", "spotcheck-30d"},
	{[]string{"-aws-ovn-installer-dualstack"}, "Installer", "AWSDualStackInstall", "candidate"},
	{[]string{"-iso-no-registry"}, "Installer / Agent based installation", "NoRegistryClusterInstall", "candidate"},
}

// setComponentAndCapability identifies the component and capability owner for a job.
// These variants indicate a job has an owning component and capability (i.e. feature).
// This is used for tailored jobs that need to be kept working to validate component
// features. Can be used in component readiness as a spot check job, or in sippy jobs
// filtering.
func setComponentAndCapability(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	for _, p := range componentCapabilityPatterns {
		if allSubstringsMatch(jobNameLower, p.substrings) {
			variants[VariantComponent] = p.component
			variants[VariantCapability] = p.capability
			return
		}
	}
}

// allSubstringsMatch returns true if jobNameLower contains every substring.
func allSubstringsMatch(jobNameLower string, substrings []string) bool {
	for _, sub := range substrings {
		if !strings.Contains(jobNameLower, sub) {
			return false
		}
	}
	return true
}

// setJobTier sets the jobTier for a job, with values like this:
//
//	blocking: blocking job on payloads, covered by component readiness
//	informing: informing job on payloads, covered by component readiness
//	standard: should be visible in default views (component readiness, sippy), covered by component readiness
//	spotcheck: jobs evaluated by spot-check analysis (job pass/fail, not junit); views opt in via JobTier include
//	candidate: not covered by component readiness, but may be promoted in the future
//	hidden: data should still be synced, but not shown by default
//	excluded: data should not be synced, and excluded from all views
//
// Note: blocking/informing/standard tiers may be downgraded to candidate by
// adjustJobTierBasedOnView if the job's variants don't match the release-main view.
func (v *OCPVariantLoader) setJobTier(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	// Check componentCapabilityPatterns first for tier overrides (e.g. spotcheck).
	// setComponentAndCapability already ran, so use its results directly.
	if variants[VariantComponent] != "" {
		for _, p := range componentCapabilityPatterns {
			if p.jobTier != "" && p.component == variants[VariantComponent] && p.capability == variants[VariantCapability] {
				variants[VariantJobTier] = p.jobTier
				return
			}
		}
	}

	jobTierPatterns := []struct {
		substrings []string
		jobTier    string
	}{
		// QE jobs allowlisted for Component Readiness
		{[]string{"-automated-release"}, "standard"},

		// OVN-Kubernetes BGP Virtualization jobs allowed for Component Readiness
		{[]string{"-ovn-bgp-virt"}, "standard"},

		// Add two-node-fencing for component readiness
		{[]string{"-two-node-fencing-recovery"}, "standard"},
		{[]string{"-two-node-fencing-dualstack-recovery"}, "standard"},
		{[]string{"-two-node-fencing-ipv6-recovery"}, "standard"},

		// Excluded jobs
		{[]string{"-okd"}, "excluded"},
		{[]string{"-recovery"}, "excluded"},
		{[]string{"alibaba"}, "excluded"},
		{[]string{"-osde2e-"}, "excluded"},

		// OVN-Kubernetes BGP jobs; candidate tier to collect data while stabilizing
		{[]string{"-bgp-"}, "candidate"},

		// Experimental new jobs using nested vsphere lvl 2 environment,
		// not ready to make release blocking yet.
		{[]string{"-vsphere-host-groups"}, "candidate"},

		// vSphere hybrid-env jobs are not yet stable enough for component readiness
		{[]string{"-hybrid-env"}, "candidate"},

		// All 4.19/4.20 MCO jobs default to candidate
		{[]string{"machine-config-operator-release-4.19"}, "candidate"},
		{[]string{"machine-config-operator-release-4.20"}, "candidate"},

		// Cloud MCO disruptive jobs set to standard for component readiness
		// This also includes techpreview variants
		{[]string{"e2e-aws-mco-disruptive"}, "standard"},
		{[]string{"e2e-azure-mco-disruptive"}, "standard"},
		{[]string{"e2e-gcp-mco-disruptive"}, "standard"},

		// All remaining MCO periodic jobs default to candidate
		{[]string{"machine-config-operator"}, "candidate"},

		// Konflux jobs aren't ready yet
		{[]string{"-konflux"}, "candidate"},
		{[]string{"-console-operator-"}, "candidate"}, // https://issues.redhat.com/browse/OCPBUGS-54873

		{[]string{"-nat-instance"}, "candidate"},

		// Operator Framework extended test jobs are not yet stable enough to make release readiness.
		// Mark candidate to collect data in Sippy while working on stabilization.
		{[]string{"periodic-ci-openshift-operator-framework-operator-controller-", "-extended-"}, "candidate"},
		{[]string{"periodic-ci-openshift-operator-framework-olm-", "-extended-"}, "candidate"},

		// GCP multi-operator periodic jobs are not yet stable enough for component readiness
		{[]string{"e2e-gcp-multi-operator-periodic"}, "candidate"},

		// Disruptive longrunning jobs promoted to candidate while stabilizing
		{[]string{"-disruptive-longrunning"}, "candidate"},

		// Hidden jobs
		{[]string{"-cilium"}, "hidden"},
		{[]string{"-disruptive"}, "hidden"},
		{[]string{"-rollback"}, "hidden"},
		{[]string{"aggregator-"}, "hidden"},
		{[]string{"-out-of-change"}, "hidden"},
		{[]string{"-sno-fips-recert"}, "hidden"},
		{[]string{"aggregated"}, "hidden"},
		{[]string{"-cert-rotation-shutdown-"}, "hidden"}, // may want to go to rare at some point
		{[]string{"-vsphere-insights-runtime"}, "hidden"},

		{[]string{"-4.19-e2e-metal-ipi-serial-ovn-ipv6-techpreview-"}, "candidate"},      // new jobs in https://github.com/openshift/release/pull/64143 have failures that need to be addressed, don't want to regress 4.19
		{[]string{"-4.19-e2e-metal-ipi-serial-ovn-dualstack-techpreview-"}, "candidate"}, // new jobs in https://github.com/openshift/release/pull/64143 have failures that need to be addressed, don't want to regress 4.19

		// Only a select few Hypershift jobs are ready for blocking signal, the rest will default to candidate below.
		{[]string{"periodic-ci-openshift-hypershift-", "-e2e-azure-aks-ovn-conformance"}, "standard"},
		{[]string{"periodic-ci-openshift-hypershift-", "-e2e-aws-ovn-conformance"}, "standard"},
		{[]string{"periodic-ci-openshift-hypershift-", "-e2e-azure-v2-self-managed"}, "standard"},

		// All other Hypershift jobs will default to candidate.
		{[]string{"periodic-ci-openshift-hypershift-"}, "candidate"},

		// Storage team job preparing for RHEL 10 to detect regressions early, not yet stable, jsafrane would like to promote eventually:
		{[]string{"periodic-ci-openshift-cluster-storage-operator", "upgrade-check-dev-symlinks"}, "candidate"},

		// z-stream techpreview jobs should generally upgrade correctly, however also get wedged in some cases (e.g. when we forcibly change an API from alpha to stable).
		{[]string{"-techpreview-upgrade"}, "candidate"},

		// Custom DNS techpreview jobs - candidate tier to collect data while stabilizing
		{[]string{"-custom-dns-techpreview"}, "candidate"},

		// AWS European Sovereign Cloud techpreview jobs - candidate tier to collect data while stabilizing
		{[]string{"-eusc-techpreview"}, "candidate"},

		// AWS DualStack Techpreview jobs - candidate tier to collect data while stabilizing
		{[]string{"-aws-ovn-dualstack"}, "candidate"},
		{[]string{"-aws-ovn-installer-dualstack-ipv6-primary-techpreview"}, "candidate"},
		{[]string{"-aws-ovn-installer-dualstack-ipv4-primary-techpreview"}, "candidate"},

		{[]string{"periodic-ci-openshift-hypershift-", "-mce-e2e-agent-", "-metal-conformance"}, "candidate"},
	}

	for _, jobTierPattern := range jobTierPatterns {
		allMatch := true
		for _, substring := range jobTierPattern.substrings {
			if !strings.Contains(jobNameLower, substring) {
				allMatch = false
				break
			}
		}
		if allMatch {
			variants[VariantJobTier] = jobTierPattern.jobTier
			return
		}
	}

	// QE default is hidden, we'll opt jobs in above as they stabilize and are
	// ready for component readiness.
	if variants[VariantOwner] == "qe" {
		variants[VariantJobTier] = "hidden"
		return
	}

	// Determine job tier from release configuration
	release := variants[VariantRelease]

	// after the master -> main branch renaming in release some of the master job names in our config have been renamed
	// check for the 'main' name as well to determine JobTier
	mainJobName := strings.Replace(jobName, "-master-", "-main-", 1)

	switch {
	case util.StrSliceContainsEither(v.config.Releases[release].BlockingJobs, jobName, mainJobName):
		variants[VariantJobTier] = "blocking"
	case util.StrSliceContainsEither(v.config.Releases[release].InformingJobs, jobName, mainJobName):
		variants[VariantJobTier] = "informing"
	case release == models.ReleasePresubmits, v.config.Releases[release].Jobs[jobName], v.config.Releases[release].Jobs[mainJobName]:
		variants[VariantJobTier] = "standard"
	default:
		variants[VariantJobTier] = "candidate"
	}
}

func setProcedure(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	// with multi value support we need a build up the value
	base := ""
	procedureValue := VariantNoValue

	if strings.Contains(jobNameLower, "-serial") {
		base = concatProcedureValues(base, "serial")
		procedureValue = base
	}
	// Job procedure patterns
	procedurePatterns := []struct {
		substring string
		procedure string
	}{
		{"-etcd-scaling", concatProcedureValues(base, "etcd-scaling")},
		{"-cpu-partitioning", concatProcedureValues(base, "cpu-partitioning")},
		{"-automated-release", concatProcedureValues(base, "automated-release")},
		{"-cert-rotation-shutdown-", concatProcedureValues(base, "cert-rotation-shutdown")},
		{"-console-operator-", concatProcedureValues(base, "console-operator")},
		{"-ipsec", concatProcedureValues(base, "ipsec")},
		{"-network-flow-matrix", concatProcedureValues(base, "network-flow-matrix")},
		{"-ocl", concatProcedureValues(base, "on-cluster-layering")},
		{"-machine-config-operator", concatProcedureValues(base, "machine-config-operator")},
		{"-usernamespace", concatProcedureValues(base, "usernamespace")},
	}

	for _, entry := range procedurePatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantProcedure] = entry.procedure
			return
		}
	}

	// Default procedure
	variants[VariantProcedure] = procedureValue
}

func concatProcedureValues(base, addition string) string {
	if len(base) == 0 {
		return addition
	}
	// shouldn't get called this way
	if len(addition) == 0 {
		return base
	}

	return fmt.Sprintf("%s-%s", base, addition)
}

func setTopology(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	topologyPatterns := []struct {
		substring string
		topology  string
	}{
		{"-sno-", "single"},                       // Previously single-node
		{"-single-node", "single"},                // Alternative format
		{"-two-node-arbiter", "two-node-arbiter"}, // Two-node
		{"-two-node-fencing", "two-node-fencing"}, // Two-node
		{"-tna-", "two-node-arbiter"},             // Two-node alternative format
		{"-tnf-", "two-node-fencing"},             // Two-node alternative format
		{"-hypershift", "external"},
		{"-hcp", "external"},
		{"_hcp", "external"},
		{"-compact", "compact"},
		{"-microshift", "microshift"},
	}

	for _, entry := range topologyPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantTopology] = entry.topology
			return
		}
	}

	variants[VariantTopology] = "ha" // Default to "ha"
}

func setInstaller(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	installationPatterns := []struct {
		substring string
		installer string
	}{
		{"-assisted", "assisted"},
		{"-azure-aro-hcp", "aro"}, // check before hypershift as job name includes -hcp
		{"-hypershift", "hypershift"},
		{"-hcp", "hypershift"},
		{"_hcp", "hypershift"},
		{"-upi", "upi"},
		{"-agent", "agent"},
		{"-e2e-external-aws", "upi"}, // clusters with platform type external can be installed in any provider with no installer automation (upi).
		{"-e2e-external-vsphere", "upi"},
		{"-e2e-oci-assisted", "assisted"},
	}

	for _, entry := range installationPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantInstaller] = entry.installer
			return
		}
	}

	variants[VariantInstaller] = "ipi" // Assume ipi by default
}

// upgradeVariant returns a variant inferred from the slice of releases involved in a job and its name
//   - releases need to be sorted and unique
func upgradeVariant(logger logrus.FieldLogger, releases []version.Version, jobName string) string {
	count := len(releases)
	if count > 2 {
		return "multi"
	}

	if count == 2 {
		fromRelease, toRelease := releases[0], releases[count-1]
		fromSegments, toSegments := fromRelease.Segments(), toRelease.Segments()
		fromMajor, fromMinor := fromSegments[0], fromSegments[1]
		toMajor, toMinor := toSegments[0], toSegments[1]

		switch {
		case fromMajor < toMajor:
			return "major"
		case fromMajor == toMajor && (toMinor-fromMinor) == 1:
			return "minor"
		case fromMajor == toMajor && (toMinor-fromMinor) > 1:
			return "multi"
		default:
			// should never happen; versions in releases are unique (=> not equal) and sorted (=> from < to)
			// if this is not true we may misclassify as minor
			logger.WithFields(
				logrus.Fields{"fromRelease": fromRelease, "toRelease": toRelease},
			).Warn("BUG: fromRelease is not lower than toRelease")
		}
	}

	// if we only have one version then we either take a hint from the job name
	// or it is a micro

	if strings.Contains(jobName, "-minor") {
		return "minor"
	}
	if strings.Contains(jobName, "-major") {
		return "major"
	}

	return "micro"
}

func setPlatform(jLog logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	// Order matters here: patterns must be checked in a specific sequence
	platformPatterns := []struct {
		substring string
		platform  string
	}{
		{"-e2e-external-aws", "external-aws"}, // platform type external can be installed in any provider. Syntax platformType(provider).
		{"-e2e-external-vsphere", "external-vsphere"},
		{"-e2e-oci-assisted", "external-oci"},
		{"-rosa", "rosa"}, // Keep above AWS as many ROSA jobs also mention AWS
		{"-aws", "aws"},
		{"-alibaba", "alibaba"},
		{"-azure-aro-hcp", "aro"},
		{"-azure", "azure"},
		{"-aks", "azure"},
		{"-osd-ccs-gcp", "osd-gcp"},
		{"-gcp", "gcp"},
		{"-libvirt", "libvirt"},
		// iso-no-registry agent baremetal jobs deploy on bare metal
		// but don't have -metal in their name; match before the generic -metal pattern.
		{"-iso-no-registry", "metal"},
		{"-metal", "metal"},
		{"-nutanix", "nutanix"},
		{"-openstack", "openstack"},
		{"-ovirt", "ovirt"},
		{"-vsphere", "vsphere"},

		// there is no cluster for the periodics-default-catalog-consistency jobs
		// forcing to aws to include signal in CR main view without adding 'none' platform
		{"-periodics-default-catalog-consistency", "aws"},
	}

	for _, entry := range platformPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantPlatform] = entry.platform
			return
		}
	}

	jLog.WithField("jobName", jobName).Warn("unable to determine platform from job name")
}

var majorMinorRegexp = regexp.MustCompile(`\d+\.\d+`)

// extractRelease returns a slice of unique major.minor version strings found in the job name sorted
// from lowest to highest, assumed to represent the installed (lowest) and final (highest) releases
// in update jobs
func extractReleases(jobName string) []version.Version {
	matches := sets.New(majorMinorRegexp.FindAllString(jobName, -1)...)

	mm := make([]version.Version, 0, len(matches))

	for match := range matches {
		// two items and successful conversion are pretty much guaranteed on regex matches but there are corner cases
		if v, err := version.NewVersion(match); err == nil && v != nil {
			mm = append(mm, *v)
		}
	}

	if len(mm) == 0 {
		return nil
	}

	sort.Slice(mm, func(i, j int) bool {
		return mm[i].LessThan(&mm[j])
	})

	return mm
}

func setArchitecture(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	architecturePatterns := []struct {
		substring    string
		architecture string
	}{
		{"-arm64", "arm64"},
		{"-multi-a-a", "arm64"},
		{"-arm", "arm64"},
		{"-ppc64le", "ppc64le"},
		{"-multi-p-p", "ppc64le"},
		{"-s390x", "s390x"},
		{"-multi-z-z", "s390x"},
		{"-heterogeneous", "multi"},
		{"-multi", "multi"},
	}

	// the use of multi in these cases do not apply to architecture so drop them out from evaluation
	ignorePatterns := []string{
		"-multi-vcenter-",
		"-multi-network-",
		"-multisubnets-",
		"-multitenant",
		"-multiarch",
		"-multinet",
	}
	for _, ignore := range ignorePatterns {
		jobNameLower = strings.ReplaceAll(jobNameLower, ignore, "-")
	}

	for _, entry := range architecturePatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantArch] = entry.architecture
			return
		}
	}

	variants[VariantArch] = "amd64"
}

func setNetwork(jLog logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	networkPatterns := []struct {
		substring string
		network   string
	}{
		{"-ovn", "ovn"},
		{"-sdn", "sdn"},
		{"-cilium", "cilium"},
	}

	// Check jobName for explicit network type
	for _, entry := range networkPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantNetwork] = entry.network
			return
		}
	}

	// Get release version from variants
	releaseVersion := releaseVersionFromVariants(jLog, variants)
	if releaseVersion == nil {
		return
	}

	// Determine network based on release
	ovnBecomesDefault, _ := version.NewVersion("4.12")

	if releaseVersion.GreaterThanOrEqual(ovnBecomesDefault) {
		variants[VariantNetwork] = "ovn"
	} else {
		variants[VariantNetwork] = "sdn"
	}
}

func setContainerRuntime(jLog logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	runtimePatterns := []struct {
		substring string
		runtime   string
	}{
		{"-crun", "crun"},
		{"-runc", "runc"},
	}

	// Check jobName for explicit container runtime
	for _, entry := range runtimePatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantContainerRuntime] = entry.runtime
			return
		}
	}

	// Get release version from variants
	releaseVersion := releaseVersionFromVariants(jLog, variants)
	if releaseVersion == nil {
		return
	}

	// Determine container runtime based on release
	crunBecomesDefault, _ := version.NewVersion("4.18")

	if releaseVersion.GreaterThanOrEqual(crunBecomesDefault) {
		variants[VariantContainerRuntime] = "crun"
	} else {
		variants[VariantContainerRuntime] = "runc"
	}
}

func releaseVersionFromVariants(jLog logrus.FieldLogger, variants map[string]string) *version.Version {
	release, exists := variants[VariantFromRelease]
	if !exists {
		release, exists = variants[VariantRelease]
	}
	if exists {
		if v, err := version.NewVersion(release); err == nil {
			return v
		}
	}

	// Synthetic releases will not be able to determine the release version using the VariantRelease or VariantFromRelease
	// We can attempt to determine it via the VariantReleaseMajor and VariantReleaseMinor variants.
	if major, ok := variants[VariantReleaseMajor]; ok {
		if minor, ok := variants[VariantReleaseMinor]; ok {
			if v, err := version.NewVersion(major + "." + minor); err == nil {
				return v
			}
		}
	}
	jLog.Warning("release version not found, unable to determine version-dependent variant")
	return nil
}

func setCGroupMode(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	cgroupPatterns := []struct {
		substring string
		mode      string
	}{
		{"-cgroupsv1", "v1"},
	}

	variants[VariantCGroupMode] = "v2" // Default to v2
	for _, entry := range cgroupPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantCGroupMode] = entry.mode
			return
		}
	}
}

func setLayeredProduct(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	layeredProductPatterns := []struct {
		substring string
		product   string
	}{
		{"-lpga-lp-ocp-compat-cr--cnv-", "lp-ocp-compat--virt--lpGA"},
		{"-lpga-lp-ocp-compat-cr--quay-", "lp-ocp-compat--quay--lpGA"},
		{"-lpga-lp-ocp-compat-cr--openshift-pipelines-", "lp-ocp-compat--openshift-pipelines--lpGA"},
		{"-lpmainline-lp-ocp-compat-cr--acs-", "lp-ocp-compat--acs--lpMainline"},
		{"-lpga-lp-ocp-compat-cr--acs-", "lp-ocp-compat--acs--lpGA"},
		{"-lpga-lp-ocp-compat-cr--odf-", "lp-ocp-compat--odf--lpGA"},
		{"-lpga-lp-ocp-compat-cr--gitops-", "lp-ocp-compat--gitops--lpGA"},
		{"-lpga-lp-ocp-compat-cr--fusion-access-", "lp-ocp-compat--fusion-access--lpGA"},
		{"-lpga-lp-ocp-compat-cr--mta-", "lp-ocp-compat--mta--lpGA"},
		{"-lpga-lp-ocp-compat-cr--oadp-", "lp-ocp-compat--oadp--lpGA"},
		{"-lpga-lp-ocp-compat-cr--servicemesh-", "lp-ocp-compat--servicemesh--lpGA"},
		{"-lpga-lp-ocp-compat-cr--operator-e2e-", "lp-ocp-compat--serverless--lpGA"},
		{"-coo-", "lp-interop-coo"},
		{"-virt", "virt"},
		{"-cnv", "virt"},
		{"-kubevirt", "virt"},
		{"-oadp-", "oadp"},
	}

	variants[VariantLayeredProduct] = VariantNoValue
	for _, entry := range layeredProductPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantLayeredProduct] = entry.product
			return
		}
	}
}

func setOS(_ logrus.FieldLogger, variants map[string]string, jobName string) {
	jobNameLower := strings.ToLower(jobName)

	// Order matters: check rhcos9-10 before rhcos10 and rhcos9 to avoid false matches.
	osPatterns := []struct {
		substring string
		os        string
	}{
		{"rhcos9-10", "rhcos9-10"},
		{"rhcos10", "rhcos10"},
		{"rhcos9", "rhcos9"},
	}

	for _, entry := range osPatterns {
		if strings.Contains(jobNameLower, entry.substring) {
			variants[VariantOS] = entry.os
			return
		}
	}

	// No explicit rhcos fragment in the job name: fall back based on OCP major version.
	isMainBranch := strings.Contains(jobNameLower, "-main-") || strings.Contains(jobNameLower, "-master-")
	switch {
	case variants[VariantReleaseMajor] == "4":
		variants[VariantOS] = "rhcos9"
	case (variants[VariantReleaseMajor] == "5" || isMainBranch) &&
		(variants[VariantUpgrade] == VariantNoValue || variants[VariantFromReleaseMajor] == "5"):
		variants[VariantOS] = "rhcos10"
	case (variants[VariantReleaseMajor] == "5" || isMainBranch) &&
		variants[VariantFromReleaseMajor] != "5":
		variants[VariantOS] = "rhcos9"
	default:
		variants[VariantOS] = "unknown"
	}
}
