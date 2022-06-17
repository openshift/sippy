package metrics

import (
	"time"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

var (
	jobPassRatioMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_job_pass_ratio",
		Help: "Ratio of passed job runs for the given job in a period (2 day, 7 day, etc)",
	}, []string{"release", "period", "name"})
	releaseWarningsMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_release_warnings",
		Help: "Number of current warnings for a release, see overview page in UI for details",
	}, []string{"release"})
	payloadConsecutiveRejectionsMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_payloads_consecutively_rejected",
		Help: "Number of consecutive rejected payloads in each release, stream and arch combo. Will be 0 if most recent payload accepted.",
	}, []string{"release", "stream", "architecture"})
)

func RefreshMetricsDB(dbc *db.DB) error {
	releases, err := query.ReleasesFromDB(dbc)
	if err != nil {
		return err
	}

	promReportTypes := buildPromReportTypes(releases)
	if err != nil {
		return err
	}

	for _, pType := range promReportTypes {
		// start, boundary and end will just be defaults
		// the api will decide based on the period
		// and current day / time
		jobsResult, err := api.JobReportsFromDB(dbc, pType.release, pType.period, nil, time.Time{}, time.Time{}, time.Time{})

		if err != nil {
			return errors.Wrapf(err, "error refreshing prom report type %s - %s", pType.period, pType.release)
		}
		for _, jobResult := range jobsResult {
			jobPassRatioMetric.WithLabelValues(pType.release, pType.period, jobResult.Name).Set(jobResult.CurrentPassPercentage / 100)
		}
	}

	// Add a metric for any warnings for each release. We can't convey exact details with prom, but we can
	// tell you x warnings are present and link you to the overview in the alert.
	for _, release := range releases {
		releaseWarnings := api.ScanForReleaseWarnings(dbc, release.Release)
		releaseWarningsMetric.WithLabelValues(release.Release).Set(float64(len(releaseWarnings)))
	}

	refreshPayloadMetrics(dbc)

	return nil
}

func refreshPayloadMetrics(dbc *db.DB) {
	// TODO: drop 4.11

	releases, err := query.ReleasesFromDB(dbc)
	if err != nil {
		log.WithError(err).Error("error querying releases from db")
		return
	}
	for _, r := range releases {
		results, err := api.ReleaseHealthReports(dbc, r.Release)
		if err != nil {
			log.WithError(err).Error("error calling ReleaseHealthReports")
			return
		}

		for _, rhr := range results {
			count := 0
			if rhr.LastPhase == apitype.PayloadRejected {
				count = rhr.Count
			}
			payloadConsecutiveRejectionsMetric.WithLabelValues(rhr.Release, rhr.Stream, rhr.Architecture).Set(float64(count))
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
