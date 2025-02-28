package componentreadiness

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	fet "github.com/glycerine/golang-fisher-exact"
	"github.com/openshift/sippy/pkg/api/componentreadiness/query"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/util"
	"github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/regressionallowances"
)

func GetTestDetails(ctx context.Context, client *bigquery.Client, prowURL, gcsBucket string, reqOptions crtype.RequestOptions,
) (crtype.ReportTestDetails, []error) {
	generator := ComponentReportGenerator{
		client:     client,
		prowURL:    prowURL,
		gcsBucket:  gcsBucket,
		ReqOptions: reqOptions,
	}

	return api.GetDataFromCacheOrGenerate[crtype.ReportTestDetails](
		ctx,
		generator.client.Cache,
		generator.ReqOptions.CacheOption,
		generator.GetComponentReportCacheKey(ctx, "TestDetailsReport~"),
		generator.GenerateTestDetailsReport,
		crtype.ReportTestDetails{})
}

func (c *ComponentReportGenerator) GenerateTestDetailsReport(ctx context.Context) (crtype.ReportTestDetails, []error) {
	if c.ReqOptions.TestIDOption.TestID == "" {
		return crtype.ReportTestDetails{}, []error{fmt.Errorf("test_id has to be defined for test details")}
	}
	for _, v := range c.ReqOptions.VariantOption.DBGroupBy.List() {
		if _, ok := c.ReqOptions.VariantOption.RequestedVariants[v]; !ok {
			return crtype.ReportTestDetails{}, []error{fmt.Errorf("all dbGroupBy variants have to be defined for test details: %s is missing in %v", v, c.ReqOptions.VariantOption.RequestedVariants)}
		}
	}

	componentJobRunTestReportStatus, errs := c.GenerateJobRunTestReportStatus(ctx)
	if len(errs) > 0 {
		return crtype.ReportTestDetails{}, errs
	}
	/*
		var err error
		bqs := NewPostgresRegressionStore(c.client)
		c.openRegressions, err = bqs.ListCurrentRegressionsForRelease(ctx, c.ReqOptions.SampleRelease.Release)
		if err != nil {
			errs = append(errs, err)
			return crtype.ReportTestDetails{}, errs
		}
	*/

	var baseOverrideReport *crtype.ReportTestDetails
	if c.ReqOptions.BaseOverrideRelease.Release != "" && c.ReqOptions.BaseOverrideRelease.Release != c.ReqOptions.BaseRelease.Release {
		// because internalGenerateTestDetailsReport modifies SampleStatus we need to copy it here
		overrideSampleStatus := map[string][]crtype.JobRunTestStatusRow{}
		for k, v := range componentJobRunTestReportStatus.SampleStatus {
			overrideSampleStatus[k] = v
		}

		overrideReport := c.internalGenerateTestDetailsReport(ctx, componentJobRunTestReportStatus.BaseOverrideStatus, c.ReqOptions.BaseOverrideRelease.Release, &c.ReqOptions.BaseOverrideRelease.Start, &c.ReqOptions.BaseOverrideRelease.End, overrideSampleStatus)
		// swap out the base dates for the override
		overrideReport.GeneratedAt = componentJobRunTestReportStatus.GeneratedAt
		baseOverrideReport = &overrideReport
	}

	report := c.internalGenerateTestDetailsReport(ctx, componentJobRunTestReportStatus.BaseStatus, c.ReqOptions.BaseRelease.Release, &c.ReqOptions.BaseRelease.Start, &c.ReqOptions.BaseRelease.End, componentJobRunTestReportStatus.SampleStatus)
	report.GeneratedAt = componentJobRunTestReportStatus.GeneratedAt

	if baseOverrideReport != nil {
		baseOverrideReport.BaseOverrideReport = crtype.ReportTestOverride{
			ReportTestStats: report.ReportTestStats,
			JobStats:        report.JobStats,
		}

		return *baseOverrideReport, nil
	}

	return report, nil
}

func (c *ComponentReportGenerator) GenerateJobRunTestReportStatus(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {
	before := time.Now()
	componentJobRunTestReportStatus, errs := c.getJobRunTestStatusFromBigQuery(ctx)
	if len(errs) > 0 {
		return crtype.JobRunTestReportStatus{}, errs
	}
	logrus.Infof("getJobRunTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentJobRunTestReportStatus.SampleStatus), len(componentJobRunTestReportStatus.BaseStatus))
	now := time.Now()
	componentJobRunTestReportStatus.GeneratedAt = &now
	return componentJobRunTestReportStatus, nil
}

func (c *ComponentReportGenerator) getBaseJobRunTestStatus(
	ctx context.Context,
	allJobVariants crtype.JobVariants,
	baseRelease string,
	baseStart time.Time,
	baseEnd time.Time) (map[string][]crtype.JobRunTestStatusRow, []error) {

	generator := query.NewBaseTestDetailsQueryGenerator(
		c.client,
		c.ReqOptions,
		allJobVariants,
		baseRelease,
		baseStart,
		baseEnd,
	)

	jobRunTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.JobRunTestReportStatus](
		ctx,
		c.client.Cache, c.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey("BaseJobRunTestStatus~", generator),
		generator.QueryTestStatus,
		crtype.JobRunTestReportStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return jobRunTestStatus.BaseStatus, nil
}

func (c *ComponentReportGenerator) getSampleJobRunTestStatus(
	ctx context.Context,
	allJobVariants crtype.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	junitTable string) (map[string][]crtype.JobRunTestStatusRow, []error) {

	generator := query.NewSampleTestDetailsQueryGenerator(
		c.client, c.ReqOptions,
		allJobVariants, includeVariants, start, end, junitTable)

	jobRunTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.JobRunTestReportStatus](
		ctx,
		c.client.Cache, c.ReqOptions.CacheOption,
		api.GetPrefixedCacheKey("SampleJobRunTestStatus~", generator),
		generator.QueryTestStatus,
		crtype.JobRunTestReportStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return jobRunTestStatus.SampleStatus, nil
}

func (c *ComponentReportGenerator) getJobRunTestStatusFromBigQuery(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {
	fLog := logrus.WithField("func", "getJobRunTestStatusFromBigQuery")
	allJobVariants, errs := GetJobVariantsFromBigQuery(ctx, c.client, c.gcsBucket)
	if len(errs) > 0 {
		logrus.Errorf("failed to get variants from bigquery")
		return crtype.JobRunTestReportStatus{}, errs
	}
	var baseStatus, baseOverrideStatus, sampleStatus map[string][]crtype.JobRunTestStatusRow
	var baseErrs, baseOverrideErrs, sampleErrs []error
	wg := sync.WaitGroup{}

	// channels for status as we may collect status from multiple queries run in separate goroutines
	statusCh := make(chan map[string][]crtype.JobRunTestStatusRow)
	statusErrCh := make(chan error)
	statusDoneCh := make(chan struct{})     // To signal when all processing is done
	statusErrsDoneCh := make(chan struct{}) // To signal when all processing is done

	if c.ReqOptions.BaseOverrideRelease.Release != "" && c.ReqOptions.BaseOverrideRelease.Release != c.ReqOptions.BaseRelease.Release {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				logrus.Infof("Context canceled while fetching base job run test status")
				return
			default:
				baseOverrideStatus, baseOverrideErrs = c.getBaseJobRunTestStatus(ctx, allJobVariants, c.ReqOptions.BaseOverrideRelease.Release, c.ReqOptions.BaseOverrideRelease.Start, c.ReqOptions.BaseOverrideRelease.End)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			logrus.Infof("Context canceled while fetching base job run test status")
			return
		default:
			baseStatus, baseErrs = c.getBaseJobRunTestStatus(ctx, allJobVariants, c.ReqOptions.BaseRelease.Release, c.ReqOptions.BaseRelease.Start, c.ReqOptions.BaseRelease.End)
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
				statusErrCh <- err
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
					statusErrCh <- err
					return
				}
				status, errs := c.getSampleJobRunTestStatus(ctx, allJobVariants, includeVariants,
					start, end, or.TableName)
				fLog.Infof("received %d test statuses and %d errors from override query", len(status), len(errs))
				statusCh <- status
				for _, err := range errs {
					statusErrCh <- err
				}
			}

		}(i, or)
	}

	go func() {
		wg.Wait()
		close(statusCh)
		close(statusErrCh)
	}()

	go func() {

		for status := range statusCh {
			fLog.Infof("received %d test statuses over channel", len(status))
			for k, v := range status {
				if sampleStatus == nil {
					fLog.Warnf("initializing sampleStatus map")
					sampleStatus = make(map[string][]crtype.JobRunTestStatusRow)
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
		for err := range statusErrCh {
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

	return crtype.JobRunTestReportStatus{BaseStatus: baseStatus, BaseOverrideStatus: baseOverrideStatus, SampleStatus: sampleStatus}, errs
}

// internalGenerateTestDetailsReport handles the report generation for the lowest level test report including
// breakdown by job as well as overall stats.
func (c *ComponentReportGenerator) internalGenerateTestDetailsReport(ctx context.Context,
	baseStatus map[string][]crtype.JobRunTestStatusRow,
	baseRelease string,
	baseStart,
	baseEnd *time.Time,
	sampleStatus map[string][]crtype.JobRunTestStatusRow) crtype.ReportTestDetails {
	result := crtype.ReportTestDetails{
		ReportTestIdentification: crtype.ReportTestIdentification{
			RowIdentification: crtype.RowIdentification{
				Component:  c.ReqOptions.TestIDOption.Component,
				Capability: c.ReqOptions.TestIDOption.Capability,
				TestID:     c.ReqOptions.TestIDOption.TestID,
			},
			ColumnIdentification: crtype.ColumnIdentification{
				Variants: c.ReqOptions.VariantOption.RequestedVariants,
			},
		},
	}
	var resolvedIssueCompensation int
	approvedRegression := regressionallowances.IntentionalRegressionFor(c.ReqOptions.SampleRelease.Release, result.ColumnIdentification, c.ReqOptions.TestIDOption.TestID)
	var baseRegression *regressionallowances.IntentionalRegression
	// if we are ignoring fallback then honor the settings for the baseRegression
	// otherwise let fallback determine the threshold
	if !c.ReqOptions.AdvancedOption.IncludeMultiReleaseAnalysis {
		baseRegression = regressionallowances.IntentionalRegressionFor(baseRelease, result.ColumnIdentification, c.ReqOptions.TestIDOption.TestID)
	}
	// ignore triage if we have an intentional regression
	if approvedRegression == nil {
		resolvedIssueCompensation, _ = c.triagedIncidentsFor(ctx, result.ReportTestIdentification)
	}

	var totalBaseFailure, totalBaseSuccess, totalBaseFlake, totalSampleFailure, totalSampleSuccess, totalSampleFlake int
	var perJobBaseFailure, perJobBaseSuccess, perJobBaseFlake, perJobSampleFailure, perJobSampleSuccess, perJobSampleFlake int

	for prowJob, baseStatsList := range baseStatus {
		jobStats := crtype.TestDetailsJobStats{
			JobName: prowJob,
		}
		perJobBaseFailure = 0
		perJobBaseSuccess = 0
		perJobBaseFlake = 0
		perJobSampleFailure = 0
		perJobSampleSuccess = 0
		perJobSampleFlake = 0
		for _, baseStats := range baseStatsList {
			if result.JiraComponent == "" && baseStats.JiraComponent != "" {
				result.JiraComponent = baseStats.JiraComponent
			}
			if result.JiraComponentID == nil && baseStats.JiraComponentID != nil {
				result.JiraComponentID = baseStats.JiraComponentID
			}

			jobStats.BaseJobRunStats = append(jobStats.BaseJobRunStats, c.getJobRunStats(baseStats, c.prowURL, c.gcsBucket))
			perJobBaseSuccess += baseStats.SuccessCount
			perJobBaseFlake += baseStats.FlakeCount
			perJobBaseFailure += getFailureCount(baseStats)
		}
		if sampleStatsList, ok := sampleStatus[prowJob]; ok {
			for _, sampleStats := range sampleStatsList {
				if result.JiraComponent == "" && sampleStats.JiraComponent != "" {
					result.JiraComponent = sampleStats.JiraComponent
				}
				if result.JiraComponentID == nil && sampleStats.JiraComponentID != nil {
					result.JiraComponentID = sampleStats.JiraComponentID
				}

				jobStats.SampleJobRunStats = append(jobStats.SampleJobRunStats, c.getJobRunStats(sampleStats, c.prowURL, c.gcsBucket))
				perJobSampleSuccess += sampleStats.SuccessCount
				perJobSampleFlake += sampleStats.FlakeCount
				perJobSampleFailure += getFailureCount(sampleStats)
			}
			delete(sampleStatus, prowJob)
		}
		jobStats.BaseStats.SuccessCount = perJobBaseSuccess
		jobStats.BaseStats.FlakeCount = perJobBaseFlake
		jobStats.BaseStats.FailureCount = perJobBaseFailure
		jobStats.BaseStats.SuccessRate = c.getPassRate(perJobBaseSuccess, perJobBaseFailure, perJobBaseFlake)
		jobStats.SampleStats.SuccessCount = perJobSampleSuccess
		jobStats.SampleStats.FlakeCount = perJobSampleFlake
		jobStats.SampleStats.FailureCount = perJobSampleFailure
		jobStats.SampleStats.SuccessRate = c.getPassRate(perJobSampleSuccess, perJobSampleFailure, perJobSampleFlake)
		perceivedSampleFailure := perJobSampleFailure
		perceivedBaseFailure := perJobBaseFailure
		perceivedSampleSuccess := perJobSampleSuccess + perJobSampleFlake
		perceivedBaseSuccess := perJobBaseSuccess + perJobBaseFlake
		if c.ReqOptions.AdvancedOption.FlakeAsFailure {
			perceivedSampleFailure = perJobSampleFailure + perJobSampleFlake
			perceivedBaseFailure = perJobBaseFailure + perJobBaseFlake
			perceivedSampleSuccess = perJobSampleSuccess
			perceivedBaseSuccess = perJobBaseSuccess
		}
		_, _, r, _ := fet.FisherExactTest(perceivedSampleFailure,
			perceivedSampleSuccess,
			perceivedBaseFailure,
			perceivedBaseSuccess)
		jobStats.Significant = r < 1-float64(c.ReqOptions.AdvancedOption.Confidence)/100

		result.JobStats = append(result.JobStats, jobStats)

		totalBaseFailure += perJobBaseFailure
		totalBaseSuccess += perJobBaseSuccess
		totalBaseFlake += perJobBaseFlake
		totalSampleFailure += perJobSampleFailure
		totalSampleSuccess += perJobSampleSuccess
		totalSampleFlake += perJobSampleFlake
	}
	for prowJob, sampleStatsList := range sampleStatus {
		jobStats := crtype.TestDetailsJobStats{
			JobName: prowJob,
		}
		perJobSampleFailure = 0
		perJobSampleSuccess = 0
		perJobSampleFlake = 0
		for _, sampleStats := range sampleStatsList {
			jobStats.SampleJobRunStats = append(jobStats.SampleJobRunStats, c.getJobRunStats(sampleStats, c.prowURL, c.gcsBucket))
			perJobSampleSuccess += sampleStats.SuccessCount
			perJobSampleFlake += sampleStats.FlakeCount
			perJobSampleFailure += getFailureCount(sampleStats)
		}
		jobStats.SampleStats.SuccessCount = perJobSampleSuccess
		jobStats.SampleStats.FlakeCount = perJobSampleFlake
		jobStats.SampleStats.FailureCount = perJobSampleFailure
		jobStats.SampleStats.SuccessRate = c.getPassRate(perJobSampleSuccess, perJobSampleFailure, perJobSampleFlake)
		result.JobStats = append(result.JobStats, jobStats)
		perceivedSampleFailure := perJobSampleFailure
		perceivedSampleSuccess := perJobSampleSuccess + perJobSampleFlake
		if c.ReqOptions.AdvancedOption.FlakeAsFailure {
			perceivedSampleFailure = perJobSampleFailure + perJobSampleFlake
			perceivedSampleSuccess = perJobSampleSuccess
		}
		_, _, r, _ := fet.FisherExactTest(perceivedSampleFailure,
			perceivedSampleSuccess,
			0,
			0)
		jobStats.Significant = r < 1-float64(c.ReqOptions.AdvancedOption.Confidence)/100

		totalSampleFailure += perJobSampleFailure
		totalSampleSuccess += perJobSampleSuccess
		totalSampleFlake += perJobSampleFlake
	}
	sort.Slice(result.JobStats, func(i, j int) bool {
		return result.JobStats[i].JobName < result.JobStats[j].JobName
	})

	// The hope is that this goes away
	// once we agree we don't need to honor a higher intentional regression pass percentage
	if baseRegression != nil && baseRegression.PreviousPassPercentage(c.ReqOptions.AdvancedOption.FlakeAsFailure) > c.getPassRate(totalBaseSuccess, totalBaseFailure, totalBaseFlake) {
		// override with  the basis regression previous values
		// testStats will reflect the expected threshold, not the computed values from the release with the allowed regression
		baseRegressionPreviousRelease, err := utils.PreviousRelease(baseRelease)
		if err != nil {
			logrus.WithError(err).Error("Failed to determine the previous release for baseRegression")
		} else {
			totalBaseFlake = baseRegression.PreviousFlakes
			totalBaseSuccess = baseRegression.PreviousSuccesses
			totalBaseFailure = baseRegression.PreviousFailures
			baseRelease = baseRegressionPreviousRelease
			logrus.Infof("BaseRegression - PreviousPassPercentage overrides baseStats.  Release: %s, Successes: %d, Flakes: %d, Failures: %d", baseRelease, totalBaseSuccess, totalBaseFlake, totalBaseFailure)
		}
	}

	requiredConfidence := c.getRequiredConfidence(c.ReqOptions.TestIDOption.TestID, c.ReqOptions.VariantOption.RequestedVariants)

	result.ReportTestStats = c.assessComponentStatus(
		requiredConfidence,
		totalSampleSuccess+totalSampleFailure+totalSampleFlake,
		totalSampleSuccess,
		totalSampleFlake,
		totalBaseSuccess+totalBaseFailure+totalBaseFlake,
		totalBaseSuccess,
		totalBaseFlake,
		approvedRegression,
		resolvedIssueCompensation,
		baseRelease,
		baseStart,
		baseEnd,
	)

	return result
}

func (c *ComponentReportGenerator) getJobRunStats(stats crtype.JobRunTestStatusRow, prowURL, gcsBucket string) crtype.TestDetailsJobRunStats {
	failure := getFailureCount(stats)
	url := fmt.Sprintf("%s/view/gs/%s/", prowURL, gcsBucket)
	subs := strings.Split(stats.FilePath, "/artifacts/")
	if len(subs) > 1 {
		url += subs[0]
	}
	jobRunStats := crtype.TestDetailsJobRunStats{
		TestStats: crtype.TestDetailsTestStats{
			SuccessRate:  c.getPassRate(stats.SuccessCount, failure, stats.FlakeCount),
			SuccessCount: stats.SuccessCount,
			FailureCount: failure,
			FlakeCount:   stats.FlakeCount,
		},
		JobURL: url,
	}
	return jobRunStats
}
