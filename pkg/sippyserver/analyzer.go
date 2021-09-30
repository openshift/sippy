package sippyserver

import (
	"fmt"
	"regexp"
	"time"

	"github.com/openshift/sippy/pkg/bigqueryanalysis"

	bigqueryv1 "github.com/openshift/sippy/pkg/apis/bigquery/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/testgridanalysis/testreportconversion"
	"github.com/openshift/sippy/pkg/util/sets"
	"k8s.io/klog"
)

// Allows one to pass in an alternative testgrid loader func for testing.
type TestGridLoader func(string, []string, *regexp.Regexp) ([]testgridv1.JobDetails, time.Time)

// TestGridLoadingOptions control the data which is loaded from disk into the testgrid structs
type TestGridLoadingConfig struct {
	// LocalData is the directory where the testgrid data is stored
	LocalData string
	// JobFilter is a regex run against job names. Only match names are loaded.
	JobFilter *regexp.Regexp
	// The function to load TestGrid results from disk, used for testing.
	Loader TestGridLoader
}

func (t TestGridLoadingConfig) loadWithFilter(dashboards []string, jobFilter *regexp.Regexp) ([]testgridv1.JobDetails, time.Time) {
	// If TestGridLoader isn't defined, use the default one
	if t.Loader == nil {
		return testgridhelpers.LoadTestGridDataFromDisk(t.LocalData, dashboards, jobFilter)
	}

	return t.Loader(t.LocalData, dashboards, jobFilter)
}

func (t TestGridLoadingConfig) load(dashboards []string) ([]testgridv1.JobDetails, time.Time) {
	return t.loadWithFilter(dashboards, t.JobFilter)
}

// RawJobResultsAnalysisOptions control which subset of data from the testgrid data is analyzed into the rawJobResults
type RawJobResultsAnalysisConfig struct {
	StartDay int
	NumDays  int
}

// DisplayDataOptions controls how the RawJobResults are processed and prepared for display
type DisplayDataConfig struct {
	MinTestRuns             int
	TestSuccessThreshold    float64
	FailureClusterThreshold int
}

// TestReportGeneratorConfig is a static configuration that can be re-used across multiple invocations of PrepareTestReport with different versions
type TestReportGeneratorConfig struct {
	TestGridLoadingConfig       TestGridLoadingConfig
	RawJobResultsAnalysisConfig RawJobResultsAnalysisConfig
	DisplayDataConfig           DisplayDataConfig
}

// PrepareTestReport is expensive.  It
//  1. gathers test grid data from disk
//  2. proceses that data to produce RawJobResults which look more how humans read testgrid
//  3. uses the RawJobResults to produce a bug cache of relevant bugs
//  4. converts the result of that into a display API object.
func (a *TestReportGeneratorConfig) PrepareTestReport(
	dashboard TestGridDashboardCoordinates,
	reportType sippyprocessingv1.ReportType,
	syntheticTestManager testgridconversion.SyntheticTestManager,
	variantManager testidentification.VariantManager,
	bugCache buganalysis.BugCache,
) sippyprocessingv1.TestReport {
	testGridJobDetails, lastUpdateTime := a.TestGridLoadingConfig.load(dashboard.TestGridDashboardNames)
	bigQueryJobDetails, bigQueryJobRuns, err := bigqueryanalysis.LoadDataFromDisk(a.TestGridLoadingConfig.LocalData)
	if err != nil {
		klog.Warningf("No BigQuery credentials found, skipping...")
		bigQueryJobDetails = nil
	}

	return a.prepareTestReportFromData(dashboard.ReportName, reportType, dashboard.BugzillaRelease, syntheticTestManager, variantManager, bugCache, testGridJobDetails, bigQueryJobDetails, bigQueryJobRuns, lastUpdateTime)
}

// prepareTestReportFromData should always remain private unless refactored. it's a convenient way to re-use the test grid data deserialized from disk.
func (a *TestReportGeneratorConfig) prepareTestReportFromData(
	reportName string,
	reportType sippyprocessingv1.ReportType,
	bugzillaRelease string,
	syntheticTestManager testgridconversion.SyntheticTestManager,
	variantManager testidentification.VariantManager,
	bugCache buganalysis.BugCache,
	testGridJobDetails []testgridv1.JobDetails,
	bigQueryJobDetails []bigqueryv1.Job,
	bigQueryJobRuns []bigqueryv1.JobRun,
	lastUpdateTime time.Time,
) sippyprocessingv1.TestReport {
	rawJobResultOptions := testgridconversion.ProcessingOptions{
		SyntheticTestManager: syntheticTestManager,
		StartDay:             a.RawJobResultsAnalysisConfig.StartDay,
		NumDays:              a.RawJobResultsAnalysisConfig.NumDays,
	}
	rawJobResults, processingWarnings := rawJobResultOptions.ProcessTestGridDataIntoRawJobResults(testGridJobDetails)
	bugCacheWarnings := updateBugCacheForJobResults(bugCache, rawJobResults)
	warnings := []string{}
	warnings = append(warnings, processingWarnings...)
	warnings = append(warnings, bugCacheWarnings...)

	return testreportconversion.PrepareTestReport(
		reportName,
		reportType,
		rawJobResults,
		variantManager,
		bugCache,
		bugzillaRelease,
		a.DisplayDataConfig.MinTestRuns,
		a.DisplayDataConfig.TestSuccessThreshold,
		a.RawJobResultsAnalysisConfig.NumDays,
		warnings,
		bigQueryJobDetails,
		bigQueryJobRuns,
		lastUpdateTime,
		a.DisplayDataConfig.FailureClusterThreshold,
	)
}

// PrepareStandardTestReports returns the current period, current two day period, and the previous seven days period
func (a TestReportGeneratorConfig) PrepareStandardTestReports(
	dashboard TestGridDashboardCoordinates,
	syntheticTestManager testgridconversion.SyntheticTestManager,
	variantManager testidentification.VariantManager,
	bugCache buganalysis.BugCache,
) StandardReport {
	testGridJobDetails, lastUpdateTime := a.TestGridLoadingConfig.load(dashboard.TestGridDashboardNames)
	bigQueryJobDetails, bigQueryJobRuns, err := bigqueryanalysis.LoadDataFromDisk(a.TestGridLoadingConfig.LocalData)
	if err != nil {
		klog.Warningf("No BigQuery credentials found, skipping...")
		bigQueryJobDetails = nil
		bigQueryJobRuns = nil
	}

	currTimePeriodConfig := a.deepCopy()
	currentTimePeriodReport := currTimePeriodConfig.prepareTestReportFromData(dashboard.ReportName, sippyprocessingv1.CurrentReport, dashboard.BugzillaRelease, syntheticTestManager, variantManager, bugCache, testGridJobDetails, bigQueryJobDetails, bigQueryJobRuns, lastUpdateTime)

	currentTwoDayPeriodConfig := a.deepCopy()
	currentTwoDayPeriodConfig.RawJobResultsAnalysisConfig.NumDays = 2
	currentTwoDayReport := currentTwoDayPeriodConfig.prepareTestReportFromData(dashboard.ReportName, sippyprocessingv1.TwoDayReport, dashboard.BugzillaRelease, syntheticTestManager, variantManager, bugCache, testGridJobDetails, bigQueryJobDetails, bigQueryJobRuns, lastUpdateTime)

	previousSevenDayPeriodConfig := a.deepCopy()
	if a.RawJobResultsAnalysisConfig.StartDay >= 0 {
		previousSevenDayPeriodConfig.RawJobResultsAnalysisConfig.StartDay = a.RawJobResultsAnalysisConfig.StartDay + a.RawJobResultsAnalysisConfig.NumDays
	} else {
		previousSevenDayPeriodConfig.RawJobResultsAnalysisConfig.StartDay = a.RawJobResultsAnalysisConfig.StartDay - a.RawJobResultsAnalysisConfig.NumDays
	}
	previousSevenDayPeriodConfig.RawJobResultsAnalysisConfig.NumDays = 7
	previousSevenDayReport := previousSevenDayPeriodConfig.prepareTestReportFromData(dashboard.ReportName, sippyprocessingv1.PreviousReport, dashboard.BugzillaRelease, syntheticTestManager, variantManager, bugCache, testGridJobDetails, bigQueryJobDetails, bigQueryJobRuns, lastUpdateTime)

	return StandardReport{
		CurrentPeriodReport: currentTimePeriodReport,
		CurrentTwoDayReport: currentTwoDayReport,
		PreviousWeekReport:  previousSevenDayReport,
	}
}

// updateBugCacheForJobResults looks up all the bugs related to every failing test in the jobResults and returns a list of
// warnings/errors that happened looking up the data
func updateBugCacheForJobResults(bugCache buganalysis.BugCache, rawJobResults testgridanalysisapi.RawData) []string {
	warnings := []string{}

	// now that we have all the test failures (remember we added sythentics), use that to update the bugzilla cache
	failedTestNamesAcrossAllJobRuns := getFailedTestNamesFromJobResults(rawJobResults.JobResults)
	if err := bugCache.UpdateForFailedTests(failedTestNamesAcrossAllJobRuns.List()...); err != nil {
		klog.Error(err)
		warnings = append(warnings, fmt.Sprintf("Bugzilla Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error()))
	}
	if err := bugCache.UpdateJobBlockers(sets.StringKeySet(rawJobResults.JobResults).List()...); err != nil {
		klog.Error(err)
		warnings = append(warnings, fmt.Sprintf("Bugzilla Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error()))
	}

	return warnings
}

func getFailedTestNamesFromJobResults(jobResults map[string]testgridanalysisapi.RawJobResult) sets.String {
	failedTestNames := sets.NewString()
	for _, jobResult := range jobResults {
		for _, jobrun := range jobResult.JobRunResults {
			failedTestNames.Insert(jobrun.FailedTestNames...)
		}
	}
	return failedTestNames
}

func (a TestReportGeneratorConfig) deepCopy() TestReportGeneratorConfig {
	ret := TestReportGeneratorConfig{
		TestGridLoadingConfig: TestGridLoadingConfig{
			LocalData: a.TestGridLoadingConfig.LocalData,
		},
		RawJobResultsAnalysisConfig: RawJobResultsAnalysisConfig{
			StartDay: a.RawJobResultsAnalysisConfig.StartDay,
			NumDays:  a.RawJobResultsAnalysisConfig.NumDays,
		},
		DisplayDataConfig: DisplayDataConfig{
			MinTestRuns:             a.DisplayDataConfig.MinTestRuns,
			TestSuccessThreshold:    a.DisplayDataConfig.TestSuccessThreshold,
			FailureClusterThreshold: a.DisplayDataConfig.FailureClusterThreshold,
		},
	}
	if a.TestGridLoadingConfig.JobFilter != nil {
		//nolint:staticcheck // Copy() is appropriate since this performs a deep copy.
		ret.TestGridLoadingConfig.JobFilter = a.TestGridLoadingConfig.JobFilter.Copy()
	}

	return ret
}
