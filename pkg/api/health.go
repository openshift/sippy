package api

import (
	"math"
	"net/http"
	"time"

	"github.com/montanaflynn/stats"
	"k8s.io/klog"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testreportconversion"
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

// PrintOverallReleaseHealth gives a summarized status of the overall health, including
// infrastructure, install, upgrade, and variant success rates.
func PrintOverallReleaseHealth(w http.ResponseWriter, curr, twoDay, prev sippyprocessingv1.TestReport) {
	indicators := make(map[string]indicator)

	// Infrastructure
	res := curr.TopLevelIndicators.Infrastructure.TestResultAcrossAllJobs
	passPercent := res.PassPercentage
	total := res.Successes + res.Failures + res.Flakes
	currentPassRate := sippyv1.PassRate{
		Percentage: passPercent,
		Runs:       total,
	}

	res = prev.TopLevelIndicators.Infrastructure.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	previousPassRate := sippyv1.PassRate{
		Percentage: passPercent,
		Runs:       total,
	}

	indicators["infrastructure"] = indicator{
		Current:  currentPassRate,
		Previous: previousPassRate,
	}

	// Install
	res = curr.TopLevelIndicators.Install.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	currentPassRate = sippyv1.PassRate{
		Percentage: passPercent,
		Runs:       total,
	}

	res = prev.TopLevelIndicators.Install.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	previousPassRate = sippyv1.PassRate{
		Percentage: passPercent,
		Runs:       total,
	}

	indicators["install"] = indicator{
		Current:  currentPassRate,
		Previous: previousPassRate,
	}

	// Upgrade
	res = curr.TopLevelIndicators.Upgrade.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	currentPassRate = sippyv1.PassRate{
		Percentage: passPercent,
		Runs:       total,
	}

	res = prev.TopLevelIndicators.Upgrade.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	previousPassRate = sippyv1.PassRate{
		Percentage: passPercent,
		Runs:       total,
	}

	indicators["upgrade"] = indicator{
		Current:  currentPassRate,
		Previous: previousPassRate,
	}

	// Tests
	res = curr.TopLevelIndicators.Tests.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	currentPassRate = sippyv1.PassRate{
		Percentage: passPercent,
		Runs:       total,
	}

	res = prev.TopLevelIndicators.Tests.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	previousPassRate = sippyv1.PassRate{
		Percentage: passPercent,
		Runs:       total,
	}

	indicators["tests"] = indicator{
		Current:  currentPassRate,
		Previous: previousPassRate,
	}

	RespondWithJSON(http.StatusOK, w, health{
		Indicators:  indicators,
		LastUpdated: curr.Timestamp,
		Variants: variants{
			Current:  curr.TopLevelIndicators.Variant,
			Previous: prev.TopLevelIndicators.Variant,
		},
		Current:  curr.JobStatistics,
		Previous: prev.JobStatistics,
		Warnings: append(curr.AnalysisWarnings, prev.AnalysisWarnings...),
	})
}

// PrintOverallReleaseHealthFromDB gives a summarized status of the overall health, including
// infrastructure, install, upgrade, and variant success rates.
func PrintOverallReleaseHealthFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB, release string) {
	indicators := make(map[string]indicator)

	// Infrastructure
	infraIndicator, err := getIndicatorForTest(dbc, release, testgridanalysisapi.InfrastructureTestName)
	if err != nil {
		klog.Errorf("error querying test report: %s", err)
		return
	}
	indicators["infrastructure"] = infraIndicator

	// Install
	installIndicator, err := getIndicatorForTest(dbc, release, testgridanalysisapi.InstallTestName)
	if err != nil {
		klog.Errorf("error querying test report: %s", err)
		return
	}
	indicators["install"] = installIndicator

	// Upgrade
	upgradeIndicator, err := getIndicatorForTest(dbc, release, testgridanalysisapi.UpgradeTestName)
	if err != nil {
		klog.Errorf("error querying test report: %s", err)
		return
	}
	indicators["upgrade"] = upgradeIndicator

	// Tests
	// NOTE: this is not actually representing the percentage of tests that passed, it's representing
	// the percentage of time that all tests passed. We should probably fix that.
	testsIndicator, err := getIndicatorForTest(dbc, release, testgridanalysisapi.OpenShiftTestsName)
	if err != nil {
		klog.Errorf("error querying test report: %s", err)
		return
	}
	indicators["tests"] = testsIndicator

	var lastUpdated time.Time
	r := dbc.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&lastUpdated)
	if r.Error != nil {
		klog.Errorf("error querying last update time: %s", r.Error)
		return
	}
	klog.Infof("ran the last update query: %s", lastUpdated)

	// Load all the job reports for this release to calculate statistics:
	filterOpts := &filter.FilterOptions{
		Filter:    &filter.Filter{},
		SortField: "current_pass_percentage",
		Sort:      apitype.SortDescending,
		Limit:     0,
	}
	start, boundary, end, err := getTimeParams(w, req)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": err.Error()})
		return
	}

	jobReports, err := query.JobReports(dbc, filterOpts, release, start, boundary, end)
	if err != nil {
		klog.Errorf("error querying job reports: %s", err)
		return
	}
	currStats, prevStats := calculateJobResultStatistics(jobReports)

	RespondWithJSON(http.StatusOK, w, health{
		Indicators:  indicators,
		LastUpdated: lastUpdated,
		Current:     currStats,
		Previous:    prevStats,
		/*
				// NOTE: looks unused in ReleaseOverview and of questionable use in general. This field would show
				// the number of variants whose jobs fully pass over 80% of the time, 60% of the time, or less than that.
				// skipping for now.
					Variants: variants{
						Current:  curr.TopLevelIndicators.Variant,
						Previous: prev.TopLevelIndicators.Variant,
					},

			    // TODO: do we have need of these anymore with the new installer tests coming from junit?
					Warnings: append(curr.AnalysisWarnings, prev.AnalysisWarnings...),
		*/
	})
}

func getIndicatorForTest(dbc *db.DB, release, testName string) (indicator, error) {
	testReport, err := query.TestReportExcludeVariants(dbc, release, testName, []string{"never-stable", "techpreview"})
	if err != nil {
		klog.Errorf("error querying test report: %s", err)
		return indicator{}, err
	}

	currentPassRate := sippyv1.PassRate{
		Percentage: testReport.CurrentPassPercentage,
		Runs:       testReport.CurrentRuns,
	}
	previousPassRate := sippyv1.PassRate{
		Percentage: testReport.PreviousPassPercentage,
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
		if testreportconversion.IsNeverStableOrTechPreview(result.Variants) {
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
