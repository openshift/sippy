package componentreadiness

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	bigquery2 "cloud.google.com/go/bigquery"
	fet "github.com/glycerine/golang-fisher-exact"
	"github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/regressionallowances"
)

func GetTestDetails(ctx context.Context, client *bigquery.Client, prowURL, gcsBucket string, reqOptions crtype.RequestOptions,
) (crtype.ReportTestDetails, []error) {
	generator := componentReportGenerator{
		client:                           client,
		prowURL:                          prowURL,
		gcsBucket:                        gcsBucket,
		cacheOption:                      reqOptions.CacheOption,
		BaseRelease:                      reqOptions.BaseRelease,
		BaseOverrideRelease:              reqOptions.BaseOverrideRelease,
		SampleRelease:                    reqOptions.SampleRelease,
		RequestTestIdentificationOptions: reqOptions.TestIDOption,
		RequestVariantOptions:            reqOptions.VariantOption,
		RequestAdvancedOptions:           reqOptions.AdvancedOption,
	}

	return api.GetDataFromCacheOrGenerate[crtype.ReportTestDetails](
		ctx,
		generator.client.Cache,
		generator.cacheOption,
		generator.GetComponentReportCacheKey(ctx, "TestDetailsReport~"),
		generator.GenerateTestDetailsReport,
		crtype.ReportTestDetails{})
}

func (c *componentReportGenerator) GenerateTestDetailsReport(ctx context.Context) (crtype.ReportTestDetails, []error) {
	if c.TestID == "" {
		return crtype.ReportTestDetails{}, []error{fmt.Errorf("test_id has to be defined for test details")}
	}
	for _, v := range c.DBGroupBy.List() {
		if _, ok := c.RequestedVariants[v]; !ok {
			return crtype.ReportTestDetails{}, []error{fmt.Errorf("all dbGroupBy variants have to be defined for test details: %s is missing", v)}
		}
	}

	componentJobRunTestReportStatus, errs := c.GenerateJobRunTestReportStatus(ctx)
	if len(errs) > 0 {
		return crtype.ReportTestDetails{}, errs
	}
	var err error
	bqs := NewBigQueryRegressionStore(c.client)
	allRegressions, err := bqs.ListCurrentRegressions(ctx)
	if err != nil {
		errs = append(errs, err)
		return crtype.ReportTestDetails{}, errs
	}

	var baseOverrideReport *crtype.ReportTestDetails
	if c.BaseOverrideRelease.Release != "" && c.BaseOverrideRelease.Release != c.BaseRelease.Release {
		// because internalGenerateTestDetailsReport modifies SampleStatus we need to copy it here
		overrideSampleStatus := map[string][]crtype.JobRunTestStatusRow{}
		for k, v := range componentJobRunTestReportStatus.SampleStatus {
			overrideSampleStatus[k] = v
		}

		overrideReport := c.internalGenerateTestDetailsReport(ctx, componentJobRunTestReportStatus.BaseOverrideStatus, c.BaseOverrideRelease.Release, &c.BaseOverrideRelease.Start, &c.BaseOverrideRelease.End, overrideSampleStatus)
		// swap out the base dates for the override
		overrideReport.GeneratedAt = componentJobRunTestReportStatus.GeneratedAt
		baseOverrideReport = &overrideReport
	}

	c.openRegressions = FilterRegressionsForRelease(allRegressions, c.SampleRelease.Release)
	report := c.internalGenerateTestDetailsReport(ctx, componentJobRunTestReportStatus.BaseStatus, c.BaseRelease.Release, &c.BaseRelease.Start, &c.BaseRelease.End, componentJobRunTestReportStatus.SampleStatus)
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

func (c *componentReportGenerator) GenerateJobRunTestReportStatus(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {
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

// getTestDetailsQuery returns the report for a specific test + variant combo, including job run data.
// This is for the bottom level most specific pages in component readiness.
func (c *componentReportGenerator) getTestDetailsQuery(allJobVariants crtype.JobVariants, isSample bool) (string, string, []bigquery2.QueryParameter) {
	jobNameQueryPortion := normalJobNameCol
	if c.SampleRelease.PullRequestOptions != nil && isSample {
		jobNameQueryPortion = pullRequestDynamicJobNameCol
	}

	// TODO: this is a temporary hack while we explore if rarely run jobs approach is actually going to work.
	// A scheduled query is copying rarely run job results to a separate much smaller table every day, so we can
	// query 3 months without spending a fortune. If this proves to work, we will work out a system of processing
	// this as generically as we can, but it will be difficult.
	junitTable := defaultJunitTable
	for k, v := range c.IncludeVariants {
		if k == "JobTier" {
			if slices.Contains(v, "rare") {
				junitTable = rarelyRunJunitTable
			}
		}
	}

	queryString := fmt.Sprintf(`WITH latest_component_mapping AS (
						SELECT *
						FROM %s.component_mapping cm
						WHERE created_at = (
								SELECT MAX(created_at)
								FROM %s.component_mapping))
					SELECT
						ANY_VALUE(test_name) AS test_name,
						ANY_VALUE(testsuite) AS test_suite,
						file_path,
						ANY_VALUE(variant_registry_job_name) AS prowjob_name,
						ANY_VALUE(cm.jira_component) AS jira_component,
						ANY_VALUE(cm.jira_component_id) AS jira_component_id,
						COUNT(*) AS total_count,
						ANY_VALUE(cm.capabilities) as capabilities,
						SUM(adjusted_success_val) AS success_count,
						SUM(adjusted_flake_count) AS flake_count,
					FROM (%s)
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name
`, c.client.Dataset, c.client.Dataset, fmt.Sprintf(dedupedJunitTable, jobNameQueryPortion, c.client.Dataset, junitTable, c.client.Dataset))

	joinVariants := ""
	for variant := range allJobVariants.Variants {
		v := api.CleanseSQLName(variant)
		joinVariants += fmt.Sprintf("LEFT JOIN %s.job_variants jv_%s ON variant_registry_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			c.client.Dataset, v, v, v, v)
	}
	queryString += joinVariants

	groupString := `
					GROUP BY
						file_path,
						modified_time
					ORDER BY
						modified_time `
	queryString += `
					WHERE
						(variant_registry_job_name LIKE 'periodic-%%' OR variant_registry_job_name LIKE 'release-%%' OR variant_registry_job_name LIKE 'aggregator-%%')
						AND NOT REGEXP_CONTAINS(variant_registry_job_name, @IgnoredJobs)
						AND cm.id = @TestId `
	commonParams := []bigquery2.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
		{
			Name:  "TestId",
			Value: c.TestID,
		},
	}

	for k, vs := range c.IncludeVariants {
		// only add in include variants that aren't part of the requested or cross-compared variants
		if _, ok := c.RequestedVariants[k]; ok {
			continue
		}
		if slices.Contains(c.VariantCrossCompare, k) {
			continue
		}

		group := api.CleanseSQLName(k)
		paramName := "IncludeVariants" + group
		queryString += fmt.Sprintf(` AND jv_%s.variant_value IN UNNEST(@%s)`, group, paramName)
		commonParams = append(commonParams, bigquery2.QueryParameter{
			Name:  paramName,
			Value: vs,
		})
	}

	for group, variant := range c.RequestedVariants {
		queryString += fmt.Sprintf(` AND jv_%s.variant_value = '%s'`, api.CleanseSQLName(group), api.CleanseSQLName(variant))
	}
	if isSample {
		queryString += filterByCrossCompareVariants(c.VariantCrossCompare, c.CompareVariants, &commonParams)
	} else {
		queryString += filterByCrossCompareVariants(c.VariantCrossCompare, c.IncludeVariants, &commonParams)
	}
	return queryString, groupString, commonParams
}

// filterByCrossCompareVariants adds the where clause for any variants being cross-compared (which are not included in RequestedVariants).
// As a side effect, it also appends any necessary parameters for the clause.
func filterByCrossCompareVariants(crossCompare []string, variantGroups map[string][]string, params *[]bigquery2.QueryParameter) (whereClause string) {
	if len(variantGroups) == 0 {
		return // avoid possible nil pointer dereference
	}
	for _, group := range crossCompare {
		if variants := variantGroups[group]; len(variants) > 0 {
			group = api.CleanseSQLName(group)
			paramName := "CrossVariants" + group
			whereClause += fmt.Sprintf(` AND jv_%s.variant_value IN UNNEST(@%s)`, group, paramName)
			*params = append(*params, bigquery2.QueryParameter{
				Name:  paramName,
				Value: variants,
			})
		}
	}
	return
}

type baseJobRunTestStatusGenerator struct {
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery2.QueryParameter
	cacheOption              cache.RequestOptions
	BaseRelease              string
	BaseStart                time.Time
	BaseEnd                  time.Time
	ComponentReportGenerator *componentReportGenerator
}

func (c *componentReportGenerator) getBaseJobRunTestStatus(ctx context.Context, commonQuery string,
	groupByQuery string,
	baseRelease string,
	baseStart time.Time,
	baseEnd time.Time,
	queryParameters []bigquery2.QueryParameter) (map[string][]crtype.JobRunTestStatusRow, []error) {
	generator := baseJobRunTestStatusGenerator{
		commonQuery:     commonQuery,
		groupByQuery:    groupByQuery,
		queryParameters: queryParameters,
		cacheOption: cache.RequestOptions{
			ForceRefresh: c.cacheOption.ForceRefresh,
			// increase the time that base query is cached since it shouldn't be changing?
			CRTimeRoundingFactor: c.cacheOption.CRTimeRoundingFactor,
		},
		BaseRelease:              baseRelease,
		BaseEnd:                  baseEnd,
		BaseStart:                baseStart,
		ComponentReportGenerator: c,
	}

	jobRunTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.JobRunTestReportStatus](
		ctx,
		generator.ComponentReportGenerator.client.Cache, generator.cacheOption,
		api.GetPrefixedCacheKey("BaseJobRunTestStatus~", generator),
		generator.queryTestStatus,
		crtype.JobRunTestReportStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return jobRunTestStatus.BaseStatus, nil
}

func (b *baseJobRunTestStatusGenerator) queryTestStatus(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {
	baseString := b.commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.ComponentReportGenerator.client.BQ.Query(baseString + b.groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, b.queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery2.QueryParameter{
		{
			Name:  "From",
			Value: b.BaseStart,
		},
		{
			Name:  "To",
			Value: b.BaseEnd,
		},
		{
			Name:  "BaseRelease",
			Value: b.BaseRelease,
		},
	}...)

	baseStatus, errs := b.ComponentReportGenerator.fetchJobRunTestStatus(ctx, baseQuery)
	return crtype.JobRunTestReportStatus{BaseStatus: baseStatus}, errs
}

type sampleJobRunTestQueryGenerator struct {
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery2.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

func (c *componentReportGenerator) getSampleJobRunTestStatus(ctx context.Context, commonQuery string,
	groupByQuery string,
	queryParameters []bigquery2.QueryParameter,
) (map[string][]crtype.JobRunTestStatusRow, []error) {
	generator := sampleJobRunTestQueryGenerator{
		commonQuery:              commonQuery,
		groupByQuery:             groupByQuery,
		queryParameters:          queryParameters,
		ComponentReportGenerator: c,
	}

	jobRunTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.JobRunTestReportStatus](
		ctx,
		c.client.Cache, c.cacheOption,
		api.GetPrefixedCacheKey("SampleJobRunTestStatus~", generator),
		generator.queryTestStatus,
		crtype.JobRunTestReportStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return jobRunTestStatus.SampleStatus, nil
}

func (s *sampleJobRunTestQueryGenerator) queryTestStatus(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {
	sampleString := s.commonQuery + ` AND branch = @SampleRelease`
	// TODO
	if s.ComponentReportGenerator.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND org = @Org AND repo = @Repo AND pr_number = @PRNumber`
	}
	sampleQuery := s.ComponentReportGenerator.client.BQ.Query(sampleString + s.groupByQuery)
	sampleQuery.Parameters = append(sampleQuery.Parameters, s.queryParameters...)
	sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery2.QueryParameter{
		{
			Name:  "From",
			Value: s.ComponentReportGenerator.SampleRelease.Start,
		},
		{
			Name:  "To",
			Value: s.ComponentReportGenerator.SampleRelease.End,
		},
		{
			Name:  "SampleRelease",
			Value: s.ComponentReportGenerator.SampleRelease.Release,
		},
	}...)
	if s.ComponentReportGenerator.SampleRelease.PullRequestOptions != nil {
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery2.QueryParameter{
			{
				Name:  "Org",
				Value: s.ComponentReportGenerator.SampleRelease.PullRequestOptions.Org,
			},
			{
				Name:  "Repo",
				Value: s.ComponentReportGenerator.SampleRelease.PullRequestOptions.Repo,
			},
			{
				Name:  "PRNumber",
				Value: s.ComponentReportGenerator.SampleRelease.PullRequestOptions.PRNumber,
			},
		}...)
	}

	sampleStatus, errs := s.ComponentReportGenerator.fetchJobRunTestStatus(ctx, sampleQuery)

	return crtype.JobRunTestReportStatus{SampleStatus: sampleStatus}, errs
}

func (c *componentReportGenerator) getJobRunTestStatusFromBigQuery(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {
	allJobVariants, errs := GetJobVariantsFromBigQuery(ctx, c.client, c.gcsBucket)
	if len(errs) > 0 {
		logrus.Errorf("failed to get variants from bigquery")
		return crtype.JobRunTestReportStatus{}, errs
	}
	var baseStatus, baseOverrideStatus, sampleStatus map[string][]crtype.JobRunTestStatusRow
	var baseErrs, baseOverrideErrs, sampleErrs []error
	wg := sync.WaitGroup{}

	if c.BaseOverrideRelease.Release != "" && c.BaseOverrideRelease.Release != c.BaseRelease.Release {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				logrus.Infof("Context canceled while fetching base job run test status")
				return
			default:
				queryString, groupString, commonParams := c.getTestDetailsQuery(allJobVariants, false)
				baseOverrideStatus, baseOverrideErrs = c.getBaseJobRunTestStatus(ctx, queryString, groupString, c.BaseOverrideRelease.Release, c.BaseOverrideRelease.Start, c.BaseOverrideRelease.End, commonParams)
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
			queryString, groupString, commonParams := c.getTestDetailsQuery(allJobVariants, false)
			baseStatus, baseErrs = c.getBaseJobRunTestStatus(ctx, queryString, groupString, c.BaseRelease.Release, c.BaseRelease.Start, c.BaseRelease.End, commonParams)
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
			queryString, groupString, commonParams := c.getTestDetailsQuery(allJobVariants, true)
			sampleStatus, sampleErrs = c.getSampleJobRunTestStatus(ctx, queryString, groupString, commonParams)
		}

	}()
	wg.Wait()
	if len(baseErrs) != 0 || len(baseOverrideErrs) != 0 || len(sampleErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, baseOverrideErrs...)
		errs = append(errs, sampleErrs...)
	}

	return crtype.JobRunTestReportStatus{BaseStatus: baseStatus, BaseOverrideStatus: baseOverrideStatus, SampleStatus: sampleStatus}, errs
}

// internalGenerateTestDetailsReport handles the report generation for the lowest level test report including
// breakdown by job as well as overall stats.
func (c *componentReportGenerator) internalGenerateTestDetailsReport(ctx context.Context,
	baseStatus map[string][]crtype.JobRunTestStatusRow,
	baseRelease string,
	baseStart,
	baseEnd *time.Time,
	sampleStatus map[string][]crtype.JobRunTestStatusRow) crtype.ReportTestDetails {
	result := crtype.ReportTestDetails{
		ReportTestIdentification: crtype.ReportTestIdentification{
			RowIdentification: crtype.RowIdentification{
				Component:  c.Component,
				Capability: c.Capability,
				TestID:     c.TestID,
			},
			ColumnIdentification: crtype.ColumnIdentification{
				Variants: c.RequestedVariants,
			},
		},
	}
	var resolvedIssueCompensation int
	approvedRegression := regressionallowances.IntentionalRegressionFor(c.SampleRelease.Release, result.ColumnIdentification, c.TestID)
	var baseRegression *regressionallowances.IntentionalRegression
	// if we are ignoring fallback then honor the settings for the baseRegression
	// otherwise let fallback determine the threshold
	if !c.IncludeMultiReleaseAnalysis {
		baseRegression = regressionallowances.IntentionalRegressionFor(baseRelease, result.ColumnIdentification, c.TestID)
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

			jobStats.BaseJobRunStats = append(jobStats.BaseJobRunStats, getJobRunStats(baseStats, c.prowURL, c.gcsBucket))
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

				jobStats.SampleJobRunStats = append(jobStats.SampleJobRunStats, getJobRunStats(sampleStats, c.prowURL, c.gcsBucket))
				perJobSampleSuccess += sampleStats.SuccessCount
				perJobSampleFlake += sampleStats.FlakeCount
				perJobSampleFailure += getFailureCount(sampleStats)
			}
			delete(sampleStatus, prowJob)
		}
		jobStats.BaseStats.SuccessCount = perJobBaseSuccess
		jobStats.BaseStats.FlakeCount = perJobBaseFlake
		jobStats.BaseStats.FailureCount = perJobBaseFailure
		jobStats.BaseStats.SuccessRate = getSuccessRate(perJobBaseSuccess, perJobBaseFailure, perJobBaseFlake)
		jobStats.SampleStats.SuccessCount = perJobSampleSuccess
		jobStats.SampleStats.FlakeCount = perJobSampleFlake
		jobStats.SampleStats.FailureCount = perJobSampleFailure
		jobStats.SampleStats.SuccessRate = getSuccessRate(perJobSampleSuccess, perJobSampleFailure, perJobSampleFlake)
		_, _, r, _ := fet.FisherExactTest(perJobSampleFailure,
			perJobSampleSuccess,
			perJobBaseFailure,
			perJobSampleSuccess)
		jobStats.Significant = r < 1-float64(c.Confidence)/100

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
			jobStats.SampleJobRunStats = append(jobStats.SampleJobRunStats, getJobRunStats(sampleStats, c.prowURL, c.gcsBucket))
			perJobSampleSuccess += sampleStats.SuccessCount
			perJobSampleFlake += sampleStats.FlakeCount
			perJobSampleFailure += getFailureCount(sampleStats)
		}
		jobStats.SampleStats.SuccessCount = perJobSampleSuccess
		jobStats.SampleStats.FlakeCount = perJobSampleFlake
		jobStats.SampleStats.FailureCount = perJobSampleFailure
		jobStats.SampleStats.SuccessRate = getSuccessRate(perJobSampleSuccess, perJobSampleFailure, perJobSampleFlake)
		result.JobStats = append(result.JobStats, jobStats)
		_, _, r, _ := fet.FisherExactTest(perJobSampleFailure,
			perJobSampleSuccess+perJobSampleFlake,
			0,
			0)
		jobStats.Significant = r < 1-float64(c.Confidence)/100

		totalSampleFailure += perJobSampleFailure
		totalSampleSuccess += perJobSampleSuccess
		totalSampleFlake += perJobSampleFlake
	}
	sort.Slice(result.JobStats, func(i, j int) bool {
		return result.JobStats[i].JobName < result.JobStats[j].JobName
	})

	// The hope is that this goes away
	// once we agree we don't need to honor a higher intentional regression pass percentage
	if baseRegression != nil && baseRegression.PreviousPassPercentage() > getSuccessRate(totalBaseSuccess, totalBaseFailure, totalBaseFlake) {
		// override with  the basis regression previous values
		// testStats will reflect the expected threshold, not the computed values from the release with the allowed regression
		baseRegressionPreviousRelease, err := previousRelease(baseRelease)
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

	requiredConfidence := c.getRequiredConfidence(c.TestID, c.RequestedVariants)

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

func getJobRunStats(stats crtype.JobRunTestStatusRow, prowURL, gcsBucket string) crtype.TestDetailsJobRunStats {
	failure := getFailureCount(stats)
	url := fmt.Sprintf("%s/view/gs/%s/", prowURL, gcsBucket)
	subs := strings.Split(stats.FilePath, "/artifacts/")
	if len(subs) > 1 {
		url += subs[0]
	}
	jobRunStats := crtype.TestDetailsJobRunStats{
		TestStats: crtype.TestDetailsTestStats{
			SuccessRate:  getSuccessRate(stats.SuccessCount, failure, stats.FlakeCount),
			SuccessCount: stats.SuccessCount,
			FailureCount: failure,
			FlakeCount:   stats.FlakeCount,
		},
		JobURL: url,
	}
	return jobRunStats
}
