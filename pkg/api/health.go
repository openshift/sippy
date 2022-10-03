package api

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/montanaflynn/stats"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testgridanalysis/testreportconversion"
	"github.com/openshift/sippy/pkg/testidentification"
)

type indicator struct {
	Current  sippyv1.PassRate `json:"current"`
	Previous sippyv1.PassRate `json:"previous"`
}

type variants struct {
	Current  sippyprocessingv1.VariantHealth `json:"current"`
	Previous sippyprocessingv1.VariantHealth `json:"previous"`
}

type health struct {
	Indicators  map[string]indicator         `json:"indicators"`
	Variants    variants                     `json:"variants"`
	LastUpdated time.Time                    `json:"last_updated"`
	Promotions  map[string]time.Time         `json:"promotions"`
	Warnings    []string                     `json:"warnings"`
	Current     sippyprocessingv1.Statistics `json:"current_statistics"`
	Previous    sippyprocessingv1.Statistics `json:"previous_statistics"`
}

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
func PrintOverallReleaseHealthFromDB(w http.ResponseWriter, dbc *db.DB, release string) {
	excludedVariants := []string{"never-stable"}
	// Minor upgrades install a previous version and should not be counted against the current version's install stat.
	excludedInstallVariants := append(excludedVariants, "upgrade-minor")

	indicators := make(map[string]indicator)

	infraTestName := testidentification.InfrastructureTestName
	installTestName := testidentification.InstallTestName
	if useNewInstallTest(release) {
		infraTestName = testidentification.NewInfrastructureTestName
		installTestName = testidentification.NewInstallTestName
	}
	// Infrastructure
	infraIndicator, err := getIndicatorForTest(dbc, release, infraTestName, excludedVariants)
	if err != nil {
		log.WithError(err).Error("error querying test report")
		return
	}
	indicators["infrastructure"] = infraIndicator

	// Install Configuration
	installConfigIndicator, err := getIndicatorForTest(dbc, release, testidentification.InstallConfigTestName, excludedInstallVariants)
	if err != nil {
		log.WithError(err).Error("error querying test report")
		return
	}
	indicators["installConfig"] = installConfigIndicator

	// Bootstrap
	bootstrapIndicator, err := getIndicatorForTest(dbc, release, testidentification.InstallBootstrapTestName, excludedInstallVariants)
	if err != nil {
		log.WithError(err).Error("error querying test report")
		return
	}
	indicators["bootstrap"] = bootstrapIndicator

	// Install Other
	installOtherIndicator, err := getIndicatorForTest(dbc, release, testidentification.InstallOtherTestName, excludedInstallVariants)
	if err != nil {
		log.WithError(err).Error("error querying test report")
		return
	}
	indicators["installOther"] = installOtherIndicator

	// Install
	installIndicator, err := getIndicatorForTest(dbc, release, installTestName, excludedInstallVariants)
	if err != nil {
		log.WithError(err).Error("error querying test report")
		return
	}
	indicators["install"] = installIndicator

	// Upgrade
	upgradeIndicator, err := getIndicatorForTest(dbc, release, testidentification.UpgradeTestName, excludedVariants)
	if err != nil {
		log.WithError(err).Error("error querying test report")
		return
	}
	indicators["upgrade"] = upgradeIndicator

	// Tests
	// NOTE: this is not actually representing the percentage of tests that passed, it's representing
	// the percentage of time that all tests passed. We should probably fix that.
	testsIndicator, err := getIndicatorForTest(dbc, release, testidentification.OpenShiftTestsName, excludedVariants)
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
	start := time.Now().Add(-14 * 24 * time.Hour)
	boundary := time.Now().Add(-7 * 24 * time.Hour)
	end := time.Now()
	jobReports, err := query.JobReports(dbc, filterOpts, release, start, boundary, end)
	if err != nil {
		log.WithError(err).Error("error querying job reports")
		return
	}
	currStats, prevStats := calculateJobResultStatistics(jobReports)

	warnings := ScanForReleaseWarnings(dbc, release)

	RespondWithJSON(http.StatusOK, w, health{
		Indicators:  indicators,
		LastUpdated: lastUpdated,
		Current:     currStats,
		Previous:    prevStats,
		Warnings:    warnings,
	})
}

func getIndicatorForTest(dbc *db.DB, release, testName string, excludedVariants []string) (indicator, error) {
	testReport, err := query.TestReportExcludeVariants(dbc, release, testName, excludedVariants)
	if err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// This would likely mean we're in upstream kube mode. Returning an empty
			// indicator will cause the UI to hide these panels.
			return indicator{}, nil
		}

		log.WithError(err).Error("error querying test report")
		return indicator{}, err
	}

	currentPassRate := sippyv1.PassRate{
		Percentage: testreportconversion.ConvertNaNToZero(testReport.CurrentPassPercentage),
		Runs:       testReport.CurrentRuns,
	}
	previousPassRate := sippyv1.PassRate{
		Percentage: testreportconversion.ConvertNaNToZero(testReport.PreviousPassPercentage),
		Runs:       testReport.PreviousRuns,
	}
	return indicator{
		Current:  currentPassRate,
		Previous: previousPassRate,
	}, nil
}

func calculateJobResultStatistics(results []apitype.Job) (currStats, prevStats sippyprocessingv1.Statistics) {
	currPercentages := []float64{}
	prevPercentages := []float64{}
	currStats.Histogram = make([]int, 10)
	prevStats.Histogram = make([]int, 10)

	for _, result := range results {
		if testreportconversion.IsNeverStable(result.Variants) {
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

	data := stats.LoadRawData(currPercentages)
	mean, _ := stats.Mean(data)
	sd, _ := stats.StandardDeviation(data)
	quartiles, _ := stats.Quartile(data)
	p95, _ := stats.Percentile(data, 95)

	currStats.Mean = mean
	currStats.StandardDeviation = sd
	currStats.Quartiles = []float64{
		testreportconversion.ConvertNaNToZero(quartiles.Q1),
		testreportconversion.ConvertNaNToZero(quartiles.Q2),
		testreportconversion.ConvertNaNToZero(quartiles.Q3),
	}
	currStats.P95 = p95

	data = stats.LoadRawData(prevPercentages)
	mean, _ = stats.Mean(data)
	sd, _ = stats.StandardDeviation(data)
	quartiles, _ = stats.Quartile(data)
	p95, _ = stats.Percentile(data, 95)

	prevStats.Mean = mean
	prevStats.StandardDeviation = sd
	prevStats.Quartiles = []float64{
		testreportconversion.ConvertNaNToZero(quartiles.Q1),
		testreportconversion.ConvertNaNToZero(quartiles.Q2),
		testreportconversion.ConvertNaNToZero(quartiles.Q3),
	}
	prevStats.P95 = p95

	return currStats, prevStats
}
