package componentreadiness

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"sync"
	"time"

	fet "github.com/glycerine/golang-fisher-exact"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
)

func GetTestDetails(ctx context.Context, provider dataprovider.DataProvider, dbc *db.DB, reqOptions reqopts.RequestOptions, releases []v1.Release, baseURL string) (testdetails.Report, []error) {
	generator := NewComponentReportGenerator(provider, reqOptions, dbc, releases, baseURL)
	if os.Getenv("DEV_MODE") == "1" {
		return generator.GenerateTestDetailsReport(ctx)
	}

	report, errs := api.GetDataFromCacheOrGenerate[testdetails.Report](
		ctx,
		generator.getCache(),
		generator.ReqOptions.CacheOption,
		api.NewCacheSpec(generator.GetCacheKey(ctx), "TestDetailsReport~", nil),
		generator.GenerateTestDetailsReport,
		testdetails.Report{})
	if len(errs) > 0 {
		return report, errs
	}

	err := generator.PostAnalysisTestDetails(&report)
	if err != nil {
		return report, []error{err}
	}

	return report, []error{}
}

// PostAnalysisTestDetails runs the PostAnalysis method for all middleware on this test details report.
// This is done outside the caching mechanism so we can load fresh data from our db (which is fast and cheap),
// and inject it into an expensive / slow report without recalculating everything.
func (c *ComponentReportGenerator) PostAnalysisTestDetails(report *testdetails.Report) error {

	// Give middleware their chance to adjust the result
	testKey := crtest.Identification{
		RowIdentification:    report.RowIdentification,
		ColumnIdentification: report.ColumnIdentification,
	}
	for i := range report.Analyses {
		if err := c.middlewares.PostAnalysis(testKey, &report.Analyses[i].TestComparison); err != nil {
			return err
		}
	}

	return nil
}

// GenerateTestDetailsReport is the main function to generate a test details report for a request, if we miss the cache.
func (c *ComponentReportGenerator) GenerateTestDetailsReport(ctx context.Context) (testdetails.Report, []error) {
	// This function is called from the API, and we assume only one TestIDOptions entry in that case.
	testIDOptions := c.ReqOptions.TestIDOptions[0]
	// load all pass/fails for specific jobs, both sample, basis, and override basis if requested
	componentJobRunTestReportStatus, errs := c.getJobRunTestStatus(ctx)
	if len(errs) > 0 {
		return testdetails.Report{}, errs
	}

	return c.GenerateDetailsReportForTest(ctx, testIDOptions, componentJobRunTestReportStatus, true)
}

// GenerateTestDetailsReportMultiTest variant of the function is for multi-test reports, used for cache priming all test detail reports for a view.
func (c *ComponentReportGenerator) GenerateTestDetailsReportMultiTest(ctx context.Context) ([]testdetails.Report, []error) {
	// load all pass/fails for specific jobs, both sample, basis, and override basis if requested
	before := time.Now()
	allTestsJobRunStatuses, errs := c.getJobRunTestStatus(ctx)
	if len(errs) > 0 {
		return []testdetails.Report{}, errs
	}
	logrus.Infof("getJobRunTestStatus completed in %s with %d sample results and %d base results",
		time.Since(before), len(allTestsJobRunStatuses.SampleStatus), len(allTestsJobRunStatuses.BaseStatus))

	// We have a struct where the statuses are mapped by prowjob to all rows results for that prowjob,
	// with multiple tests intermingled in that layer.
	// Build out a new struct where these are split up by test ID.
	// split the status on test ID, and pass only that tests data in for reporting:
	testKeyTestJobRunStatuses := map[string]crstatus.TestJobRunStatuses{}
	for jobName, rows := range allTestsJobRunStatuses.BaseStatus {
		for _, row := range rows {
			testKeyStr := row.TestKeyStr
			if _, ok := testKeyTestJobRunStatuses[testKeyStr]; !ok {
				testKeyTestJobRunStatuses[testKeyStr] = crstatus.TestJobRunStatuses{
					BaseStatus:         map[string][]crstatus.TestJobRunRows{},
					BaseOverrideStatus: map[string][]crstatus.TestJobRunRows{},
					SampleStatus:       map[string][]crstatus.TestJobRunRows{},
					GeneratedAt:        allTestsJobRunStatuses.GeneratedAt,
				}
			}
			if testKeyTestJobRunStatuses[testKeyStr].BaseStatus[jobName] == nil {
				testKeyTestJobRunStatuses[testKeyStr].BaseStatus[jobName] = []crstatus.TestJobRunRows{}
			}
			testKeyTestJobRunStatuses[testKeyStr].BaseStatus[jobName] =
				append(testKeyTestJobRunStatuses[testKeyStr].BaseStatus[jobName], row)
		}
	}
	for jobName, rows := range allTestsJobRunStatuses.BaseOverrideStatus {
		for _, row := range rows {
			testKeyStr := row.TestKeyStr
			if _, ok := testKeyTestJobRunStatuses[testKeyStr]; !ok {
				testKeyTestJobRunStatuses[testKeyStr] = crstatus.TestJobRunStatuses{
					BaseStatus:         map[string][]crstatus.TestJobRunRows{},
					BaseOverrideStatus: map[string][]crstatus.TestJobRunRows{},
					SampleStatus:       map[string][]crstatus.TestJobRunRows{},
					GeneratedAt:        allTestsJobRunStatuses.GeneratedAt,
				}
			}
			if testKeyTestJobRunStatuses[testKeyStr].BaseOverrideStatus[jobName] == nil {
				testKeyTestJobRunStatuses[testKeyStr].BaseOverrideStatus[jobName] = []crstatus.TestJobRunRows{}
			}
			testKeyTestJobRunStatuses[testKeyStr].BaseOverrideStatus[jobName] =
				append(testKeyTestJobRunStatuses[testKeyStr].BaseOverrideStatus[jobName], row)
		}
	}
	for jobName, rows := range allTestsJobRunStatuses.SampleStatus {
		for _, row := range rows {
			testKeyStr := row.TestKeyStr
			if _, ok := testKeyTestJobRunStatuses[testKeyStr]; !ok {
				testKeyTestJobRunStatuses[testKeyStr] = crstatus.TestJobRunStatuses{
					BaseStatus:         map[string][]crstatus.TestJobRunRows{},
					BaseOverrideStatus: map[string][]crstatus.TestJobRunRows{},
					SampleStatus:       map[string][]crstatus.TestJobRunRows{},
					GeneratedAt:        allTestsJobRunStatuses.GeneratedAt,
				}
			}
			if testKeyTestJobRunStatuses[testKeyStr].SampleStatus[jobName] == nil {
				testKeyTestJobRunStatuses[testKeyStr].SampleStatus[jobName] = []crstatus.TestJobRunRows{}
			}
			testKeyTestJobRunStatuses[testKeyStr].SampleStatus[jobName] =
				append(testKeyTestJobRunStatuses[testKeyStr].SampleStatus[jobName], row)
		}
	}

	reports := []testdetails.Report{}
	for _, tOpt := range c.ReqOptions.TestIDOptions {
		testKey := crtest.KeyWithVariants{
			TestID:   tOpt.TestID,
			Variants: tOpt.RequestedVariants,
		}
		testKeyStr := testKey.KeyOrDie()
		if statuses, ok := testKeyTestJobRunStatuses[testKeyStr]; ok {
			report, generateReportErrs := c.GenerateDetailsReportForTest(ctx, tOpt, statuses, false)
			if len(generateReportErrs) > 0 {
				errs = append(errs, generateReportErrs...)
				continue
			}
			reports = append(reports, report)
		} else {
			logrus.Errorf("missing test key in results: %v", testKeyStr)

		}

	}
	return reports, errs
}

// GenerateDetailsReportForTest generates a test detail report for a per-test + variant combo.
func (c *ComponentReportGenerator) GenerateDetailsReportForTest(
	ctx context.Context,
	testIDOption reqopts.TestIdentification,
	componentJobRunTestReportStatus crstatus.TestJobRunStatuses,
	allowUnregressedReports bool,
) (testdetails.Report, []error) {

	if testIDOption.TestID == "" {
		return testdetails.Report{}, []error{fmt.Errorf("test_id has to be defined for test details")}
	}
	for _, v := range c.ReqOptions.VariantOption.DBGroupBy.List() {
		if _, ok := testIDOption.RequestedVariants[v]; !ok {
			return testdetails.Report{}, []error{
				fmt.Errorf("all dbGroupBy variants have to be defined for test details: %s is missing in %v",
					v, testIDOption.RequestedVariants),
			}
		}
	}

	timeRanges, errs := c.dataProvider.QueryReleaseDates(ctx, c.ReqOptions)
	if errs != nil {
		return testdetails.Report{}, errs
	}

	now := time.Now()
	componentJobRunTestReportStatus.GeneratedAt = &now

	// Generate the report for the main release that was originally requested:
	report := c.internalGenerateTestDetailsReport(
		c.ReqOptions.BaseRelease.Name,
		&c.ReqOptions.BaseRelease.Start, &c.ReqOptions.BaseRelease.End,
		componentJobRunTestReportStatus.BaseStatus, componentJobRunTestReportStatus.SampleStatus,
		testIDOption)
	report.GeneratedAt = componentJobRunTestReportStatus.GeneratedAt

	// Generate the report for the fallback release if one was found:
	// TODO: this belongs in the releasefallback middleware, but our goal to return and display multiple
	// reports means the PreAnalysis state cannot be used for test details. The second call to
	// internalGenerateTestDetailsReport does not extract easily off "c". We cannot pass a ref to "c" due
	// to a circular dep. This is an unfortunate compromise in the middleware goal I didn't have time to unwind.
	// For now, the middleware does the querying for test details, and passes the override status out
	// by adding it to componentJobRunTestReportStatus.BaseOverrideStatus.
	var baseOverrideReport *testdetails.Report
	if testIDOption.BaseOverrideRelease != "" &&
		testIDOption.BaseOverrideRelease != c.ReqOptions.BaseRelease.Name {

		testKey := crtest.KeyWithVariants{
			TestID:   testIDOption.TestID,
			Variants: testIDOption.RequestedVariants,
		}
		if err := c.middlewares.PreTestDetailsAnalysis(testKey, &componentJobRunTestReportStatus); err != nil {
			return testdetails.Report{}, []error{err}
		}

		start, end, err := utils.FindStartEndTimesForRelease(timeRanges, testIDOption.BaseOverrideRelease)
		if err != nil {
			return testdetails.Report{}, []error{err}
		}

		overrideReport := c.internalGenerateTestDetailsReport(
			testIDOption.BaseOverrideRelease,
			start, end,
			componentJobRunTestReportStatus.BaseOverrideStatus, componentJobRunTestReportStatus.SampleStatus,
			testIDOption)
		// swap out the base dates for the override
		overrideReport.GeneratedAt = componentJobRunTestReportStatus.GeneratedAt
		baseOverrideReport = &overrideReport

		// Inject the override report stats into the first position on the main report,
		// which callers will interpret as the authoritative report in the event multiple are returned
		report.Analyses = append([]testdetails.Analysis{baseOverrideReport.Analyses[0]}, report.Analyses...)
	}

	if !allowUnregressedReports {
		regressed := false
		for _, analysis := range report.Analyses {
			if analysis.ReportStatus < crtest.NotSignificant {
				regressed = true
				break
			}
		}
		if !regressed {
			return testdetails.Report{}, []error{fmt.Errorf(
				"report for test is not regressed as expected in release %s; ID=%v", c.ReqOptions.SampleRelease.Name, testIDOption,
			)}
		}
	}

	// Add a "latest" link if the requested sample data is more than 48 hours old
	if time.Since(c.ReqOptions.SampleRelease.End) > 48*time.Hour {
		if report.Links == nil {
			report.Links = make(map[string]string)
		}

		// Calculate default sample date range: 7 days ago to now, rounded to nearest 4 hours EST
		roundingFactor := c.ReqOptions.CacheOption.CRTimeRoundingFactor
		if roundingFactor == 0 {
			roundingFactor = 4 * time.Hour // default from flags/component_readiness.go
		}

		// Create updated sample release options with the newer date range
		// End time: now, rounded down to nearest rounding factor
		now := time.Now().UTC()
		newSampleEnd := now.Truncate(roundingFactor)

		// Start time: 7 days before the end time, rounded to start of day
		newSampleStart := newSampleEnd.Add(-7 * 24 * time.Hour)
		newSampleStart = time.Date(newSampleStart.Year(), newSampleStart.Month(), newSampleStart.Day(), 0, 0, 0, 0, time.UTC)

		// Create a copy of the sample release with updated dates
		newSampleRelease := reqopts.Release{
			Name:               c.ReqOptions.SampleRelease.Name,
			PullRequestOptions: c.ReqOptions.SampleRelease.PullRequestOptions,
			PayloadOptions:     c.ReqOptions.SampleRelease.PayloadOptions,
			Start:              newSampleStart,
			End:                newSampleEnd,
		}

		// Convert variants map to string slice for GenerateTestDetailsURL
		variants := utils.VariantsMapToStringSlice(testIDOption.RequestedVariants)

		// Determine base release override if one was used
		baseReleaseOverride := ""
		if baseOverrideReport != nil && baseOverrideReport.Analyses[0].BaseStats != nil {
			baseReleaseOverride = baseOverrideReport.Analyses[0].BaseStats.Release
		}

		// Generate test details URL with the newer sample date range
		// For the "latest" link, we should not use the view (as the view's date range may be stale)
		// Instead, we generate a full URL with updated sample dates
		latestURL, err := utils.GenerateTestDetailsURL(
			testIDOption.TestID,
			c.baseURL,
			"", // Don't use view for "latest" link - we want fresh dates, not view's dates
			c.ReqOptions.BaseRelease,
			newSampleRelease,
			c.ReqOptions.AdvancedOption,
			c.ReqOptions.VariantOption,
			c.ReqOptions.TestFilters,
			testIDOption.Component,
			testIDOption.Capability,
			variants,
			baseReleaseOverride,
		)
		if err != nil {
			logrus.WithError(err).Warnf("failed to generate latest test details URL for test %s", testIDOption.TestID)
		} else {
			report.Links["latest"] = latestURL
		}
	}

	return report, nil
}

func (c *ComponentReportGenerator) getBaseJobRunTestStatus(
	ctx context.Context,
	allJobVariants crtest.JobVariants,
	baseRelease string,
	baseStart time.Time,
	baseEnd time.Time) (map[string][]crstatus.TestJobRunRows, []error) {

	reqOpts := c.ReqOptions
	reqOpts.BaseRelease.Name = baseRelease
	reqOpts.BaseRelease.Start = baseStart
	reqOpts.BaseRelease.End = baseEnd
	return c.dataProvider.QueryBaseJobRunTestStatus(ctx, reqOpts, allJobVariants)
}

func (c *ComponentReportGenerator) getSampleJobRunTestStatus(
	ctx context.Context,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time) (map[string][]crstatus.TestJobRunRows, []error) {

	return c.dataProvider.QuerySampleJobRunTestStatus(ctx, c.ReqOptions, allJobVariants, includeVariants, start, end)
}

func (c *ComponentReportGenerator) getJobRunTestStatus(ctx context.Context) (crstatus.TestJobRunStatuses, []error) {
	fLog := logrus.WithField("func", "getJobRunTestStatus")
	allJobVariants, errs := GetJobVariants(ctx, c.dataProvider)
	if len(errs) > 0 {
		logrus.Errorf("failed to get job variants")
		return crstatus.TestJobRunStatuses{}, errs
	}
	var baseStatus, sampleStatus map[string][]crstatus.TestJobRunRows
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}

	errCh := make(chan error)

	c.middlewares.QueryTestDetails(ctx, &wg, errCh, allJobVariants)

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			logrus.Infof("Context canceled while fetching base job run test status")
			return
		default:
			baseStatus, baseErrs = c.getBaseJobRunTestStatus(ctx, allJobVariants, c.ReqOptions.BaseRelease.Name, c.ReqOptions.BaseRelease.Start, c.ReqOptions.BaseRelease.End)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			logrus.Infof("Context canceled while fetching sample job run test status")
			return
		default:
			fLog.Infof("running sample status query with includeVariants: %+v", c.ReqOptions.VariantOption.IncludeVariants)
			status, errs := c.getSampleJobRunTestStatus(ctx, allJobVariants, c.ReqOptions.VariantOption.IncludeVariants,
				c.ReqOptions.SampleRelease.Start, c.ReqOptions.SampleRelease.End)
			fLog.Infof("received %d test statuses and %d errors from sample query", len(status), len(errs))
			sampleStatus = status
			sampleErrs = errs
		}
	}()

	go func() {
		wg.Wait()
		close(errCh)
	}()

	var middlewareErrs []error
	for err := range errCh {
		middlewareErrs = append(middlewareErrs, err)
	}

	fLog.Infof("total test statuses: %d", len(sampleStatus))
	if len(baseErrs) != 0 || len(sampleErrs) != 0 || len(middlewareErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, sampleErrs...)
		errs = append(errs, middlewareErrs...)
	}

	return crstatus.TestJobRunStatuses{BaseStatus: baseStatus, SampleStatus: sampleStatus}, errs
}

// internalGenerateTestDetailsReport handles the report generation for the lowest level test report including
// breakdown by job as well as overall stats.
func (c *ComponentReportGenerator) internalGenerateTestDetailsReport(
	baseRelease string,
	baseStart, baseEnd *time.Time,
	baseStatus, sampleStatus map[string][]crstatus.TestJobRunRows,
	testIDOption reqopts.TestIdentification,
) testdetails.Report {
	testKey := crtest.Identification{
		RowIdentification: crtest.RowIdentification{
			Component:  testIDOption.Component,
			Capability: testIDOption.Capability,
			TestID:     testIDOption.TestID,
		},
		ColumnIdentification: crtest.ColumnIdentification{
			Variants: testIDOption.RequestedVariants,
		},
	}

	totalBase, totalSample, report, result, lastFailure := c.summarizeRecordedTestStats(baseStatus, sampleStatus, testKey)

	testStats := testdetails.TestComparison{
		RequiredConfidence: c.ReqOptions.AdvancedOption.Confidence,
		SampleStats: testdetails.ReleaseStats{
			Release: c.ReqOptions.SampleRelease.Name,
			Start:   &c.ReqOptions.SampleRelease.Start,
			End:     &c.ReqOptions.SampleRelease.End,
			Stats:   totalSample,
		},
		BaseStats: &testdetails.ReleaseStats{
			Release: baseRelease,
			Start:   baseStart,
			End:     baseEnd,
			Stats:   totalBase,
		},
	}
	if !lastFailure.IsZero() {
		testStats.LastFailure = &lastFailure
	}
	log := logrus.WithFields(logrus.Fields{"testID": testIDOption.TestID, "variants": testIDOption.RequestedVariants})
	log.Debugf("computed test stats prior to PreAnalysis: %+v", testStats)

	if err := c.middlewares.PreAnalysis(testKey, &testStats); err != nil {
		logrus.WithError(err).Error("Failure from middleware analysis")
	}

	c.assessComponentStatus(&testStats, log)
	report.TestComparison = testStats
	result.Analyses = []testdetails.Analysis{report}

	return result
}

// go through all the job runs that had a test and summarize the results
func (c *ComponentReportGenerator) summarizeRecordedTestStats(
	baseStatus, sampleStatus map[string][]crstatus.TestJobRunRows, testKey crtest.Identification,
) (
	totalBase, totalSample crtest.Stats,
	report testdetails.Analysis,
	result testdetails.Report,
	lastFailure time.Time, // track the last failure we observe in the sample, used by triage middleware to adjust status
) {
	result = testdetails.Report{Identification: testKey}
	faf := c.ReqOptions.AdvancedOption.FlakeAsFailure

	// merge the job names from both base and sample status and assess each once
	jobNames := sets.NewString(slices.Collect(maps.Keys(baseStatus))...)
	jobNames.Insert(slices.Collect(maps.Keys(sampleStatus))...)
	for job := range jobNames {
		// tally up base job stats and matching sample job stats (if any); record job names, component, etc in the result
		jobStats := testdetails.JobStats{}
		if sampleStatsList, ok := sampleStatus[job]; ok {
			c.assessTestStats(sampleStatsList, &jobStats.SampleStats, &jobStats.SampleJobRunStats, &jobStats.SampleJobName, &lastFailure, &result, faf)
			totalSample = totalSample.Add(jobStats.SampleStats, faf)
		}
		if baseStatsList, ok := baseStatus[job]; ok {
			c.assessTestStats(baseStatsList, &jobStats.BaseStats, &jobStats.BaseJobRunStats, &jobStats.BaseJobName, nil, &result, faf)
			totalBase = totalBase.Add(jobStats.BaseStats, faf)
		}

		// determine the statistical significance to report in the job stats
		sFail, sPass := jobStats.SampleStats.FailPassWithFlakes(faf)
		bFail, bPass := jobStats.BaseStats.FailPassWithFlakes(faf)
		_, _, r, _ := fet.FisherExactTest(sFail, sPass, bFail, bPass)
		jobStats.Significant = r < 1-float64(c.ReqOptions.AdvancedOption.Confidence)/100

		report.JobStats = append(report.JobStats, jobStats)
	}

	// sort stats by job name in the results
	sort.Slice(report.JobStats, func(i, j int) bool {
		return report.JobStats[i].SampleJobName+":"+report.JobStats[i].BaseJobName <
			report.JobStats[j].SampleJobName+":"+report.JobStats[j].BaseJobName
	})
	return
}

// assessTestStats calculates the test stats for a given list of job rows
// and updates by-reference parameters with information found in the job rows.
func (c *ComponentReportGenerator) assessTestStats(
	jobRowsList []crstatus.TestJobRunRows,
	testStats *crtest.Stats,
	jobRunStatsList *[]testdetails.JobRunStats,
	jobName *string, lastFailure *time.Time,
	result *testdetails.Report,
	flakeAsFailure bool,
) {
	for _, jobRow := range jobRowsList {
		*jobName = jobRow.ProwJob

		start := jobRow.StartTime.In(time.UTC)
		if lastFailure != nil && start.After(*lastFailure) && jobRow.Failures() > 0 {
			*lastFailure = start
		}

		if result.JiraComponent == "" && jobRow.JiraComponent != "" {
			result.JiraComponent = jobRow.JiraComponent
		}
		if result.JiraComponentID == nil && jobRow.JiraComponentID != nil {
			result.JiraComponentID = jobRow.JiraComponentID
		}
		if result.TestName == "" && jobRow.TestName != "" {
			result.TestName = jobRow.TestName
		}

		*testStats = testStats.AddTestCount(jobRow.Count, flakeAsFailure)
		*jobRunStatsList = append(*jobRunStatsList, c.getJobRunStats(jobRow))
	}
}

func (c *ComponentReportGenerator) getJobRunStats(stats crstatus.TestJobRunRows) testdetails.JobRunStats {
	jobRunStats := testdetails.JobRunStats{
		TestStats: crtest.NewTestStats(
			stats.SuccessCount,
			stats.Failures(),
			stats.FlakeCount,
			c.ReqOptions.AdvancedOption.FlakeAsFailure,
		),
		JobURL:       stats.ProwJobURL,
		JobRunID:     stats.ProwJobRunID,
		StartTime:    stats.StartTime,
		JobLabels:    stats.JobLabels,
		TestFailures: stats.TestFailures,
	}
	return jobRunStats
}
