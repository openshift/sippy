package componentreadiness

import (
	bigquery2 "cloud.google.com/go/bigquery"
	"fmt"
	"github.com/glycerine/golang-fisher-exact"
	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/componentreadiness/tracker"
	"github.com/openshift/sippy/pkg/regressionallowances"
	"github.com/sirupsen/logrus"
	"sort"
	"strings"
	"sync"
	"time"
)

func GetTestDetails(client *bigquery.Client, prowURL, gcsBucket string,
	baseRelease, sampleRelease componentreport.RequestReleaseOptions,
	testIDOption componentreport.RequestTestIdentificationOptions,
	variantOption componentreport.RequestVariantOptions,
	advancedOption componentreport.RequestAdvancedOptions,
	cacheOption cache.RequestOptions) (componentreport.ReportTestDetails, []error) {
	generator := componentReportGenerator{
		client:                           client,
		prowURL:                          prowURL,
		gcsBucket:                        gcsBucket,
		cacheOption:                      cacheOption,
		BaseRelease:                      baseRelease,
		SampleRelease:                    sampleRelease,
		RequestTestIdentificationOptions: testIDOption,
		RequestVariantOptions:            variantOption,
		RequestAdvancedOptions:           advancedOption,
	}

	return api.GetDataFromCacheOrGenerate[componentreport.ReportTestDetails](
		generator.client.Cache,
		generator.cacheOption,
		generator.GetComponentReportCacheKey("TestDetailsReport~"),
		generator.GenerateTestDetailsReport,
		componentreport.ReportTestDetails{})
}

func (c *componentReportGenerator) GenerateTestDetailsReport() (componentreport.ReportTestDetails, []error) {
	if c.TestID == "" {
		return componentreport.ReportTestDetails{}, []error{fmt.Errorf("test_id has to be defined for test details")}
	}
	for _, v := range c.DBGroupBy.List() {
		if _, ok := c.RequestedVariants[v]; !ok {
			return componentreport.ReportTestDetails{}, []error{fmt.Errorf("all dbGroupBy variants have to be defined for test details: %s is missing", v)}
		}
	}

	componentJobRunTestReportStatus, errs := c.GenerateJobRunTestReportStatus()
	if len(errs) > 0 {
		return componentreport.ReportTestDetails{}, errs
	}
	var err error
	bqs := tracker.NewBigQueryRegressionStore(c.client)
	c.openRegressions, err = bqs.ListCurrentRegressions(c.SampleRelease.Release)
	if err != nil {
		errs = append(errs, err)
		return componentreport.ReportTestDetails{}, errs
	}
	report := c.generateTestDetailsReport(componentJobRunTestReportStatus.BaseStatus, componentJobRunTestReportStatus.SampleStatus)
	report.GeneratedAt = componentJobRunTestReportStatus.GeneratedAt
	return report, nil
}

func (c *componentReportGenerator) GenerateJobRunTestReportStatus() (componentreport.JobRunTestReportStatus, []error) {
	before := time.Now()
	componentJobRunTestReportStatus, errs := c.getJobRunTestStatusFromBigQuery()
	if len(errs) > 0 {
		return componentreport.JobRunTestReportStatus{}, errs
	}
	logrus.Infof("getJobRunTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentJobRunTestReportStatus.SampleStatus), len(componentJobRunTestReportStatus.BaseStatus))
	now := time.Now()
	componentJobRunTestReportStatus.GeneratedAt = &now
	return componentJobRunTestReportStatus, nil
}

// getTestDetailsQuery returns the report for a specific test + variant combo, including job run data.
// This is for the bottom level most specific pages in component readiness.
func (c *componentReportGenerator) getTestDetailsQuery(allJobVariants componentreport.JobVariants, isSample bool) (string, string, []bigquery2.QueryParameter) {
	joinVariants := ""
	for v := range allJobVariants.Variants {
		joinVariants += fmt.Sprintf("LEFT JOIN %s.job_variants jv_%s ON variant_registry_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			c.client.Dataset, v, v, v, v)
	}

	jobNameQueryPortion := normalJobNameCol
	if c.SampleRelease.PullRequestOptions != nil && isSample {
		jobNameQueryPortion = pullRequestDynamicJobNameCol
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
`, c.client.Dataset, c.client.Dataset, fmt.Sprintf(dedupedJunitTable, jobNameQueryPortion, c.client.Dataset, c.client.Dataset))

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
	for k, v := range c.RequestedVariants {
		queryString += fmt.Sprintf(` AND jv_%s.variant_value = '%s'`, k, v)
	}
	return queryString, groupString, commonParams
}

type baseJobRunTestStatusGenerator struct {
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery2.QueryParameter
	cacheOption              cache.RequestOptions
	ComponentReportGenerator *componentReportGenerator
}

func (c *componentReportGenerator) getBaseJobRunTestStatus(commonQuery string,
	groupByQuery string,
	queryParameters []bigquery2.QueryParameter) (map[string][]componentreport.JobRunTestStatusRow, []error) {
	generator := baseJobRunTestStatusGenerator{
		commonQuery:     commonQuery,
		groupByQuery:    groupByQuery,
		queryParameters: queryParameters,
		cacheOption: cache.RequestOptions{
			ForceRefresh: c.cacheOption.ForceRefresh,
			// increase the time that base query is cached since it shouldn't be changing?
			CRTimeRoundingFactor: c.cacheOption.CRTimeRoundingFactor,
		},
		ComponentReportGenerator: c,
	}

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[componentreport.JobRunTestReportStatus](generator.ComponentReportGenerator.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("BaseJobRunTestStatus~", generator), generator.queryTestStatus, componentreport.JobRunTestReportStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.BaseStatus, nil
}

func (b *baseJobRunTestStatusGenerator) queryTestStatus() (componentreport.JobRunTestReportStatus, []error) {
	baseString := b.commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.ComponentReportGenerator.client.BQ.Query(baseString + b.groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, b.queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery2.QueryParameter{
		{
			Name:  "From",
			Value: b.ComponentReportGenerator.BaseRelease.Start,
		},
		{
			Name:  "To",
			Value: b.ComponentReportGenerator.BaseRelease.End,
		},
		{
			Name:  "BaseRelease",
			Value: b.ComponentReportGenerator.BaseRelease.Release,
		},
	}...)

	baseStatus, errs := b.ComponentReportGenerator.fetchJobRunTestStatus(baseQuery)
	return componentreport.JobRunTestReportStatus{BaseStatus: baseStatus}, errs
}

type sampleJobRunTestQueryGenerator struct {
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery2.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

func (c *componentReportGenerator) getSampleJobRunTestStatus(commonQuery string,
	groupByQuery string,
	queryParameters []bigquery2.QueryParameter) (map[string][]componentreport.JobRunTestStatusRow, []error) {
	generator := sampleJobRunTestQueryGenerator{
		commonQuery:              commonQuery,
		groupByQuery:             groupByQuery,
		queryParameters:          queryParameters,
		ComponentReportGenerator: c,
	}

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[componentreport.JobRunTestReportStatus](c.client.Cache, c.cacheOption, api.GetPrefixedCacheKey("SampleJobRunTestStatus~", generator), generator.queryTestStatus, componentreport.JobRunTestReportStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.SampleStatus, nil
}

func (s *sampleJobRunTestQueryGenerator) queryTestStatus() (componentreport.JobRunTestReportStatus, []error) {
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

	sampleStatus, errs := s.ComponentReportGenerator.fetchJobRunTestStatus(sampleQuery)

	return componentreport.JobRunTestReportStatus{SampleStatus: sampleStatus}, errs
}

func (c *componentReportGenerator) getJobRunTestStatusFromBigQuery() (componentreport.JobRunTestReportStatus, []error) {
	allJobVariants, errs := GetJobVariantsFromBigQuery(c.client, c.gcsBucket)
	if len(errs) > 0 {
		logrus.Errorf("failed to get variants from bigquery")
		return componentreport.JobRunTestReportStatus{}, errs
	}
	queryString, groupString, commonParams := c.getTestDetailsQuery(allJobVariants, false)
	var baseStatus, sampleStatus map[string][]componentreport.JobRunTestStatusRow
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		baseStatus, baseErrs = c.getBaseJobRunTestStatus(queryString, groupString, commonParams)
	}()

	queryString, groupString, commonParams = c.getTestDetailsQuery(allJobVariants, true)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sampleStatus, sampleErrs = c.getSampleJobRunTestStatus(queryString, groupString, commonParams)
	}()
	wg.Wait()
	if len(baseErrs) != 0 || len(sampleErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, sampleErrs...)
	}

	return componentreport.JobRunTestReportStatus{BaseStatus: baseStatus, SampleStatus: sampleStatus}, errs
}

// generateTestDetailsReport handles the report generation for the lowest level test report including
// breakdown by job as well as overall stats.
func (c *componentReportGenerator) generateTestDetailsReport(baseStatus map[string][]componentreport.JobRunTestStatusRow,
	sampleStatus map[string][]componentreport.JobRunTestStatusRow) componentreport.ReportTestDetails {
	result := componentreport.ReportTestDetails{
		ReportTestIdentification: componentreport.ReportTestIdentification{
			RowIdentification: componentreport.RowIdentification{
				Component:  c.Component,
				Capability: c.Capability,
				TestID:     c.TestID,
			},
			ColumnIdentification: componentreport.ColumnIdentification{
				Variants: c.RequestedVariants,
			},
		},
	}
	approvedRegression := regressionallowances.IntentionalRegressionFor(c.SampleRelease.Release, result.ColumnIdentification, c.TestID)
	resolvedIssueCompensation, _ := c.triagedIncidentsFor(result.ReportTestIdentification)

	var totalBaseFailure, totalBaseSuccess, totalBaseFlake, totalSampleFailure, totalSampleSuccess, totalSampleFlake int
	var perJobBaseFailure, perJobBaseSuccess, perJobBaseFlake, perJobSampleFailure, perJobSampleSuccess, perJobSampleFlake int

	for prowJob, baseStatsList := range baseStatus {
		jobStats := componentreport.TestDetailsJobStats{
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
		jobStats := componentreport.TestDetailsJobStats{
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
	)

	return result
}

func getJobRunStats(stats componentreport.JobRunTestStatusRow, prowURL, gcsBucket string) componentreport.TestDetailsJobRunStats {
	failure := getFailureCount(stats)
	url := fmt.Sprintf("%s/view/gs/%s/", prowURL, gcsBucket)
	subs := strings.Split(stats.FilePath, "/artifacts/")
	if len(subs) > 1 {
		url += subs[0]
	}
	jobRunStats := componentreport.TestDetailsJobRunStats{
		TestStats: componentreport.TestDetailsTestStats{
			SuccessRate:  getSuccessRate(stats.SuccessCount, failure, stats.FlakeCount),
			SuccessCount: stats.SuccessCount,
			FailureCount: failure,
			FlakeCount:   stats.FlakeCount,
		},
		JobURL: url,
	}
	return jobRunStats
}
