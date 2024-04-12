package api

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openshift/sippy/pkg/componentreadiness/resolvedissues"

	"cloud.google.com/go/bigquery"
	fischer "github.com/glycerine/golang-fisher-exact"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/regressionallowances"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	triagedIncidentsTableID = "triaged_incidents"

	ignoredJobsRegexp = `-okd|-recovery|aggregator-|alibaba|-disruptive|-rollback|-out-of-change|-sno-fips-recert`

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
			SELECT  *,
				ROW_NUMBER() OVER(PARTITION BY file_path, test_name, testsuite ORDER BY
					CASE
						WHEN flake_count > 0 THEN 0
						WHEN success_val > 0 THEN 1
						ELSE 2
					END) AS row_num
			FROM
				%s.junit
			WHERE modified_time >= DATETIME(@From)
			AND modified_time < DATETIME(@To)
			AND skipped = false
		)
		SELECT * FROM deduped_testcases WHERE row_num = 1`
)

type GeneratorType string

var (
	// Default filters, these are also hardcoded in the UI. Both must be updated.
	// TODO: TRT-1237 should centralize these configurations for consumption by both the front and backends
	DefaultExcludePlatforms = "openstack,ibmcloud,libvirt,ovirt,unknown"
	DefaultExcludeArches    = "arm64,heterogeneous,ppc64le,s390x"
	DefaultExcludeVariants  = "hypershift,osd,microshift,techpreview,single-node,assisted,compact"
	DefaultGroupBy          = "cloud,arch,network"
	DefaultMinimumFailure   = 3
	DefaultConfidence       = 95
	DefaultPityFactor       = 5
	DefaultIgnoreMissing    = false
	DefaultIgnoreDisruption = true
)

func getSingleColumnResultToSlice(query *bigquery.Query) ([]string, error) {
	names := []string{}
	it, err := query.Read(context.TODO())
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

func GetComponentTestVariantsFromBigQuery(client *bqcachedclient.Client, gcsBucket string) (apitype.ComponentReportTestVariants, []error) {
	generator := componentReportGenerator{
		client:    client,
		gcsBucket: gcsBucket,
	}

	return getDataFromCacheOrGenerate[apitype.ComponentReportTestVariants](client.Cache, cache.RequestOptions{}, GetPrefixedCacheKey("TestVariants~", generator), generator.GenerateVariants, apitype.ComponentReportTestVariants{})
}

func GetComponentReportFromBigQuery(client *bqcachedclient.Client, gcsBucket string,
	baseRelease, sampleRelease apitype.ComponentReportRequestReleaseOptions,
	testIDOption apitype.ComponentReportRequestTestIdentificationOptions,
	variantOption apitype.ComponentReportRequestVariantOptions,
	excludeOption apitype.ComponentReportRequestExcludeOptions,
	advancedOption apitype.ComponentReportRequestAdvancedOptions,
	cacheOption cache.RequestOptions,
) (apitype.ComponentReport, []error) {
	generator := componentReportGenerator{
		client:        client,
		gcsBucket:     gcsBucket,
		cacheOption:   cacheOption,
		BaseRelease:   baseRelease,
		SampleRelease: sampleRelease,
		triagedIssues: nil,
		ComponentReportRequestTestIdentificationOptions: testIDOption,
		ComponentReportRequestVariantOptions:            variantOption,
		ComponentReportRequestExcludeOptions:            excludeOption,
		ComponentReportRequestAdvancedOptions:           advancedOption,
	}

	return getDataFromCacheOrGenerate[apitype.ComponentReport](generator.client.Cache, generator.cacheOption,
		generator.GetComponentReportCacheKey("ComponentReport~"), generator.GenerateReport, apitype.ComponentReport{})
}

func GetComponentReportTestDetailsFromBigQuery(client *bqcachedclient.Client, gcsBucket string,
	baseRelease, sampleRelease apitype.ComponentReportRequestReleaseOptions,
	testIDOption apitype.ComponentReportRequestTestIdentificationOptions,
	variantOption apitype.ComponentReportRequestVariantOptions,
	excludeOption apitype.ComponentReportRequestExcludeOptions,
	advancedOption apitype.ComponentReportRequestAdvancedOptions,
	cacheOption cache.RequestOptions) (apitype.ComponentReportTestDetails, []error) {
	generator := componentReportGenerator{
		client:        client,
		gcsBucket:     gcsBucket,
		cacheOption:   cacheOption,
		BaseRelease:   baseRelease,
		SampleRelease: sampleRelease,
		ComponentReportRequestTestIdentificationOptions: testIDOption,
		ComponentReportRequestVariantOptions:            variantOption,
		ComponentReportRequestExcludeOptions:            excludeOption,
		ComponentReportRequestAdvancedOptions:           advancedOption,
	}

	return getDataFromCacheOrGenerate[apitype.ComponentReportTestDetails](generator.client.Cache, generator.cacheOption,
		generator.GetComponentReportCacheKey("TestDetailsReport~"), generator.GenerateTestDetailsReport,
		apitype.ComponentReportTestDetails{})
}

// componentReportGenerator contains the information needed to generate a CR report. Do
// not add public fields to this struct if they are not valid as a cache key.
// GeneratorVersion is used to indicate breaking changes in the versions of
// the cached data.  It is used when the struct
// is marshalled for the cache key and should be changed when the object being
// cached changes in a way that will no longer be compatible with any prior cached version.
type componentReportGenerator struct {
	ReportModified *time.Time
	client         *bqcachedclient.Client
	gcsBucket      string
	cacheOption    cache.RequestOptions
	BaseRelease    apitype.ComponentReportRequestReleaseOptions
	SampleRelease  apitype.ComponentReportRequestReleaseOptions
	triagedIssues  *resolvedissues.TriagedIncidentsForRelease
	apitype.ComponentReportRequestTestIdentificationOptions
	apitype.ComponentReportRequestVariantOptions
	apitype.ComponentReportRequestExcludeOptions
	apitype.ComponentReportRequestAdvancedOptions
}

func (c *componentReportGenerator) GetComponentReportCacheKey(prefix string) CacheData {
	// Make sure we have initialized the report modified field
	if c.ReportModified == nil {
		c.ReportModified = c.GetLastReportModifiedTime(c.client, c.cacheOption)
	}
	return GetPrefixedCacheKey(prefix, c)
}

func (c *componentReportGenerator) GenerateVariants() (apitype.ComponentReportTestVariants, []error) {
	errs := []error{}
	columns := make(map[string][]string)

	for _, column := range []string{"platform", "network", "arch", "upgrade", "variants"} {
		values, err := c.getUniqueJUnitColumnValues(column, column == "variants")
		if err != nil {
			wrappedErr := errors.Wrapf(err, "couldn't fetch %s", column)
			log.WithError(wrappedErr).Errorf("error generating variants")
			errs = append(errs, wrappedErr)
		}
		columns[column] = values
	}

	return apitype.ComponentReportTestVariants{
		Platform: columns["platform"],
		Network:  columns["network"],
		Arch:     columns["arch"],
		Upgrade:  columns["upgrade"],
		Variant:  columns["variants"],
	}, errs
}

func (c *componentReportGenerator) GenerateReport() (apitype.ComponentReport, []error) {
	before := time.Now()
	componentReportTestStatus, errs := c.GenerateComponentReportTestStatus()
	if len(errs) > 0 {
		return apitype.ComponentReport{}, errs
	}
	report := c.generateComponentTestReport(componentReportTestStatus.BaseStatus, componentReportTestStatus.SampleStatus)
	report.GeneratedAt = componentReportTestStatus.GeneratedAt
	log.Infof("GenerateReport completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentReportTestStatus.SampleStatus), len(componentReportTestStatus.BaseStatus))

	return report, nil
}

func (c *componentReportGenerator) GenerateComponentReportTestStatus() (apitype.ComponentReportTestStatus, []error) {
	before := time.Now()
	componentReportTestStatus, errs := c.getTestStatusFromBigQuery()
	if len(errs) > 0 {
		return apitype.ComponentReportTestStatus{}, errs
	}
	log.Infof("getTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentReportTestStatus.SampleStatus), len(componentReportTestStatus.BaseStatus))
	now := time.Now()
	componentReportTestStatus.GeneratedAt = &now
	return componentReportTestStatus, nil
}

func (c *componentReportGenerator) GenerateTestDetailsReport() (apitype.ComponentReportTestDetails, []error) {
	if c.TestID == "" ||
		c.Platform == "" ||
		c.Network == "" ||
		c.Upgrade == "" ||
		c.Arch == "" ||
		c.Variant == "" {
		return apitype.ComponentReportTestDetails{}, []error{fmt.Errorf("all parameters have to be defined for test details: test_id, platform, network, upgrade, arch, variant")}
	}
	componentJobRunTestReportStatus, errs := c.GenerateJobRunTestReportStatus()
	if len(errs) > 0 {
		return apitype.ComponentReportTestDetails{}, errs
	}
	report := c.generateComponentTestDetailsReport(componentJobRunTestReportStatus.BaseStatus, componentJobRunTestReportStatus.SampleStatus)
	report.GeneratedAt = componentJobRunTestReportStatus.GeneratedAt
	return report, nil
}

func (c *componentReportGenerator) GenerateJobRunTestReportStatus() (apitype.ComponentJobRunTestReportStatus, []error) {
	before := time.Now()
	componentJobRunTestReportStatus, errs := c.getJobRunTestStatusFromBigQuery()
	if len(errs) > 0 {
		return apitype.ComponentJobRunTestReportStatus{}, errs
	}
	log.Infof("getJobRunTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(componentJobRunTestReportStatus.SampleStatus), len(componentJobRunTestReportStatus.BaseStatus))
	now := time.Now()
	componentJobRunTestReportStatus.GeneratedAt = &now
	return componentJobRunTestReportStatus, nil
}

func (c *componentReportGenerator) getCommonJobRunTestStatusQuery() (string, string, []bigquery.QueryParameter) {
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
						ANY_VALUE(prowjob_name) AS prowjob_name,
						ANY_VALUE(cm.jira_component) AS jira_component,
						ANY_VALUE(cm.jira_component_id) AS jira_component_id,
						COUNT(*) AS total_count,
						ANY_VALUE(cm.capabilities) as capabilities,
						SUM(success_val) AS success_count,
						SUM(flake_count) AS flake_count,
					FROM (%s)
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name`, c.client.Dataset, c.client.Dataset, fmt.Sprintf(dedupedJunitTable, c.client.Dataset))

	groupString := `
					GROUP BY
						file_path,
						modified_time
					ORDER BY
						modified_time `
	queryString += `
					WHERE
						(prowjob_name LIKE 'periodic-%%' OR prowjob_name LIKE 'release-%%' OR prowjob_name LIKE 'aggregator-%%')
						AND NOT REGEXP_CONTAINS(prowjob_name, @IgnoredJobs)
						AND upgrade = @Upgrade
						AND arch = @Arch
						AND network = @Network
						AND platform = @Platform
						AND flat_variants = @Variant
						AND cm.id = @TestId `
	commonParams := []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
		{
			Name:  "Upgrade",
			Value: c.Upgrade,
		},
		{
			Name:  "Arch",
			Value: c.Arch,
		},
		{
			Name:  "Network",
			Value: c.Network,
		},
		{
			Name:  "Platform",
			Value: c.Platform,
		},
		{
			Name:  "TestId",
			Value: c.TestID,
		},
		{
			Name:  "Variant",
			Value: c.Variant,
		},
	}

	return queryString, groupString, commonParams
}

type baseJobRunTestStatusGenerator struct {
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery.QueryParameter
	cacheOption              cache.RequestOptions
	ComponentReportGenerator *componentReportGenerator
}

func (c *componentReportGenerator) getBaseJobRunTestStatus(commonQuery string,
	groupByQuery string,
	queryParameters []bigquery.QueryParameter) (map[string][]apitype.ComponentJobRunTestStatusRow, []error) {
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

	componentReportTestStatus, errs := getDataFromCacheOrGenerate[apitype.ComponentJobRunTestReportStatus](generator.ComponentReportGenerator.client.Cache, generator.cacheOption, GetPrefixedCacheKey("BaseJobRunTestStatus~", generator), generator.queryTestStatus, apitype.ComponentJobRunTestReportStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.BaseStatus, nil
}

func (b *baseJobRunTestStatusGenerator) queryTestStatus() (apitype.ComponentJobRunTestReportStatus, []error) {
	baseString := b.commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.ComponentReportGenerator.client.BQ.Query(baseString + b.groupByQuery)

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

	baseStatus, errs := b.ComponentReportGenerator.fetchJobRunTestStatus(baseQuery)
	return apitype.ComponentJobRunTestReportStatus{BaseStatus: baseStatus}, errs
}

type sampleJobRunTestQueryGenerator struct {
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

func (c *componentReportGenerator) getSampleJobRunTestStatus(commonQuery string,
	groupByQuery string,
	queryParameters []bigquery.QueryParameter) (map[string][]apitype.ComponentJobRunTestStatusRow, []error) {
	generator := sampleJobRunTestQueryGenerator{
		commonQuery:              commonQuery,
		groupByQuery:             groupByQuery,
		queryParameters:          queryParameters,
		ComponentReportGenerator: c,
	}

	componentReportTestStatus, errs := getDataFromCacheOrGenerate[apitype.ComponentJobRunTestReportStatus](c.client.Cache, c.cacheOption, GetPrefixedCacheKey("SampleJobRunTestStatus~", generator), generator.queryTestStatus, apitype.ComponentJobRunTestReportStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.SampleStatus, nil
}

func (s *sampleJobRunTestQueryGenerator) queryTestStatus() (apitype.ComponentJobRunTestReportStatus, []error) {
	sampleString := s.commonQuery + ` AND branch = @SampleRelease`
	sampleQuery := s.ComponentReportGenerator.client.BQ.Query(sampleString + s.groupByQuery)
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

	sampleStatus, errs := s.ComponentReportGenerator.fetchJobRunTestStatus(sampleQuery)

	return apitype.ComponentJobRunTestReportStatus{SampleStatus: sampleStatus}, errs
}

func (c *componentReportGenerator) getJobRunTestStatusFromBigQuery() (apitype.ComponentJobRunTestReportStatus, []error) {
	errs := []error{}

	queryString, groupString, commonParams := c.getCommonJobRunTestStatusQuery()
	var baseStatus, sampleStatus map[string][]apitype.ComponentJobRunTestStatusRow
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		baseStatus, baseErrs = c.getBaseJobRunTestStatus(queryString, groupString, commonParams)
	}()

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

	return apitype.ComponentJobRunTestReportStatus{BaseStatus: baseStatus, SampleStatus: sampleStatus}, errs
}

func (c *componentReportGenerator) getCommonTestStatusQuery() (string, string, []bigquery.QueryParameter) {
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
						network,
						upgrade,
						arch,
						platform,
						flat_variants,
						ANY_VALUE(variants) AS variants,
						COUNT(cm.id) AS total_count,
						SUM(success_val) AS success_count,
						SUM(flake_count) AS flake_count,
						ANY_VALUE(cm.component) AS component,
						ANY_VALUE(cm.capabilities) AS capabilities,
						ANY_VALUE(cm.jira_component) AS jira_component,
						ANY_VALUE(cm.jira_component_id) AS jira_component_id
					FROM (%s)
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name`, c.client.Dataset, c.client.Dataset, fmt.Sprintf(dedupedJunitTable, c.client.Dataset))

	groupString := `
					GROUP BY
						network,
						upgrade,
						arch,
						platform,
						flat_variants,
						cm.id `

	queryString += `
					WHERE cm.staff_approved_obsolete = false AND (prowjob_name LIKE 'periodic-%%' OR prowjob_name LIKE 'release-%%' OR prowjob_name LIKE 'aggregator-%%') AND NOT REGEXP_CONTAINS(prowjob_name, @IgnoredJobs)`

	commonParams := []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
	}
	if c.IgnoreDisruption {
		queryString += ` AND NOT 'Disruption' in UNNEST(capabilities)`
	}
	if c.Upgrade != "" {
		queryString += ` AND upgrade = @Upgrade`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "Upgrade",
			Value: c.Upgrade,
		})
	}
	if c.Arch != "" {
		queryString += ` AND arch = @Arch`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "Arch",
			Value: c.Arch,
		})
	}
	if c.Network != "" {
		queryString += ` AND network = @Network`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "Network",
			Value: c.Network,
		})
	}
	if c.Platform != "" {
		queryString += ` AND platform = @Platform`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "Platform",
			Value: c.Platform,
		})
	}
	if c.TestID != "" {
		queryString += ` AND cm.id = @TestId`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "TestId",
			Value: c.TestID,
		})
	}

	if c.Variant != "" {
		queryString += ` AND flat_variants = @Variant`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "Variant",
			Value: c.Variant,
		})
	}

	if c.ExcludePlatforms != "" {
		queryString += ` AND platform NOT IN UNNEST(@ExcludePlatforms)`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "ExcludePlatforms",
			Value: strings.Split(c.ExcludePlatforms, ","),
		})
	}
	if c.ExcludeArches != "" {
		queryString += ` AND arch NOT IN UNNEST(@ExcludeArches)`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "ExcludeArches",
			Value: strings.Split(c.ExcludeArches, ","),
		})
	}
	if c.ExcludeNetworks != "" {
		queryString += ` AND network NOT IN UNNEST(@ExcludeNetworks)`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "ExcludeNetworks",
			Value: strings.Split(c.ExcludeNetworks, ","),
		})
	}
	if c.ExcludeUpgrades != "" {
		queryString += ` AND upgrade NOT IN UNNEST(@ExcludeUpgrades)`
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  "ExcludeUpgrades",
			Value: strings.Split(c.ExcludeUpgrades, ","),
		})
	}
	if c.ExcludeVariants != "" {
		variants := strings.Split(c.ExcludeVariants, ",")
		for i, variant := range variants {
			paramName := fmt.Sprintf("ExcludeVariant%d", i)
			queryString += ` AND @` + paramName + ` NOT IN UNNEST(variants)`
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  paramName,
				Value: variant,
			})
		}
	}

	return queryString, groupString, commonParams

}

type baseQueryGenerator struct {
	client                   *bqcachedclient.Client
	cacheOption              cache.RequestOptions
	BaseRelease              apitype.ComponentReportRequestReleaseOptions
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

func (c *componentReportGenerator) getBaseQueryStatus(commonQuery string,
	groupByQuery string,
	queryParameters []bigquery.QueryParameter) (map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus, []error) {
	generator := baseQueryGenerator{
		client: c.client,
		cacheOption: cache.RequestOptions{
			ForceRefresh: c.cacheOption.ForceRefresh,
			// increase the time that base query is cached for since it shouldn't be changing?
			CRTimeRoundingFactor: c.cacheOption.CRTimeRoundingFactor,
		},
		BaseRelease:              c.BaseRelease,
		commonQuery:              commonQuery,
		groupByQuery:             groupByQuery,
		queryParameters:          queryParameters,
		ComponentReportGenerator: c,
	}

	componentReportTestStatus, errs := getDataFromCacheOrGenerate[apitype.ComponentReportTestStatus](c.client.Cache, generator.cacheOption, GetPrefixedCacheKey("BaseTestStatus~", generator), generator.queryTestStatus, apitype.ComponentReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.BaseStatus, nil
}

func (b *baseQueryGenerator) queryTestStatus() (apitype.ComponentReportTestStatus, []error) {
	before := time.Now()
	errs := []error{}
	baseString := b.commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.client.BQ.Query(baseString + b.groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, b.queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: b.BaseRelease.Start,
		},
		{
			Name:  "To",
			Value: b.BaseRelease.End,
		},
		{
			Name:  "BaseRelease",
			Value: b.BaseRelease.Release,
		},
	}...)

	baseStatus, baseErrs := fetchTestStatus(baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Base QueryTestStatus completed in %s with %d base results from db", time.Since(before), len(baseStatus))

	return apitype.ComponentReportTestStatus{BaseStatus: baseStatus}, errs
}

type sampleQueryGenerator struct {
	client                   *bqcachedclient.Client
	SampleRelease            apitype.ComponentReportRequestReleaseOptions
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

func (c *componentReportGenerator) getSampleQueryStatus(
	commonQuery string,
	groupByQuery string,
	queryParameters []bigquery.QueryParameter) (map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus, []error) {
	generator := sampleQueryGenerator{
		client:                   c.client,
		SampleRelease:            c.SampleRelease,
		commonQuery:              commonQuery,
		groupByQuery:             groupByQuery,
		queryParameters:          queryParameters,
		ComponentReportGenerator: c,
	}

	componentReportTestStatus, errs := getDataFromCacheOrGenerate[apitype.ComponentReportTestStatus](c.client.Cache, c.cacheOption, GetPrefixedCacheKey("SampleTestStatus~", generator), generator.queryTestStatus, apitype.ComponentReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.SampleStatus, nil
}

func (s *sampleQueryGenerator) queryTestStatus() (apitype.ComponentReportTestStatus, []error) {
	before := time.Now()
	errs := []error{}
	sampleString := s.commonQuery + ` AND branch = @SampleRelease`
	sampleQuery := s.client.BQ.Query(sampleString + s.groupByQuery)
	sampleQuery.Parameters = append(sampleQuery.Parameters, s.queryParameters...)
	sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: s.SampleRelease.Start,
		},
		{
			Name:  "To",
			Value: s.SampleRelease.End,
		},
		{
			Name:  "SampleRelease",
			Value: s.SampleRelease.Release,
		},
	}...)

	sampleStatus, sampleErrs := fetchTestStatus(sampleQuery)

	if len(sampleErrs) != 0 {
		errs = append(errs, sampleErrs...)
	}

	log.Infof("Sample QueryTestStatus completed in %s with %d sample results db", time.Since(before), len(sampleStatus))

	return apitype.ComponentReportTestStatus{SampleStatus: sampleStatus}, errs
}

func (c *componentReportGenerator) getTestStatusFromBigQuery() (apitype.ComponentReportTestStatus, []error) {
	before := time.Now()
	errs := []error{}
	queryString, groupString, commonParams := c.getCommonTestStatusQuery()

	var baseStatus, sampleStatus map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		baseStatus, baseErrs = c.getBaseQueryStatus(queryString, groupString, commonParams)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sampleStatus, sampleErrs = c.getSampleQueryStatus(queryString, groupString, commonParams)
	}()
	wg.Wait()
	if len(baseErrs) != 0 || len(sampleErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, sampleErrs...)
	}
	log.Infof("getTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(sampleStatus), len(baseStatus))
	return apitype.ComponentReportTestStatus{BaseStatus: baseStatus, SampleStatus: sampleStatus}, errs
}

var componentAndCapabilityGetter func(test apitype.ComponentTestIdentification, stats apitype.ComponentTestStatus) (string, []string)

/*
func testToComponentAndCapabilityUseRegex(test *apitype.ComponentTestIdentification, stats *apitype.ComponentTestStatus) (string, []string) {
	name := test.TestName
	component := "other_component"
	capability := "other_capability"
	r := regexp.MustCompile(`.*(?P<component>\[sig-[A-Za-z]*\]).*(?P<feature>\[Feature:[A-Za-z]*\]).*`)
	subMatches := r.FindStringSubmatch(name)
	if len(subMatches) >= 2 {
		subNames := r.SubexpNames()
		for i, sName := range subNames {
			switch sName {
			case "component":
				component = subMatches[i]
			case "feature":
				capability = subMatches[i]
			}
		}
	}
	return component, []string{capability}
}*/

func testToComponentAndCapability(test apitype.ComponentTestIdentification, stats apitype.ComponentTestStatus) (string, []string) {
	return stats.Component, stats.Capabilities
}

// getRowColumnIdentifications defines the rows and columns since they are variable. For rows, different pages have different row titles (component, capability etc)
// Columns titles depends on the groupBy parameter user requests. A particular test can belong to multiple rows of different capabilities.
func (c *componentReportGenerator) getRowColumnIdentifications(test apitype.ComponentTestIdentification, stats apitype.ComponentTestStatus) ([]apitype.ComponentReportRowIdentification, []apitype.ComponentReportColumnIdentification) {
	component, capabilities := componentAndCapabilityGetter(test, stats)
	rows := []apitype.ComponentReportRowIdentification{}
	// First Page with no component requested
	if c.Component == "" {
		rows = append(rows, apitype.ComponentReportRowIdentification{Component: component})
	} else if c.Component == component {
		// Exact test match
		if c.TestID != "" {
			row := apitype.ComponentReportRowIdentification{
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
						row := apitype.ComponentReportRowIdentification{
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
					rows = append(rows, apitype.ComponentReportRowIdentification{Component: component, Capability: capability})
				}
			}
		}
	}
	columns := []apitype.ComponentReportColumnIdentification{}
	if c.TestID != "" {
		// When testID is specified, ignore groupBy to disambiguate the test
		column := apitype.ComponentReportColumnIdentification{}
		column.Platform = test.Platform
		column.Network = test.Network
		column.Arch = test.Arch
		column.Upgrade = test.Upgrade
		column.Variant = test.FlatVariants
		columns = append(columns, column)
	} else {
		groups := sets.NewString(strings.Split(c.GroupBy, ",")...)
		column := apitype.ComponentReportColumnIdentification{}
		if groups.Has("cloud") {
			column.Platform = test.Platform
		}
		if groups.Has("network") {
			column.Network = test.Network
		}
		if groups.Has("arch") {
			column.Arch = test.Arch
		}
		if groups.Has("upgrade") {
			column.Upgrade = test.Upgrade
		}
		if groups.Has("variants") {
			column.Variant = test.FlatVariants
		}
		columns = append(columns, column)
	}

	return rows, columns
}

func fetchTestStatus(query *bigquery.Query) (map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus, []error) {
	errs := []error{}
	status := map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{}
	log.Infof("Fetching test status with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(context.TODO())
	if err != nil {
		log.WithError(err).Error("error querying test status from bigquery")
		errs = append(errs, err)
		return status, errs
	}

	for {
		testStatus := apitype.ComponentTestStatusRow{}
		err := it.Next(&testStatus)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing component from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing prowjob from bigquery"))
			continue
		}
		testIdentification := apitype.ComponentTestIdentification{
			TestID:       testStatus.TestID,
			Network:      testStatus.Network,
			Upgrade:      testStatus.Upgrade,
			Arch:         testStatus.Arch,
			Platform:     testStatus.Platform,
			FlatVariants: testStatus.FlatVariants,
		}
		status[testIdentification] = apitype.ComponentTestStatus{
			TestName:     testStatus.TestName,
			TestSuite:    testStatus.TestSuite,
			Component:    testStatus.Component,
			Capabilities: testStatus.Capabilities,
			Variants:     testStatus.Variants,
			TotalCount:   testStatus.TotalCount,
			FlakeCount:   testStatus.FlakeCount,
			SuccessCount: testStatus.SuccessCount,
		}
		log.Tracef("testStatus is %+v", testStatus)
	}
	return status, errs
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

func (c *componentReportGenerator) fetchJobRunTestStatus(query *bigquery.Query) (map[string][]apitype.ComponentJobRunTestStatusRow, []error) {
	errs := []error{}
	status := map[string][]apitype.ComponentJobRunTestStatusRow{}
	log.Infof("Fetching job run test details with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(context.TODO())
	if err != nil {
		log.WithError(err).Error("error querying job run test status from bigquery")
		errs = append(errs, err)
		return status, errs
	}

	for {
		testStatus := apitype.ComponentJobRunTestStatusRow{}
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
			status[prowName] = []apitype.ComponentJobRunTestStatusRow{testStatus}
		} else {
			rows = append(rows, testStatus)
			status[prowName] = rows
		}
	}
	return status, errs
}

type cellStatus struct {
	status         apitype.ComponentReportStatus
	regressedTests []apitype.ComponentReportTestSummary
}

func getNewCellStatus(testID apitype.ComponentReportTestIdentification,
	reportStatus apitype.ComponentReportStatus, sampleStats apitype.ComponentTestStatus,
	existingCellStatus *cellStatus, logger log.FieldLogger) cellStatus {

	var newCellStatus cellStatus
	if existingCellStatus != nil {
		if (reportStatus < apitype.NotSignificant && reportStatus < existingCellStatus.status) ||
			(existingCellStatus.status == apitype.NotSignificant && reportStatus == apitype.SignificantImprovement) {
			// We want to show the significant improvement if assessment is not regression
			newCellStatus.status = reportStatus
		} else {
			newCellStatus.status = existingCellStatus.status
		}
		newCellStatus.regressedTests = existingCellStatus.regressedTests
	} else {
		newCellStatus.status = reportStatus
	}
	if reportStatus < apitype.MissingSample {
		logger.Infof("adding regressed test")
		newCellStatus.regressedTests = append(newCellStatus.regressedTests, apitype.ComponentReportTestSummary{
			ComponentReportTestIdentification: testID,
			Status:                            reportStatus,
			TotalCount:                        sampleStats.TotalCount,
			FlakeCount:                        sampleStats.FlakeCount,
			SuccessCount:                      sampleStats.SuccessCount,
		})
	}
	return newCellStatus
}

func updateCellStatus(rowIdentifications []apitype.ComponentReportRowIdentification,
	columnIdentifications []apitype.ComponentReportColumnIdentification,
	testID apitype.ComponentReportTestIdentification,
	reportStatus apitype.ComponentReportStatus,
	sampleStats apitype.ComponentTestStatus,
	status map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]cellStatus,
	allRows map[apitype.ComponentReportRowIdentification]struct{},
	allColumns map[apitype.ComponentReportColumnIdentification]struct{}) {
	logger := log.WithField("testID", testID.TestID)
	logger.Infof("called updateCellStatus with %d row IDs and %d col IDs", len(rowIdentifications), len(columnIdentifications))
	logger.Infof("row IDs: %+v", rowIdentifications)
	logger.Infof("col IDs: %+v", columnIdentifications)
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
			row = map[apitype.ComponentReportColumnIdentification]cellStatus{}
			for _, columnIdentification := range columnIdentifications {
				logger.Infof("getNewCellStatus for %+v", columnIdentification)
				logger.Infof("  status rowIdentification %+v", rowIdentification)
				row[columnIdentification] = getNewCellStatus(testID, reportStatus, sampleStats, nil, logger)
				status[rowIdentification] = row
			}
		} else {
			for _, columnIdentification := range columnIdentifications {
				existing, ok := row[columnIdentification]
				if !ok {
					logger.Infof("getNewCellStatus 2 for %+v", columnIdentification)
					row[columnIdentification] = getNewCellStatus(testID, reportStatus, sampleStats, nil, logger)
				} else {
					logger = logger.WithField("columnID", columnIdentification).WithField("row", rowIdentification)
					logger.Infof("getNewCellStatus 3")
					row[columnIdentification] = getNewCellStatus(testID, reportStatus, sampleStats, &existing, logger)
				}
			}
		}
	}
}

func (c *componentReportGenerator) getTriagedIssuesFromBigQuery(testID apitype.ComponentReportTestIdentification) (int, []error) {
	generator := triagedIncidentsGenerator{
		ReportModified: c.GetLastReportModifiedTime(c.client, c.cacheOption),
		client:         c.client,
		cacheOption:    c.cacheOption,
		SampleRelease:  c.SampleRelease,
	}

	// we want to fetch this once per generator instance which should be once per UI load
	// this is the full list from the cache if available that will be subset to specific test
	// in triagedIssuesFor
	if c.triagedIssues == nil {
		releaseTriagedIncidents, errs := getDataFromCacheOrGenerate[resolvedissues.TriagedIncidentsForRelease](generator.client.Cache, generator.cacheOption, GetPrefixedCacheKey("TriagedIncidents~", generator), generator.generateTriagedIssuesFor, resolvedissues.TriagedIncidentsForRelease{})

		if len(errs) > 0 {
			return 0, errs
		}
		c.triagedIssues = &releaseTriagedIncidents
	}
	impactedRuns := triagedIssuesFor(c.triagedIssues, testID.ComponentReportColumnIdentification, testID.TestID, c.SampleRelease.Start, c.SampleRelease.End)

	return impactedRuns, nil
}

func (c *componentReportGenerator) GetLastReportModifiedTime(client *bqcachedclient.Client, options cache.RequestOptions) *time.Time {

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
		lastModifiedTime, errs := getDataFromCacheOrGenerate[*time.Time](generator.client.Cache, generator.cacheOption, GetPrefixedCacheKey("TriageLastModified~", generator), generator.generateTriagedIssuesLastModifiedTime, generator.LastModifiedStartTime)

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

func (t *triagedIncidentsModifiedTimeGenerator) generateTriagedIssuesLastModifiedTime() (*time.Time, []error) {
	before := time.Now()
	lastModifiedTime, errs := t.queryTriagedIssuesLastModified()

	log.Infof("generateTriagedIssuesLastModifiedTime query completed in %s ", time.Since(before))

	if errs != nil {
		return nil, errs
	}

	return lastModifiedTime, nil
}

func (t *triagedIncidentsModifiedTimeGenerator) queryTriagedIssuesLastModified() (*time.Time, []error) {
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

	return t.fetchLastModified(sampleQuery)
}

func (t *triagedIncidentsModifiedTimeGenerator) fetchLastModified(query *bigquery.Query) (*time.Time, []error) {
	log.Infof("Fetching triaged incidents last modified time with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(context.TODO())
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
	SampleRelease  apitype.ComponentReportRequestReleaseOptions
}

func (t *triagedIncidentsGenerator) generateTriagedIssuesFor() (resolvedissues.TriagedIncidentsForRelease, []error) {
	before := time.Now()
	incidents, errs := t.queryTriagedIssues()

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

func triagedIssuesFor(releaseIncidents *resolvedissues.TriagedIncidentsForRelease, variant apitype.ComponentReportColumnIdentification, testID string, startTime, endTime time.Time) int {
	if releaseIncidents == nil {
		return 0
	}

	inKey := resolvedissues.KeyForTriagedIssue(testID, resolvedissues.TransformVariant(variant))

	triagedIncidents := releaseIncidents.TriagedIncidents[inKey]

	impactedJobRuns := sets.NewString() // because multiple issues could impact the same job run, be sure to count each job run only once
	numJobRunsToSuppress := 0
	for _, triagedIncident := range triagedIncidents {
		for _, impactedJobRun := range triagedIncident.JobRuns {
			if impactedJobRuns.Has(impactedJobRun.URL) {
				continue
			}
			impactedJobRuns.Insert(impactedJobRun.URL)

			if impactedJobRun.StartTime.After(startTime) && impactedJobRun.StartTime.Before(endTime) {
				numJobRunsToSuppress++
			}
		}
	}

	return numJobRunsToSuppress
}

func (t *triagedIncidentsGenerator) queryTriagedIssues() ([]resolvedissues.TriagedIncident, []error) {
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

	return t.fetchTriagedIssues(sampleQuery)
}

func (t *triagedIncidentsGenerator) fetchTriagedIssues(query *bigquery.Query) ([]resolvedissues.TriagedIncident, []error) {
	errs := make([]error, 0)
	incidents := make([]resolvedissues.TriagedIncident, 0)
	log.Infof("Fetching triaged incidents with:\n%s\nParameters:\n%+v\n", query.Q, query.Parameters)

	it, err := query.Read(context.TODO())
	if err != nil {
		log.WithError(err).Error("error querying triaged incidents from bigquery")
		errs = append(errs, err)
		return incidents, errs
	}

	for {
		var triagedIncident resolvedissues.TriagedIncident
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

func (c *componentReportGenerator) triagedIncidentsFor(testID apitype.ComponentReportTestIdentification) int {

	// handle test case / missing client
	if c.client == nil {
		return 0
	}

	impactedRuns, errs := c.getTriagedIssuesFromBigQuery(testID)

	if errs != nil {
		for _, err := range errs {
			log.WithError(err).Error("error getting triaged issues component from bigquery")
		}
		return 0
	}

	return impactedRuns
}

func (c *componentReportGenerator) generateComponentTestReport(
	baseStatus map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus,
	sampleStatus map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus) apitype.ComponentReport {

	report := apitype.ComponentReport{
		Rows: []apitype.ComponentReportRow{},
	}
	// aggregatedStatus is the aggregated status based on the requested rows and columns
	aggregatedStatus := map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]cellStatus{}
	// allRows and allColumns are used to make sure rows are ordered and all rows have the same columns in the same order
	allRows := map[apitype.ComponentReportRowIdentification]struct{}{}
	allColumns := map[apitype.ComponentReportColumnIdentification]struct{}{}
	// testID is used to identify the most regressed test. With this, we can
	// create a shortcut link from any page to go straight to the most regressed test page.
	var testID apitype.ComponentReportTestIdentification
	for testIdentification, baseStats := range baseStatus {
		testID = apitype.ComponentReportTestIdentification{
			ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
				Component: baseStats.Component,
				TestName:  baseStats.TestName,
				TestSuite: baseStats.TestSuite,
				TestID:    testIdentification.TestID,
			},
			ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
				Network:  testIdentification.Network,
				Upgrade:  testIdentification.Upgrade,
				Arch:     testIdentification.Arch,
				Platform: testIdentification.Platform,
				Variant:  testIdentification.FlatVariants,
			},
		}
		// Take the first cap for now. When we reach to a cell with specific capability, we will override the value.
		if len(baseStats.Capabilities) > 0 {
			testID.Capability = baseStats.Capabilities[0]
		}

		var reportStatus apitype.ComponentReportStatus
		sampleStats, ok := sampleStatus[testIdentification]
		if !ok {
			reportStatus = apitype.MissingSample
		} else {
			approvedRegression := regressionallowances.IntentionalRegressionFor(c.SampleRelease.Release, testID.ComponentReportColumnIdentification, testID.TestID)
			resolvedIssueCompensation := c.triagedIncidentsFor(testID)
			// if we need to fall back
			// _, resolvedIssueCompensation := resolvedissues.ResolvedIssuesFor(c.SampleRelease.Release, testID.ComponentReportColumnIdentification, testID.TestID, c.SampleRelease.Start, c.SampleRelease.End)
			reportStatus, _ = c.assessComponentStatus(sampleStats.TotalCount, sampleStats.SuccessCount, sampleStats.FlakeCount, baseStats.TotalCount, baseStats.SuccessCount, baseStats.FlakeCount, approvedRegression, resolvedIssueCompensation)
		}
		delete(sampleStatus, testIdentification)

		rowIdentifications, columnIdentifications := c.getRowColumnIdentifications(testIdentification, baseStats)
		updateCellStatus(rowIdentifications, columnIdentifications, testID, reportStatus, sampleStats, aggregatedStatus, allRows, allColumns)
	}

	// The remaining entries in sampleStatus must have no results in the basis, and are likely new tests:
	newTests := map[string][]apitype.ComponentTestIdentification{}
	for testIdentification, sampleStats := range sampleStatus {
		logger := log.WithField("testID", testIdentification.TestID)
		// Optionally flag as regressions based on pass rate. This is not the default behavior yet but may be in the future.
		// TODO: make optional?
		status := apitype.MissingBasis
		samplePassPercentage := float64(sampleStats.SuccessCount+sampleStats.FlakeCount) / float64(sampleStats.TotalCount)
		switch {
		case samplePassPercentage < 0.95:
			status = apitype.ExtremeRegression
			if _, ok := newTests[testIdentification.TestID]; !ok {
				newTests[testIdentification.TestID] = []apitype.ComponentTestIdentification{}
			}
			newTests[testIdentification.TestID] = append(newTests[testIdentification.TestID], testIdentification)
			logger.Info("new test flagged as extreme regression")
		case samplePassPercentage < 0.99:
			status = apitype.SignificantRegression
			if _, ok := newTests[testIdentification.TestID]; !ok {
				newTests[testIdentification.TestID] = []apitype.ComponentTestIdentification{}
			}
			newTests[testIdentification.TestID] = append(newTests[testIdentification.TestID], testIdentification)
			logger.Info("new test flagged as significant regression")

		}
		rowIdentifications, columnIdentification := c.getRowColumnIdentifications(testIdentification, sampleStats)
		for _, ri := range rowIdentifications {
			logger.Infof("row id: %s", ri)
		}
		for _, ci := range columnIdentification {
			logger.Infof("col id: %s", ci)
		}
		updateCellStatus(rowIdentifications, columnIdentification, testID, status, sampleStats, aggregatedStatus, allRows, allColumns)
	}

	for tid, tids := range newTests {
		log.Infof(tid)
		for _, t := range tids {
			log.Infof("   %+v", t)
		}
	}

	// Sort the row identifications
	sortedRows := []apitype.ComponentReportRowIdentification{}
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
	sortedColumns := []apitype.ComponentReportColumnIdentification{}
	for columnID := range allColumns {
		sortedColumns = append(sortedColumns, columnID)
	}
	sort.Slice(sortedColumns, func(i, j int) bool {
		less := sortedColumns[i].Platform < sortedColumns[j].Platform
		if sortedColumns[i].Platform == sortedColumns[j].Platform {
			less = sortedColumns[i].Arch < sortedColumns[j].Arch
			if sortedColumns[i].Arch == sortedColumns[j].Arch {
				less = sortedColumns[i].Network < sortedColumns[j].Network
				if sortedColumns[i].Network == sortedColumns[j].Network {
					less = sortedColumns[i].Upgrade < sortedColumns[j].Upgrade
					if sortedColumns[i].Upgrade == sortedColumns[j].Upgrade {
						less = sortedColumns[i].Variant < sortedColumns[j].Variant
					}
				}
			}
		}
		return less
	})

	// Now build the report
	var regressionRows, goodRows []apitype.ComponentReportRow
	for _, rowID := range sortedRows {
		columns, ok := aggregatedStatus[rowID]
		if !ok {
			continue
		}
		reportRow := apitype.ComponentReportRow{ComponentReportRowIdentification: rowID}
		hasRegression := false
		for _, columnID := range sortedColumns {
			if reportRow.Columns == nil {
				reportRow.Columns = []apitype.ComponentReportColumn{}
			}
			reportColumn := apitype.ComponentReportColumn{ComponentReportColumnIdentification: columnID}
			status, ok := columns[columnID]
			if !ok {
				reportColumn.Status = apitype.MissingBasisAndSample
			} else {
				reportColumn.Status = status.status
				reportColumn.RegressedTests = status.regressedTests
				sort.Slice(reportColumn.RegressedTests, func(i, j int) bool {
					return reportColumn.RegressedTests[i].Status < reportColumn.RegressedTests[j].Status
				})
			}
			reportRow.Columns = append(reportRow.Columns, reportColumn)
			if reportColumn.Status <= apitype.SignificantRegression {
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

	report.Rows = append(regressionRows, goodRows...)
	return report
}

func getFailureCount(status apitype.ComponentJobRunTestStatusRow) int {
	failure := status.TotalCount - status.SuccessCount - status.FlakeCount
	if failure < 0 {
		failure = 0
	}
	return failure
}

func getSuccessRate(success, failure, flake int) float64 {
	total := success + failure + flake
	if total == 0 {
		return 0.0
	}
	return float64(success+flake) / float64(total)
}

func getJobRunStats(stats apitype.ComponentJobRunTestStatusRow, gcsBucket string) apitype.ComponentReportTestDetailsJobRunStats {
	failure := getFailureCount(stats)
	url := fmt.Sprintf("https://prow.ci.openshift.org/view/gs/%s/", gcsBucket)
	subs := strings.Split(stats.FilePath, "/artifacts/")
	if len(subs) > 1 {
		url += subs[0]
	}
	jobRunStats := apitype.ComponentReportTestDetailsJobRunStats{
		TestStats: apitype.ComponentReportTestDetailsTestStats{
			SuccessRate:  getSuccessRate(stats.SuccessCount, failure, stats.FlakeCount),
			SuccessCount: stats.SuccessCount,
			FailureCount: failure,
			FlakeCount:   stats.FlakeCount,
		},
		JobURL: url,
	}
	return jobRunStats
}

func (c *componentReportGenerator) generateComponentTestDetailsReport(baseStatus map[string][]apitype.ComponentJobRunTestStatusRow,
	sampleStatus map[string][]apitype.ComponentJobRunTestStatusRow) apitype.ComponentReportTestDetails {
	result := apitype.ComponentReportTestDetails{
		ComponentReportTestIdentification: apitype.ComponentReportTestIdentification{
			ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
				Component:  c.Component,
				Capability: c.Capability,
				TestID:     c.TestID,
			},
			ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
				Platform: c.Platform,
				Upgrade:  c.Upgrade,
				Arch:     c.Arch,
				Network:  c.Network,
				Variant:  c.Variant,
			},
		},
	}
	approvedRegression := regressionallowances.IntentionalRegressionFor(c.SampleRelease.Release, result.ComponentReportColumnIdentification, c.TestID)
	resolvedIssueCompensation := c.triagedIncidentsFor(result.ComponentReportTestIdentification)
	// if we need to fall back
	// _, resolvedIssueCompensation := resolvedissues.ResolvedIssuesFor(c.SampleRelease.Release, testID.ComponentReportColumnIdentification, testID.TestID, c.SampleRelease.Start, c.SampleRelease.End)

	var totalBaseFailure, totalBaseSuccess, totalBaseFlake, totalSampleFailure, totalSampleSuccess, totalSampleFlake int
	var perJobBaseFailure, perJobBaseSuccess, perJobBaseFlake, perJobSampleFailure, perJobSampleSuccess, perJobSampleFlake int
	for prowJob, baseStatsList := range baseStatus {
		jobStats := apitype.ComponentReportTestDetailsJobStats{
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

			jobStats.BaseJobRunStats = append(jobStats.BaseJobRunStats, getJobRunStats(baseStats, c.gcsBucket))
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

				jobStats.SampleJobRunStats = append(jobStats.SampleJobRunStats, getJobRunStats(sampleStats, c.gcsBucket))
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
		_, _, r, _ := fischer.FisherExactTest(perJobSampleFailure,
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
		jobStats := apitype.ComponentReportTestDetailsJobStats{
			JobName: prowJob,
		}
		perJobSampleFailure = 0
		perJobSampleSuccess = 0
		perJobSampleFlake = 0
		for _, sampleStats := range sampleStatsList {
			jobStats.SampleJobRunStats = append(jobStats.SampleJobRunStats, getJobRunStats(sampleStats, c.gcsBucket))
			perJobSampleSuccess += sampleStats.SuccessCount
			perJobSampleFlake += sampleStats.FlakeCount
			perJobSampleFailure += getFailureCount(sampleStats)
		}
		jobStats.SampleStats.SuccessCount = perJobSampleSuccess
		jobStats.SampleStats.FlakeCount = perJobSampleFlake
		jobStats.SampleStats.FailureCount = perJobSampleFailure
		jobStats.SampleStats.SuccessRate = getSuccessRate(perJobSampleSuccess, perJobSampleFailure, perJobSampleFlake)
		result.JobStats = append(result.JobStats, jobStats)
		_, _, r, _ := fischer.FisherExactTest(perJobSampleFailure,
			perJobSampleSuccess+perJobSampleFlake,
			0,
			0)
		jobStats.Significant = r < 1-float64(c.Confidence)/100

		totalSampleFailure += perJobSampleFailure
		totalSampleSuccess += perJobSampleSuccess
		totalSampleFlake += perJobSampleFlake
	}
	result.BaseStats.Release = c.BaseRelease.Release
	result.BaseStats.SuccessCount = totalBaseSuccess
	result.BaseStats.FailureCount = totalBaseFailure
	result.BaseStats.FlakeCount = totalBaseFlake
	result.BaseStats.SuccessRate = getSuccessRate(totalBaseSuccess, totalBaseFailure, totalBaseFlake)
	result.SampleStats.Release = c.SampleRelease.Release
	result.SampleStats.SuccessCount = totalSampleSuccess
	result.SampleStats.FailureCount = totalSampleFailure
	result.SampleStats.FlakeCount = totalSampleFlake
	result.SampleStats.SuccessRate = getSuccessRate(totalSampleSuccess, totalSampleFailure, totalSampleFlake)
	result.ReportStatus, result.FisherExact = c.assessComponentStatus(
		totalSampleSuccess+totalSampleFailure+totalSampleFlake,
		totalSampleSuccess,
		totalSampleFlake,
		totalBaseSuccess+totalBaseFailure+totalBaseFlake,
		totalBaseSuccess,
		totalBaseFlake,
		approvedRegression,
		resolvedIssueCompensation,
	)
	sort.Slice(result.JobStats, func(i, j int) bool {
		return result.JobStats[i].JobName < result.JobStats[j].JobName
	})
	return result
}

func (c *componentReportGenerator) assessComponentStatus(sampleTotal, sampleSuccess, sampleFlake, baseTotal, baseSuccess, baseFlake int, approvedRegression *regressionallowances.IntentionalRegression, numberOfIgnoredSampleJobRuns int) (apitype.ComponentReportStatus, float64) {
	adjustedSampleTotal := sampleTotal - numberOfIgnoredSampleJobRuns
	if adjustedSampleTotal < sampleSuccess {
		log.Errorf("adjustedSampleTotal is too small: sampleTotal=%d, numberOfIgnoredSampleJobRuns=%d, sampleSuccess=%d", sampleTotal, numberOfIgnoredSampleJobRuns, sampleSuccess)
	} else {
		sampleTotal = adjustedSampleTotal
	}

	status := apitype.MissingBasis
	fischerExact := 0.0
	if baseTotal != 0 {
		if sampleTotal == 0 {
			if c.IgnoreMissing {
				status = apitype.NotSignificant

			} else {
				status = apitype.MissingSample
			}
		} else {
			if c.MinimumFailure != 0 && (sampleTotal-sampleSuccess-sampleFlake) < c.MinimumFailure {
				return apitype.NotSignificant, fischerExact
			}

			basisPassPercentage := float64(baseSuccess+baseFlake) / float64(baseTotal)
			samplePassPercentage := float64(sampleSuccess+sampleFlake) / float64(sampleTotal)
			effectivePityFactor := c.PityFactor
			if approvedRegression != nil && approvedRegression.RegressedPassPercentage < int(basisPassPercentage*100) {
				// product owner chose a required pass percentage, so we all pity to cover that approved pass percent
				// plus the existing pity factor to limit, "well, it's just *barely* lower" arguments.
				effectivePityFactor = int(basisPassPercentage*100) - approvedRegression.RegressedPassPercentage + c.PityFactor

				if effectivePityFactor < c.PityFactor {
					log.Errorf("effective pity factor for %+v is below zero: %d", approvedRegression, effectivePityFactor)
					effectivePityFactor = c.PityFactor
				}
			}

			significant := false
			improved := samplePassPercentage >= basisPassPercentage

			if improved {
				_, _, r, _ := fischer.FisherExactTest(baseTotal-baseSuccess-baseFlake,
					baseSuccess+baseFlake,
					sampleTotal-sampleSuccess-sampleFlake,
					sampleSuccess+sampleFlake)
				significant = r < 1-float64(c.Confidence)/100
				fischerExact = r
			} else if basisPassPercentage-samplePassPercentage > float64(effectivePityFactor)/100 {
				_, _, r, _ := fischer.FisherExactTest(sampleTotal-sampleSuccess-sampleFlake,
					sampleSuccess+sampleFlake,
					baseTotal-baseSuccess-baseFlake,
					baseSuccess+baseFlake)
				significant = r < 1-float64(c.Confidence)/100
				fischerExact = r
			}
			if significant {
				if improved {
					status = apitype.SignificantImprovement
				} else {
					if (basisPassPercentage - samplePassPercentage) > 0.15 {
						status = apitype.ExtremeRegression
					} else {
						status = apitype.SignificantRegression
					}
				}
			} else {
				status = apitype.NotSignificant
			}
		}
	}
	return status, fischerExact
}

func (c *componentReportGenerator) getUniqueJUnitColumnValues(field string, nested bool) ([]string, error) {
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
					ORDER BY
						name`, field, c.client.Dataset, unnest)

	query := c.client.BQ.Query(queryString)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
	}

	return getSingleColumnResultToSlice(query)
}

func init() {
	componentAndCapabilityGetter = testToComponentAndCapability
}
