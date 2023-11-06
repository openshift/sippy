package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"

	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/releaseloader"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
)

const (
	blockerScoreToAlertOn = 70
)

var (
	buildClusterHealthMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_build_cluster_pass_ratio",
		Help: "Ratio of passed job runs for a build cluster in a period (2 day, 7 day, etc)",
	}, []string{"cluster", "period"})
	jobPassRatioMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_job_pass_ratio",
		Help: "Ratio of passed job runs for the given job in a period (2 day, 7 day, etc)",
	}, []string{"release", "period", "name", "silenced"})
	infraSuccessMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_infra_success_ratio",
		Help: "Ratio of successful infrastructure in a period (2 day, 7 day, etc)",
	}, []string{"platform", "period"})
	releaseWarningsMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_release_warnings",
		Help: "Number of current warnings for a release, see overview page in UI for details",
	}, []string{"release"})
	payloadConsecutiveRejectionsMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_payloads_consecutively_rejected",
		Help: "Number of consecutive rejected payloads in each release, stream and arch combo. Will be 0 if most recent payload accepted.",
	}, []string{"release", "stream", "architecture"})
	payloadHoursSinceLastAcceptedMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_payloads_hours_since_last_accepted",
		Help: "Number of hours since last accepted payload in each release, stream and arch combo.",
	}, []string{"release", "stream", "architecture"})
	payloadHoursSinceLastOSUpgrade = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_payloads_hours_since_last_os_upgrade",
		Help: "Number of hours since last OS upgrade.",
	}, []string{"release", "stream", "architecture"})
	payloadPossibleTestBlockersMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_payloads_possible_test_blockers",
		Help: "Number of possible test blockers identified for a given payload stream.",
	}, []string{"release", "stream", "architecture"})
	hoursSinceLastUpdate = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_hours_since_last_update",
		Help: "Number of hours since Sippy last successfully fetched new data.",
	}, []string{})
	componentReadinessMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_component_readiness",
		Help: "Regression score for components",
	}, []string{"component", "network", "arch", "platform"})
)

// presume in a historical context there won't be scraping of these metrics
// pinning the time just to be consistent
func RefreshMetricsDB(dbc *db.DB, bqc *bqclient.Client, variantManager testidentification.VariantManager, reportEnd time.Time) error {
	start := time.Now()
	log.Info("beginning refresh metrics")
	releases, err := query.ReleasesFromDB(dbc)
	if err != nil {
		return err
	}

	promReportTypes := buildPromReportTypes(releases)
	if err != nil {
		return err
	}

	// Get last updated job run
	var lastUpdated time.Time
	if r := dbc.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&lastUpdated); r.Error != nil {
		return errors.Wrapf(err, "could not fetch last updated time")
	}
	hoursSinceLastUpdate.WithLabelValues().Set(time.Since(lastUpdated).Hours())

	for _, pType := range promReportTypes {
		// start, boundary and end will just be defaults
		// the api will decide based on the period
		// and current day / time
		jobsResult, err := api.JobReportsFromDB(dbc, pType.release, pType.period, nil, time.Time{}, time.Time{}, time.Time{}, reportEnd)

		if err != nil {
			return errors.Wrapf(err, "error refreshing prom report type %s - %s", pType.period, pType.release)
		}
		for _, jobResult := range jobsResult {
			silenced := "false"
			if jobResult.CurrentRuns == 0 {
				silenced = "true"
			}
			if jobResult.OpenBugs > 0 {
				silenced = "true"
			}
			jobPassRatioMetric.WithLabelValues(pType.release, pType.period, jobResult.Name, silenced).Set(jobResult.CurrentPassPercentage / 100)
		}
	}

	// Add a metric for any warnings for each release. We can't convey exact details with prom, but we can
	// tell you x warnings are present and link you to the overview in the alert.
	for _, release := range releases {
		releaseWarnings := api.ScanForReleaseWarnings(dbc, release.Release, reportEnd)
		releaseWarningsMetric.WithLabelValues(release.Release).Set(float64(len(releaseWarnings)))
	}

	if err := refreshBuildClusterMetrics(dbc, reportEnd); err != nil {
		log.WithError(err).Error("error refreshing build cluster metrics")
	}

	refreshPayloadMetrics(dbc, reportEnd)

	if bqc != nil {
		if err := refreshComponentReadinessMetrics(dbc, bqc); err != nil {
			log.WithError(err).Error("error refreshing component readiness metrics")
		}
	}

	if err := refreshInstallSuccessMetrics(dbc); err != nil {
		log.WithError(err).Error("error refreshing install success metrics")
	}
	if err := refreshUpgradeSuccessMetrics(dbc); err != nil {
		log.WithError(err).Error("error refreshing upgrade success metrics")
	}
	if err := refreshInfraMetrics(dbc, variantManager); err != nil {
		log.WithError(err).Error("error refreshing infrastructure success metrics")
	}
	log.Infof("refresh metrics completed in %s", time.Since(start))

	return nil
}

func refreshComponentReadinessMetrics(dbc *db.DB, client *bqclient.Client) error {
	if client == nil || client.BQ == nil {
		log.Warningf("not generating component readiness metrics as we don't have a bigquery client")
		return nil
	}

	// Make sure we're not updating this every 5 minutes
	metricsUpdate := models.MetricsRefresh{
		Name: "component_readiness",
	}
	if res := dbc.DB.FirstOrInit(&metricsUpdate); res.Error != nil {
		return res.Error
	}
	if metricsUpdate.UpdatedAt.After(time.Now().Add(-6 * time.Hour)) {
		log.Warningf("not refreshing component readiness metrics, last updated %+v", time.Since(metricsUpdate.UpdatedAt))
		return nil
	}

	// Sort our known GA releases, and get the most recent one: that's our base release
	type releaseInfo struct {
		Version *version.Version
		Release string
	}
	var releases []releaseInfo
	for release := range releaseloader.GADateMap {
		v, err := version.NewVersion(release)
		if err != nil {
			log.WithError(err).Error("unparseable release " + release)
			return err
		}
		releases = append(releases, releaseInfo{v, release})
	}
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Version.LessThan(releases[j].Version)
	})
	mostRecentGA := releases[len(releases)-1].Release
	log.Debugf("most recent GA is %q", mostRecentGA)
	baseRelease := apitype.ComponentReportRequestReleaseOptions{
		Release: mostRecentGA,
		// Match what UI sends to API, although it's not correct TRT-1346
		Start: releaseloader.GADateMap[mostRecentGA].AddDate(0, 0, -29),
		End:   releaseloader.GADateMap[mostRecentGA].Add(-1 * time.Second),
	}

	// Get the next minor, that's our sample release
	next, err := nextMinor(mostRecentGA)
	if err != nil {
		log.WithError(err).Error("couldn't calculate next minor")
		return err
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	sampleRelease := apitype.ComponentReportRequestReleaseOptions{
		Release: next,
		Start:   today.AddDate(0, 0, -7),
		// Match what UI sends to API, although it's not correct TRT-1346
		End: today.Add(24 * time.Hour).Add(-1 * time.Second),
	}

	testIDOption := apitype.ComponentReportRequestTestIdentificationOptions{}
	excludeOption := apitype.ComponentReportRequestExcludeOptions{
		ExcludePlatforms: api.DefaultExcludePlatforms,
		ExcludeArches:    api.DefaultExcludeArches,
		ExcludeVariants:  api.DefaultExcludeVariants,
	}
	variantOption := apitype.ComponentReportRequestVariantOptions{
		GroupBy: api.DefaultGroupBy,
	}
	advancedOption := apitype.ComponentReportRequestAdvancedOptions{
		MinimumFailure:   api.DefaultMinimumFailure,
		Confidence:       api.DefaultConfidence,
		PityFactor:       api.DefaultPityFactor,
		IgnoreMissing:    api.DefaultIgnoreMissing,
		IgnoreDisruption: api.DefaultIgnoreDisruption,
	}

	// Get report
	rows, errs := api.GetComponentReportFromBigQuery(client, baseRelease, sampleRelease, testIDOption, variantOption, excludeOption, advancedOption)
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return fmt.Errorf("component report generation encountered errors: " + strings.Join(strErrors, "; "))
	}

	for _, row := range rows.Rows {
		for _, col := range row.Columns {
			componentReadinessMetric.WithLabelValues(row.Component, col.Network, col.Arch, col.Platform).Set(float64(col.Status))
		}
	}

	log.Infof("component readiness metrics updated...")
	metricsUpdate.UpdatedAt = time.Now()
	return dbc.DB.Save(&metricsUpdate).Error
}

func refreshInfraMetrics(dbc *db.DB, variantManager testidentification.VariantManager) error {
	for _, period := range []string{"current", "twoDay"} {
		platforms, err := query.PlatformInfraSuccess(dbc, variantManager.AllPlatforms(), period)
		if err != nil {
			return err
		}

		for platform, percent := range platforms {
			infraSuccessMetric.WithLabelValues(platform, period).Set(percent)
		}
	}

	return nil
}

func refreshBuildClusterMetrics(dbc *db.DB, reportEnd time.Time) error {
	for _, period := range []string{"current", "twoDay"} {
		start, boundary, end := util.PeriodToDates(period, reportEnd)
		result, err := query.BuildClusterHealth(dbc, start, boundary, end)
		if err != nil {
			return err
		}

		for _, cluster := range result {
			buildClusterHealthMetric.WithLabelValues(cluster.Cluster, period).Set(cluster.CurrentPassPercentage / 100)
		}
	}

	return nil
}

func refreshPayloadMetrics(dbc *db.DB, reportEnd time.Time) {
	releases, err := query.ReleasesFromDB(dbc)
	if err != nil {
		log.WithError(err).Error("error querying releases from db")
		return
	}
	for _, r := range releases {
		results, err := api.ReleaseHealthReports(dbc, r.Release, reportEnd)
		if err != nil {
			log.WithError(err).Error("error calling ReleaseHealthReports")
			return
		}

		for _, rhr := range results {
			count := 0
			if rhr.LastPhase == apitype.PayloadRejected {
				count = rhr.Count
			}
			payloadConsecutiveRejectionsMetric.WithLabelValues(r.Release, rhr.Stream, rhr.Architecture).Set(float64(count))

			// Piggy back the results here to use the list of arch+streams:
			if rhr.LastPhase == apitype.PayloadRejected {
				possibleTestBlockers, err := api.GetPayloadStreamTestFailures(dbc, r.Release, rhr.Stream,
					rhr.Architecture, &filter.FilterOptions{Filter: &filter.Filter{}}, reportEnd)
				if err != nil {
					log.WithError(err).Error("error getting payload stream test failures")
					return
				}
				blockersFound := 0
				for _, t := range possibleTestBlockers {
					if t.BlockerScore >= blockerScoreToAlertOn {
						blockersFound++
					}
				}
				payloadPossibleTestBlockersMetric.WithLabelValues(r.Release, rhr.Stream, rhr.Architecture).
					Set(float64(blockersFound))
			}
		}

		lastAcceptedReleaseTags, err := query.GetLastAcceptedByArchitectureAndStream(dbc.DB, r.Release, reportEnd)
		if err != nil {
			log.WithError(err).Error("error querying last accepted payloads")
			return
		}

		for _, archStream := range lastAcceptedReleaseTags {
			sinceLastAccepted := time.Since(archStream.ReleaseTime)
			payloadHoursSinceLastAcceptedMetric.WithLabelValues(r.Release, archStream.Stream, archStream.Architecture).Set(sinceLastAccepted.Hours())
		}

		lastOSUpgradeTags, err := query.GetLastOSUpgradeByArchitectureAndStream(dbc.DB, r.Release)
		if err != nil {
			log.WithError(err).Error("error querying last os upgrades")
			return
		}
		for _, archStream := range lastOSUpgradeTags {
			sinceLastOS := time.Since(archStream.ReleaseTime)
			payloadHoursSinceLastOSUpgrade.WithLabelValues(r.Release, archStream.Stream, archStream.Architecture).Set(sinceLastOS.Hours())
		}

	}
}

type promReportType struct {
	release string
	period  string
}

func buildPromReportTypes(releases []query.Release) []promReportType {
	var promReportTypes []promReportType

	for _, release := range releases {
		promReportTypes = append(promReportTypes, promReportType{release: release.Release, period: string(sippyprocessingv1.TwoDayReport)})
		promReportTypes = append(promReportTypes, promReportType{release: release.Release, period: string(sippyprocessingv1.CurrentReport)})
	}

	return promReportTypes
}

func nextMinor(vStr string) (string, error) {
	// Parse the version string
	v, err := version.NewVersion(vStr)
	if err != nil {
		return "", err
	}

	// Get the segments of the version
	segments := v.Segments()
	if len(segments) < 2 {
		return "", fmt.Errorf("version '%s' does not have enough segments to determine minor", vStr)
	}

	// Increment the minor segment
	segments[1]++

	// Reconstruct version string with incremented minor version
	// Only consider major and minor segments
	nextMinorSegments := segments[:2]
	nextMinorVersionStr := make([]string, len(nextMinorSegments))
	for i, seg := range nextMinorSegments {
		nextMinorVersionStr[i] = strconv.Itoa(seg)
	}

	// Concatenate the segments to form the new version string
	return strings.Join(nextMinorVersionStr, "."), nil
}
