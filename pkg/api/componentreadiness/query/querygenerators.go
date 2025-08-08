package query

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/param"
)

const (
	DefaultJunitTable        = "junit"
	jobRunAnnotationToIgnore = "InfraFailure"

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
				jobs.prowjob_start as prowjob_start,
				jobs.org,
				jobs.repo,
				jobs.pr_number,
				jobs.pr_sha,
				jobs.release_verify_tag,
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
			LEFT JOIN %s.job_labels job_labels ON
				junit.prowjob_build_id = job_labels.prowjob_build_id
				AND job_labels.prowjob_start >= DATETIME(@From)
				AND job_labels.prowjob_start < DATETIME(@To)
				AND job_labels.label = '%s'
			WHERE modified_time >= DATETIME(@From)
			AND modified_time < DATETIME(@To)
			AND skipped = false
			AND job_labels.label IS NULL
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
	allVariants crtest.JobVariants
	ReqOptions  reqopts.RequestOptions
}

func NewBaseQueryGenerator(
	client *bqcachedclient.Client,
	reqOptions reqopts.RequestOptions,
	allVariants crtest.JobVariants) baseQueryGenerator {
	generator := baseQueryGenerator{
		client:      client,
		allVariants: allVariants,
		ReqOptions:  reqOptions,
	}
	return generator
}

func (b *baseQueryGenerator) QueryTestStatus(ctx context.Context) (bq.ReportTestStatus, []error) {

	commonQuery, groupByQuery, queryParameters := BuildComponentReportQuery(b.client,
		b.ReqOptions,
		b.allVariants,
		b.ReqOptions.VariantOption.IncludeVariants,
		DefaultJunitTable, false, false)

	before := time.Now()
	errs := []error{}
	baseString := commonQuery + ` AND jv_Release.variant_value = @BaseRelease`
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
			Value: b.ReqOptions.BaseRelease.Name,
		},
	}...)

	baseStatus, baseErrs := FetchTestStatusResults(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Base QueryTestStatus completed in %s with %d base results from db", time.Since(before), len(baseStatus))

	return bq.ReportTestStatus{BaseStatus: baseStatus}, errs
}

type sampleQueryGenerator struct {
	client      *bqcachedclient.Client
	allVariants crtest.JobVariants
	ReqOptions  reqopts.RequestOptions
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
	reqOptions reqopts.RequestOptions,
	allVariants crtest.JobVariants,
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

func (s *sampleQueryGenerator) QueryTestStatus(ctx context.Context) (bq.ReportTestStatus, []error) {
	commonQuery, groupByQuery, queryParameters := BuildComponentReportQuery(s.client, s.ReqOptions,
		s.allVariants, s.IncludeVariants, s.JunitTable, true, false)

	before := time.Now()
	errs := []error{}
	sampleString := commonQuery
	// Only set sample release when PR and payload options are not set
	if s.ReqOptions.SampleRelease.PullRequestOptions == nil && s.ReqOptions.SampleRelease.PayloadOptions == nil {
		sampleString += ` AND jv_Release.variant_value = @SampleRelease`
	}
	if s.ReqOptions.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND org = @Org AND repo = @Repo AND pr_number = @PRNumber`
	}
	if s.ReqOptions.SampleRelease.PayloadOptions != nil {
		sampleString += `  AND release_verify_tag IN UNNEST(@Tags)`
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
	}...)
	if s.ReqOptions.SampleRelease.PullRequestOptions == nil && s.ReqOptions.SampleRelease.PayloadOptions == nil {
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
			{
				Name:  "SampleRelease",
				Value: s.ReqOptions.SampleRelease.Name,
			},
		}...)
	}
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
	if s.ReqOptions.SampleRelease.PayloadOptions != nil {
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
			{
				Name:  "Tags",
				Value: s.ReqOptions.SampleRelease.PayloadOptions.Tags,
			},
		}...)
	}

	sampleStatus, sampleErrs := FetchTestStatusResults(ctx, sampleQuery)

	if len(sampleErrs) != 0 {
		errs = append(errs, sampleErrs...)
	}

	log.Infof("Sample QueryTestStatus completed in %s with %d sample results db", time.Since(before), len(sampleStatus))

	return bq.ReportTestStatus{SampleStatus: sampleStatus}, errs
}

// BuildComponentReportQuery returns the common query for the higher level summary component summary.
func BuildComponentReportQuery(
	client *bqcachedclient.Client,
	reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
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
	// TODO: last_failure here explicitly uses success_val not adjusted_success_val, this ensures we
	// show the last time the test failed, not flaked. if you enable the flakes as failures feature (which is
	// non default today), the last failure time will be wrong which can impact things like failed fix detection.
	queryString := fmt.Sprintf(`WITH latest_component_mapping AS (
						SELECT *
						FROM %s.component_mapping cm
						WHERE created_at = (
								SELECT MAX(created_at)
								FROM %s.component_mapping))
					SELECT
						ANY_VALUE(test_name HAVING MAX prowjob_start) AS test_name,
						ANY_VALUE(testsuite HAVING MAX prowjob_start) AS test_suite,
						cm.id as test_id,
						%s
						COUNT(cm.id) AS total_count,
						SUM(adjusted_success_val) AS success_count,
						SUM(adjusted_flake_count) AS flake_count,
						MAX(CASE WHEN success_val = 0 THEN prowjob_start ELSE NULL END) AS last_failure,
						ANY_VALUE(cm.component) AS component,
						ANY_VALUE(cm.capabilities) AS capabilities,
					FROM (%s)
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name
`,
		client.Dataset, client.Dataset, selectVariants, fmt.Sprintf(dedupedJunitTable, jobNameQueryPortion, client.Dataset, junitTable, client.Dataset, client.Dataset, jobRunAnnotationToIgnore))

	queryString += joinVariants

	groupString := fmt.Sprintf(`
					GROUP BY
						%s
						cm.id `, groupByVariants)

	queryString += `WHERE cm.staff_approved_obsolete = false AND
						(variant_registry_job_name LIKE 'periodic-%%' OR variant_registry_job_name LIKE 'release-%%' OR variant_registry_job_name LIKE 'aggregator-%%')`
	commonParams := []bigquery.QueryParameter{}
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

		// In this context, a component report, multiple test ID options should not be specified. Thus
		// here we assume just one for the filtering purposes here. This code triggers as you drill down
		// on a main report into component > capability > tests, but it does not get used on a test details page.
		if len(reqOptions.TestIDOptions) == 1 {
			for _, group := range sortedKeys(reqOptions.TestIDOptions[0].RequestedVariants) {
				group = param.Cleanse(group) // should be clean already, but just to make sure
				paramName := fmt.Sprintf("ReqVariant_%s", group)
				queryString += fmt.Sprintf(` AND jv_%s.variant_value = @%s`, group, paramName)
				commonParams = append(commonParams, bigquery.QueryParameter{
					Name:  paramName,
					Value: reqOptions.TestIDOptions[0].RequestedVariants[group],
				})
			}
			if reqOptions.TestIDOptions[0].Capability != "" {
				queryString += " AND @Capability in UNNEST(capabilities)"
				commonParams = append(commonParams, bigquery.QueryParameter{
					Name:  "Capability",
					Value: reqOptions.TestIDOptions[0].Capability,
				})
			}
			if reqOptions.TestIDOptions[0].TestID != "" {
				queryString += ` AND cm.id = @TestId`
				commonParams = append(commonParams, bigquery.QueryParameter{
					Name:  "TestId",
					Value: reqOptions.TestIDOptions[0].TestID,
				})
			}
		}
	}

	return queryString, groupString, commonParams
}

// buildTestDetailsQuery returns the report for a specific test + variant combo, including job run data.
// This is for the bottom level most specific pages in component readiness.
// TODO: I think we're querying more than we need here, there are a lot of long columns returned in this query that are
// never used, test name, component, file path, url, etc.
func buildTestDetailsQuery(
	client *bqcachedclient.Client,
	testIDOpts []reqopts.TestIdentification,
	c reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	junitTable string,
	isSample bool) (string, string, []bigquery.QueryParameter) {

	jobNameQueryPortion := normalJobNameCol
	if c.SampleRelease.PullRequestOptions != nil && isSample {
		jobNameQueryPortion = pullRequestDynamicJobNameCol
	}

	// Because this query can now be used with multiple test id / variant combos, we need to return dynamic variant
	// columns so we can separate the results in code later.
	selectVariants := ""
	groupByVariants := ""
	joinVariants := ""
	for _, variant := range sortedKeys(allJobVariants.Variants) {
		v := param.Cleanse(variant) // should be clean anyway, but just to make sure
		joinVariants += fmt.Sprintf("LEFT JOIN %s.job_variants jv_%s ON variant_registry_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			client.Dataset, v, v, v, v)
	}
	for _, v := range c.VariantOption.DBGroupBy.List() {
		v = param.Cleanse(v)
		selectVariants += fmt.Sprintf("jv_%s.variant_value AS variant_%s,\n", v, v) // Note: Variants are camelcase, so the query columns come back like: variant_Architecture
		groupByVariants += fmt.Sprintf("jv_%s.variant_value,\n", v)
	}

	queryString := fmt.Sprintf(`
					WITH latest_component_mapping AS (
						SELECT *
							FROM
								%s.component_mapping cm
							WHERE
								created_at = (SELECT MAX(created_at) FROM %s.component_mapping)
					)
					SELECT
						cm.id AS test_id,
						ANY_VALUE(test_name) AS test_name,
						ANY_VALUE(testsuite) AS test_suite,
						%s
						file_path,
						ANY_VALUE(variant_registry_job_name) AS prowjob_name,
						ANY_VALUE(cm.jira_component) AS jira_component,
						ANY_VALUE(cm.jira_component_id) AS jira_component_id,
						COUNT(*) AS total_count,
						ANY_VALUE(jobs.prowjob_url) AS prowjob_url,
						ANY_VALUE(jobs.prowjob_build_id) AS prowjob_run_id,
						ANY_VALUE(jobs.prowjob_start) AS prowjob_start,
						ANY_VALUE(cm.capabilities) as capabilities,
						SUM(adjusted_success_val) AS success_count,
						SUM(adjusted_flake_count) AS flake_count,
					FROM (%s) junit
					INNER JOIN %s.jobs jobs ON junit.prowjob_build_id = jobs.prowjob_build_id
					INNER JOIN latest_component_mapping cm ON testsuite = cm.suite AND test_name = cm.name
`, client.Dataset, client.Dataset, selectVariants, fmt.Sprintf(dedupedJunitTable, jobNameQueryPortion, client.Dataset, junitTable, client.Dataset, client.Dataset, jobRunAnnotationToIgnore), client.Dataset)

	queryString += joinVariants

	groupString := fmt.Sprintf(`
					GROUP BY
						%s
						file_path,
						modified_time,
                        cm.id
					ORDER BY
						modified_time `, groupByVariants)
	queryString += `
					WHERE
						(variant_registry_job_name LIKE 'periodic-%%' OR variant_registry_job_name LIKE 'release-%%' OR variant_registry_job_name LIKE 'aggregator-%%')
						AND
`
	commonParams := []bigquery.QueryParameter{}

	queryString += "("
	for i, testIDOption := range testIDOpts {
		queryString = addTestFilters(testIDOption, i, queryString, c, includeVariants)

	}
	queryString += ")"

	if isSample {
		queryString += filterByCrossCompareVariants(c.VariantOption.VariantCrossCompare, c.VariantOption.CompareVariants, &commonParams)
		// Only set sample release when PR and payload options are not set
		if c.SampleRelease.PayloadOptions == nil && c.SampleRelease.PullRequestOptions == nil {
			queryString += ` AND jv_Release.variant_value = @SampleRelease`
		}
	} else {
		queryString += filterByCrossCompareVariants(c.VariantOption.VariantCrossCompare, includeVariants, &commonParams)
		queryString += ` AND jv_Release.variant_value = @BaseRelease`
	}
	return queryString, groupString, commonParams
}

// addTestFilters injects query params to limit to one test and variants combo.
func addTestFilters(
	testIDOption reqopts.TestIdentification,
	index int,
	queryString string,
	c reqopts.RequestOptions,
	includeVariants map[string][]string) string {

	if index > 0 {
		queryString += " OR "
	}

	queryString += fmt.Sprintf(`(cm.id = '%s'

`, param.Cleanse(testIDOption.TestID))

	for _, key := range sortedKeys(includeVariants) {
		// only add in include variants that aren't part of the requested or cross-compared variants
		if _, ok := testIDOption.RequestedVariants[key]; ok {
			continue
		}
		if slices.Contains(c.VariantOption.VariantCrossCompare, key) {
			continue
		}

		group := param.Cleanse(key)
		queryString += fmt.Sprintf(` AND jv_%s.variant_value IN UNNEST(%s)`, group,
			FormatStringSliceForBigQuery(c.VariantOption.IncludeVariants[key]))
	}

	for _, group := range sortedKeys(testIDOption.RequestedVariants) {
		group = param.Cleanse(group) // should be clean anyway, but just to make sure
		queryString += fmt.Sprintf(` AND jv_%s.variant_value = "%s"`, group, param.Cleanse(testIDOption.RequestedVariants[group]))
	}
	queryString += `)
`
	return queryString
}

// FormatStringSliceForBigQuery takes a slice of strings and returns a formatted
// string suitable for use in a BigQuery UNNEST clause (e.g., ["crun", "runc"]).
func FormatStringSliceForBigQuery(sl []string) string {
	quotedStrings := make([]string, len(sl))
	for i, s := range sl {
		quotedStrings[i] = fmt.Sprintf("\"%s\"", param.Cleanse(s))
	}
	return fmt.Sprintf("[%s]", strings.Join(quotedStrings, ", "))
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

func FetchTestStatusResults(ctx context.Context, query *bigquery.Query) (map[string]bq.TestStatus, []error) {
	errs := []error{}
	status := map[string]bq.TestStatus{}

	logQueryWithParamsReplaced(log.WithField("type", "ComponentReport"), query)
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
			deserializationErr := errors.Wrap(err, "error deserializing row from bigquery")
			log.Error(deserializationErr.Error())
			errs = append(errs, deserializationErr)
			continue
		}

		status[testIDStr] = testStatus
	}
	return status, errs
}

// deserializeRowToTestStatus deserializes a single row into a testID string and matching status.
// This is where we handle the dynamic variant_ columns, parsing these into a map on the test identification.
// Other fixed columns we expect are serialized directly to their appropriate columns.
func deserializeRowToTestStatus(row []bigquery.Value, schema bigquery.Schema) (string, bq.TestStatus, error) {
	if len(row) != len(schema) {
		log.Infof("row is %+v, schema is %+v", row, schema)
		return "", bq.TestStatus{}, fmt.Errorf("number of values in row doesn't match schema length")
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
	tid := crtest.KeyWithVariants{
		Variants: map[string]string{},
	}
	cts := bq.TestStatus{}
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

	return tid.KeyOrDie(), cts, nil
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
	logger         log.FieldLogger
	client         *bqcachedclient.Client
	ReqOptions     reqopts.RequestOptions
	allJobVariants crtest.JobVariants
	BaseRelease    string
	BaseStart      time.Time
	BaseEnd        time.Time
	TestIDOpts     []reqopts.TestIdentification
}

func NewBaseTestDetailsQueryGenerator(logger log.FieldLogger, client *bqcachedclient.Client,
	reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	baseRelease string, baseStart time.Time, baseEnd time.Time,
	testIDOpts []reqopts.TestIdentification) *baseTestDetailsQueryGenerator {

	return &baseTestDetailsQueryGenerator{
		logger:         logger,
		client:         client,
		ReqOptions:     reqOptions,
		allJobVariants: allJobVariants,
		BaseRelease:    baseRelease,
		BaseEnd:        baseEnd,
		BaseStart:      baseStart,
		TestIDOpts:     testIDOpts,
	}
}

func (b *baseTestDetailsQueryGenerator) QueryTestStatus(ctx context.Context) (bq.TestJobRunStatuses, []error) {
	commonQuery, groupByQuery, queryParameters := buildTestDetailsQuery(
		b.client,
		b.TestIDOpts,
		b.ReqOptions,
		b.allJobVariants,
		b.ReqOptions.VariantOption.IncludeVariants, DefaultJunitTable, false)
	baseString := commonQuery
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

	baseStatus, errs := fetchJobRunTestStatusResults(ctx, b.logger, baseQuery, b.ReqOptions)
	return bq.TestJobRunStatuses{BaseStatus: baseStatus}, errs
}

// sampleTestDetailsQueryGenerator generates the query we use for the sample on the test details page.
type sampleTestDetailsQueryGenerator struct {
	allJobVariants crtest.JobVariants
	client         *bqcachedclient.Client
	ReqOptions     reqopts.RequestOptions

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
	reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
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

func (s *sampleTestDetailsQueryGenerator) QueryTestStatus(ctx context.Context) (bq.TestJobRunStatuses, []error) {

	commonQuery, groupByQuery, queryParameters := buildTestDetailsQuery(
		s.client,
		s.ReqOptions.TestIDOptions,
		s.ReqOptions,
		s.allJobVariants,
		s.IncludeVariants, s.JunitTable, true)

	sampleString := commonQuery
	if s.ReqOptions.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND jobs.org = @Org AND jobs.repo = @Repo AND jobs.pr_number = @PRNumber`
	}
	if s.ReqOptions.SampleRelease.PayloadOptions != nil {
		sampleString += `  AND jobs.release_verify_tag IN UNNEST(@Tags)`
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
	}...)
	if s.ReqOptions.SampleRelease.PullRequestOptions == nil && s.ReqOptions.SampleRelease.PayloadOptions == nil {
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
			{
				Name:  "SampleRelease",
				Value: s.ReqOptions.SampleRelease.Name,
			},
		}...)
	}
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
	if s.ReqOptions.SampleRelease.PayloadOptions != nil {
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
			{
				Name:  "Tags",
				Value: s.ReqOptions.SampleRelease.PayloadOptions.Tags,
			},
		}...)
	}

	sampleStatus, errs := fetchJobRunTestStatusResults(ctx, log.WithField("generator", "SampleQuery"), sampleQuery, s.ReqOptions)

	return bq.TestJobRunStatuses{SampleStatus: sampleStatus}, errs
}

// logQueryWithParamsReplaced is intended to give developers a query they can copy out of logs and work with directly,
// which has all the parameters replaced. This query is NOT the one we run live, we let bigquery do it's param replacement
// itself.
// Without this, logrus logs the query in one line with everything escaped, and parameters have to be manually replaced by the user.
// This will only log if we're logging at Debug level.
func logQueryWithParamsReplaced(logger log.FieldLogger, query *bigquery.Query) {
	if log.GetLevel() == log.DebugLevel {
		// Attempt to log a usable version of the query with params swapped in.
		strQuery := query.Q
		for _, p := range query.Parameters {
			paramName := "@" + p.Name
			paramValue := p.Value

			// Special handling for time.Time values
			if t, ok := paramValue.(time.Time); ok {
				// Format time.Time to "YYYY-MM-DD HH:MM:SS"
				// Note: BigQuery's DATETIME type does not store timezone info.
				// This format aligns with what BigQuery expects for DATETIME literals.
				// Without it, you'll copy the query and attempt to run it and be told you're not filtering on
				// modified time.
				formattedTime := t.Format("2006-01-02 15:04:05")
				strQuery = strings.ReplaceAll(strQuery, paramName, fmt.Sprintf(`DATETIME("%s")`, formattedTime))
			} else {
				// Default handling for other types, wrap in quotes for string literals
				strQuery = strings.ReplaceAll(strQuery, paramName, fmt.Sprintf(`"%v"`, paramValue))
			}
		}
		logger.Debugf("fetching bigquery data with query:")
		fmt.Println(strQuery)
	}
}

func fetchJobRunTestStatusResults(ctx context.Context, logger log.FieldLogger, query *bigquery.Query, reqOptions reqopts.RequestOptions) (map[string][]bq.TestJobRunRows, []error) {
	errs := []error{}
	status := map[string][]bq.TestJobRunRows{}

	logQueryWithParamsReplaced(logger.WithField("type", "TestDetails"), query)

	it, err := query.Read(ctx)
	if err != nil {
		logger.WithError(err).Error("error querying job run test status from bigquery")
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
			logger.WithError(err).Error("error parsing component from bigquery")
			errs = append(errs, errors.Wrap(err, "error parsing prowjob from bigquery"))
			continue
		}

		jobRunTestStatusRow, err := deserializeRowToJobRunTestReportStatus(row, it.Schema)
		if err != nil {
			err2 := errors.Wrap(err, "error deserializing row from bigquery")
			logger.Error(err2.Error())
			errs = append(errs, err2)
			continue
		}
		prowName := utils.NormalizeProwJobName(jobRunTestStatusRow.ProwJob, reqOptions)
		status[prowName] = append(status[prowName], jobRunTestStatusRow)
	}
	return status, errs
}

// deserializeRowToJobRunTestReportStatus deserializes a single row into a testID string and matching status.
// This is where we handle the dynamic variant_ columns, parsing these into a map on the test identification.
// Other fixed columns we expect are serialized directly to their appropriate columns.
func deserializeRowToJobRunTestReportStatus(row []bigquery.Value, schema bigquery.Schema) (bq.TestJobRunRows, error) {
	if len(row) != len(schema) {
		log.Infof("row is %+v, schema is %+v", row, schema)
		return bq.TestJobRunRows{}, fmt.Errorf("number of values in row doesn't match schema length")
	}

	cts := bq.TestJobRunRows{
		TestKey: crtest.KeyWithVariants{Variants: map[string]string{}},
	}
	for i, fieldSchema := range schema {
		col := fieldSchema.Name
		// Some rows we know what to expect, others are dynamic (variants) and go into the map.
		switch {
		case col == "total_count":
			cts.TotalCount = int(row[i].(int64))
		case col == "success_count":
			cts.SuccessCount = int(row[i].(int64))
		case col == "flake_count":
			cts.FlakeCount = int(row[i].(int64))
		case col == "prowjob_name":
			cts.ProwJob = row[i].(string)
		case col == "prowjob_run_id":
			cts.ProwJobRunID = row[i].(string)
		case col == "prowjob_url":
			if row[i] != nil {
				cts.ProwJobURL = row[i].(string)
			}
		case col == "prowjob_start":
			cts.StartTime = row[i].(civil.DateTime)
		case col == "test_id":
			cts.TestKey.TestID = row[i].(string)
		case col == "test_name":
			cts.TestName = row[i].(string)
		case col == "jira_component":
			cts.JiraComponent = row[i].(string)
		case col == "jira_component_id":
			cts.JiraComponentID = row[i].(*big.Rat)
		case strings.HasPrefix(col, "variant_"):
			variantName := col[len("variant_"):]
			if row[i] != nil {
				cts.TestKey.Variants[variantName] = row[i].(string)
			}
		default:
		}
	}

	// Serialize the test key once only so we don't have to keep recalculating
	cts.TestKeyStr = cts.TestKey.KeyOrDie()

	return cts, nil
}
