package componentreadiness

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openshift/sippy/pkg/util"

	"cloud.google.com/go/bigquery"
	"github.com/apache/thrift/lib/go/thrift"
	fischer "github.com/glycerine/golang-fisher-exact"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/componentreadiness/resolvedissues"
	"github.com/openshift/sippy/pkg/regressionallowances"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	triagedIncidentsTableID = "triaged_incidents"

	ignoredJobsRegexp = `-okd|-recovery|aggregator-|alibaba|-disruptive|-rollback|-out-of-change|-sno-fips-recert`

	// openRegressionConfidenceAdjustment is subtracted from the requested confidence for regressed tests that have
	// an open regression.
	openRegressionConfidenceAdjustment = 5

	explanationNoRegression = "No significant regressions found"

	defaultJunitTable   = "junit"
	rarelyRunJunitTable = "junit_rarely_run_jobs"

	// This query de-dupes the test results. There are multiple issues present in
	// our data set:
	//
	// 1. Some test suites in OpenShift retry, resulting in potentially multiple
	//    failures for the same test in a job.  Component Readiness is currently
	//    counting these as separate failures, resulting in an outsized impact on
	//    our statistical analysis.
	//
	// 2. There is a second bug where successful test cases are sometimes
	//    recorded by openshift-tests more than once, it's tracked by
	//    https://issues.redhat.com/browse/OCPBUGS-16039
	//
	// 3. Flaked tests also have rows for the failures, so we need to ensure we only collect the flakes.
	//
	// 4. Flaked tests appear to be able to have success_val as 0 or 1.
	//
	// So, this sorts the data, partitioning by the 3-tuple of file_path/test_name/testsuite -
	// preferring flakes, then successes, then failures, and we get the first row of each
	// partition.
	dedupedJunitTable = `
		WITH deduped_testcases AS (
			SELECT  
				junit.*,
				ROW_NUMBER() OVER(PARTITION BY file_path, test_name, testsuite ORDER BY
					CASE
						WHEN flake_count > 0 THEN 0
						WHEN success_val > 0 THEN 1
						ELSE 2
					END) AS row_num,
%s
				jobs.org,
				jobs.repo,
				jobs.pr_number,
				jobs.pr_sha,
				CASE
					WHEN flake_count > 0 THEN 0
					ELSE success_val
				END AS adjusted_success_val,
				CASE
					WHEN flake_count > 0 THEN 1
					ELSE 0
				END AS adjusted_flake_count
			FROM
				%s.%s AS junit
			INNER JOIN %s.jobs  jobs ON 
				junit.prowjob_build_id = jobs.prowjob_build_id 
				AND jobs.prowjob_start >= DATETIME(@From)
				AND jobs.prowjob_start < DATETIME(@To)
			WHERE modified_time >= DATETIME(@From)
			AND modified_time < DATETIME(@To)
			AND skipped = false
		)
		SELECT * FROM deduped_testcases WHERE row_num = 1`

	// normalJobNameCol simply uses the prow job name for regular (non-pull-request) component reports.
	normalJobNameCol = `
				jobs.prowjob_job_name AS variant_registry_job_name,
`
	// pullRequestDynamicJobNameCol is used for pull-request component reports and will use the releaseJobName
	// annotation for /payload jobs if it exists, otherwise the normal prow job name. This is done as /payload
	// jobs get custom job names which will not be in the variant registry.
	pullRequestDynamicJobNameCol = `
				CASE
					WHEN EXISTS (
						SELECT 1
						FROM UNNEST(jobs.prowjob_annotations) AS annotation
						WHERE annotation LIKE 'releaseJobName=%%'
					) THEN (
						SELECT
						SPLIT(SPLIT(annotation, 'releaseJobName=')[OFFSET(1)], ',')[SAFE_OFFSET(0)]
						FROM UNNEST(jobs.prowjob_annotations) AS annotation	
						WHERE annotation LIKE 'releaseJobName=%%'
						LIMIT 1
					)
					ELSE jobs.prowjob_job_name
		    	END AS variant_registry_job_name,
`
	// consider fallback data good for 7 days
	fallbackQueryTimeRoundingOverride = 24 * 7 * time.Hour
)

type GeneratorType string

var (
	// Default filters, these are also hardcoded in the UI. Both must be updated.
	// TODO: TRT-1237 should centralize these configurations for consumption by both the front and backends

	DefaultColumnGroupBy   = "Platform,Architecture,Network"
	DefaultDBGroupBy       = "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Suite,Installer"
	DefaultIncludeVariants = []string{
		"Architecture:amd64",
		"FeatureSet:default",
		"Installer:ipi",
		"Installer:upi",
		"Owner:eng",
		"Platform:aws",
		"Platform:azure",
		"Platform:gcp",
		"Platform:metal",
		"Platform:vsphere",
		"Topology:ha",
		"CGroupMode:v2",
		"ContainerRuntime:runc",
	}
	DefaultMinimumFailure   = 3
	DefaultConfidence       = 95
	DefaultPityFactor       = 5
	DefaultIgnoreMissing    = false
	DefaultIgnoreDisruption = true
)

func newFallbackReleases() crtype.FallbackReleases {
	fb := crtype.FallbackReleases{
		Releases: map[string]crtype.ReleaseTestMap{},
	}
	return fb
}

func getSingleColumnResultToSlice(ctx context.Context, query *bigquery.Query) ([]string, error) {
	names := []string{}
	it, err := query.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying test status from bigquery")
		return names, err
	}

	for {
		row := struct{ Name string }{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing component from bigquery")
			return names, err
		}
		names = append(names, row.Name)
	}
	return names, nil
}

func GetComponentTestVariantsFromBigQuery(ctx context.Context, client *bqcachedclient.Client,
	gcsBucket string) (crtype.TestVariants, []error) {
	generator := componentReportGenerator{
		client:    client,
		gcsBucket: gcsBucket,
	}

	return api.GetDataFromCacheOrGenerate[crtype.TestVariants](ctx, client.Cache, cache.RequestOptions{},
		api.GetPrefixedCacheKey("TestVariants~", generator), generator.GenerateVariants, crtype.TestVariants{})
}

func GetReleaseDatesFromBigQuery(ctx context.Context, client *bqcachedclient.Client) ([]crtype.Release, []error) {
	generator := componentReportGenerator{
		client: client,
	}

	return api.GetDataFromCacheOrGenerate[[]crtype.Release](ctx, client.Cache, cache.RequestOptions{}, api.GetPrefixedCacheKey("CRReleaseDates~", generator), generator.GenerateReleaseDates, []crtype.Release{})
}

func GetJobVariantsFromBigQuery(ctx context.Context, client *bqcachedclient.Client,
	gcsBucket string) (crtype.JobVariants,
	[]error) {
	generator := componentReportGenerator{
		client:    client,
		gcsBucket: gcsBucket,
	}

	return api.GetDataFromCacheOrGenerate[crtype.JobVariants](ctx, client.Cache, cache.RequestOptions{},
		api.GetPrefixedCacheKey("TestAllVariants~", generator), generator.GenerateJobVariants, crtype.JobVariants{})
}

func GetComponentReportFromBigQuery(ctx context.Context, client *bqcachedclient.Client, prowURL, gcsBucket string,
	reqOptions crtype.RequestOptions,
) (crtype.ComponentReport, []error) {
	generator := componentReportGenerator{
		client:                           client,
		prowURL:                          prowURL,
		gcsBucket:                        gcsBucket,
		cacheOption:                      reqOptions.CacheOption,
		BaseRelease:                      reqOptions.BaseRelease,
		SampleRelease:                    reqOptions.SampleRelease,
		triagedIssues:                    nil,
		RequestTestIdentificationOptions: reqOptions.TestIDOption,
		RequestVariantOptions:            reqOptions.VariantOption,
		RequestAdvancedOptions:           reqOptions.AdvancedOption,
	}

	return api.GetDataFromCacheOrGenerate[crtype.ComponentReport](
		ctx,
		generator.client.Cache, generator.cacheOption,
		generator.GetComponentReportCacheKey(ctx, "ComponentReport~"),
		generator.GenerateReport,
		crtype.ComponentReport{})
}

// componentReportGenerator contains the information needed to generate a CR report. Do
// not add public fields to this struct if they are not valid as a cache key.
// GeneratorVersion is used to indicate breaking changes in the versions of
// the cached data.  It is used when the struct
// is marshalled for the cache key and should be changed when the object being
// cached changes in a way that will no longer be compatible with any prior cached version.
type componentReportGenerator struct {
	ReportModified             *time.Time
	client                     *bqcachedclient.Client
	prowURL                    string
	gcsBucket                  string
	cacheOption                cache.RequestOptions
	BaseRelease                crtype.RequestReleaseOptions
	BaseOverrideRelease        crtype.RequestReleaseOptions
	SampleRelease              crtype.RequestReleaseOptions
	triagedIssues              *resolvedissues.TriagedIncidentsForRelease
	cachedFallbackTestStatuses *crtype.FallbackReleases
	crtype.RequestTestIdentificationOptions
	crtype.RequestVariantOptions
	crtype.RequestAdvancedOptions
	openRegressions []*crtype.TestRegression
}

func (c *componentReportGenerator) GetComponentReportCacheKey(ctx context.Context, prefix string) api.CacheData {
	// Make sure we have initialized the report modified field
	if c.ReportModified == nil {
		c.ReportModified = c.GetLastReportModifiedTime(ctx, c.client, c.cacheOption)
	}
	return api.GetPrefixedCacheKey(prefix, c)
}

func (c *componentReportGenerator) GenerateVariants(ctx context.Context) (crtype.TestVariants, []error) {
	errs := []error{}
	columns := make(map[string][]string)

	for _, column := range []string{"platform", "network", "arch", "upgrade", "variants"} {
		values, err := c.getUniqueJUnitColumnValuesLast60Days(ctx, column, column == "variants")
		if err != nil {
			wrappedErr := errors.Wrapf(err, "couldn't fetch %s", column)
			log.WithError(wrappedErr).Errorf("error generating variants")
			errs = append(errs, wrappedErr)
		}
		columns[column] = values
	}

	return crtype.TestVariants{
		Platform: columns["platform"],
		Network:  columns["network"],
		Arch:     columns["arch"],
		Upgrade:  columns["upgrade"],
		Variant:  columns["variants"],
	}, errs
}

func (c *componentReportGenerator) GenerateReleaseDates(ctx context.Context) ([]crtype.Release, []error) {
	releases, err := api.GetReleasesFromBigQuery(ctx, c.client)
	if err != nil {
		return nil, []error{err}
	}
	crReleases := []crtype.Release{}
	for _, release := range releases {
		crRelease := crtype.Release{Release: release.Release}
		if release.GADate != nil {
			prior := util.AdjustReleaseTime(*release.GADate, true, "30", c.cacheOption.CRTimeRoundingFactor)
			crRelease.Start = &prior
			crRelease.End = release.GADate
		}
		crReleases = append(crReleases, crRelease)
	}
	return crReleases, nil
}

func (c *componentReportGenerator) GenerateJobVariants(ctx context.Context) (crtype.JobVariants, []error) {
	errs := []error{}
	variants := crtype.JobVariants{Variants: map[string][]string{}}
	queryString := fmt.Sprintf(`SELECT variant_name, ARRAY_AGG(DISTINCT variant_value ORDER BY variant_value) AS variant_values
					FROM
						%s.job_variants
					WHERE
						variant_value!=""
					GROUP BY
						variant_name`, c.client.Dataset)
	query := c.client.BQ.Query(queryString)
	it, err := query.Read(ctx)
	if err != nil {
		log.WithError(err).Errorf("error querying variants from bigquery for %s", queryString)
		return variants, []error{err}
	}

	floatVariants := sets.NewString("FromRelease", "FromReleaseMajor", "FromReleaseMinor", "Release", "ReleaseMajor", "ReleaseMinor")
	for {
		row := crtype.JobVariant{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			wrappedErr := errors.Wrapf(err, "error fetching variant row")
			log.WithError(err).Error("error fetching variants from bigquery")
			errs = append(errs, wrappedErr)
			return variants, errs
		}

		// Sort all releases in proper orders
		if floatVariants.Has(row.VariantName) {
			sort.Slice(row.VariantValues, func(i, j int) bool {
				iStrings := strings.Split(row.VariantValues[i], ".")
				jStrings := strings.Split(row.VariantValues[j], ".")
				for idx, iString := range iStrings {
					if iValue, err := strconv.ParseInt(iString, 10, 32); err == nil {
						if jValue, err := strconv.ParseInt(jStrings[idx], 10, 32); err == nil {
							if iValue != jValue {
								return iValue < jValue
							}
						}
					}
				}
				return false
			})
		}
		variants.Variants[row.VariantName] = row.VariantValues
	}
	return variants, nil
}

func (c *componentReportGenerator) GenerateReport(ctx context.Context) (crtype.ComponentReport, []error) {
	before := time.Now()
	componentReportTestStatus, errs := c.GenerateComponentReportTestStatus(ctx)
	if len(errs) > 0 {
		return crtype.ComponentReport{}, errs
	}
	bqs := NewBigQueryRegressionStore(c.client)
	var err error
	allRegressions, err := bqs.ListCurrentRegressions(ctx)
	if err != nil {
		errs = append(errs, err)
		return crtype.ComponentReport{}, errs
	}
	c.openRegressions = FilterRegressionsForRelease(allRegressions, c.SampleRelease.Release)
	report, err := c.generateComponentTestReport(ctx, componentReportTestStatus.BaseStatus,
		componentReportTestStatus.SampleStatus)
	if err != nil {
		errs = append(errs, err)
		return crtype.ComponentReport{}, errs
	}
	report.GeneratedAt = componentReportTestStatus.GeneratedAt
	log.Infof("GenerateReport completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentReportTestStatus.SampleStatus), len(componentReportTestStatus.BaseStatus))

	return report, nil
}

func (c *componentReportGenerator) GenerateComponentReportTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	componentReportTestStatus, errs := c.getTestStatusFromBigQuery(ctx)
	if len(errs) > 0 {
		return crtype.ReportTestStatus{}, errs
	}
	log.Infof("getTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentReportTestStatus.SampleStatus), len(componentReportTestStatus.BaseStatus))
	now := time.Now()
	componentReportTestStatus.GeneratedAt = &now
	return componentReportTestStatus, nil
}

// getCommonTestStatusQuery returns the common query for the higher level summary component summary.
func (c *componentReportGenerator) getCommonTestStatusQuery(allJobVariants crtype.JobVariants, isSample, isFallback bool) (string, string, []bigquery.QueryParameter) {
	// Parts of the query, including the columns returned, are dynamic, based on the list of variants we're told to work with.
	// Variants will be returned as columns with names like: variant_[VariantName]
	// See fetchTestStatus for where we dynamically handle these columns.
	selectVariants := ""
	joinVariants := ""
	groupByVariants := ""
	for v := range allJobVariants.Variants {
		joinVariants += fmt.Sprintf("LEFT JOIN %s.job_variants jv_%s ON variant_registry_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			c.client.Dataset, v, v, v, v)
	}
	for _, v := range c.DBGroupBy.List() {
		selectVariants += fmt.Sprintf("jv_%s.variant_value AS variant_%s,\n", v, v) // Note: Variants are camelcase, so the query columns come back like: variant_Architecture
		groupByVariants += fmt.Sprintf("jv_%s.variant_value,\n", v)
	}

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

	// WARNING: returning additional columns from this query will require explicit parsing in deserializeRowToTestStatus
	// TODO: jira_component and jira_component_id appear to not be used? Could save bigquery costs if we remove them.
	queryString := fmt.Sprintf(`WITH latest_component_mapping AS (
						SELECT *
						FROM %s.component_mapping cm
						WHERE created_at = (
								SELECT MAX(created_at)
								FROM %s.component_mapping))
					SELECT
						ANY_VALUE(test_name) AS test_name,
						ANY_VALUE(testsuite) AS test_suite,
						cm.id as test_id,
						%s
						COUNT(cm.id) AS total_count,
						SUM(adjusted_success_val) AS success_count,
						SUM(adjusted_flake_count) AS flake_count,
						ANY_VALUE(cm.component) AS component,
						ANY_VALUE(cm.capabilities) AS capabilities,
					FROM (%s)
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name
`,
		c.client.Dataset, c.client.Dataset, selectVariants, fmt.Sprintf(dedupedJunitTable, jobNameQueryPortion, c.client.Dataset, junitTable, c.client.Dataset))

	queryString += joinVariants

	groupString := fmt.Sprintf(`
					GROUP BY
						%s
						cm.id `, groupByVariants)

	queryString += `WHERE cm.staff_approved_obsolete = false AND 
						(variant_registry_job_name LIKE 'periodic-%%' OR variant_registry_job_name LIKE 'release-%%' OR variant_registry_job_name LIKE 'aggregator-%%') 
						AND NOT REGEXP_CONTAINS(variant_registry_job_name, @IgnoredJobs)`

	commonParams := []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
	}
	if c.IgnoreDisruption {
		queryString += ` AND NOT 'Disruption' in UNNEST(capabilities)`
	}

	// fallback queries get all variants with no filtering
	// so all tests are fetched then cached
	if !isFallback {
		variantGroups := c.IncludeVariants
		// potentially cross-compare variants for the sample
		if isSample && len(c.VariantCrossCompare) > 0 {
			variantGroups = c.CompareVariants
		}
		if variantGroups == nil { // server-side view definitions may omit a variants map
			variantGroups = map[string][]string{}
		}

		for group, variants := range variantGroups {
			queryString += " AND ("
			first := true
			for _, variant := range variants {
				if first {
					queryString += fmt.Sprintf(`jv_%s.variant_value = '%s'`, group, variant)
					first = false
				} else {
					queryString += fmt.Sprintf(` OR jv_%s.variant_value = '%s'`, group, variant)
				}
			}
			queryString += ")"
		}

		for k, v := range c.RequestedVariants {
			queryString += fmt.Sprintf(` AND jv_%s.variant_value = '%s'`, k, v)
		}
		if c.Capability != "" {
			queryString += " AND @Capability in UNNEST(capabilities)"
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  "Capability",
				Value: c.Capability,
			})
		}
		if c.TestID != "" {
			queryString += ` AND cm.id = @TestId`
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  "TestId",
				Value: c.TestID,
			})
		}
	}
	return queryString, groupString, commonParams
}

type baseQueryGenerator struct {
	client                   *bqcachedclient.Client
	cacheOption              cache.RequestOptions
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

// getBaseQueryStatus builds the basis query, executes it, and returns the basis test status.
func (c *componentReportGenerator) getBaseQueryStatus(ctx context.Context,
	allJobVariants crtype.JobVariants) (map[string]crtype.TestStatus, []error) {
	baseQuery, baseGrouping, baseParams := c.getCommonTestStatusQuery(allJobVariants, false, false)
	generator := baseQueryGenerator{
		client: c.client,
		cacheOption: cache.RequestOptions{
			ForceRefresh: c.cacheOption.ForceRefresh,
			// increase the time that base query is cached for since it shouldn't be changing?
			CRTimeRoundingFactor: c.cacheOption.CRTimeRoundingFactor,
		},
		commonQuery:              baseQuery,
		groupByQuery:             baseGrouping,
		queryParameters:          baseParams,
		ComponentReportGenerator: c,
	}

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, c.client.Cache,
		generator.cacheOption, api.GetPrefixedCacheKey("BaseTestStatus~", generator), generator.queryTestStatus, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.BaseStatus, nil
}

func (c *componentReportGenerator) getFallbackBaseQueryStatus(ctx context.Context, allJobVariants crtype.JobVariants, release string, start, end time.Time) []error {
	baseQuery, baseGrouping, baseParams := c.getCommonTestStatusQuery(allJobVariants, false, true)
	generator := fallbackTestQueryReleasesGenerator{
		client: c.client,
		cacheOption: cache.RequestOptions{
			ForceRefresh: c.cacheOption.ForceRefresh,
			// increase the time that fallback queries are cached for
			CRTimeRoundingFactor: fallbackQueryTimeRoundingOverride,
		},
		commonQuery:     baseQuery,
		groupByQuery:    baseGrouping,
		BaseRelease:     release,
		BaseStart:       start,
		BaseEnd:         end,
		queryParameters: baseParams,
		lock:            &sync.Mutex{},
	}

	cachedFallbackTestStatuses, errs := api.GetDataFromCacheOrGenerate[*crtype.FallbackReleases](ctx, c.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("FallbackReleases~", generator), generator.getTestFallbackReleases, &crtype.FallbackReleases{})

	if len(errs) > 0 {
		return errs
	}

	c.cachedFallbackTestStatuses = cachedFallbackTestStatuses
	return nil
}

func (b *baseQueryGenerator) queryTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	errs := []error{}
	baseString := b.commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.client.BQ.Query(baseString + b.groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, b.queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
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

	baseStatus, baseErrs := fetchTestStatus(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Base QueryTestStatus completed in %s with %d base results from db", time.Since(before), len(baseStatus))

	return crtype.ReportTestStatus{BaseStatus: baseStatus}, errs
}

type sampleQueryGenerator struct {
	client                   *bqcachedclient.Client
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

// getSampleQueryStatus builds the sample query, executes it, and returns the sample test status.
func (c *componentReportGenerator) getSampleQueryStatus(
	ctx context.Context, allJobVariants crtype.JobVariants) (map[string]crtype.TestStatus, []error) {
	commonQuery, groupByQuery, queryParameters := c.getCommonTestStatusQuery(allJobVariants, true, false)
	generator := sampleQueryGenerator{
		client:                   c.client,
		commonQuery:              commonQuery,
		groupByQuery:             groupByQuery,
		queryParameters:          queryParameters,
		ComponentReportGenerator: c,
	}

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx,
		c.client.Cache, c.cacheOption,
		api.GetPrefixedCacheKey("SampleTestStatus~", generator),
		generator.queryTestStatus, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.SampleStatus, nil
}

func (s *sampleQueryGenerator) queryTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	errs := []error{}
	sampleString := s.commonQuery + ` AND branch = @SampleRelease`
	if s.ComponentReportGenerator.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND org = @Org AND repo = @Repo AND pr_number = @PRNumber`
	}
	sampleQuery := s.client.BQ.Query(sampleString + s.groupByQuery)
	sampleQuery.Parameters = append(sampleQuery.Parameters, s.queryParameters...)
	sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
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
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
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

	sampleStatus, sampleErrs := fetchTestStatus(ctx, sampleQuery)

	if len(sampleErrs) != 0 {
		errs = append(errs, sampleErrs...)
	}

	log.Infof("Sample QueryTestStatus completed in %s with %d sample results db", time.Since(before), len(sampleStatus))

	return crtype.ReportTestStatus{SampleStatus: sampleStatus}, errs
}

type fallbackTestQueryReleasesGenerator struct {
	client                     *bqcachedclient.Client
	cacheOption                cache.RequestOptions
	commonQuery                string
	groupByQuery               string
	queryParameters            []bigquery.QueryParameter
	allJobVariants             crtype.JobVariants
	BaseRelease                string
	BaseStart                  time.Time
	BaseEnd                    time.Time
	CachedFallbackTestStatuses crtype.FallbackReleases
	lock                       *sync.Mutex
}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackReleases(ctx context.Context) (*crtype.FallbackReleases, []error) {
	wg := sync.WaitGroup{}
	f.CachedFallbackTestStatuses = newFallbackReleases()
	releases, errs := GetReleaseDatesFromBigQuery(ctx, f.client)

	if errs != nil {
		return nil, errs
	}

	// currently gets current base plus previous 3
	// current base is just for testing but use could be
	// extended to no longer require the base query
	var selectedReleases []*crtype.Release
	fallbackRelease := f.BaseRelease

	// Get up to 3 fallback releases
	for i := 0; i < 3; i++ {
		var crRelease *crtype.Release

		fallbackRelease, err := previousRelease(fallbackRelease)
		if err != nil {
			log.WithError(err).Errorf("Failure determining fallback release for %s", fallbackRelease)
			break
		}

		for i := range releases {
			if releases[i].Release == fallbackRelease {
				crRelease = &releases[i]
				break
			}
		}

		if crRelease != nil {
			selectedReleases = append(selectedReleases, crRelease)
		}
	}

	for _, crRelease := range selectedReleases {

		start := f.BaseStart
		end := f.BaseEnd

		// we want our base release validation to match the base release report dates
		if crRelease.Release != f.BaseRelease && crRelease.End != nil && crRelease.Start != nil {
			end = *crRelease.End
			start = *crRelease.Start
		}

		wg.Add(1)
		go func(queryRelease crtype.Release, queryStart, queryEnd time.Time) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				log.Infof("Context canceled while fetching fallback base query status")
				return
			default:
				stats, errs := f.getTestFallbackRelease(ctx, queryRelease.Release, queryStart, queryEnd)
				if len(errs) > 0 {
					log.Errorf("FallbackBaseQueryStatus for %s failed with: %v", queryRelease, errs)
					return
				}

				f.updateTestStatuses(queryRelease, stats.BaseStatus)
			}
		}(*crRelease, start, end)
	}
	wg.Wait()

	return &f.CachedFallbackTestStatuses, nil
}

func (f *fallbackTestQueryReleasesGenerator) updateTestStatuses(release crtype.Release, updateStatuses map[string]crtype.TestStatus) {

	var testStatuses crtype.ReleaseTestMap
	var ok bool
	// since we  can be called for multiple releases and
	// we update the map below we need to block concurrent map writes
	f.lock.Lock()
	defer f.lock.Unlock()
	if testStatuses, ok = f.CachedFallbackTestStatuses.Releases[release.Release]; !ok {
		testStatuses = crtype.ReleaseTestMap{Release: release, Tests: map[string]crtype.TestStatus{}}
		f.CachedFallbackTestStatuses.Releases[release.Release] = testStatuses
	}

	for key, value := range updateStatuses {
		testStatuses.Tests[key] = value
	}
}

type fallbackTestQueryGenerator struct {
	client          *bqcachedclient.Client
	cacheOption     cache.RequestOptions
	commonQuery     string
	groupByQuery    string
	queryParameters []bigquery.QueryParameter
	allJobVariants  crtype.JobVariants
	BaseRelease     string
	BaseStart       time.Time
	BaseEnd         time.Time
}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackRelease(ctx context.Context, release string, start, end time.Time) (crtype.ReportTestStatus, []error) {
	generator := fallbackTestQueryGenerator{
		client: f.client,
		cacheOption: cache.RequestOptions{
			ForceRefresh: f.cacheOption.ForceRefresh,
			// increase the time that base query is cached for since it shouldn't be changing
			CRTimeRoundingFactor: fallbackQueryTimeRoundingOverride,
		},
		commonQuery:     f.commonQuery,
		groupByQuery:    f.groupByQuery,
		BaseRelease:     release,
		BaseStart:       start,
		BaseEnd:         end,
		queryParameters: f.queryParameters,
	}

	testStatuses, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, f.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("FallbackBaseTestStatus~", generator), generator.getTestFallbackRelease, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return crtype.ReportTestStatus{}, errs
	}

	return testStatuses, nil
}

func (f *fallbackTestQueryGenerator) getTestFallbackRelease(ctx context.Context) (crtype.ReportTestStatus, []error) {
	return f.getFallbackBaseQueryStatus(ctx, f.BaseRelease, f.BaseStart, f.BaseEnd)
}

func (f *fallbackTestQueryGenerator) getFallbackBaseQueryStatus(ctx context.Context, release string, start, end time.Time) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	log.Infof("Starting Fallback (%s) QueryTestStatus", release)
	errs := []error{}
	baseString := f.commonQuery + ` AND branch = @BaseRelease`
	baseQuery := f.client.BQ.Query(baseString + f.groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, f.queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: start,
		},
		{
			Name:  "To",
			Value: end,
		},
		{
			Name:  "BaseRelease",
			Value: release,
		},
	}...)

	baseStatus, baseErrs := fetchTestStatus(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Fallback (%s) QueryTestStatus completed in %s with %d base results from db", release, time.Since(before), len(baseStatus))

	return crtype.ReportTestStatus{BaseStatus: baseStatus}, errs
}

func (c *componentReportGenerator) getTestStatusFromBigQuery(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	allJobVariants, errs := GetJobVariantsFromBigQuery(ctx, c.client, c.gcsBucket)
	if len(errs) > 0 {
		log.Errorf("failed to get variants from bigquery")
		return crtype.ReportTestStatus{}, errs
	}

	var baseStatus, sampleStatus map[string]crtype.TestStatus
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}

	if c.IncludeMultiReleaseAnalysis {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				log.Infof("Context canceled while fetching fallback query status")
				return
			default:
				c.getFallbackBaseQueryStatus(ctx, allJobVariants, c.BaseRelease.Release, c.BaseRelease.Start, c.BaseRelease.End)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			baseStatus, baseErrs = c.getBaseQueryStatus(ctx, allJobVariants)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		default:
			sampleStatus, sampleErrs = c.getSampleQueryStatus(ctx, allJobVariants)
		}

	}()
	wg.Wait()
	if len(baseErrs) != 0 || len(sampleErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, sampleErrs...)
	}
	log.Infof("getTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(sampleStatus), len(baseStatus))
	return crtype.ReportTestStatus{BaseStatus: baseStatus, SampleStatus: sampleStatus}, errs
}

var componentAndCapabilityGetter func(test crtype.TestIdentification, stats crtype.TestStatus) (string, []string)

func testToComponentAndCapability(_ crtype.TestIdentification, stats crtype.TestStatus) (string, []string) {
	return stats.Component, stats.Capabilities
}

// getRowColumnIdentifications defines the rows and columns since they are variable. For rows, different pages have different row titles (component, capability etc)
// Columns titles depends on the columnGroupBy parameter user requests. A particular test can belong to multiple rows of different capabilities.
func (c *componentReportGenerator) getRowColumnIdentifications(testIDStr string, stats crtype.TestStatus) ([]crtype.RowIdentification, []crtype.ColumnID, error) {
	var test crtype.TestIdentification
	columnGroupByVariants := c.ColumnGroupBy
	// We show column groups by DBGroupBy only for the last page before test details
	if c.TestID != "" {
		columnGroupByVariants = c.DBGroupBy
	}
	// TODO: is this too slow?
	err := json.Unmarshal([]byte(testIDStr), &test)
	if err != nil {
		return []crtype.RowIdentification{}, []crtype.ColumnID{}, err
	}

	component, capabilities := componentAndCapabilityGetter(test, stats)
	rows := []crtype.RowIdentification{}
	// First Page with no component requested
	if c.Component == "" {
		rows = append(rows, crtype.RowIdentification{Component: component})
	} else if c.Component == component {
		// Exact test match
		if c.TestID != "" {
			row := crtype.RowIdentification{
				Component: component,
				TestID:    test.TestID,
				TestName:  stats.TestName,
				TestSuite: stats.TestSuite,
			}
			if c.Capability != "" {
				row.Capability = c.Capability
			}
			rows = append(rows, row)
		} else {
			for _, capability := range capabilities {
				// Exact capability match only produces one row
				if c.Capability != "" {
					if c.Capability == capability {
						row := crtype.RowIdentification{
							Component:  component,
							TestID:     test.TestID,
							TestName:   stats.TestName,
							TestSuite:  stats.TestSuite,
							Capability: capability,
						}
						rows = append(rows, row)
						break
					}
				} else {
					rows = append(rows, crtype.RowIdentification{Component: component, Capability: capability})
				}
			}
		}
	}
	columns := []crtype.ColumnID{}
	column := crtype.ColumnIdentification{Variants: map[string]string{}}
	for key, value := range test.Variants {
		if columnGroupByVariants.Has(key) {
			column.Variants[key] = value
		}
	}
	columnKeyBytes, err := json.Marshal(column)
	if err != nil {
		return []crtype.RowIdentification{}, []crtype.ColumnID{}, err
	}
	columns = append(columns, crtype.ColumnID(columnKeyBytes))

	return rows, columns, nil
}

func fetchTestStatus(ctx context.Context, query *bigquery.Query) (map[string]crtype.TestStatus, []error) {
	errs := []error{}
	status := map[string]crtype.TestStatus{}
	log.Infof("Fetching test status with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying test status from bigquery")
		errs = append(errs, err)
		return status, errs
	}

	for {
		var row []bigquery.Value

		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing component from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing prowjob from bigquery"))
			continue
		}

		testIDStr, testStatus, err := deserializeRowToTestStatus(row, it.Schema)
		if err != nil {
			err2 := errors.Wrap(err, "error deserializing row from bigquery")
			log.Error(err2.Error())
			errs = append(errs, err2)
			continue
		}

		status[testIDStr] = testStatus
	}
	return status, errs
}

// deserializeRowToTestStatus deserializes a single row into a testID string and matching status.
// This is where we handle the dynamic variant_ columns, parsing these into a map on the test identification.
// Other fixed columns we expect are serialized directly to their appropriate columns.
func deserializeRowToTestStatus(row []bigquery.Value, schema bigquery.Schema) (string, crtype.TestStatus, error) {
	if len(row) != len(schema) {
		log.Infof("row is %+v, schema is %+v", row, schema)
		return "", crtype.TestStatus{}, fmt.Errorf("number of values in row doesn't match schema length")
	}

	// Expect:
	//
	// INFO[2024-04-22T13:31:23.123-03:00] test_id = openshift-tests:75895eeec137789cab3570a252306058
	// INFO[2024-04-22T13:31:23.123-03:00] variants = [standard]
	// INFO[2024-04-22T13:31:23.123-03:00] variant_Network = ovn
	// INFO[2024-04-22T13:31:23.123-03:00] variant_Upgrade = none
	// INFO[2024-04-22T13:31:23.123-03:00] variant_Architecture = amd64
	// INFO[2024-04-22T13:31:23.123-03:00] variant_Platform = gcp
	// INFO[2024-04-22T13:31:23.123-03:00] flat_variants = fips,serial
	// INFO[2024-04-22T13:31:23.123-03:00] variants = [fips serial]
	// INFO[2024-04-22T13:31:23.123-03:00] total_count = %!s(int64=1)
	// INFO[2024-04-22T13:31:23.123-03:00] success_count = %!s(int64=1)
	// INFO[2024-04-22T13:31:23.123-03:00] flake_count = %!s(int64=0)
	// INFO[2024-04-22T13:31:23.124-03:00] component = Cluster Version Operator
	// INFO[2024-04-22T13:31:23.124-03:00] capabilities = [Other]
	// INFO[2024-04-22T13:31:23.124-03:00] jira_component = Cluster Version Operator
	// INFO[2024-04-22T13:31:23.124-03:00] jira_component_id = 12367602000000000/1000000000
	// INFO[2024-04-22T13:31:23.124-03:00] test_name = [sig-storage] [Serial] Volume metrics Ephemeral should create volume metrics in Volume Manager [Suite:openshift/conformance/serial] [Suite:k8s]
	// INFO[2024-04-22T13:31:23.124-03:00] test_suite = openshift-tests
	tid := crtype.TestIdentification{
		Variants: map[string]string{},
	}
	cts := crtype.TestStatus{}
	for i, fieldSchema := range schema {
		col := fieldSchema.Name
		// Some rows we know what to expect, others are dynamic (variants) and go into the map.
		switch {
		case col == "test_id":
			tid.TestID = row[i].(string)
		case col == "test_name":
			cts.TestName = row[i].(string)
		case col == "test_suite":
			cts.TestSuite = row[i].(string)
		case col == "total_count":
			cts.TotalCount = int(row[i].(int64))
		case col == "success_count":
			cts.SuccessCount = int(row[i].(int64))
		case col == "flake_count":
			cts.FlakeCount = int(row[i].(int64))
		case col == "component":
			cts.Component = row[i].(string)
		case col == "capabilities":
			capArr := row[i].([]bigquery.Value)
			cts.Capabilities = make([]string, len(capArr))
			for i := range capArr {
				cts.Capabilities[i] = capArr[i].(string)
			}
		case strings.HasPrefix(col, "variant_"):
			variantName := col[len("variant_"):]
			if row[i] != nil {
				tid.Variants[variantName] = row[i].(string)
			}
		default:
			log.Warnf("ignoring column in query: %s", col)
		}
	}

	// Create a string representation of the test ID so we can use it as a map key throughout:
	// TODO: json better? reversible if we do...
	testIDBytes, err := json.Marshal(tid)

	return string(testIDBytes), cts, err
}

func getMajor(in string) (int, error) {
	major, err := strconv.ParseInt(strings.Split(in, ".")[0], 10, 32)
	if err != nil {
		return 0, err
	}
	return int(major), err
}

func getMinor(in string) (int, error) {
	minor, err := strconv.ParseInt(strings.Split(in, ".")[1], 10, 32)
	if err != nil {
		return 0, err
	}
	return int(minor), err
}

func previousRelease(release string) (string, error) {
	prev := release
	var err error
	var major, minor int
	if major, err = getMajor(release); err == nil {
		if minor, err = getMinor(release); err == nil && minor > 0 {
			prev = fmt.Sprintf("%d.%d", major, minor-1)
		}
	}

	return prev, err
}

func (c *componentReportGenerator) normalizeProwJobName(prowName string) string {
	name := prowName
	if c.BaseRelease.Release != "" {
		name = strings.ReplaceAll(name, c.BaseRelease.Release, "X.X")
		if prev, err := previousRelease(c.BaseRelease.Release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}
	if c.BaseOverrideRelease.Release != "" {
		name = strings.ReplaceAll(name, c.BaseOverrideRelease.Release, "X.X")
		if prev, err := previousRelease(c.BaseOverrideRelease.Release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}
	if c.SampleRelease.Release != "" {
		name = strings.ReplaceAll(name, c.SampleRelease.Release, "X.X")
		if prev, err := previousRelease(c.SampleRelease.Release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}
	// Some jobs encode frequency in their name, which can change
	re := regexp.MustCompile(`-f\d+`)
	name = re.ReplaceAllString(name, "-fXX")

	return name
}

func (c *componentReportGenerator) fetchJobRunTestStatus(ctx context.Context,
	query *bigquery.Query) (map[string][]crtype.
	JobRunTestStatusRow, []error) {
	errs := []error{}
	status := map[string][]crtype.JobRunTestStatusRow{}
	log.Infof("Fetching job run test details with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying job run test status from bigquery")
		errs = append(errs, err)
		return status, errs
	}

	for {
		testStatus := crtype.JobRunTestStatusRow{}
		err := it.Next(&testStatus)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing component from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing prowjob from bigquery"))
			continue
		}
		prowName := c.normalizeProwJobName(testStatus.ProwJob)
		rows, ok := status[prowName]
		if !ok {
			status[prowName] = []crtype.JobRunTestStatusRow{testStatus}
		} else {
			rows = append(rows, testStatus)
			status[prowName] = rows
		}
	}
	return status, errs
}

type cellStatus struct {
	status           crtype.Status
	regressedTests   []crtype.ReportTestSummary
	triagedIncidents []crtype.TriageIncidentSummary
}

func getNewCellStatus(testID crtype.ReportTestIdentification,
	testStats crtype.ReportTestStats,
	existingCellStatus *cellStatus,
	triagedIncidents []crtype.TriagedIncident,
	openRegressions []*crtype.TestRegression) cellStatus {
	var newCellStatus cellStatus
	if existingCellStatus != nil {
		if (testStats.ReportStatus < crtype.NotSignificant && testStats.ReportStatus < existingCellStatus.status) ||
			(existingCellStatus.status == crtype.NotSignificant && testStats.ReportStatus == crtype.SignificantImprovement) {
			// We want to show the significant improvement if assessment is not regression
			newCellStatus.status = testStats.ReportStatus
		} else {
			newCellStatus.status = existingCellStatus.status
		}
		newCellStatus.regressedTests = existingCellStatus.regressedTests
		newCellStatus.triagedIncidents = existingCellStatus.triagedIncidents
	} else {
		newCellStatus.status = testStats.ReportStatus
	}
	// don't show triaged regressions in the regressed tests
	// need a new UI to show active triaged incidents
	if testStats.ReportStatus < crtype.ExtremeTriagedRegression {
		rt := crtype.ReportTestSummary{
			ReportTestIdentification: testID,
			ReportTestStats:          testStats,
		}
		if len(openRegressions) > 0 {
			view := openRegressions[0].View // grab view from first regression, they were queried only for sample release
			or := FindOpenRegression(view, rt.TestID, rt.Variants, openRegressions)
			if or != nil {
				rt.Opened = &or.Opened
			}
		}
		newCellStatus.regressedTests = append(newCellStatus.regressedTests, rt)
	} else if testStats.ReportStatus < crtype.MissingSample {
		ti := crtype.TriageIncidentSummary{
			TriagedIncidents: triagedIncidents,
			ReportTestSummary: crtype.ReportTestSummary{
				ReportTestIdentification: testID,
				ReportTestStats:          testStats,
			}}
		if len(openRegressions) > 0 {
			view := openRegressions[0].View // grab view from first regression, they were queried only for sample release
			or := FindOpenRegression(view, ti.ReportTestSummary.TestID,
				ti.ReportTestSummary.Variants, openRegressions)
			if or != nil {
				ti.ReportTestSummary.Opened = &or.Opened
			}
		}
		newCellStatus.triagedIncidents = append(newCellStatus.triagedIncidents, ti)
	}
	return newCellStatus
}

func updateCellStatus(rowIdentifications []crtype.RowIdentification,
	columnIdentifications []crtype.ColumnID,
	testID crtype.ReportTestIdentification,
	testStats crtype.ReportTestStats,
	status map[crtype.RowIdentification]map[crtype.ColumnID]cellStatus,
	allRows map[crtype.RowIdentification]struct{},
	allColumns map[crtype.ColumnID]struct{},
	triagedIncidents []crtype.TriagedIncident,
	openRegressions []*crtype.TestRegression) {
	for _, columnIdentification := range columnIdentifications {
		if _, ok := allColumns[columnIdentification]; !ok {
			allColumns[columnIdentification] = struct{}{}
		}
	}

	for _, rowIdentification := range rowIdentifications {
		// Each test might have multiple Capabilities. Initial ID just pick the first on
		// the list. If we are on a page with specific capability, this needs to be rewritten.
		if rowIdentification.Capability != "" {
			testID.Capability = rowIdentification.Capability
		}
		if _, ok := allRows[rowIdentification]; !ok {
			allRows[rowIdentification] = struct{}{}
		}
		row, ok := status[rowIdentification]
		if !ok {
			row = map[crtype.ColumnID]cellStatus{}
			for _, columnIdentification := range columnIdentifications {
				row[columnIdentification] = getNewCellStatus(testID, testStats, nil, triagedIncidents, openRegressions)
				status[rowIdentification] = row
			}
		} else {
			for _, columnIdentification := range columnIdentifications {
				existing, ok := row[columnIdentification]
				if !ok {
					row[columnIdentification] = getNewCellStatus(testID, testStats, nil, triagedIncidents, openRegressions)
				} else {
					row[columnIdentification] = getNewCellStatus(testID, testStats, &existing, triagedIncidents, openRegressions)
				}
			}
		}
	}
}

func (c *componentReportGenerator) getTriagedIssuesFromBigQuery(ctx context.Context,
	testID crtype.ReportTestIdentification) (
	int, []crtype.TriagedIncident, []error) {
	generator := triagedIncidentsGenerator{
		ReportModified: c.GetLastReportModifiedTime(ctx, c.client, c.cacheOption),
		client:         c.client,
		cacheOption:    c.cacheOption,
		SampleRelease:  c.SampleRelease,
	}

	// we want to fetch this once per generator instance which should be once per UI load
	// this is the full list from the cache if available that will be subset to specific test
	// in triagedIssuesFor
	if c.triagedIssues == nil {
		releaseTriagedIncidents, errs := api.GetDataFromCacheOrGenerate[resolvedissues.TriagedIncidentsForRelease](
			ctx, generator.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("TriagedIncidents~", generator),
			generator.generateTriagedIssuesFor, resolvedissues.TriagedIncidentsForRelease{})

		if len(errs) > 0 {
			return 0, nil, errs
		}
		c.triagedIssues = &releaseTriagedIncidents
	}
	impactedRuns, triagedIncidents := triagedIssuesFor(c.triagedIssues, testID.ColumnIdentification, testID.TestID, c.SampleRelease.Start, c.SampleRelease.End)

	return impactedRuns, triagedIncidents, nil
}

func (c *componentReportGenerator) GetLastReportModifiedTime(ctx context.Context, client *bqcachedclient.Client,
	options cache.RequestOptions) *time.Time {

	if c.ReportModified == nil {

		// check each component of the report that may change asynchronously and require refreshing the report
		// return the most recent time

		// cache only for 5 minutes
		lastModifiedTimeCacheDuration := 5 * time.Minute
		now := time.Now().UTC()
		// Only cache until the next rounding duration
		cacheDuration := now.Truncate(lastModifiedTimeCacheDuration).Add(lastModifiedTimeCacheDuration).Sub(now)

		// default our last modified time to within the last 12 hours
		// any newer modifications will be picked up
		initLastModifiedTime := time.Now().UTC().Truncate(12 * time.Hour)

		generator := triagedIncidentsModifiedTimeGenerator{
			client: client,
			cacheOption: cache.RequestOptions{
				ForceRefresh:         options.ForceRefresh,
				CRTimeRoundingFactor: cacheDuration,
			},
			LastModifiedStartTime: &initLastModifiedTime,
		}

		// this gets called a lot, so we want to set it once on the componentReportGenerator
		lastModifiedTime, errs := api.GetDataFromCacheOrGenerate[*time.Time](ctx, generator.client.Cache,
			generator.cacheOption, api.GetPrefixedCacheKey("TriageLastModified~", generator), generator.generateTriagedIssuesLastModifiedTime, generator.LastModifiedStartTime)

		if len(errs) > 0 {
			c.ReportModified = generator.LastModifiedStartTime
		}

		c.ReportModified = lastModifiedTime
	}

	return c.ReportModified
}

type triagedIncidentsModifiedTimeGenerator struct {
	client                *bqcachedclient.Client
	cacheOption           cache.RequestOptions
	LastModifiedStartTime *time.Time
}

func (t *triagedIncidentsModifiedTimeGenerator) generateTriagedIssuesLastModifiedTime(ctx context.Context) (*time.Time,
	[]error) {
	before := time.Now()
	lastModifiedTime, errs := t.queryTriagedIssuesLastModified(ctx)

	log.Infof("generateTriagedIssuesLastModifiedTime query completed in %s ", time.Since(before))

	if errs != nil {
		return nil, errs
	}

	return lastModifiedTime, nil
}

func (t *triagedIncidentsModifiedTimeGenerator) queryTriagedIssuesLastModified(ctx context.Context) (*time.Time, []error) {
	// Look for the most recent modified time after our lastModifiedTime.
	// Using the partition to limit the query, we don't need the actual most recent modified time just need to know
	// if it has changed / is greater than our default
	queryString := fmt.Sprintf("SELECT max(modified_time) as LastModification FROM %s.%s WHERE modified_time > TIMESTAMP(@Last)", t.client.Dataset, triagedIncidentsTableID)

	params := make([]bigquery.QueryParameter, 0)

	params = append(params, []bigquery.QueryParameter{
		{
			Name:  "Last",
			Value: *t.LastModifiedStartTime,
		},
	}...)

	sampleQuery := t.client.BQ.Query(queryString)
	sampleQuery.Parameters = append(sampleQuery.Parameters, params...)

	return t.fetchLastModified(ctx, sampleQuery)
}

func (t *triagedIncidentsModifiedTimeGenerator) fetchLastModified(ctx context.Context,
	query *bigquery.Query) (*time.Time,
	[]error) {
	log.Infof("Fetching triaged incidents last modified time with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying triaged incidents last modified time from bigquery")
		return nil, []error{err}
	}

	lastModification := t.LastModifiedStartTime
	var triagedIncidentModifiedTime struct {
		LastModification bigquery.NullTimestamp
	}
	err = it.Next(&triagedIncidentModifiedTime)
	if err != nil && err != iterator.Done {
		log.WithError(err).Error("error parsing triaged incident last modification time from bigquery")
		return nil, []error{err}
	}
	if triagedIncidentModifiedTime.LastModification.Valid {
		lastModification = &triagedIncidentModifiedTime.LastModification.Timestamp
	}

	return lastModification, nil
}

type triagedIncidentsGenerator struct {
	ReportModified *time.Time
	client         *bqcachedclient.Client
	cacheOption    cache.RequestOptions
	SampleRelease  crtype.RequestReleaseOptions
}

func (t *triagedIncidentsGenerator) generateTriagedIssuesFor(ctx context.Context) (resolvedissues.
	TriagedIncidentsForRelease,
	[]error) {
	before := time.Now()
	incidents, errs := t.queryTriagedIssues(ctx)

	log.Infof("generateTriagedIssuesFor query completed in %s with %d incidents from db", time.Since(before), len(incidents))

	if len(errs) > 0 {
		return resolvedissues.TriagedIncidentsForRelease{}, errs
	}

	triagedIncidents := resolvedissues.NewTriagedIncidentsForRelease(resolvedissues.Release(t.SampleRelease.Release))

	for _, incident := range incidents {
		k := resolvedissues.KeyForTriagedIssue(incident.TestID, incident.Variants)
		triagedIncidents.TriagedIncidents[k] = append(triagedIncidents.TriagedIncidents[k], incident)
	}

	log.Infof("generateTriagedIssuesFor completed in %s with %d incidents from db", time.Since(before), len(incidents))

	return triagedIncidents, nil
}

func triagedIssuesFor(releaseIncidents *resolvedissues.TriagedIncidentsForRelease, variant crtype.ColumnIdentification, testID string, startTime, endTime time.Time) (int, []crtype.TriagedIncident) {
	if releaseIncidents == nil {
		return 0, nil
	}

	inKey := resolvedissues.KeyForTriagedIssue(testID, resolvedissues.TransformVariant(variant))

	triagedIncidents := releaseIncidents.TriagedIncidents[inKey]
	relevantIncidents := []crtype.TriagedIncident{}

	impactedJobRuns := sets.NewString() // because multiple issues could impact the same job run, be sure to count each job run only once
	numJobRunsToSuppress := 0
	for _, triagedIncident := range triagedIncidents {
		startNumRunsSuppressed := numJobRunsToSuppress
		for _, impactedJobRun := range triagedIncident.JobRuns {
			if impactedJobRuns.Has(impactedJobRun.URL) {
				continue
			}
			impactedJobRuns.Insert(impactedJobRun.URL)

			compareTime := impactedJobRun.StartTime
			// preference is to compare to CompletedTime as it will more closely match jobrun modified time
			// but, it is optional so default to StartTime and set to CompletedTime when present
			if impactedJobRun.CompletedTime.Valid {
				compareTime = impactedJobRun.CompletedTime.Timestamp
			}

			if compareTime.After(startTime) && compareTime.Before(endTime) {
				numJobRunsToSuppress++
			}
		}

		if numJobRunsToSuppress > startNumRunsSuppressed {
			relevantIncidents = append(relevantIncidents, triagedIncident)
		}
	}

	// if we didn't have any jobs that matched the compare time then return nil
	if numJobRunsToSuppress == 0 {
		relevantIncidents = nil
	}

	return numJobRunsToSuppress, relevantIncidents
}

func (t *triagedIncidentsGenerator) queryTriagedIssues(ctx context.Context) ([]crtype.TriagedIncident, []error) {
	// Look for issue.start_date < TIMESTAMP(@TO) AND
	// (issue.resolution_date IS NULL OR issue.resolution_date >= TIMESTAMP(@FROM))
	// we could add a range for modified_time if we want to leverage the partitions
	// presume modification would be within 6 months of start / end
	// shouldn't be so many of these that would query too much data
	queryString := fmt.Sprintf("SELECT * FROM %s.%s WHERE release = @SampleRelease AND issue.start_date <= TIMESTAMP(@TO) AND (issue.resolution_date IS NULL OR issue.resolution_date >= TIMESTAMP(@FROM))", t.client.Dataset, triagedIncidentsTableID)

	params := make([]bigquery.QueryParameter, 0)

	params = append(params, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: t.SampleRelease.Start,
		},
		{
			Name:  "To",
			Value: t.SampleRelease.End,
		},
		{
			Name:  "SampleRelease",
			Value: t.SampleRelease.Release,
		},
	}...)

	sampleQuery := t.client.BQ.Query(queryString)
	sampleQuery.Parameters = append(sampleQuery.Parameters, params...)

	return t.fetchTriagedIssues(ctx, sampleQuery)
}

func (t *triagedIncidentsGenerator) fetchTriagedIssues(ctx context.Context,
	query *bigquery.Query) ([]crtype.TriagedIncident,
	[]error) {
	errs := make([]error, 0)
	incidents := make([]crtype.TriagedIncident, 0)
	log.Infof("Fetching triaged incidents with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying triaged incidents from bigquery")
		errs = append(errs, err)
		return incidents, errs
	}

	for {
		var triagedIncident crtype.TriagedIncident
		err := it.Next(&triagedIncident)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing triaged incident from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing triaged incident from bigquery"))
			continue
		}
		incidents = append(incidents, triagedIncident)
	}
	return incidents, errs
}

func (c *componentReportGenerator) triagedIncidentsFor(ctx context.Context,
	testID crtype.ReportTestIdentification) (int,
	[]crtype.TriagedIncident) {
	// handle test case / missing client
	if c.client == nil {
		return 0, nil
	}

	impactedRuns, triagedIncidents, errs := c.getTriagedIssuesFromBigQuery(ctx, testID)

	if errs != nil {
		for _, err := range errs {
			log.WithError(err).Error("error getting triaged issues component from bigquery")
		}
		return 0, nil
	}

	return impactedRuns, triagedIncidents
}

// getRequiredConfidence returns the required certainty of a regression before we include it in the report as a
// regressed test. This is to introduce some hysteresis into the process so once a regression creeps over the 95%
// confidence we typically use, dropping to 94.9% should not make the cell immediately green.
//
// Instead, once you cross the confidence threshold and a regression begins tracking in the openRegressions list,
// we'll require less confidence for that test until the regression is closed. (-5%) Once the certainty drops below that
// modified confidence, the regression will be closed and the -5% adjuster is gone.
//
// ie. if the request was for 95% confidence, but we see that a test has an open regression (meaning at some point recently
// we were over 95% certain of a regression), we're going to only require 90% certainty to mark that test red.
func (c *componentReportGenerator) getRequiredConfidence(testID string, variants map[string]string) int {
	if len(c.openRegressions) > 0 {
		view := c.openRegressions[0].View // grab view from first regression, they were queried only for sample release
		or := FindOpenRegression(view, testID, variants, c.openRegressions)
		if or != nil {
			log.Debugf("adjusting required regression confidence from %d to %d because %s (%v) has an open regression since %s",
				c.RequestAdvancedOptions.Confidence,
				c.RequestAdvancedOptions.Confidence-openRegressionConfidenceAdjustment,
				testID,
				variants,
				or.Opened)
			return c.RequestAdvancedOptions.Confidence /*- openRegressionConfidenceAdjustment*/
		}
	}
	return c.RequestAdvancedOptions.Confidence
}

// matchBaseRegression returns a testStatus that reflects the allowances specified
// in an intentional regression that accepted a lower threshold but maintains the higher
// threshold when used as a basis.  It will ignore intentional regressions if we are relying
// on fallback to find the highest threshold.  It will return the original testStatus if there
// is no intentional regression or the testStatus has a higher threshold
func (c *componentReportGenerator) matchBaseRegression(testID crtype.ReportTestIdentification, baseRelease string, baseStats crtype.TestStatus) (crtype.TestStatus, string) {
	var baseRegression *regressionallowances.IntentionalRegression
	if !c.IncludeMultiReleaseAnalysis && len(c.VariantCrossCompare) == 0 {
		// only really makes sense when not cross-comparing variants:
		// look for corresponding regressions we can account for in the analysis
		// only if we are ignoring fallback, otherwise we will let fallback determine the threshold
		baseRegression = regressionallowances.IntentionalRegressionFor(baseRelease, testID.ColumnIdentification, testID.TestID)

		// This could go away if we remove the option for ignoring fallback
		if baseRegression != nil && baseRegression.PreviousPassPercentage() > getTestStatusSuccessRate(baseStats) {
			// override with  the basis regression previous values
			// testStats will reflect the expected threshold, not the computed values from the release with the allowed regression
			baseRegressionPreviousRelease, err := previousRelease(c.BaseRelease.Release)
			if err != nil {
				log.WithError(err).Error("Failed to determine the previous release for baseRegression")
			} else {
				// create a clone since we might be updating a cached item though the same regression would likely apply each time...
				updatedStats := crtype.TestStatus{TestName: baseStats.TestName, TestSuite: baseStats.TestSuite, Capabilities: baseStats.Capabilities,
					Component: baseStats.Component, Variants: baseStats.Variants,
					FlakeCount:   baseRegression.PreviousFlakes,
					SuccessCount: baseRegression.PreviousSuccesses,
					TotalCount:   baseRegression.PreviousFailures + baseRegression.PreviousFlakes + baseRegression.PreviousSuccesses,
				}
				baseStats = updatedStats
				baseRelease = baseRegressionPreviousRelease
				log.Infof("BaseRegression - PreviousPassPercentage overrides baseStats.  Release: %s, Successes: %d, Flakes: %d, Total: %d", baseRelease, baseStats.SuccessCount, baseStats.FlakeCount, baseStats.TotalCount)
			}
		}
	}

	return baseStats, baseRelease
}

// matchBestBaseStats returns the testStatus, release and reportTestStatus
// that has the highest threshold across the basis release and previous releases included
// in fallback comparison
func (c *componentReportGenerator) matchBestBaseStats(testID crtype.ReportTestIdentification, testIdentification, baseRelease string, baseStats, sampleStats crtype.TestStatus, requiredConfidence int, approvedRegression *regressionallowances.IntentionalRegression, numberOfIgnoredSampleJobRuns int) (crtype.TestStatus, string, crtype.ReportTestStats) {

	// The hope is that this goes away
	// once we agree we don't need to honor a higher intentional regression pass percentage
	baseStats, baseRelease = c.matchBaseRegression(testID, baseRelease, baseStats)
	baseStatsTotal := baseStats.TotalCount

	baseTestStats := c.assessComponentStatus(requiredConfidence, sampleStats.TotalCount, sampleStats.SuccessCount,
		sampleStats.FlakeCount, baseStats.TotalCount, baseStats.SuccessCount,
		baseStats.FlakeCount, approvedRegression, numberOfIgnoredSampleJobRuns, baseRelease, nil, nil)

	if !c.IncludeMultiReleaseAnalysis {
		return baseStats, baseRelease, baseTestStats
	}

	if c.cachedFallbackTestStatuses == nil {
		log.Errorf("Invalid fallback test statuses")
		return baseStats, baseRelease, baseTestStats
	}

	var priorRelease = baseRelease
	var err error
	for err == nil {
		var cachedTestStatuses crtype.ReleaseTestMap
		var cTestStats crtype.TestStatus
		ok := false
		priorRelease, err = previousRelease(priorRelease)
		// if we fail to determine the previous release then stop
		if err != nil {
			return baseStats, baseRelease, baseTestStats
		}
		// if we hit a missing release then stop
		if cachedTestStatuses, ok = c.cachedFallbackTestStatuses.Releases[priorRelease]; !ok {
			return baseStats, baseRelease, baseTestStats
		}
		// it's ok if we don't have a testIdentification for this release
		// we likely won't have it for earlier releases either, but we can keep going
		if cTestStats, ok = cachedTestStatuses.Tests[testIdentification]; ok {

			// what is our base total compared to the original base
			// this happens when jobs shift like sdn -> ovn
			// if we get below threshold that's a sign we are reducing our base signal
			if float64(cTestStats.TotalCount)/float64(baseStatsTotal) < .6 {
				log.Debugf("Fallback base total: %d to low for fallback analysis compared to original: %d", cTestStats.TotalCount, baseStatsTotal)
				return baseStats, baseRelease, baseTestStats
			}

			cTestStats, priorRelease = c.matchBaseRegression(testID, priorRelease, cTestStats)

			priorTestStats := c.assessComponentStatus(requiredConfidence, sampleStats.TotalCount, sampleStats.SuccessCount,
				sampleStats.FlakeCount, cTestStats.TotalCount, cTestStats.SuccessCount,
				cTestStats.FlakeCount, approvedRegression, numberOfIgnoredSampleJobRuns, priorRelease, cachedTestStatuses.Start, cachedTestStatuses.End)

			if priorTestStats.ReportStatus < baseTestStats.ReportStatus {
				baseStats = cTestStats
				baseTestStats = priorTestStats
				baseRelease = priorRelease
			}
		}
	}

	return baseStats, baseRelease, baseTestStats
}

// TODO: break this function down and remove this nolint
// nolint:gocyclo
func (c *componentReportGenerator) generateComponentTestReport(ctx context.Context,
	baseStatus map[string]crtype.TestStatus,
	sampleStatus map[string]crtype.TestStatus) (crtype.ComponentReport, error) {
	report := crtype.ComponentReport{
		Rows: []crtype.ReportRow{},
	}

	// aggregatedStatus is the aggregated status based on the requested rows and columns
	aggregatedStatus := map[crtype.RowIdentification]map[crtype.ColumnID]cellStatus{}
	// allRows and allColumns are used to make sure rows are ordered and all rows have the same columns in the same order
	allRows := map[crtype.RowIdentification]struct{}{}
	allColumns := map[crtype.ColumnID]struct{}{}
	// testID is used to identify the most regressed test. With this, we can
	// create a shortcut link from any page to go straight to the most regressed test page.

	// using the baseStatus range here makes it hard to do away with the baseQuery
	// but if we did and just enumerated the sampleStatus instead
	// we wouldn't need the base query each time
	//
	// understand we use this to find tests associated with base that we don't see now in sample
	// meaning they have been renamed or removed
	baseReleaseMatches := 0
	baseReleaseMisses := 0
	overriddenBaseMatches := 0

	for testIdentification, baseStats := range baseStatus {
		testID, err := buildTestID(baseStats, testIdentification)
		if err != nil {
			return crtype.ComponentReport{}, err
		}

		var testStats crtype.ReportTestStats
		var triagedIncidents []crtype.TriagedIncident
		var resolvedIssueCompensation int // triaged job run failures to ignore
		sampleStats, ok := sampleStatus[testIdentification]
		if !ok {
			testStats.ReportStatus = crtype.MissingSample
		} else {
			// requiredConfidence is lowered for on-going regressions to prevent cells from flapping:
			requiredConfidence := c.getRequiredConfidence(testID.TestID, testID.Variants)
			var approvedRegression *regressionallowances.IntentionalRegression
			if len(c.VariantCrossCompare) == 0 { // only really makes sense when not cross-comparing variants:
				// look for corresponding regressions we can account for in the analysis
				approvedRegression = regressionallowances.IntentionalRegressionFor(c.SampleRelease.Release, testID.ColumnIdentification, testID.TestID)
				// ignore triage if we have an intentional regression
				if approvedRegression == nil {
					resolvedIssueCompensation, triagedIncidents = c.triagedIncidentsFor(ctx, testID)
				}
			}

			// this is where we look to see if a previous release has a higher pass rate
			matchedBaseRelease := c.BaseRelease.Release
			baseStats, matchedBaseRelease, testStats = c.matchBestBaseStats(testID, testIdentification, matchedBaseRelease, baseStats, sampleStats, requiredConfidence, approvedRegression, resolvedIssueCompensation)

			if matchedBaseRelease != c.BaseRelease.Release {
				log.Infof("Overrode base stats using release %s for Test: %s - %s", matchedBaseRelease, baseStats.TestName, testIdentification)
				overriddenBaseMatches++
			}

			if testStats.IsTriaged() {
				// we are within the triage range
				// do we want to show the triage icon or flip reportStatus
				canClearReportStatus := true
				for _, ti := range triagedIncidents {
					if ti.Issue.Type != string(resolvedissues.TriageIssueTypeInfrastructure) {
						// if a non Infrastructure regression isn't marked resolved or the resolution date is after the end of our sample query
						// then we won't clear it.  Otherwise, we can.
						if !ti.Issue.ResolutionDate.Valid || ti.Issue.ResolutionDate.Timestamp.After(c.SampleRelease.End) {
							canClearReportStatus = false
						}
					}
				}

				// sanity check to make sure we aren't just defaulting to clear without any incidents (not likely)
				if len(triagedIncidents) > 0 && canClearReportStatus {
					testStats.ReportStatus = crtype.NotSignificant
				}
			}
		}
		delete(sampleStatus, testIdentification)

		rowIdentifications, columnIdentifications, err := c.getRowColumnIdentifications(testIdentification, baseStats)
		if err != nil {
			return crtype.ComponentReport{}, err
		}
		updateCellStatus(rowIdentifications, columnIdentifications, testID, testStats, aggregatedStatus, allRows, allColumns, triagedIncidents, c.openRegressions)
	}

	log.Infof("BaseStats: %d, baseMatches: %d, baseMisses: %d.  Overridden base stats: %d", len(baseStatus), baseReleaseMatches, baseReleaseMisses, overriddenBaseMatches)

	// Anything we saw in the basis was removed above, all that remains are tests with no basis, typically new
	// tests, or tests that were renamed without submitting a rename to the test mapping repo.
	for testIdentification, sampleStats := range sampleStatus {
		testID, err := buildTestID(sampleStats, testIdentification)
		if err != nil {
			return crtype.ComponentReport{}, err
		}

		// Check for approved regressions, and triaged incidents, which may adjust our counts and pass rate:
		var testStats crtype.ReportTestStats
		var triagedIncidents []crtype.TriagedIncident
		var resolvedIssueCompensation int // triaged job run failures to ignore
		// look for corresponding regressions we can account for in the analysis
		approvedRegression := regressionallowances.IntentionalRegressionFor(c.SampleRelease.Release, testID.ColumnIdentification, testID.TestID)
		// ignore triage if we have an intentional regression
		if approvedRegression == nil {
			resolvedIssueCompensation, triagedIncidents = c.triagedIncidentsFor(ctx, testID)
		}

		requiredConfidence := 0 // irrelevant for pass rate comparison
		testStats = c.assessComponentStatus(requiredConfidence, sampleStats.TotalCount, sampleStats.SuccessCount,
			sampleStats.FlakeCount, 0, 0, 0, // pass 0s for base stats
			approvedRegression, resolvedIssueCompensation, "", nil, nil)

		if testStats.IsTriaged() {
			// we are within the triage range
			// do we want to show the triage icon or flip reportStatus
			canClearReportStatus := true
			for _, ti := range triagedIncidents {
				if ti.Issue.Type != string(resolvedissues.TriageIssueTypeInfrastructure) {
					// if a non Infrastructure regression isn't marked resolved or the resolution date is after the end of our sample query
					// then we won't clear it.  Otherwise, we can.
					if !ti.Issue.ResolutionDate.Valid || ti.Issue.ResolutionDate.Timestamp.After(c.SampleRelease.End) {
						canClearReportStatus = false
					}
				}
			}

			// sanity check to make sure we aren't just defaulting to clear without any incidents (not likely)
			if len(triagedIncidents) > 0 && canClearReportStatus {
				testStats.ReportStatus = crtype.NotSignificant
			}
		}

		rowIdentifications, columnIdentification, err := c.getRowColumnIdentifications(testIdentification, sampleStats)
		if err != nil {
			return crtype.ComponentReport{}, err
		}
		updateCellStatus(rowIdentifications, columnIdentification, testID, testStats, aggregatedStatus, allRows, allColumns, nil, c.openRegressions)
	}

	// Sort the row identifications
	sortedRows := []crtype.RowIdentification{}
	for rowID := range allRows {
		sortedRows = append(sortedRows, rowID)
	}
	sort.Slice(sortedRows, func(i, j int) bool {
		less := sortedRows[i].Component < sortedRows[j].Component
		if sortedRows[i].Component == sortedRows[j].Component {
			less = sortedRows[i].Capability < sortedRows[j].Capability
			if sortedRows[i].Capability == sortedRows[j].Capability {
				less = sortedRows[i].TestName < sortedRows[j].TestName
				if sortedRows[i].TestName == sortedRows[j].TestName {
					less = sortedRows[i].TestID < sortedRows[j].TestID
				}
			}
		}
		return less
	})

	// Sort the column identifications
	sortedColumns := []crtype.ColumnID{}
	for columnID := range allColumns {
		sortedColumns = append(sortedColumns, columnID)
	}
	sort.Slice(sortedColumns, func(i, j int) bool {
		return sortedColumns[i] < sortedColumns[j]
	})

	rows, err := buildReport(sortedRows, sortedColumns, aggregatedStatus)
	if err != nil {
		return crtype.ComponentReport{}, err
	}
	report.Rows = rows
	return report, nil
}

func buildReport(sortedRows []crtype.RowIdentification, sortedColumns []crtype.ColumnID, aggregatedStatus map[crtype.RowIdentification]map[crtype.ColumnID]cellStatus) ([]crtype.ReportRow, error) {
	// Now build the report
	var regressionRows, goodRows []crtype.ReportRow
	for _, rowID := range sortedRows {
		columns, ok := aggregatedStatus[rowID]
		if !ok {
			continue
		}
		reportRow := crtype.ReportRow{RowIdentification: rowID}
		hasRegression := false
		for _, columnID := range sortedColumns {
			if reportRow.Columns == nil {
				reportRow.Columns = []crtype.ReportColumn{}
			}
			var colIDStruct crtype.ColumnIdentification
			err := json.Unmarshal([]byte(columnID), &colIDStruct)
			if err != nil {
				return nil, err
			}
			reportColumn := crtype.ReportColumn{ColumnIdentification: colIDStruct}
			status, ok := columns[columnID]
			if !ok {
				reportColumn.Status = crtype.MissingBasisAndSample
			} else {
				reportColumn.Status = status.status
				reportColumn.RegressedTests = status.regressedTests
				sort.Slice(reportColumn.RegressedTests, func(i, j int) bool {
					return reportColumn.RegressedTests[i].ReportStatus < reportColumn.RegressedTests[j].ReportStatus
				})
				reportColumn.TriagedIncidents = status.triagedIncidents
				sort.Slice(reportColumn.TriagedIncidents, func(i, j int) bool {
					return reportColumn.TriagedIncidents[i].ReportStatus < reportColumn.TriagedIncidents[j].ReportStatus
				})
			}
			reportRow.Columns = append(reportRow.Columns, reportColumn)
			if reportColumn.Status <= crtype.SignificantTriagedRegression {
				hasRegression = true
			}
		}
		// Any rows with regression should appear first, so make two slices
		// and assemble them later.
		if hasRegression {
			regressionRows = append(regressionRows, reportRow)
		} else {
			goodRows = append(goodRows, reportRow)
		}
	}

	regressionRows = append(regressionRows, goodRows...)
	return regressionRows, nil
}

func buildTestID(stats crtype.TestStatus, testIdentificationStr string) (crtype.ReportTestIdentification, error) {
	// TODO: function needs a rename, there's a lot of references to test ID/identification around.
	var testIdentification crtype.TestIdentification
	// TODO: is this too slow?
	err := json.Unmarshal([]byte(testIdentificationStr), &testIdentification)
	if err != nil {
		log.WithError(err).Errorf("trying to unmarshel %s", testIdentificationStr)
		return crtype.ReportTestIdentification{}, err
	}
	testID := crtype.ReportTestIdentification{
		RowIdentification: crtype.RowIdentification{
			Component: stats.Component,
			TestName:  stats.TestName,
			TestSuite: stats.TestSuite,
			TestID:    testIdentification.TestID,
		},
		ColumnIdentification: crtype.ColumnIdentification{
			Variants: testIdentification.Variants,
		},
	}
	// Take the first cap for now. When we reach to a cell with specific capability, we will override the value.
	if len(stats.Capabilities) > 0 {
		testID.Capability = stats.Capabilities[0]
	}
	return testID, nil
}

func getFailureCount(status crtype.JobRunTestStatusRow) int {
	failure := status.TotalCount - status.SuccessCount - status.FlakeCount
	if failure < 0 {
		failure = 0
	}
	return failure
}

func getTestStatusSuccessRate(testStatus crtype.TestStatus) float64 {
	return getSuccessRate(testStatus.SuccessCount, testStatus.TotalCount-testStatus.SuccessCount-testStatus.FlakeCount, testStatus.FlakeCount)
}

func getSuccessRate(success, failure, flake int) float64 {
	total := success + failure + flake
	if total == 0 {
		return 0.0
	}
	return float64(success+flake) / float64(total)
}

func getRegressionStatus(basisPassPercentage, samplePassPercentage float64, isTriage bool) crtype.Status {
	if (basisPassPercentage - samplePassPercentage) > 0.15 {
		if isTriage {
			return crtype.ExtremeTriagedRegression
		}
		return crtype.ExtremeRegression
	}

	if isTriage {
		return crtype.SignificantTriagedRegression
	}
	return crtype.SignificantRegression
}

func (c *componentReportGenerator) getEffectivePityFactor(basisPassPercentage float64, approvedRegression *regressionallowances.IntentionalRegression) int {
	if approvedRegression != nil && approvedRegression.RegressedFailures > 0 {
		regressedPassPercentage := approvedRegression.RegressedPassPercentage()
		if regressedPassPercentage < basisPassPercentage {
			// product owner chose a required pass percentage, so we allow pity to cover that approved pass percent
			// plus the existing pity factor to limit, "well, it's just *barely* lower" arguments.
			effectivePityFactor := int(basisPassPercentage*100) - int(regressedPassPercentage*100) + c.PityFactor

			if effectivePityFactor < c.PityFactor {
				log.Errorf("effective pity factor for %+v is below zero: %d", approvedRegression, effectivePityFactor)
				effectivePityFactor = c.PityFactor
			}

			return effectivePityFactor
		}
	}
	return c.PityFactor
}

func (c *componentReportGenerator) assessComponentStatus(
	requiredConfidence,
	sampleTotal,
	sampleSuccess,
	sampleFlake,
	baseTotal,
	baseSuccess,
	baseFlake int,
	approvedRegression *regressionallowances.IntentionalRegression,
	numberOfIgnoredSampleJobRuns int,
	baseRelease string,
	baseStart,
	baseEnd *time.Time) crtype.ReportTestStats {

	// if we don't have a valid set of start and end dates we default to the baseRelease values
	if baseStart == nil || baseEnd == nil {
		baseStart = &c.BaseRelease.Start
		baseEnd = &c.BaseRelease.End
	}
	// preserve the initial sampleTotal, so we can check
	// to see if numberOfIgnoredSampleJobRuns impacts the status
	initialSampleTotal := sampleTotal
	adjustedSampleTotal := sampleTotal - numberOfIgnoredSampleJobRuns
	if adjustedSampleTotal < sampleSuccess {
		log.Errorf("adjustedSampleTotal is too small: sampleTotal=%d, numberOfIgnoredSampleJobRuns=%d, sampleSuccess=%d", sampleTotal, numberOfIgnoredSampleJobRuns, sampleSuccess)
	} else {
		sampleTotal = adjustedSampleTotal
	}

	sampleFailure := sampleTotal - sampleSuccess - sampleFlake
	// The adjusted total for ignored runs can push failure count into the negatives if there were
	// more ignored runs than actual failures. (or no failures at all)
	if sampleFailure < 0 {
		sampleFailure = 0
	}
	baseFailure := baseTotal - baseSuccess - baseFlake

	if baseTotal == 0 && c.RequestAdvancedOptions.PassRateRequiredNewTests > 0 {
		// If we have no base stats, fall back to a raw pass rate comparison for new or improperly renamed tests:
		testStats := c.buildPassRateTestStats(sampleSuccess, sampleFailure, sampleFlake,
			float64(c.RequestAdvancedOptions.PassRateRequiredNewTests))
		// If a new test reports no regression, and we're not using pass rate mode for all tests, we alter
		// status to be missing basis for the pre-existing Fisher Exact behavior:
		if testStats.ReportStatus == crtype.NotSignificant && c.RequestAdvancedOptions.PassRateRequiredAllTests == 0 {
			testStats.ReportStatus = crtype.MissingBasis
		}
		return testStats
	} else if c.RequestAdvancedOptions.PassRateRequiredAllTests > 0 {
		// If requested, switch to pass rate only testing to see what does not meet the criteria:
		testStats := c.buildPassRateTestStats(sampleSuccess, sampleFailure, sampleFlake,
			float64(c.RequestAdvancedOptions.PassRateRequiredAllTests))
		// include base stats even though we didn't do fishers exact here, this is helpful
		// for the test details page to give a visual on how the test behaved in the basis
		testStats.BaseStats = &crtype.TestDetailsReleaseStats{
			Release: baseRelease,
			Start:   baseStart,
			End:     baseEnd,
			TestDetailsTestStats: crtype.TestDetailsTestStats{
				SuccessRate:  getSuccessRate(baseSuccess, baseFailure, baseFlake),
				SuccessCount: baseSuccess,
				FailureCount: baseFailure,
				FlakeCount:   baseFlake,
			},
		}

		return testStats
	}

	// Otherwise we fall back to default behavior of Fishers Exact test:
	testStats := c.buildFisherExactTestStats(requiredConfidence, sampleTotal, sampleSuccess, sampleFlake, sampleFailure, baseTotal, baseSuccess, baseFlake, baseFailure, approvedRegression, initialSampleTotal, baseRelease, baseStart, baseEnd)
	return testStats
}

func (c *componentReportGenerator) buildFisherExactTestStats(requiredConfidence, sampleTotal, sampleSuccess, sampleFlake, sampleFailure, baseTotal, baseSuccess, baseFlake, baseFailure int, approvedRegression *regressionallowances.IntentionalRegression, initialSampleTotal int, baseRelease string, baseStart, baseEnd *time.Time) crtype.ReportTestStats {

	fisherExact := 0.0
	baseStats := &crtype.TestDetailsReleaseStats{
		Release: baseRelease,
		Start:   baseStart,
		End:     baseEnd,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  getSuccessRate(baseSuccess, baseFailure, baseFlake),
			SuccessCount: baseSuccess,
			FailureCount: baseFailure,
			FlakeCount:   baseFlake,
		},
	}

	testStats := crtype.ReportTestStats{
		Comparison: crtype.FisherExact,
		SampleStats: crtype.TestDetailsReleaseStats{
			Release: c.SampleRelease.Release,
			Start:   &c.SampleRelease.Start,
			End:     &c.SampleRelease.End,
			TestDetailsTestStats: crtype.TestDetailsTestStats{
				SuccessRate:  getSuccessRate(sampleSuccess, sampleFailure, sampleFlake),
				SuccessCount: sampleSuccess,
				FailureCount: sampleFailure,
				FlakeCount:   sampleFlake,
			},
		},
		BaseStats:    baseStats,
		Explanations: []string{explanationNoRegression},
	}
	status := crtype.MissingBasis
	// if the unadjusted sample was 0 then nothing to do
	if initialSampleTotal == 0 {
		if c.IgnoreMissing {
			status = crtype.NotSignificant
		} else {
			status = crtype.MissingSample
		}
	} else {
		// see if we had a significant regression prior to adjusting
		basisPassPercentage := float64(baseSuccess+baseFlake) / float64(baseTotal)
		initialPassPercentage := float64(sampleSuccess+sampleFlake) / float64(initialSampleTotal)
		effectivePityFactor := c.getEffectivePityFactor(basisPassPercentage, approvedRegression)

		wasSignificant := false
		// only consider wasSignificant if the sampleTotal has been changed and our sample
		// pass percentage is below the basis
		if initialSampleTotal > sampleTotal && initialPassPercentage < basisPassPercentage {
			if basisPassPercentage-initialPassPercentage > float64(c.PityFactor)/100 {
				wasSignificant, _ = c.fischerExactTest(requiredConfidence, initialSampleTotal, sampleSuccess, sampleFlake, baseTotal, baseSuccess, baseFlake)
			}
			// if it was significant without the adjustment use
			// ExtremeTriagedRegression or SignificantTriagedRegression
			if wasSignificant {
				status = getRegressionStatus(basisPassPercentage, initialPassPercentage, true)
			}
		}

		if sampleTotal == 0 {
			if !wasSignificant {
				if c.IgnoreMissing {
					status = crtype.NotSignificant

				} else {
					status = crtype.MissingSample
				}
			}
			return crtype.ReportTestStats{
				Comparison:   crtype.FisherExact,
				ReportStatus: status,
				FisherExact:  thrift.Float64Ptr(0.0),
				Explanations: []string{explanationNoRegression},
			}
		}

		// if we didn't detect a significant regression prior to adjusting set our default here
		if !wasSignificant {
			status = crtype.NotSignificant
		}

		// now that we know sampleTotal is non zero
		samplePassPercentage := float64(sampleSuccess+sampleFlake) / float64(sampleTotal)

		// did we remove enough failures that we are below the MinimumFailure threshold?
		if c.MinimumFailure != 0 && (sampleTotal-sampleSuccess-sampleFlake) < c.MinimumFailure {
			testStats.ReportStatus = status
			testStats.FisherExact = thrift.Float64Ptr(0.0)
			return testStats
		}
		significant := false
		improved := samplePassPercentage >= basisPassPercentage

		if improved {
			// flip base and sample when improved
			significant, fisherExact = c.fischerExactTest(requiredConfidence, baseTotal, baseSuccess, baseFlake, sampleTotal, sampleSuccess, sampleFlake)
		} else if basisPassPercentage-samplePassPercentage > float64(effectivePityFactor)/100 {
			significant, fisherExact = c.fischerExactTest(requiredConfidence, sampleTotal, sampleSuccess, sampleFlake, baseTotal, baseSuccess, baseFlake)
		}
		if significant {
			if improved {
				// only show improvements if we are not dropping out triaged results
				if initialSampleTotal == sampleTotal {
					status = crtype.SignificantImprovement
				}
			} else {
				status = getRegressionStatus(basisPassPercentage, samplePassPercentage, false)
			}
		}
	}
	testStats.ReportStatus = status
	testStats.FisherExact = thrift.Float64Ptr(fisherExact)

	// If we have a regression, include explanations:
	if testStats.ReportStatus <= crtype.SignificantTriagedRegression {
		testStats.Explanations = []string{
			fmt.Sprintf("%s regression detected.", crtype.StringForStatus(testStats.ReportStatus)),
			fmt.Sprintf("Fishers Exact probability of a regression: %.2f%%.", float64(100)-*testStats.FisherExact),
			fmt.Sprintf("Test pass rate dropped from %.2f%% to %.2f%%.",
				testStats.BaseStats.SuccessRate*float64(100),
				testStats.SampleStats.SuccessRate*float64(100)),
		}
		// check for override
		if baseRelease != c.BaseRelease.Release {
			testStats.Explanations = append(testStats.Explanations, fmt.Sprintf("Overrode base stats using release %s", baseRelease))
		}
	}

	return testStats
}

func (c *componentReportGenerator) buildPassRateTestStats(sampleSuccess, sampleFailure, sampleFlake int, requiredSuccessRate float64) crtype.ReportTestStats {
	successRate := getSuccessRate(sampleSuccess, sampleFailure, sampleFlake)

	// Assume 2x our allowed failure rate = an extreme regression.
	// i.e. if we require 90%, extreme is anything below 80%
	//      if we require 95%, extreme is anything below 90%
	severeRegressionSuccessRate := requiredSuccessRate - (100 - requiredSuccessRate)

	// Require 7 runs in the sample (typically 1 week) for us to consider a pass rate requirement for a new test:
	sufficientRuns := (sampleSuccess + sampleFailure + sampleFlake) >= 7

	if sufficientRuns && successRate*100 < requiredSuccessRate && sampleFailure >= c.MinimumFailure {
		rStatus := crtype.SignificantRegression
		if successRate*100 < severeRegressionSuccessRate {
			rStatus = crtype.ExtremeRegression
		}
		return crtype.ReportTestStats{
			ReportStatus: rStatus,
			Explanations: []string{
				fmt.Sprintf("Test has a %.2f%% pass rate, but %.2f%% is required.", successRate*100, requiredSuccessRate),
			},
			Comparison: crtype.PassRate,
			SampleStats: crtype.TestDetailsReleaseStats{
				Release: c.SampleRelease.Release,
				Start:   &c.SampleRelease.Start,
				End:     &c.SampleRelease.End,
				TestDetailsTestStats: crtype.TestDetailsTestStats{
					SuccessRate:  successRate,
					SuccessCount: sampleSuccess,
					FailureCount: sampleFailure,
					FlakeCount:   sampleFlake,
				},
			},
		}
	}
	return crtype.ReportTestStats{
		ReportStatus: crtype.NotSignificant,
		Explanations: []string{explanationNoRegression},
	}
}

func (c *componentReportGenerator) fischerExactTest(confidenceRequired, sampleTotal, sampleSuccess, sampleFlake, baseTotal, baseSuccess, baseFlake int) (bool, float64) {
	_, _, r, _ := fischer.FisherExactTest(sampleTotal-sampleSuccess-sampleFlake,
		sampleSuccess+sampleFlake,
		baseTotal-baseSuccess-baseFlake,
		baseSuccess+baseFlake)
	return r < 1-float64(confidenceRequired)/100, r
}

func (c *componentReportGenerator) getUniqueJUnitColumnValuesLast60Days(ctx context.Context, field string,
	nested bool) ([]string,
	error) {
	unnest := ""
	if nested {
		unnest = fmt.Sprintf(", UNNEST(%s) nested", field)
		field = "nested"
	}

	queryString := fmt.Sprintf(`SELECT
						DISTINCT %s as name
					FROM
						%s.junit %s
					WHERE
						NOT REGEXP_CONTAINS(prowjob_name, @IgnoredJobs)
						AND modified_time > DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 60 DAY)
					ORDER BY
						name`, field, c.client.Dataset, unnest)

	query := c.client.BQ.Query(queryString)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
	}

	return getSingleColumnResultToSlice(ctx, query)
}

func init() {
	componentAndCapabilityGetter = testToComponentAndCapability
}
