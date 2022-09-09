package metrics

import (
	"math"

	api "github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

var (
	installSuccessMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_install_success_last_7d",
		Help: "Successful install percentage over the last 7 days for variants we're interested in, and All.",
	}, []string{"release", "variant"})

	upgradeSuccessMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_upgrade_success_last_7d",
		Help: "Successful upgrade percentage over the last 7 days for variants we're interested in, and All.",
	}, []string{"release", "variant"})

	installSuccessDeltaToPrevWeekMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_install_success_delta_last_7d_vs_prev_7d",
		Help: "Change in successful install percentage over the last 7 days vs previous 7 days. If positive we're improving.",
	}, []string{"release", "variant"})

	upgradeSuccessDeltaToPrevWeekMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_upgrade_success_delta_last_7d_vs_prev_7d",
		Help: "Change in successful upgrade percentage over the last 7 days vs previous 7 days. If positive we're improving.",
	}, []string{"release", "variant"})
)

// refreshInstallSuccessMetrics publishes metrics for the install success test for specific variants we care about.
func refreshInstallSuccessMetrics(dbc *db.DB) error {
	return refreshTestSuccessMetrics(dbc,
		testidentification.NewInstallTestName, installSuccessMetric, installSuccessDeltaToPrevWeekMetric)
}

// refreshUpgradeSuccessMetrics publishes metrics for the install success test for specific variants we care about.
func refreshUpgradeSuccessMetrics(dbc *db.DB) error {
	return refreshTestSuccessMetrics(dbc,
		testidentification.UpgradeTestName, upgradeSuccessMetric, upgradeSuccessDeltaToPrevWeekMetric)
}

func refreshTestSuccessMetrics(dbc *db.DB, testName string, successMetric, successDeltaMetric *prometheus.GaugeVec) error {
	releases, err := query.ReleasesFromDB(dbc)
	if err != nil {
		return err
	}
	for _, release := range releases {
		_, testToVariantToResults, err := api.VariantTestsReport(dbc, release.Release,
			sets.NewString(testName), sets.NewString(), sets.NewString())
		if err != nil {
			return err
		}
		// Just use the one install test we're interested in:
		testVariants, ok := testToVariantToResults[testName]
		if !ok {
			log.WithField("release", release).Warnf("upgrade report for release did not include test: %s",
				testidentification.UpgradeTestName)
			return nil
		}

		for variant, testReport := range testVariants {
			successMetric.WithLabelValues(release.Release, variant).Set(math.Round(testReport.CurrentPassPercentage*100) / 100)
			successDeltaMetric.WithLabelValues(release.Release, variant).Set(
				math.Round((testReport.CurrentPassPercentage-testReport.PreviousPassPercentage)*100) / 100)
		}
	}

	return nil
}
