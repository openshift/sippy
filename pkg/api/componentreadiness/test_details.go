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
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness/query"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util"
)

func GetTestDetails(ctx context.Context, client *bigquery.Client, dbc *db.DB, reqOptions reqopts.RequestOptions,
) (crtype.ReportTestDetails, []error) {
	generator := NewComponentReportGenerator(client, reqOptions, dbc, nil)
	if os.Getenv("DEV_MODE") == "1" {
		return generator.GenerateTestDetailsReport(ctx)
	}

	report, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestDetails](
		ctx,
		generator.client.Cache,
		generator.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey("TestDetailsReport~", generator.GetCacheKey(ctx)),
		generator.GenerateTestDetailsReport,
		crtype.ReportTestDetails{})
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
func (c *ComponentReportGenerator) PostAnalysisTestDetails(report *crtype.ReportTestDetails) error {

	// Give middleware their chance to adjust the result
	testKey := crtest.ReportTestIdentification{
		RowIdentification:    report.RowIdentification,
		ColumnIdentification: report.ColumnIdentification,
	}
	for i := range report.Analyses {
		if err := c.middlewares.PostAnalysis(testKey, &report.Analyses[i].ReportTestStats); err != nil {
			return err
		}
	}

	return nil
}

// GenerateTestDetailsReport is the main function to generate a test details report for a request, if we miss the cache.
func (c *ComponentReportGenerator) GenerateTestDetailsReport(ctx context.Context) (crtype.ReportTestDetails, []error) {
	// This function is called from the API, and we assume only one TestIDOptions entry in that case.
	testIDOptions := c.ReqOptions.TestIDOptions[0]
	// load all pass/fails for specific jobs, both sample, basis, and override basis if requested
	componentJobRunTestReportStatus, errs := c.getJobRunTestStatusFromBigQuery(ctx)
	if len(errs) > 0 {
		return crtype.ReportTestDetails{}, errs
	}

	return c.GenerateDetailsReportForTest(ctx, testIDOptions, componentJobRunTestReportStatus)
}

// GenerateTestDetailsReportMultiTest variant of the function is for multi-test reports, used for cache priming all test detail reports for a view.
func (c *ComponentReportGenerator) GenerateTestDetailsReportMultiTest(ctx context.Context) ([]crtype.ReportTestDetails, []error) {
	// load all pass/fails for specific jobs, both sample, basis, and override basis if requested
	before := time.Now()
	allTestsJobRunStatuses, errs := c.getJobRunTestStatusFromBigQuery(ctx)
	if len(errs) > 0 {
		return []crtype.ReportTestDetails{}, errs
	}
	logrus.Infof("getJobRunTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db",
		time.Since(before), len(allTestsJobRunStatuses.SampleStatus), len(allTestsJobRunStatuses.BaseStatus))

	// We have a struct where the statuses are mapped by prowjob to all rows results for that prowjob,
	// with multiple tests intermingled in that layer.
	// Build out a new struct where these are split up by test ID.
	// split the status on test ID, and pass only that tests data in for reporting:
	testKeyTestJobRunStatuses := map[string]crtype.TestJobRunStatuses{}
	for jobName, rows := range allTestsJobRunStatuses.BaseStatus {
		for _, row := range rows {
			testKeyStr := row.TestKeyStr
			if _, ok := testKeyTestJobRunStatuses[testKeyStr]; !ok {
				testKeyTestJobRunStatuses[testKeyStr] = crtype.TestJobRunStatuses{
					BaseStatus:         map[string][]crtype.TestJobRunRows{},
					BaseOverrideStatus: map[string][]crtype.TestJobRunRows{},
					SampleStatus:       map[string][]crtype.TestJobRunRows{},
					GeneratedAt:        allTestsJobRunStatuses.GeneratedAt,
				}
			}
			if testKeyTestJobRunStatuses[testKeyStr].BaseStatus[jobName] == nil {
				testKeyTestJobRunStatuses[testKeyStr].BaseStatus[jobName] = []crtype.TestJobRunRows{}
			}
			testKeyTestJobRunStatuses[testKeyStr].BaseStatus[jobName] =
				append(testKeyTestJobRunStatuses[testKeyStr].BaseStatus[jobName], row)
		}
	}
	for jobName, rows := range allTestsJobRunStatuses.BaseOverrideStatus {
		for _, row := range rows {
			testKeyStr := row.TestKeyStr
			if _, ok := testKeyTestJobRunStatuses[testKeyStr]; !ok {
				testKeyTestJobRunStatuses[testKeyStr] = crtype.TestJobRunStatuses{
					BaseStatus:         map[string][]crtype.TestJobRunRows{},
					BaseOverrideStatus: map[string][]crtype.TestJobRunRows{},
					SampleStatus:       map[string][]crtype.TestJobRunRows{},
					GeneratedAt:        allTestsJobRunStatuses.GeneratedAt,
				}
			}
			if testKeyTestJobRunStatuses[testKeyStr].BaseOverrideStatus[jobName] == nil {
				testKeyTestJobRunStatuses[testKeyStr].BaseOverrideStatus[jobName] = []crtype.TestJobRunRows{}
			}
			testKeyTestJobRunStatuses[testKeyStr].BaseOverrideStatus[jobName] =
				append(testKeyTestJobRunStatuses[testKeyStr].BaseOverrideStatus[jobName], row)
		}
	}
	for jobName, rows := range allTestsJobRunStatuses.SampleStatus {
		for _, row := range rows {
			testKeyStr := row.TestKeyStr
			if _, ok := testKeyTestJobRunStatuses[testKeyStr]; !ok {
				testKeyTestJobRunStatuses[testKeyStr] = crtype.TestJobRunStatuses{
					BaseStatus:         map[string][]crtype.TestJobRunRows{},
					BaseOverrideStatus: map[string][]crtype.TestJobRunRows{},
					SampleStatus:       map[string][]crtype.TestJobRunRows{},
					GeneratedAt:        allTestsJobRunStatuses.GeneratedAt,
				}
			}
			if testKeyTestJobRunStatuses[testKeyStr].SampleStatus[jobName] == nil {
				testKeyTestJobRunStatuses[testKeyStr].SampleStatus[jobName] = []crtype.TestJobRunRows{}
			}
			testKeyTestJobRunStatuses[testKeyStr].SampleStatus[jobName] =
				append(testKeyTestJobRunStatuses[testKeyStr].SampleStatus[jobName], row)
		}
	}

	reports := []crtype.ReportTestDetails{}
	for _, tOpt := range c.ReqOptions.TestIDOptions {
		testKey := crtest.TestWithVariantsKey{
			TestID:   tOpt.TestID,
			Variants: tOpt.RequestedVariants,
		}
		testKeyStr := testKey.KeyOrDie()
		if statuses, ok := testKeyTestJobRunStatuses[testKeyStr]; ok {
			report, generateReportErrs := c.GenerateDetailsReportForTest(ctx, tOpt, statuses)
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
func (c *ComponentReportGenerator) GenerateDetailsReportForTest(ctx context.Context, testIDOption reqopts.TestIdentification, componentJobRunTestReportStatus crtype.TestJobRunStatuses) (crtype.ReportTestDetails, []error) {

	if testIDOption.TestID == "" {
		return crtype.ReportTestDetails{}, []error{fmt.Errorf("test_id has to be defined for test details")}
	}
	for _, v := range c.ReqOptions.VariantOption.DBGroupBy.List() {
		if _, ok := testIDOption.RequestedVariants[v]; !ok {
			return crtype.ReportTestDetails{}, []error{
				fmt.Errorf("all dbGroupBy variants have to be defined for test details: %s is missing in %v",
					v, testIDOption.RequestedVariants),
			}
		}
	}

	releases, errs := query.GetReleaseDatesFromBigQuery(ctx, c.client, c.ReqOptions)
	if errs != nil {
		return crtype.ReportTestDetails{}, errs
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
	var baseOverrideReport *crtype.ReportTestDetails
	if testIDOption.BaseOverrideRelease != "" &&
		testIDOption.BaseOverrideRelease != c.ReqOptions.BaseRelease.Name {

		testKey := crtest.TestWithVariantsKey{
			TestID:   testIDOption.TestID,
			Variants: testIDOption.RequestedVariants,
		}
		if err := c.middlewares.PreTestDetailsAnalysis(testKey, &componentJobRunTestReportStatus); err != nil {
			return crtype.ReportTestDetails{}, []error{err}
		}

		start, end, err := utils.FindStartEndTimesForRelease(releases, testIDOption.BaseOverrideRelease)
		if err != nil {
			return crtype.ReportTestDetails{}, []error{err}
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
		report.Analyses = append([]crtype.TestDetailsAnalysis{baseOverrideReport.Analyses[0]}, report.Analyses...)
	}

	return report, nil
}

func (c *ComponentReportGenerator) getBaseJobRunTestStatus(
	ctx context.Context,
	allJobVariants crtest.JobVariants,
	baseRelease string,
	baseStart time.Time,
	baseEnd time.Time) (map[string][]crtype.TestJobRunRows, []error) {

	generator := query.NewBaseTestDetailsQueryGenerator(
		logrus.WithField("func", "getBaseJobRunTestStatus"),
		c.client,
		c.ReqOptions,
		allJobVariants,
		baseRelease,
		baseStart,
		baseEnd,
		c.ReqOptions.TestIDOptions,
	)

	jobRunTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.TestJobRunStatuses](
		ctx,
		c.client.Cache, c.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey("BaseJobRunTestStatus~", generator),
		generator.QueryTestStatus,
		crtype.TestJobRunStatuses{})

	if len(errs) > 0 {
		return nil, errs
	}

	return jobRunTestStatus.BaseStatus, nil
}

func (c *ComponentReportGenerator) getSampleJobRunTestStatus(
	ctx context.Context,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	junitTable string) (map[string][]crtype.TestJobRunRows, []error) {

	generator := query.NewSampleTestDetailsQueryGenerator(
		c.client, c.ReqOptions,
		allJobVariants, includeVariants, start, end, junitTable)

	jobRunTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.TestJobRunStatuses](
		ctx,
		c.client.Cache, c.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey("SampleJobRunTestStatus~", generator),
		generator.QueryTestStatus,
		crtype.TestJobRunStatuses{})

	if len(errs) > 0 {
		return nil, errs
	}

	return jobRunTestStatus.SampleStatus, nil
}

func (c *ComponentReportGenerator) getJobRunTestStatusFromBigQuery(ctx context.Context) (crtype.TestJobRunStatuses, []error) {
	fLog := logrus.WithField("func", "getJobRunTestStatusFromBigQuery")
	allJobVariants, errs := GetJobVariantsFromBigQuery(ctx, c.client)
	if len(errs) > 0 {
		logrus.Errorf("failed to get variants from bigquery")
		return crtype.TestJobRunStatuses{}, errs
	}
	var baseStatus, sampleStatus map[string][]crtype.TestJobRunRows
	var baseErrs, baseOverrideErrs, sampleErrs []error
	wg := sync.WaitGroup{}

	// channels for status as we may collect status from multiple queries run in separate goroutines
	statusCh := make(chan map[string][]crtype.TestJobRunRows)
	errCh := make(chan error)
	statusDoneCh := make(chan struct{})     // To signal when all processing is done
	statusErrsDoneCh := make(chan struct{}) // To signal when all processing is done

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
			includeVariants, skipQuery := copyIncludeVariantsAndRemoveOverrides(c.variantJunitTableOverrides, -1, c.ReqOptions.VariantOption.IncludeVariants)
			if skipQuery {
				fLog.Infof("skipping default status query as all values for a variant were overridden")
				return
			}
			fLog.Infof("running default status query with includeVariants: %+v", includeVariants)
			status, errs := c.getSampleJobRunTestStatus(ctx, allJobVariants, includeVariants,
				c.ReqOptions.SampleRelease.Start, c.ReqOptions.SampleRelease.End, query.DefaultJunitTable)
			fLog.Infof("received %d test statuses and %d errors from default query", len(status), len(errs))
			statusCh <- status
			for _, err := range errs {
				errCh <- err
			}
		}

	}()

	// fork additional sample queries for the overrides
	for i, or := range c.variantJunitTableOverrides {
		if !containsOverriddenVariant(c.ReqOptions.VariantOption.IncludeVariants, or.VariantName, or.VariantValue) {
			continue
		}
		// only do this additional query if the specified override variant is actually included in this request
		wg.Add(1)
		go func(i int, or configv1.VariantJunitTableOverride) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
				includeVariants, skipQuery := copyIncludeVariantsAndRemoveOverrides(c.variantJunitTableOverrides, i, c.ReqOptions.VariantOption.IncludeVariants)
				if skipQuery {
					fLog.Infof("skipping override status query as all values for a variant were overridden")
					return
				}
				fLog.Infof("running override status query for %+v with includeVariants: %+v", or, includeVariants)
				// Calculate a start time relative to the requested end time: (i.e. for rarely run jobs)
				end := c.ReqOptions.SampleRelease.End
				start, err := util.ParseCRReleaseTime([]v1.Release{}, "", or.RelativeStart,
					true, &c.ReqOptions.SampleRelease.End, c.ReqOptions.CacheOption.CRTimeRoundingFactor)
				if err != nil {
					errCh <- err
					return
				}
				status, errs := c.getSampleJobRunTestStatus(ctx, allJobVariants, includeVariants,
					start, end, or.TableName)
				fLog.Infof("received %d job run test statuses and %d errors from override query", len(status), len(errs))
				statusCh <- status
				for _, err := range errs {
					errCh <- err
				}
			}

		}(i, or)
	}

	go func() {
		wg.Wait()
		close(statusCh)
		close(errCh)
	}()

	go func() {

		for status := range statusCh {
			fLog.Infof("received %d job run test statuses over channel", len(status))
			for k, v := range status {
				if sampleStatus == nil {
					fLog.Warnf("initializing sampleStatus map")
					sampleStatus = make(map[string][]crtype.TestJobRunRows)
				}
				if v2, ok := sampleStatus[k]; ok {
					fLog.Warnf("sampleStatus already had key: %+v", k)
					fLog.Warnf("sampleStatus new value: %+v", v)
					fLog.Warnf("sampleStatus old value: %+v", v2)
				}
				sampleStatus[k] = v
			}
		}
		close(statusDoneCh)
	}()

	go func() {
		for err := range errCh {
			sampleErrs = append(sampleErrs, err)
		}
		close(statusErrsDoneCh)
	}()

	<-statusDoneCh
	<-statusErrsDoneCh
	fLog.Infof("total test statuses: %d", len(sampleStatus))
	if len(baseErrs) != 0 || len(baseOverrideErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, baseOverrideErrs...)
	}

	return crtype.TestJobRunStatuses{BaseStatus: baseStatus, SampleStatus: sampleStatus}, errs
}

// internalGenerateTestDetailsReport handles the report generation for the lowest level test report including
// breakdown by job as well as overall stats.
func (c *ComponentReportGenerator) internalGenerateTestDetailsReport(
	baseRelease string,
	baseStart, baseEnd *time.Time,
	baseStatus, sampleStatus map[string][]crtype.TestJobRunRows,
	testIDOption reqopts.TestIdentification,
) crtype.ReportTestDetails {
	testKey := crtest.ReportTestIdentification{
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

	testStats := crtype.ReportTestStats{
		RequiredConfidence: c.ReqOptions.AdvancedOption.Confidence,
		SampleStats: crtype.TestDetailsReleaseStats{
			Release:              c.ReqOptions.SampleRelease.Name,
			Start:                &c.ReqOptions.SampleRelease.Start,
			End:                  &c.ReqOptions.SampleRelease.End,
			TestDetailsTestStats: totalSample,
		},
		BaseStats: &crtype.TestDetailsReleaseStats{
			Release:              baseRelease,
			Start:                baseStart,
			End:                  baseEnd,
			TestDetailsTestStats: totalBase,
		},
	}
	if !lastFailure.IsZero() {
		testStats.LastFailure = &lastFailure
	}

	if err := c.middlewares.PreAnalysis(testKey, &testStats); err != nil {
		logrus.WithError(err).Error("Failure from middleware analysis")
	}

	c.assessComponentStatus(&testStats)
	report.ReportTestStats = testStats
	result.Analyses = []crtype.TestDetailsAnalysis{report}

	return result
}

// go through all the job runs that had a test and summarize the results
func (c *ComponentReportGenerator) summarizeRecordedTestStats(
	baseStatus, sampleStatus map[string][]crtype.TestJobRunRows, testKey crtest.ReportTestIdentification,
) (
	totalBase, totalSample crtest.TestDetailsTestStats,
	report crtype.TestDetailsAnalysis,
	result crtype.ReportTestDetails,
	lastFailure time.Time, // track the last failure we observe in the sample, used by triage middleware to adjust status
) {
	result = crtype.ReportTestDetails{ReportTestIdentification: testKey}
	faf := c.ReqOptions.AdvancedOption.FlakeAsFailure

	// merge the job names from both base and sample status and assess each once
	jobNames := sets.NewString(slices.Collect(maps.Keys(baseStatus))...)
	jobNames.Insert(slices.Collect(maps.Keys(sampleStatus))...)
	for job := range jobNames {
		// tally up base job stats and matching sample job stats (if any); record job names, component, etc in the result
		jobStats := crtype.TestDetailsJobStats{}
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
	jobRowsList []crtype.TestJobRunRows,
	testStats *crtest.TestDetailsTestStats,
	jobRunStatsList *[]crtype.TestDetailsJobRunStats,
	jobName *string, lastFailure *time.Time,
	result *crtype.ReportTestDetails,
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

		*testStats = testStats.AddTestCount(jobRow.TestCount, flakeAsFailure)
		*jobRunStatsList = append(*jobRunStatsList, c.getJobRunStats(jobRow))
	}
}

func (c *ComponentReportGenerator) getJobRunStats(stats crtype.TestJobRunRows) crtype.TestDetailsJobRunStats {
	jobRunStats := crtype.TestDetailsJobRunStats{
		TestStats: crtest.NewTestStats(
			stats.SuccessCount,
			stats.Failures(),
			stats.FlakeCount,
			c.ReqOptions.AdvancedOption.FlakeAsFailure,
		),
		JobURL:    stats.ProwJobURL,
		JobRunID:  stats.ProwJobRunID,
		StartTime: stats.StartTime,
	}
	return jobRunStats
}
