package api

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	fischer "github.com/glycerine/golang-fisher-exact"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/sets"
)

const (
	ignoredJobsRegexp = `-okd|-recovery|aggregator-`

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
				ci_analysis_us.junit
			WHERE modified_time >= DATETIME(@From)
			AND modified_time < DATETIME(@To)
		)
		SELECT * FROM deduped_testcases WHERE row_num = 1`
)

var (
	componentReadinessCacheDuration = 8 * time.Hour

	// Default filters, these are also hardcoded in the UI. Both must be updated.
	// TODO: TRT-1237 should centralize these configurations for consumption by both the front and backends
	DefaultExcludePlatforms = "openstack,alibaba,ibmcloud,libvirt,ovirt,unknown"
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

func GetComponentTestVariantsFromBigQuery(client *bqcachedclient.Client) (apitype.ComponentReportTestVariants, []error) {
	generator := componentReportGenerator{
		client: client.BQ,
	}

	return getReportFromCacheOrGenerate[apitype.ComponentReportTestVariants](client.Cache, "component_readiness_variants", generator.GenerateVariants, apitype.ComponentReportTestVariants{})
}

func GetComponentReportFromBigQuery(client *bqcachedclient.Client,
	baseRelease, sampleRelease apitype.ComponentReportRequestReleaseOptions,
	testIDOption apitype.ComponentReportRequestTestIdentificationOptions,
	variantOption apitype.ComponentReportRequestVariantOptions,
	excludeOption apitype.ComponentReportRequestExcludeOptions,
	advancedOption apitype.ComponentReportRequestAdvancedOptions) (apitype.ComponentReport, []error) {
	generator := componentReportGenerator{
		client:        client.BQ,
		BaseRelease:   baseRelease,
		SampleRelease: sampleRelease,
		ComponentReportRequestTestIdentificationOptions: testIDOption,
		ComponentReportRequestVariantOptions:            variantOption,
		ComponentReportRequestExcludeOptions:            excludeOption,
		ComponentReportRequestAdvancedOptions:           advancedOption,
	}

	return getReportFromCacheOrGenerate[apitype.ComponentReport](client.Cache, generator, generator.GenerateReport, apitype.ComponentReport{})
}

func GetComponentReportTestDetailsFromBigQuery(client *bqcachedclient.Client,
	baseRelease, sampleRelease apitype.ComponentReportRequestReleaseOptions,
	testIDOption apitype.ComponentReportRequestTestIdentificationOptions,
	variantOption apitype.ComponentReportRequestVariantOptions,
	excludeOption apitype.ComponentReportRequestExcludeOptions,
	advancedOption apitype.ComponentReportRequestAdvancedOptions) (apitype.ComponentReportTestDetails, []error) {
	generator := componentReportGenerator{
		client:        client.BQ,
		BaseRelease:   baseRelease,
		SampleRelease: sampleRelease,
		ComponentReportRequestTestIdentificationOptions: testIDOption,
		ComponentReportRequestVariantOptions:            variantOption,
		ComponentReportRequestExcludeOptions:            excludeOption,
		ComponentReportRequestAdvancedOptions:           advancedOption,
	}

	return getReportFromCacheOrGenerate[apitype.ComponentReportTestDetails](client.Cache, generator, generator.GenerateTestDetailsReport, apitype.ComponentReportTestDetails{})
}

type componentReportGenerator struct {
	client        *bigquery.Client
	BaseRelease   apitype.ComponentReportRequestReleaseOptions
	SampleRelease apitype.ComponentReportRequestReleaseOptions
	apitype.ComponentReportRequestTestIdentificationOptions
	apitype.ComponentReportRequestVariantOptions
	apitype.ComponentReportRequestExcludeOptions
	apitype.ComponentReportRequestAdvancedOptions
}

func (c *componentReportGenerator) GenerateVariants() (apitype.ComponentReportTestVariants, []error) {
	result := apitype.ComponentReportTestVariants{}
	errs := []error{}
	var err error
	queryString := `SELECT DISTINCT platform as name FROM ci_analysis_us.junit ORDER BY name`
	query := c.client.Query(queryString)
	result.Platform, err = getSingleColumnResultToSlice(query)
	if err != nil {
		log.WithError(err).Error("error querying platforms from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	queryString = `SELECT DISTINCT network as name FROM ci_analysis_us.junit ORDER BY name`
	query = c.client.Query(queryString)
	result.Network, err = getSingleColumnResultToSlice(query)
	if err != nil {
		log.WithError(err).Error("error querying networks from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	queryString = `SELECT DISTINCT arch as name FROM ci_analysis_us.junit ORDER BY name`
	query = c.client.Query(queryString)
	result.Arch, err = getSingleColumnResultToSlice(query)
	if err != nil {
		log.WithError(err).Error("error querying arches from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	queryString = `SELECT DISTINCT upgrade as name FROM ci_analysis_us.junit ORDER BY name`
	query = c.client.Query(queryString)
	result.Upgrade, err = getSingleColumnResultToSlice(query)
	if err != nil {
		log.WithError(err).Error("error querying upgrades from bigquery")
		errs = append(errs, err)
		return result, errs
	}
	queryString = `SELECT DISTINCT variant as name FROM ci_analysis_us.junit, UNNEST(variants) variant`
	query = c.client.Query(queryString)
	result.Variant, err = getSingleColumnResultToSlice(query)
	if err != nil {
		log.WithError(err).Error("error querying variants from bigquery")
		errs = append(errs, err)
	}

	return result, errs
}

func (c *componentReportGenerator) GenerateReport() (apitype.ComponentReport, []error) {
	before := time.Now()
	baseStatus, sampleStatus, errs := c.getTestStatusFromBigQuery()
	if len(errs) > 0 {
		return apitype.ComponentReport{}, errs
	}
	log.Infof("getTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(sampleStatus), len(baseStatus))

	report := c.generateComponentTestReport(baseStatus, sampleStatus)
	return report, nil
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
	before := time.Now()
	baseStatus, sampleStatus, errs := c.getJobRunTestStatusFromBigQuery()
	if len(errs) > 0 {
		return apitype.ComponentReportTestDetails{}, errs
	}
	log.Infof("getJobRunTestStatusFromBigQuery completed in %s with %d sample results and %d base results from db", time.Since(before), len(sampleStatus), len(baseStatus))
	report := c.generateComponentTestDetailsReport(baseStatus, sampleStatus)
	return report, nil
}

func (c *componentReportGenerator) getJobRunTestStatusFromBigQuery() (
	map[string][]apitype.ComponentJobRunTestStatusRow,
	map[string][]apitype.ComponentJobRunTestStatusRow,
	[]error,
) {
	errs := []error{}
	queryString := fmt.Sprintf(`WITH latest_component_mapping AS (
						SELECT *
						FROM ci_analysis_us.component_mapping cm
						WHERE created_at = (
								SELECT MAX(created_at)
								FROM openshift-gce-devel.ci_analysis_us.component_mapping))
					SELECT
						ANY_VALUE(test_name) AS test_name,
						ANY_VALUE(testsuite) AS test_suite,
						file_path,
						ANY_VALUE(prowjob_name) AS prowjob_name,
						ANY_VALUE(cm.jira_component) AS jira_component,
						ANY_VALUE(cm.jira_component_id) AS jira_component_id,
						COUNT(*) AS total_count,
						SUM(success_val) AS success_count,
						SUM(flake_count) AS flake_count,
					FROM (%s)
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name`, dedupedJunitTable)

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
						AND @Variant IN UNNEST(variants)
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

	baseString := queryString + ` AND branch = @BaseRelease`
	baseQuery := c.client.Query(baseString + groupString)

	baseQuery.Parameters = append(baseQuery.Parameters, commonParams...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: c.BaseRelease.Start,
		},
		{
			Name:  "To",
			Value: c.BaseRelease.End,
		},
		{
			Name:  "BaseRelease",
			Value: c.BaseRelease.Release,
		},
	}...)

	var baseStatus, sampleStatus map[string][]apitype.ComponentJobRunTestStatusRow
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		baseStatus, baseErrs = c.fetchJobRunTestStatus(baseQuery)
	}()

	sampleString := queryString + ` AND branch = @SampleRelease`
	sampleQuery := c.client.Query(sampleString + groupString)
	sampleQuery.Parameters = append(sampleQuery.Parameters, commonParams...)
	sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: c.SampleRelease.Start,
		},
		{
			Name:  "To",
			Value: c.SampleRelease.End,
		},
		{
			Name:  "SampleRelease",
			Value: c.SampleRelease.Release,
		},
	}...)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sampleStatus, sampleErrs = c.fetchJobRunTestStatus(sampleQuery)
	}()
	wg.Wait()
	if len(baseErrs) != 0 || len(sampleErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, sampleErrs...)
	}

	return baseStatus, sampleStatus, errs
}

func (c *componentReportGenerator) getTestStatusFromBigQuery() (
	map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus,
	map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus,
	[]error,
) {
	errs := []error{}
	queryString := fmt.Sprintf(`WITH latest_component_mapping AS (
						SELECT *
						FROM ci_analysis_us.component_mapping cm
						WHERE created_at = (
								SELECT MAX(created_at)
								FROM openshift-gce-devel.ci_analysis_us.component_mapping))
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
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name`, dedupedJunitTable)

	groupString := `
					GROUP BY
						network,
						upgrade,
						arch,
						platform,
						flat_variants,
						cm.id `

	queryString += `
					WHERE (prowjob_name LIKE 'periodic-%%' OR prowjob_name LIKE 'release-%%' OR prowjob_name LIKE 'aggregator-%%') AND NOT REGEXP_CONTAINS(prowjob_name, @IgnoredJobs)`

	commonParams := []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
	}
	if c.IgnoreDisruption {
		queryString += ` AND test_name NOT LIKE '%disruption/%'`
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
		queryString += ` AND @Variant IN UNNEST(variants)`
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

	baseString := queryString + ` AND branch = @BaseRelease`
	baseQuery := c.client.Query(baseString + groupString)

	baseQuery.Parameters = append(baseQuery.Parameters, commonParams...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: c.BaseRelease.Start,
		},
		{
			Name:  "To",
			Value: c.BaseRelease.End,
		},
		{
			Name:  "BaseRelease",
			Value: c.BaseRelease.Release,
		},
	}...)

	var baseStatus, sampleStatus map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus
	var baseErrs, sampleErrs []error
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		baseStatus, baseErrs = c.fetchTestStatus(baseQuery)
	}()

	sampleString := queryString + ` AND branch = @SampleRelease`
	sampleQuery := c.client.Query(sampleString + groupString)
	sampleQuery.Parameters = append(sampleQuery.Parameters, commonParams...)
	sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: c.SampleRelease.Start,
		},
		{
			Name:  "To",
			Value: c.SampleRelease.End,
		},
		{
			Name:  "SampleRelease",
			Value: c.SampleRelease.Release,
		},
	}...)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sampleStatus, sampleErrs = c.fetchTestStatus(sampleQuery)
	}()
	wg.Wait()
	if len(baseErrs) != 0 || len(sampleErrs) != 0 {
		errs = append(errs, baseErrs...)
		errs = append(errs, sampleErrs...)
	}
	return baseStatus, sampleStatus, errs
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
		index := 0
		for {
			column := apitype.ComponentReportColumnIdentification{}
			column.Platform = test.Platform
			column.Network = test.Network
			column.Arch = test.Arch
			column.Upgrade = test.Upgrade
			if len(stats.Variants) == 0 {
				columns = append(columns, column)
				break
			}
			if index < len(stats.Variants) {
				column.Variant = stats.Variants[index]
				columns = append(columns, column)
			}
			index++
			if index >= len(stats.Variants) {
				break
			}
		}
	} else {
		groups := sets.NewString(strings.Split(c.GroupBy, ",")...)
		index := 0
		for {
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
			if !groups.Has("variant") || len(stats.Variants) == 0 {
				columns = append(columns, column)
				break
			}
			if index < len(stats.Variants) {
				column.Variant = stats.Variants[index]
				columns = append(columns, column)
			}
			index++
			if index >= len(stats.Variants) {
				break
			}
		}
	}

	return rows, columns
}

func (c *componentReportGenerator) fetchTestStatus(query *bigquery.Query) (map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus, []error) {
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
	name = strings.ReplaceAll(name, c.BaseRelease.Release, "X.X")
	if prev, err := previousRelease(c.BaseRelease.Release); err == nil {
		name = strings.ReplaceAll(name, prev, "X.X")
	}
	name = strings.ReplaceAll(name, c.SampleRelease.Release, "X.X")
	if prev, err := previousRelease(c.SampleRelease.Release); err == nil {
		name = strings.ReplaceAll(name, prev, "X.X")
	}
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

func updateStatus(rowIdentifications []apitype.ComponentReportRowIdentification,
	columnIdentifications []apitype.ComponentReportColumnIdentification,
	reportStatus apitype.ComponentReportStatus,
	status map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]apitype.ComponentReportStatus,
	allRows map[apitype.ComponentReportRowIdentification]struct{},
	allColumns map[apitype.ComponentReportColumnIdentification]struct{}) {
	for _, columnIdentification := range columnIdentifications {
		if _, ok := allColumns[columnIdentification]; !ok {
			allColumns[columnIdentification] = struct{}{}
		}
	}
	for _, rowIdentification := range rowIdentifications {
		if _, ok := allRows[rowIdentification]; !ok {
			allRows[rowIdentification] = struct{}{}
		}
		row, ok := status[rowIdentification]
		if !ok {
			row = map[apitype.ComponentReportColumnIdentification]apitype.ComponentReportStatus{}
			for _, columnIdentification := range columnIdentifications {
				row[columnIdentification] = reportStatus
				status[rowIdentification] = row
			}
		} else {
			for _, columnIdentification := range columnIdentifications {
				existing, ok := row[columnIdentification]
				if !ok {
					row[columnIdentification] = reportStatus
				} else if (reportStatus < apitype.NotSignificant && reportStatus < existing) ||
					(existing == apitype.NotSignificant && reportStatus == apitype.SignificantImprovement) {
					// We want to show the significant improvement if assessment is not regression
					row[columnIdentification] = reportStatus
				}
			}
		}
	}
}

func (c *componentReportGenerator) generateComponentTestReport(baseStatus map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus,
	sampleStatus map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus) apitype.ComponentReport {
	report := apitype.ComponentReport{
		Rows: []apitype.ComponentReportRow{},
	}
	// aggregatedStatus is the aggregated status based on the requested rows and columns
	aggregatedStatus := map[apitype.ComponentReportRowIdentification]map[apitype.ComponentReportColumnIdentification]apitype.ComponentReportStatus{}
	// allRows and allColumns are used to make sure rows are ordered and all rows have the same columns in the same order
	allRows := map[apitype.ComponentReportRowIdentification]struct{}{}
	allColumns := map[apitype.ComponentReportColumnIdentification]struct{}{}
	for testIdentification, baseStats := range baseStatus {
		var reportStatus apitype.ComponentReportStatus
		sampleStats, ok := sampleStatus[testIdentification]
		if !ok {
			reportStatus = apitype.MissingSample
		} else {
			reportStatus, _ = c.assessComponentStatus(sampleStats.TotalCount, sampleStats.SuccessCount, sampleStats.FlakeCount, baseStats.TotalCount, baseStats.SuccessCount, baseStats.FlakeCount)
		}
		delete(sampleStatus, testIdentification)

		rowIdentifications, columnIdentifications := c.getRowColumnIdentifications(testIdentification, baseStats)
		updateStatus(rowIdentifications, columnIdentifications, reportStatus, aggregatedStatus, allRows, allColumns)
	}
	// Those sample ones are missing base stats
	for testIdentification, sampleStats := range sampleStatus {
		rowIdentifications, columnIdentification := c.getRowColumnIdentifications(testIdentification, sampleStats)
		updateStatus(rowIdentifications, columnIdentification, apitype.MissingBasis, aggregatedStatus, allRows, allColumns)
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
				reportColumn.Status = status
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

const prowJobPrefix = "https://prow.ci.openshift.org/view/gs/origin-ci-test/"

func getJobRunStats(stats apitype.ComponentJobRunTestStatusRow) apitype.ComponentReportTestDetailsJobRunStats {
	failure := getFailureCount(stats)
	url := prowJobPrefix
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
	}
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

			jobStats.BaseJobRunStats = append(jobStats.BaseJobRunStats, getJobRunStats(baseStats))
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

				jobStats.SampleJobRunStats = append(jobStats.SampleJobRunStats, getJobRunStats(sampleStats))
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
			jobStats.SampleJobRunStats = append(jobStats.SampleJobRunStats, getJobRunStats(sampleStats))
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
		totalBaseFlake)
	sort.Slice(result.JobStats, func(i, j int) bool {
		return result.JobStats[i].JobName < result.JobStats[j].JobName
	})
	return result
}

func (c *componentReportGenerator) assessComponentStatus(sampleTotal, sampleSuccess, sampleFlake, baseTotal, baseSuccess, baseFlake int) (apitype.ComponentReportStatus, float64) {
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
			significant := false
			improved := samplePassPercentage >= basisPassPercentage
			if improved {
				_, _, r, _ := fischer.FisherExactTest(baseTotal-baseSuccess-baseFlake,
					baseSuccess+baseFlake,
					sampleTotal-sampleSuccess-sampleFlake,
					sampleSuccess+sampleFlake)
				significant = r < 1-float64(c.Confidence)/100
				fischerExact = r
			} else if basisPassPercentage-samplePassPercentage > float64(c.PityFactor)/100 {
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

// getReportFromCacheOrGenerate attempts to find a cached record otherwise generates a new report.
func getReportFromCacheOrGenerate[T any](c cache.Cache, cacheKey interface{}, generateFn func() (T, []error), defaultVal T) (T, []error) {
	if c != nil {
		cacheKey, err := json.Marshal(cacheKey)
		if err != nil {
			return defaultVal, []error{err}
		}
		if res, err := c.Get(string(cacheKey)); err == nil {
			log.WithFields(log.Fields{
				"key":  string(cacheKey),
				"type": reflect.TypeOf(defaultVal).String(),
			}).Debugf("cache hit")
			var cr T
			if err := json.Unmarshal(res, &cr); err != nil {
				return defaultVal, []error{err}
			}
			return cr, nil
		}
		log.Infof("cache miss for cache key: %s", string(cacheKey))
		result, errs := generateFn()
		if len(errs) == 0 {
			cr, err := json.Marshal(result)
			if err == nil {
				if err := c.Set(string(cacheKey), cr, componentReadinessCacheDuration); err != nil {
					log.WithError(err).Warningf("couldn't persist new item to cache")
				} else {
					log.Debugf("cache set for cache key: %s", string(cacheKey))
				}
			}
		}
		return result, errs
	}

	return generateFn()
}

func init() {
	componentAndCapabilityGetter = testToComponentAndCapability
}
