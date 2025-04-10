package metrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"

	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
)

const (
	blockerScoreToAlertOn = 70
)

const (
	jobPassRatioMetricName                  = "sippy_job_pass_ratio" // #nosec G101
	disruptionVsPrevGAMetricName            = "sippy_disruption_vs_prev_ga"
	payloadHoursSinceLastAcceptedMetricName = "sippy_payloads_hours_since_last_accepted"
	payloadConsecutiveRejectionsMetricName  = "sippy_payloads_consecutively_rejected"
	payloadPossibleTestBlockersMetricName   = "sippy_payloads_possible_test_blockers"
	installSuccessMetricName                = "sippy_install_success_last"
	upgradeSuccessMetricName                = "sippy_upgrade_success_last"
	installSuccessDeltaToPrevWeekMetricName = "sippy_install_success_delta_last"
	upgradeSuccessDeltaToPrevWeekMetricName = "sippy_upgrade_success_delta_last"
	payloadHoursSinceLastOSUpgradeName      = "sippy_payloads_hours_since_last_os_upgrade"
)

const (
	silencedTrue  = "true"
	silencedFalse = "false"
)

const (
	releaseStatusEOL = "End of life"
)

var (
	buildClusterHealthMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_build_cluster_pass_ratio",
		Help: "Ratio of passed job runs for a build cluster in a period (2 day, 7 day, etc)",
	}, []string{"cluster", "period"})
	jobPassRatioMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: jobPassRatioMetricName,
		Help: "Ratio of passed job runs for the given job in a period (2 day, 7 day, etc)",
	}, []string{"release", "period", "name", "silenced", "releaseStatus"})
	payloadConsecutiveRejectionsMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: payloadConsecutiveRejectionsMetricName,
		Help: "Number of consecutive rejected payloads in each release, stream and arch combo. Will be 0 if most recent payload accepted.",
	}, []string{"release", "stream", "architecture", "releaseStatus"})
	payloadHoursSinceLastAcceptedMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: payloadHoursSinceLastAcceptedMetricName,
		Help: "Number of hours since last accepted payload in each release, stream and arch combo.",
	}, []string{"release", "stream", "architecture", "releaseStatus"})
	payloadHoursSinceLastOSUpgrade = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: payloadHoursSinceLastOSUpgradeName,
		Help: "Number of hours since last OS upgrade.",
	}, []string{"release", "stream", "architecture", "releaseStatus"})
	hoursSinceLastUpdate = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_hours_since_last_update",
		Help: "Number of hours since Sippy last successfully fetched new data.",
	}, []string{})
	componentReadinessMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_component_readiness",
		Help: "Regression score for components",
	}, []string{"release", "releaseStatus", "view", "component", "network", "arch", "platform"})
	componentReadinessUniqueRegressionsMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_component_readiness_unique_regressions",
		Help: "Number of unique tests regressed per component",
	}, []string{"release", "releaseStatus", "view", "component"})
	componentReadinessTotalRegressionsMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_component_readiness_total_regressions",
		Help: "Number of regressions per component, includes tests multiple times when regressed on multiple NURP's",
	}, []string{"release", "releaseStatus", "view", "component"})
	disruptionVsPrevGAMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: disruptionVsPrevGAMetricName,
		Help: "Delta of percentiles now vs the 30 days prior to previous release GA date",
	}, []string{"delta", "release", "compare_release", "platform", "backend", "upgrade_type", "master_nodes_updated", "network", "topology", "architecture", "feature_set", "releaseStatus"})
	disruptionVsPrevGARelevanceMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_disruption_vs_prev_ga_relevance",
		Help: "Rating of how relevant we feel our data is for regression detection.",
	}, []string{"release", "compare_release", "platform", "backend", "upgrade_type", "master_nodes_updated", "network", "topology", "architecture", "feature_set", "releaseStatus"})
)

func getReleaseStatus(releases []v1.Release, release string) string {
	releaseStatus := releaseStatusEOL
	for _, r := range releases {
		if r.Release == release && len(r.Status) != 0 {
			releaseStatus = r.Status
			break
		}
	}
	return releaseStatus
}

// presume in a historical context there won't be scraping of these metrics
// pinning the time just to be consistent
func RefreshMetricsDB(ctx context.Context, dbc *db.DB, bqc *bqclient.Client, reportEnd time.Time, cacheOptions cache.RequestOptions, views []crtype.View, variantJunitTableOverrides []configv1.VariantJunitTableOverride) error {
	start := time.Now()
	log.Info("beginning refresh metrics")
	releases, err := api.GetReleases(context.Background(), bqc)
	if err != nil {
		return err
	}

	// Local DB metrics
	if dbc != nil {
		promReportTypes := buildPromReportTypes(releases)

		// Get last updated job run
		var lastUpdated time.Time
		if r := dbc.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&lastUpdated); r.Error != nil {
			return errors.Wrapf(r.Error, "could not fetch last updated time")
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
				silenced := silencedFalse
				if jobResult.CurrentRuns == 0 {
					silenced = silencedTrue
				}
				if jobResult.OpenBugs > 0 {
					silenced = silencedTrue
				}
				releaseStatus := getReleaseStatus(releases, pType.release)
				jobPassRatioMetric.WithLabelValues(pType.release, pType.period, jobResult.Name, silenced, releaseStatus).Set(jobResult.CurrentPassPercentage / 100)
			}
		}

		if err := refreshBuildClusterMetrics(dbc, reportEnd); err != nil {
			log.WithError(err).Error("error refreshing build cluster metrics")
		}

		refreshPayloadMetrics(dbc, reportEnd, releases)

	}

	// BigQuery metrics
	if bqc != nil {
		refreshComponentReadinessMetrics(ctx, bqc, dbc, cacheOptions, views, releases, variantJunitTableOverrides)

		if err := refreshDisruptionMetrics(bqc, releases); err != nil {
			log.WithError(err).Error("error refreshing disruption metrics")
		}
	}

	log.Infof("refresh metrics completed in %s", time.Since(start))

	return nil
}

func refreshComponentReadinessMetrics(ctx context.Context, client *bqclient.Client, dbc *db.DB,
	cacheOptions cache.RequestOptions, views []crtype.View, releases []v1.Release, variantJunitTableOverrides []configv1.VariantJunitTableOverride) {
	if client == nil || client.BQ == nil {
		log.Warningf("not generating component readiness metrics as we don't have a bigquery client")
		return
	}

	if client.Cache == nil {
		log.Warningf("not generating component readiness metrics as we don't have a cache configured")
		return
	}

	for _, view := range views {
		if view.Metrics.Enabled {
			err := updateComponentReadinessMetricsForView(ctx, client, dbc, cacheOptions, view, releases, variantJunitTableOverrides)
			if err != nil {
				log.WithError(err).Error("error")
				log.WithError(err).WithField("view", view.Name).Error("error refreshing metrics/regressions for view")
				// continue to next view
			}
		}
	}
}

// updateComponentReadinessTrackingForView queries the report for the given view, and then updates metrics.
func updateComponentReadinessMetricsForView(ctx context.Context, client *bqclient.Client, dbc *db.DB, cacheOptions cache.RequestOptions, view crtype.View, releases []v1.Release, overrides []configv1.VariantJunitTableOverride) error {

	logger := log.WithField("view", view.Name)
	logger.Info("generating report for view")

	baseRelease, err := componentreadiness.GetViewReleaseOptions(
		releases, "basis", view.BaseRelease, cacheOptions.CRTimeRoundingFactor)
	if err != nil {
		return err
	}

	sampleRelease, err := componentreadiness.GetViewReleaseOptions(
		releases, "sample", view.SampleRelease, cacheOptions.CRTimeRoundingFactor)
	if err != nil {
		return err
	}

	variantOption := view.VariantOptions
	advancedOption := view.AdvancedOptions

	// Get component readiness report
	reportOpts := crtype.RequestOptions{
		BaseRelease:    baseRelease,
		SampleRelease:  sampleRelease,
		TestIDOption:   crtype.RequestTestIdentificationOptions{},
		VariantOption:  variantOption,
		AdvancedOption: advancedOption,
		CacheOption:    cacheOptions,
	}

	report, errs := componentreadiness.GetComponentReportFromBigQuery(ctx, client, dbc, reportOpts, overrides)
	if len(errs) > 0 {
		var strErrors []string
		for _, err := range errs {
			strErrors = append(strErrors, err.Error())
		}
		return fmt.Errorf("component report generation encountered errors: %s", strings.Join(strErrors, "; "))
	}

	logger.Info("publishing metrics for view")

	releaseStatus := getReleaseStatus(releases, view.SampleRelease.Release)
	for _, row := range report.Rows {
		totalRegressedTestsByComponent := 0
		uniqueRegressedTestsByComponent := sets.NewString()
		for _, col := range row.Columns {
			// Calculate total number of regressions by component, this can include a test multiple times
			// if it's regressed in multiple NURP's.
			totalRegressedTestsByComponent += len(col.RegressedTests)
			// Calculate total number of unique tests that are regressed by component
			for _, regressedTest := range col.RegressedTests {
				uniqueRegressedTestsByComponent.Insert(regressedTest.TestID)
			}
			// TODO: why specific variants here?
			networkLabel, ok := col.Variants["Network"]
			if !ok {
				networkLabel = ""
			}
			archLabel, ok := col.Variants["Architecture"]
			if !ok {
				archLabel = ""
			}
			platLabel, ok := col.Variants["Platform"]
			if !ok {
				platLabel = ""
			}
			componentReadinessMetric.WithLabelValues(view.SampleRelease.Release, releaseStatus, view.Name,
				row.Component, networkLabel, archLabel, platLabel).Set(float64(col.Status))
		}
		componentReadinessTotalRegressionsMetric.WithLabelValues(view.SampleRelease.Release, releaseStatus, view.Name, row.Component).Set(float64(totalRegressedTestsByComponent))
		componentReadinessUniqueRegressionsMetric.WithLabelValues(view.SampleRelease.Release, releaseStatus, view.Name, row.Component).Set(float64(uniqueRegressedTestsByComponent.Len()))
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

func refreshPayloadMetrics(dbc *db.DB, reportEnd time.Time, releases []v1.Release) {
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
			payloadConsecutiveRejectionsMetric.WithLabelValues(r.Release, rhr.Stream, rhr.Architecture, getReleaseStatus(releases, r.Release)).Set(float64(count))

		}

		lastAcceptedReleaseTags, err := query.GetLastAcceptedByArchitectureAndStream(dbc.DB, r.Release, reportEnd)
		if err != nil {
			log.WithError(err).Error("error querying last accepted payloads")
			return
		}

		for _, archStream := range lastAcceptedReleaseTags {
			sinceLastAccepted := time.Since(archStream.ReleaseTime)
			payloadHoursSinceLastAcceptedMetric.WithLabelValues(r.Release, archStream.Stream, archStream.Architecture,
				getReleaseStatus(releases, r.Release)).Set(sinceLastAccepted.Hours())
		}

		lastOSUpgradeTags, err := query.GetLastOSUpgradeByArchitectureAndStream(dbc.DB, r.Release)
		if err != nil {
			log.WithError(err).Error("error querying last os upgrades")
			return
		}
		for _, archStream := range lastOSUpgradeTags {
			sinceLastOS := time.Since(archStream.ReleaseTime)
			payloadHoursSinceLastOSUpgrade.WithLabelValues(r.Release, archStream.Stream, archStream.Architecture, getReleaseStatus(releases, r.Release)).Set(sinceLastOS.Hours())
		}

	}
}

// refreshDisruptionMetrics queries our BigQuery views for current release vs two weeks ago, and previous release GA.
// Metrics are published for the delta for each NURP which can then be alerted on if certain thresholds are exceeded.
// The previous GA view should have its release and GA date updated on each release GA.
func refreshDisruptionMetrics(client *bqclient.Client, releases []v1.Release) error {
	if client == nil || client.BQ == nil {
		log.Warningf("not generating disruption metrics as we don't have a bigquery client")
		return nil
	}

	if client.Cache == nil {
		log.Warningf("not generating disruption metrics as we don't have a cache configured")
		return nil
	}

	disruptionReport, err := api.GetDisruptionVsPrevGAReportFromBigQuery(context.Background(), client)
	if err != nil {
		return fmt.Errorf("errors returned: %v", err)
	}

	for _, row := range disruptionReport.Rows {
		releaseStatus := getReleaseStatus(releases, row.Release)
		disruptionVsPrevGAMetric.WithLabelValues("P50",
			row.Release, row.CompareRelease, row.Platform, row.BackendName, row.UpgradeType,
			row.MasterNodesUpdated, row.Network, row.Topology, row.Architecture, row.FeatureSet, releaseStatus).Set(float64(row.P50))
		disruptionVsPrevGAMetric.WithLabelValues("P75",
			row.Release, row.CompareRelease, row.Platform, row.BackendName, row.UpgradeType,
			row.MasterNodesUpdated, row.Network, row.Topology, row.Architecture, row.FeatureSet, releaseStatus).Set(float64(row.P75))
		disruptionVsPrevGAMetric.WithLabelValues("P95",
			row.Release, row.CompareRelease, row.Platform, row.BackendName, row.UpgradeType,
			row.MasterNodesUpdated, row.Network, row.Topology, row.Architecture, row.FeatureSet, releaseStatus).Set(float64(row.P95))
		disruptionVsPrevGAMetric.WithLabelValues("PercentageAboveZero",
			row.Release, row.CompareRelease, row.Platform, row.BackendName, row.UpgradeType,
			row.MasterNodesUpdated, row.Network, row.Topology, row.Architecture, row.FeatureSet, releaseStatus).Set(float64(row.PercentageAboveZeroDelta))
		disruptionVsPrevGARelevanceMetric.WithLabelValues(
			row.Release, row.CompareRelease, row.Platform, row.BackendName, row.UpgradeType,
			row.MasterNodesUpdated, row.Network, row.Topology, row.Architecture, row.FeatureSet, releaseStatus).Set(float64(row.Relevance))
	}

	return nil
}

type promReportType struct {
	release string
	period  string
}

func buildPromReportTypes(releases []v1.Release) []promReportType {
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
