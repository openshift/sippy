package metrics

import (
	"fmt"

	api "github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	installSuccessMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_install_success",
		Help: "Successful install percentage over the last 7 days for variants we're interested in, and All.",
	}, []string{"release", "variant"})

	upgradeSuccessMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_upgrade_success",
		Help: "Successful upgrade percentage over the last 7 days for variants we're interested in, and All.",
	}, []string{"release", "variant"})

	// testMetricPlatformVariants will each get an install and upgrade metric label.
	testMetricVariants = sets.NewString(
		"All", // include the total All result across all variants, often what we're most interested in monitoring
		"aws",
		"gcp",
		"azure",
		"openstack",
		"metal-ipi",
		"single-node",
		"vsphere-ipi",
	)
)

// TODO: should we sum metrics for specific variants? platforms? techpreview?

// refreshInstallSuccessMetrics publishes metrics for the install success test for specific variants we care about.
func refreshInstallSuccessMetrics(dbc *db.DB) error {
	releases, err := query.ReleasesFromDB(dbc)
	if err != nil {
		return err
	}
	for _, release := range releases {
		_, testToVariantToResults, err := api.VariantTestsReport(dbc, release.Release,
			sets.NewString(testidentification.NewInstallTestName), sets.NewString(), sets.NewString())
		if err != nil {
			return err
		}
		// Just use the one install test we're interested in:
		instTestVariants, ok := testToVariantToResults[testidentification.NewInstallTestName]
		if !ok {
			return fmt.Errorf("install report did not include test: %s", testidentification.NewInstallTestName)
		}

		for variant, testReport := range instTestVariants {
			if testMetricVariants.Has(variant) {
				installSuccessMetric.WithLabelValues(release.Release, variant).Set(testReport.CurrentPassPercentage)
			}
		}
	}

	return nil
}

// refreshUpgradeSuccessMetrics publishes metrics for the install success test for specific variants we care about.
func refreshUpgradeSuccessMetrics(dbc *db.DB) error {
	releases, err := query.ReleasesFromDB(dbc)
	if err != nil {
		return err
	}
	for _, release := range releases {
		_, testToVariantToResults, err := api.VariantTestsReport(dbc, release.Release,
			sets.NewString(testidentification.UpgradeTestName), sets.NewString(), sets.NewString())
		if err != nil {
			return err
		}
		// Just use the one install test we're interested in:
		testVariants, ok := testToVariantToResults[testidentification.UpgradeTestName]
		if !ok {
			return fmt.Errorf("upgrade report did not include test: %s", testidentification.UpgradeTestName)
		}

		for variant, testReport := range testVariants {
			if testMetricVariants.Has(variant) {
				upgradeSuccessMetric.WithLabelValues(release.Release, variant).Set(testReport.CurrentPassPercentage)
			}
		}
	}

	return nil
}
