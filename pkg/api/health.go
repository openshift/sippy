package api

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/montanaflynn/stats"
	log "github.com/sirupsen/logrus"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util"
)

// useNewInstallTest decides which install test name to use based on releases. For
// release 4.11 and above, it uses the new install test names
func useNewInstallTest(release string) bool {
	digits := strings.Split(release, ".")
	if len(digits) < 2 {
		return false
	}
	major, err := strconv.Atoi(digits[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(digits[1])
	if err != nil {
		return false
	}
	if major < 4 {
		return false
	} else if major == 4 && minor < 11 {
		return false
	}
	return true
}

// PrintOverallReleaseHealthFromDB gives a summarized status of the overall health, including
// infrastructure, install, upgrade, and variant success rates.
func PrintOverallReleaseHealthFromDB(w http.ResponseWriter, dbc *db.DB, release string, reportEnd time.Time) {
	excludedVariants := testidentification.DefaultExcludedVariants
	// Minor upgrades install a previous version and should not be counted against the current version's install stat.
	excludedInstallVariants := testidentification.DefaultExcludedVariants
	excludedInstallVariants = append(excludedInstallVariants, "upgrade-minor")

	indicators := make(map[string]apitype.Test)

	infraTestName := testidentification.InfrastructureTestName
	installTestName := testidentification.InstallTestName
	if useNewInstallTest(release) {
		infraTestName = testidentification.NewInfrastructureTestName
		installTestName = testidentification.NewInstallTestName
	}
	// Infrastructure
	infraIndicator, err := query.TestReportExcludeVariants(dbc, release, infraTestName, excludedVariants)
	if err != nil {
		log.WithError(err).Error("error querying infrastructure test report")
		return
	}
	indicators["infrastructure"] = infraIndicator

	// Install Configuration
	installConfigIndicator, err := query.TestReportExcludeVariants(dbc, release, testidentification.InstallConfigTestName, excludedInstallVariants)
	if err != nil {
		log.WithError(err).Error("error querying install test report")
		return
	}
	indicators["installConfig"] = installConfigIndicator

	// Bootstrap
	bootstrapIndicator, err := query.TestReportExcludeVariants(dbc, release, testidentification.InstallBootstrapTestName, excludedInstallVariants)
	if err != nil {
		log.WithError(err).Error("error querying bootstrap test report")
		return
	}
	indicators["bootstrap"] = bootstrapIndicator

	// Install Other
	installOtherIndicator, err := query.TestReportExcludeVariants(dbc, release, testidentification.InstallOtherTestName, excludedInstallVariants)
	if err != nil {
		log.WithError(err).Error("error querying install (other) test report")
		return
	}
	indicators["installOther"] = installOtherIndicator

	// Install
	installIndicator, err := query.TestReportExcludeVariants(dbc, release, installTestName, excludedInstallVariants)
	if err != nil {
		log.WithError(err).Error("error querying install test report")
		return
	}
	indicators["install"] = installIndicator

	// Upgrade
	upgradeIndicator, err := query.TestReportExcludeVariants(dbc, release, testidentification.UpgradeTestName, excludedVariants)
	if err != nil {
		log.WithError(err).Error("error querying upgrade test report")
		return
	}
	indicators["upgrade"] = upgradeIndicator

	// Tests
	// NOTE: this is not actually representing the percentage of tests that passed, it's representing
	// the percentage of time that all tests passed. We should probably fix that.
	testsIndicator, err := query.TestReportExcludeVariants(dbc, release, testidentification.OpenShiftTestsName, excludedVariants)
	if err != nil {
		log.WithError(err).Error("error querying test report")
		return
	}
	indicators["tests"] = testsIndicator

	var lastUpdated time.Time
	r := dbc.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&lastUpdated)
	if r.Error != nil {
		log.WithError(err).Error("error querying last update time")
		return
	}
	log.WithField("lastUpdated", lastUpdated).Info("ran the last update query")

	// Load all the job reports for this release to calculate statistics:
	filterOpts := &filter.FilterOptions{
		Filter:    &filter.Filter{},
		SortField: "current_pass_percentage",
		Sort:      apitype.SortDescending,
		Limit:     0,
	}
	start := reportEnd.Add(-14 * 24 * time.Hour)
	boundary := reportEnd.Add(-7 * 24 * time.Hour)
	end := reportEnd
	jobReports, err := query.JobReports(dbc, filterOpts, release, start, boundary, end)
	if err != nil {
		log.WithError(err).Error("error querying job reports")
		return
	}
	currStats, prevStats := calculateJobResultStatistics(jobReports)

	// Warnings used to report CoreOS version mismatches, but it is no longer tied to the OCP
	// release. These warnings are shown as a banner on the release overview page.  I left it here
	// in case we ever want to show these banners again for something else.
	// TODO: use or remove this logic
	var warnings []string

	RespondWithJSON(http.StatusOK, w, apitype.Health{
		Indicators:  indicators,
		LastUpdated: lastUpdated,
		Current:     currStats,
		Previous:    prevStats,
		Warnings:    warnings,
	})
}

func calculateJobResultStatistics(results []apitype.Job) (currStats, prevStats sippyprocessingv1.Statistics) {
	currPercentages := []float64{}
	prevPercentages := []float64{}
	currStats.Histogram = make([]int, 10)
	prevStats.Histogram = make([]int, 10)

	for _, result := range results {
		// Skip jobs that have few runs
		if result.CurrentRuns < 7 {
			continue
		}

		if util.IsNeverStable(result.Variants) {
			continue
		}

		index := int(math.Floor(result.CurrentPassPercentage / 10))
		if index == 10 { // 100% gets bucketed in the 10th bucket
			index = 9
		}
		currStats.Histogram[index]++
		currPercentages = append(currPercentages, result.CurrentPassPercentage)

		index = int(math.Floor(result.PreviousPassPercentage / 10))
		if index == 10 { // 100% gets bucketed in the 10th bucket
			index = 9
		}
		prevStats.Histogram[index]++
		prevPercentages = append(prevPercentages, result.PreviousPassPercentage)
	}

	if len(currPercentages) > 0 {
		data := stats.LoadRawData(currPercentages)
		mean, _ := stats.Mean(data)
		sd, _ := stats.StandardDeviation(data)
		quartiles, _ := stats.Quartile(data)
		p95, _ := stats.Percentile(data, 95)

		currStats.Mean = mean
		currStats.StandardDeviation = sd
		currStats.Quartiles = []float64{
			util.ConvertNaNToZero(quartiles.Q1),
			util.ConvertNaNToZero(quartiles.Q2),
			util.ConvertNaNToZero(quartiles.Q3),
		}
		currStats.P95 = p95
	}

	if len(prevPercentages) > 0 {
		data := stats.LoadRawData(prevPercentages)
		mean, _ := stats.Mean(data)
		sd, _ := stats.StandardDeviation(data)
		quartiles, _ := stats.Quartile(data)
		p95, _ := stats.Percentile(data, 95)

		prevStats.Mean = mean
		prevStats.StandardDeviation = sd
		prevStats.Quartiles = []float64{
			util.ConvertNaNToZero(quartiles.Q1),
			util.ConvertNaNToZero(quartiles.Q2),
			util.ConvertNaNToZero(quartiles.Q3),
		}
		prevStats.P95 = p95
	}

	return currStats, prevStats
}
