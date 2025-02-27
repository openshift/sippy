package query

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/param"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const (
	DefaultJunitTable = "junit"

	IgnoredJobsRegexp = `-okd|-recovery|aggregator-|alibaba|-disruptive|-rollback|-out-of-change|-sno-fips-recert|-bgp-`

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
)

type baseQueryGenerator struct {
	client      *bqcachedclient.Client
	allVariants crtype.JobVariants
	ReqOptions  crtype.RequestOptions
}

func NewBaseQueryGenerator(
	client *bqcachedclient.Client,
	reqOptions crtype.RequestOptions,
	allVariants crtype.JobVariants) baseQueryGenerator {
	generator := baseQueryGenerator{
		client:      client,
		allVariants: allVariants,
		ReqOptions:  reqOptions,
	}
	return generator
}

func (b *baseQueryGenerator) QueryTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {

	commonQuery, groupByQuery, queryParameters := BuildCommonTestStatusQuery(b.client,
		b.ReqOptions,
		b.allVariants,
		b.ReqOptions.VariantOption.IncludeVariants,
		DefaultJunitTable, false, false)

	before := time.Now()
	errs := []error{}
	baseString := commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.client.BQ.Query(baseString + groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: b.ReqOptions.BaseRelease.Start,
		},
		{
			Name:  "To",
			Value: b.ReqOptions.BaseRelease.End,
		},
		{
			Name:  "BaseRelease",
			Value: b.ReqOptions.BaseRelease.Release,
		},
	}...)

	baseStatus, baseErrs := FetchTestStatusResults(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Base QueryTestStatus completed in %s with %d base results from db", time.Since(before), len(baseStatus))

	return crtype.ReportTestStatus{BaseStatus: baseStatus}, errs
}

type sampleQueryGenerator struct {
	client      *bqcachedclient.Client
	allVariants crtype.JobVariants
	ReqOptions  crtype.RequestOptions
	// JunitTable is the bigquery table (in the normal dataset configured), where this sample query generator should
	// pull its data from. It is a public field as we want it included in the cache
	// key to differentiate this request from other sample queries that might be using a junit table override.
	// Normally, this would just be the default junit table, but in some cases we pull from other tables. (rarely run jobs)
	JunitTable string
	// IncludeVariants is a potentially slightly adjusted copy of the ComponentReportGenerator, used in conjunction with
	// junit table overrides to tweak the query.
	IncludeVariants map[string][]string

	Start time.Time
	End   time.Time
}

func NewSampleQueryGenerator(
	client *bqcachedclient.Client,
	reqOptions crtype.RequestOptions,
	allVariants crtype.JobVariants,
	includeVariants map[string][]string, // separate from ReqOptions as caller sometimes has to modify them
	start, end time.Time,
	junitTable string) sampleQueryGenerator {

	generator := sampleQueryGenerator{
		ReqOptions:      reqOptions,
		client:          client,
		allVariants:     allVariants,
		JunitTable:      junitTable,
		IncludeVariants: includeVariants,
		Start:           start,
		End:             end,
	}
	return generator
}

func (s *sampleQueryGenerator) QueryTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {
	commonQuery, groupByQuery, queryParameters := BuildCommonTestStatusQuery(s.client, s.ReqOptions,
		s.allVariants, s.IncludeVariants, s.JunitTable, true, false)

	before := time.Now()
	errs := []error{}
	sampleString := commonQuery + ` AND branch = @SampleRelease`
	if s.ReqOptions.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND org = @Org AND repo = @Repo AND pr_number = @PRNumber`
	}
	sampleQuery := s.client.BQ.Query(sampleString + groupByQuery)
	sampleQuery.Parameters = append(sampleQuery.Parameters, queryParameters...)
	sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: s.Start,
		},
		{
			Name:  "To",
			Value: s.End,
		},
		{
			Name:  "SampleRelease",
			Value: s.ReqOptions.SampleRelease.Release,
		},
	}...)
	if s.ReqOptions.SampleRelease.PullRequestOptions != nil {
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
			{
				Name:  "Org",
				Value: s.ReqOptions.SampleRelease.PullRequestOptions.Org,
			},
			{
				Name:  "Repo",
				Value: s.ReqOptions.SampleRelease.PullRequestOptions.Repo,
			},
			{
				Name:  "PRNumber",
				Value: s.ReqOptions.SampleRelease.PullRequestOptions.PRNumber,
			},
		}...)
	}

	sampleStatus, sampleErrs := FetchTestStatusResults(ctx, sampleQuery)

	if len(sampleErrs) != 0 {
		errs = append(errs, sampleErrs...)
	}

	log.Infof("Sample QueryTestStatus completed in %s with %d sample results db", time.Since(before), len(sampleStatus))

	return crtype.ReportTestStatus{SampleStatus: sampleStatus}, errs
}

// BuildCommonTestStatusQuery returns the common query for the higher level summary component summary.
func BuildCommonTestStatusQuery(
	client *bqcachedclient.Client,
	reqOptions crtype.RequestOptions,
	allJobVariants crtype.JobVariants,
	includeVariants map[string][]string,
	junitTable string,
	isSample, isFallback bool) (string, string, []bigquery.QueryParameter) {
	// Parts of the query, including the columns returned, are dynamic, based on the list of variants we're told to work with.
	// Variants will be returned as columns with names like: variant_[VariantName]
	// See FetchTestStatusResults for where we dynamically handle these columns.
	selectVariants := ""
	joinVariants := ""
	groupByVariants := ""
	for _, v := range sortedKeys(allJobVariants.Variants) {
		joinVariants += fmt.Sprintf("LEFT JOIN %s.job_variants jv_%s ON variant_registry_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			client.Dataset, v, v, v, v)
	}
	for _, v := range reqOptions.VariantOption.DBGroupBy.List() {
		v = param.Cleanse(v)
		selectVariants += fmt.Sprintf("jv_%s.variant_value AS variant_%s,\n", v, v) // Note: Variants are camelcase, so the query columns come back like: variant_Architecture
		groupByVariants += fmt.Sprintf("jv_%s.variant_value,\n", v)
	}

	jobNameQueryPortion := normalJobNameCol
	if reqOptions.SampleRelease.PullRequestOptions != nil && isSample {
		jobNameQueryPortion = pullRequestDynamicJobNameCol
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
						MAX(CASE WHEN adjusted_success_val = 0 THEN modified_time ELSE NULL END) AS last_failure,
						ANY_VALUE(cm.component) AS component,
						ANY_VALUE(cm.capabilities) AS capabilities,
					FROM (%s)
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name
`,
		client.Dataset, client.Dataset, selectVariants, fmt.Sprintf(dedupedJunitTable, jobNameQueryPortion, client.Dataset, junitTable, client.Dataset))

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
			Value: IgnoredJobsRegexp,
		},
	}
	if reqOptions.AdvancedOption.IgnoreDisruption {
		queryString += ` AND NOT 'Disruption' in UNNEST(capabilities)`
	}

	// fallback queries get all variants with no filtering
	// so all tests are fetched then cached
	if !isFallback {
		variantGroups := includeVariants
		// potentially cross-compare variants for the sample
		if isSample && len(reqOptions.VariantOption.VariantCrossCompare) > 0 {
			variantGroups = reqOptions.VariantOption.CompareVariants
		}
		if variantGroups == nil { // server-side view definitions may omit a variants map
			variantGroups = map[string][]string{}
		}

		for _, group := range sortedKeys(variantGroups) {
			group = param.Cleanse(group) // should be clean already, but just to make sure
			paramName := fmt.Sprintf("variantGroup_%s", group)
			queryString += fmt.Sprintf(" AND (jv_%s.variant_value in UNNEST(@%s))", group, paramName)
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  paramName,
				Value: variantGroups[group],
			})
		}

		for _, group := range sortedKeys(reqOptions.VariantOption.RequestedVariants) {
			group = param.Cleanse(group) // should be clean already, but just to make sure
			paramName := fmt.Sprintf("ReqVariant_%s", group)
			queryString += fmt.Sprintf(` AND jv_%s.variant_value = @%s`, group, paramName)
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  paramName,
				Value: reqOptions.VariantOption.RequestedVariants[group],
			})
		}
		if reqOptions.TestIDOption.Capability != "" {
			queryString += " AND @Capability in UNNEST(capabilities)"
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  "Capability",
				Value: reqOptions.TestIDOption.Capability,
			})
		}
		if reqOptions.TestIDOption.TestID != "" {
			queryString += ` AND cm.id = @TestId`
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  "TestId",
				Value: reqOptions.TestIDOption.TestID,
			})
		}
	}
	return queryString, groupString, commonParams
}

// getTestDetailsQuery returns the report for a specific test + variant combo, including job run data.
// This is for the bottom level most specific pages in component readiness.
func getTestDetailsQuery(
	client *bqcachedclient.Client,
	c crtype.RequestOptions,
	allJobVariants crtype.JobVariants,
	includeVariants map[string][]string,
	junitTable string,
	isSample bool) (string, string, []bigquery.QueryParameter) {

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
`, client.Dataset, client.Dataset, fmt.Sprintf(dedupedJunitTable, jobNameQueryPortion, client.Dataset, junitTable, client.Dataset))

	joinVariants := ""
	for _, variant := range sortedKeys(allJobVariants.Variants) {
		v := param.Cleanse(variant) // should be clean anyway, but just to make sure
		joinVariants += fmt.Sprintf("LEFT JOIN %s.job_variants jv_%s ON variant_registry_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			client.Dataset, v, v, v, v)
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
	commonParams := []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: IgnoredJobsRegexp,
		},
		{
			Name:  "TestId",
			Value: c.TestIDOption.TestID,
		},
	}

	for _, key := range sortedKeys(includeVariants) {
		// only add in include variants that aren't part of the requested or cross-compared variants

		if _, ok := c.VariantOption.RequestedVariants[key]; ok {
			continue
		}
		if slices.Contains(c.VariantOption.VariantCrossCompare, key) {
			continue
		}

		group := param.Cleanse(key)
		paramName := "IncludeVariants" + group
		queryString += fmt.Sprintf(` AND jv_%s.variant_value IN UNNEST(@%s)`, group, paramName)
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  paramName,
			Value: c.VariantOption.IncludeVariants[key],
		})
	}

	for _, group := range sortedKeys(c.VariantOption.RequestedVariants) {
		group = param.Cleanse(group) // should be clean anyway, but just to make sure
		paramName := "IncludeVariantValue" + group
		queryString += fmt.Sprintf(` AND jv_%s.variant_value = @%s`, group, paramName)
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  paramName,
			Value: c.VariantOption.RequestedVariants[group],
		})
	}
	if isSample {
		queryString += filterByCrossCompareVariants(c.VariantOption.VariantCrossCompare, c.VariantOption.CompareVariants, &commonParams)
	} else {
		queryString += filterByCrossCompareVariants(c.VariantOption.VariantCrossCompare, includeVariants, &commonParams)
	}
	return queryString, groupString, commonParams
}

// filterByCrossCompareVariants adds the where clause for any variants being cross-compared (which are not included in RequestedVariants).
// As a side effect, it also appends any necessary parameters for the clause.
func filterByCrossCompareVariants(crossCompare []string, variantGroups map[string][]string, params *[]bigquery.QueryParameter) (whereClause string) {
	if len(variantGroups) == 0 {
		return // avoid possible nil pointer dereference
	}
	sort.StringSlice(crossCompare).Sort()
	for _, group := range crossCompare {
		if variants := variantGroups[group]; len(variants) > 0 {
			group = param.Cleanse(group)
			paramName := "CrossVariants" + group
			whereClause += fmt.Sprintf(` AND jv_%s.variant_value IN UNNEST(@%s)`, group, paramName)
			*params = append(*params, bigquery.QueryParameter{
				Name:  paramName,
				Value: variants,
			})
		}
	}
	return
}

func FetchTestStatusResults(ctx context.Context, query *bigquery.Query) (map[string]crtype.TestStatus, []error) {
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
	tid := crtype.TestWithVariantsKey{
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
		case col == "last_failure":
			// ignore when we cant parse, its usually null
			var err error
			if row[i] != nil {
				layout := "2006-01-02T15:04:05"
				lftCivilDT := row[i].(civil.DateTime)
				cts.LastFailure, err = time.Parse(layout, lftCivilDT.String())
				if err != nil {
					log.WithError(err).Error("error parsing last failure time from bigquery")
				}
			}
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

// sortedKeys is a helper that sorts the keys of a variant group map for consistent ordering.
func sortedKeys[T any](it map[string]T) []string {
	keys := make([]string, 0, len(it))
	for k := range it {
		keys = append(keys, k)
	}
	sort.StringSlice(keys).Sort()
	return keys
}

// baseTestDetailsQueryGenerator generates the query we use for the basis on the test details page.
type baseTestDetailsQueryGenerator struct {
	client         *bqcachedclient.Client
	ReqOptions     crtype.RequestOptions
	allJobVariants crtype.JobVariants
	BaseRelease    string
	BaseStart      time.Time
	BaseEnd        time.Time
}

func NewBaseTestDetailsQueryGenerator(client *bqcachedclient.Client,
	reqOptions crtype.RequestOptions,
	allJobVariants crtype.JobVariants,
	baseRelease string, baseStart time.Time, baseEnd time.Time) *baseTestDetailsQueryGenerator {

	return &baseTestDetailsQueryGenerator{
		client:         client,
		ReqOptions:     reqOptions,
		allJobVariants: allJobVariants,
		BaseRelease:    baseRelease,
		BaseEnd:        baseEnd,
		BaseStart:      baseStart,
	}
}

func (b *baseTestDetailsQueryGenerator) QueryTestStatus(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {
	commonQuery, groupByQuery, queryParameters := getTestDetailsQuery(
		b.client,
		b.ReqOptions,
		b.allJobVariants,
		b.ReqOptions.VariantOption.IncludeVariants, DefaultJunitTable, false)
	baseString := commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.client.BQ.Query(baseString + groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
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

	baseStatus, errs := fetchJobRunTestStatusResults(ctx, baseQuery, b.ReqOptions)
	return crtype.JobRunTestReportStatus{BaseStatus: baseStatus}, errs
}

// sampleTestDetailsQueryGenerator generates the query we use for the sample on the test details page.
type sampleTestDetailsQueryGenerator struct {
	allJobVariants crtype.JobVariants
	client         *bqcachedclient.Client
	ReqOptions     crtype.RequestOptions

	// JunitTable is the bigquery table (in the normal dataset configured), where this sample query generator should
	// pull its data from. It is a public field as we want it included in the cache
	// key to differentiate this request from other sample queries that might be using a junit table override.
	// Normally, this would just be the default junit table, but in some cases we pull from other tables. (rarely run jobs)
	JunitTable string
	// IncludeVariants is a potentially slightly adjusted copy of the ComponentReportGenerator, used in conjunction with
	// junit table overrides to tweak the query.
	IncludeVariants map[string][]string

	Start time.Time
	End   time.Time
}

func NewSampleTestDetailsQueryGenerator(
	client *bqcachedclient.Client,
	reqOptions crtype.RequestOptions,
	allJobVariants crtype.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	junitTable string) *sampleTestDetailsQueryGenerator {
	return &sampleTestDetailsQueryGenerator{
		allJobVariants:  allJobVariants,
		client:          client,
		ReqOptions:      reqOptions,
		IncludeVariants: includeVariants,
		Start:           start,
		End:             end,
		JunitTable:      junitTable,
	}
}

func (s *sampleTestDetailsQueryGenerator) QueryTestStatus(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {

	commonQuery, groupByQuery, queryParameters := getTestDetailsQuery(
		s.client,
		s.ReqOptions,
		s.allJobVariants,
		s.IncludeVariants, s.JunitTable, true)

	sampleString := commonQuery + ` AND branch = @SampleRelease`
	if s.ReqOptions.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND org = @Org AND repo = @Repo AND pr_number = @PRNumber`
	}
	sampleQuery := s.client.BQ.Query(sampleString + groupByQuery)
	sampleQuery.Parameters = append(sampleQuery.Parameters, queryParameters...)
	sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: s.Start,
		},
		{
			Name:  "To",
			Value: s.End,
		},
		{
			Name:  "SampleRelease",
			Value: s.ReqOptions.SampleRelease.Release,
		},
	}...)
	if s.ReqOptions.SampleRelease.PullRequestOptions != nil {
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
			{
				Name:  "Org",
				Value: s.ReqOptions.SampleRelease.PullRequestOptions.Org,
			},
			{
				Name:  "Repo",
				Value: s.ReqOptions.SampleRelease.PullRequestOptions.Repo,
			},
			{
				Name:  "PRNumber",
				Value: s.ReqOptions.SampleRelease.PullRequestOptions.PRNumber,
			},
		}...)
	}

	sampleStatus, errs := fetchJobRunTestStatusResults(ctx, sampleQuery, s.ReqOptions)

	return crtype.JobRunTestReportStatus{SampleStatus: sampleStatus}, errs
}

func fetchJobRunTestStatusResults(ctx context.Context,
	query *bigquery.Query, reqOptions crtype.RequestOptions) (map[string][]crtype.JobRunTestStatusRow, []error) {
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
		prowName := utils.NormalizeProwJobName(testStatus.ProwJob, reqOptions)
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
