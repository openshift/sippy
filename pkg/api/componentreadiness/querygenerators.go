package componentreadiness

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/param"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const (
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

type baseQueryGenerator struct {
	client                   *bqcachedclient.Client
	cacheOption              cache.RequestOptions
	allVariants              crtype.JobVariants
	ComponentReportGenerator *componentReportGenerator
}

func newBaseQueryGenerator(c *componentReportGenerator, allVariants crtype.JobVariants) baseQueryGenerator {
	generator := baseQueryGenerator{
		client:      c.client,
		allVariants: allVariants,
		cacheOption: cache.RequestOptions{
			ForceRefresh: c.cacheOption.ForceRefresh,
			// increase the time that base query is cached for since it shouldn't be changing?
			CRTimeRoundingFactor: c.cacheOption.CRTimeRoundingFactor,
		},
		ComponentReportGenerator: c,
	}
	return generator
}

func (b *baseQueryGenerator) queryTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {

	commonQuery, groupByQuery, queryParameters := getCommonTestStatusQuery(b.ComponentReportGenerator,
		b.allVariants, defaultJunitTable, false, false)

	before := time.Now()
	errs := []error{}
	baseString := commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.client.BQ.Query(baseString + groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, queryParameters...)
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

	baseStatus, baseErrs := fetchTestStatusResults(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Base QueryTestStatus completed in %s with %d base results from db", time.Since(before), len(baseStatus))

	return crtype.ReportTestStatus{BaseStatus: baseStatus}, errs
}

type sampleQueryGenerator struct {
	client                   *bqcachedclient.Client
	allVariants              crtype.JobVariants
	ComponentReportGenerator *componentReportGenerator
	// JunitTable is the bigquery table (in the normal dataset configured), where this sample query generator should
	// pull its data from. It is a public field as we want it included in the cache
	// key to differentiate this request from other sample queries that might be using a junit table override.
	// Normally, this would just be the default junit table, but in some cases we pull from other tables. (rarely run jobs)
	JunitTable string
	// IncludeVariants is a potentially slightly adjusted copy of the ComponentReportGenerator, used in conjunction with
	// junit table overrides to tweak the query.
	IncludeVariants map[string][]string
}

func newSampleQueryGenerator(
	c *componentReportGenerator,
	allVariants crtype.JobVariants,
	includeVariants map[string][]string,
	junitTable string) sampleQueryGenerator {

	generator := sampleQueryGenerator{
		client:                   c.client,
		allVariants:              allVariants,
		ComponentReportGenerator: c,
		JunitTable:               junitTable,
		IncludeVariants:          includeVariants,
	}
	return generator
}

func (s *sampleQueryGenerator) queryTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {
	commonQuery, groupByQuery, queryParameters := getCommonTestStatusQuery(s.ComponentReportGenerator,
		s.allVariants, s.JunitTable, true, false)

	before := time.Now()
	errs := []error{}
	sampleString := commonQuery + ` AND branch = @SampleRelease`
	if s.ComponentReportGenerator.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND org = @Org AND repo = @Repo AND pr_number = @PRNumber`
	}
	sampleQuery := s.client.BQ.Query(sampleString + groupByQuery)
	sampleQuery.Parameters = append(sampleQuery.Parameters, queryParameters...)
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

	sampleStatus, sampleErrs := fetchTestStatusResults(ctx, sampleQuery)

	if len(sampleErrs) != 0 {
		errs = append(errs, sampleErrs...)
	}

	log.Infof("Sample QueryTestStatus completed in %s with %d sample results db", time.Since(before), len(sampleStatus))

	return crtype.ReportTestStatus{SampleStatus: sampleStatus}, errs
}

// fallbackTestQueryReleasesGenerator iterates the configured number of past releases, querying base status for
// each, which can then be used to return the best basis data from those past releases for comparison.
type fallbackTestQueryReleasesGenerator struct {
	client                     *bqcachedclient.Client
	cacheOption                cache.RequestOptions
	allVariants                crtype.JobVariants
	BaseRelease                string
	BaseStart                  time.Time
	BaseEnd                    time.Time
	CachedFallbackTestStatuses crtype.FallbackReleases
	lock                       *sync.Mutex
	ComponentReportGenerator   *componentReportGenerator
}

func newFallbackTestQueryReleasesGenerator(c *componentReportGenerator, allVariants crtype.JobVariants,
	release string, start, end time.Time) fallbackTestQueryReleasesGenerator {
	generator := fallbackTestQueryReleasesGenerator{
		client:      c.client,
		allVariants: allVariants,
		cacheOption: cache.RequestOptions{
			ForceRefresh: c.cacheOption.ForceRefresh,
			// increase the time that fallback queries are cached for
			CRTimeRoundingFactor: fallbackQueryTimeRoundingOverride,
		},
		BaseRelease:              release,
		BaseStart:                start,
		BaseEnd:                  end,
		lock:                     &sync.Mutex{},
		ComponentReportGenerator: c,
	}
	return generator

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

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackRelease(ctx context.Context, release string, start, end time.Time) (crtype.ReportTestStatus, []error) {
	generator := newFallbackBaseQueryGenerator(f.ComponentReportGenerator, f.allVariants, release, start, end)

	testStatuses, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, f.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("FallbackBaseTestStatus~", generator), generator.getTestFallbackRelease, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return crtype.ReportTestStatus{}, errs
	}

	return testStatuses, nil
}

type fallbackTestQueryGenerator struct {
	client                   *bqcachedclient.Client
	cacheOption              cache.RequestOptions
	allVariants              crtype.JobVariants
	BaseRelease              string
	BaseStart                time.Time
	BaseEnd                  time.Time
	ComponentReportGenerator *componentReportGenerator
}

func newFallbackBaseQueryGenerator(c *componentReportGenerator, allVariants crtype.JobVariants,
	baseRelease string, baseStart, baseEnd time.Time) fallbackTestQueryGenerator {
	generator := fallbackTestQueryGenerator{
		ComponentReportGenerator: c,
		client:                   c.client,
		allVariants:              allVariants,
		cacheOption: cache.RequestOptions{
			ForceRefresh: c.cacheOption.ForceRefresh,
			// increase the time that base query is cached for since it shouldn't be changing
			CRTimeRoundingFactor: fallbackQueryTimeRoundingOverride,
		},
		BaseRelease: baseRelease,
		BaseStart:   baseStart,
		BaseEnd:     baseEnd,
	}
	return generator
}

func (f *fallbackTestQueryGenerator) getTestFallbackRelease(ctx context.Context) (crtype.ReportTestStatus, []error) {
	commonQuery, groupByQuery, queryParameters := getCommonTestStatusQuery(f.ComponentReportGenerator,
		f.allVariants, defaultJunitTable, false, true)
	before := time.Now()
	log.Infof("Starting Fallback (%s) QueryTestStatus", f.BaseRelease)
	errs := []error{}
	baseString := commonQuery + ` AND branch = @BaseRelease`
	baseQuery := f.client.BQ.Query(baseString + groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: f.BaseStart,
		},
		{
			Name:  "To",
			Value: f.BaseEnd,
		},
		{
			Name:  "BaseRelease",
			Value: f.BaseRelease,
		},
	}...)

	baseStatus, baseErrs := fetchTestStatusResults(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Fallback (%s) QueryTestStatus completed in %s with %d base results from db", f.BaseRelease, time.Since(before), len(baseStatus))

	return crtype.ReportTestStatus{BaseStatus: baseStatus}, errs
}

// getCommonTestStatusQuery returns the common query for the higher level summary component summary.
func getCommonTestStatusQuery(
	c *componentReportGenerator,
	allJobVariants crtype.JobVariants,
	junitTable string,
	isSample, isFallback bool) (string, string, []bigquery.QueryParameter) {
	// Parts of the query, including the columns returned, are dynamic, based on the list of variants we're told to work with.
	// Variants will be returned as columns with names like: variant_[VariantName]
	// See fetchTestStatusResults for where we dynamically handle these columns.
	selectVariants := ""
	joinVariants := ""
	groupByVariants := ""
	for _, v := range sortedKeys(allJobVariants.Variants) {
		joinVariants += fmt.Sprintf("LEFT JOIN %s.job_variants jv_%s ON variant_registry_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			c.client.Dataset, v, v, v, v)
	}
	for _, v := range c.DBGroupBy.List() {
		v = param.Cleanse(v)
		selectVariants += fmt.Sprintf("jv_%s.variant_value AS variant_%s,\n", v, v) // Note: Variants are camelcase, so the query columns come back like: variant_Architecture
		groupByVariants += fmt.Sprintf("jv_%s.variant_value,\n", v)
	}

	jobNameQueryPortion := normalJobNameCol
	if c.SampleRelease.PullRequestOptions != nil && isSample {
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

		for _, group := range sortedKeys(variantGroups) {
			group = param.Cleanse(group) // should be clean already, but just to make sure
			paramName := fmt.Sprintf("variantGroup_%s", group)
			queryString += fmt.Sprintf(" AND (jv_%s.variant_value in UNNEST(@%s))", group, paramName)
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  paramName,
				Value: variantGroups[group],
			})
		}

		for _, group := range sortedKeys(c.RequestedVariants) {
			group = param.Cleanse(group) // should be clean already, but just to make sure
			paramName := fmt.Sprintf("ReqVariant_%s", group)
			queryString += fmt.Sprintf(` AND jv_%s.variant_value = @%s`, group, paramName)
			commonParams = append(commonParams, bigquery.QueryParameter{
				Name:  paramName,
				Value: c.RequestedVariants[group],
			})
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

// getTestDetailsQuery returns the report for a specific test + variant combo, including job run data.
// This is for the bottom level most specific pages in component readiness.
func getTestDetailsQuery(c *componentReportGenerator, allJobVariants crtype.JobVariants, isSample bool) (string, string, []bigquery.QueryParameter) {
	jobNameQueryPortion := normalJobNameCol
	if c.SampleRelease.PullRequestOptions != nil && isSample {
		jobNameQueryPortion = pullRequestDynamicJobNameCol
	}

	// TODO: this is a temporary hack while we explore if rarely run jobs approach is actually going to work.
	// A scheduled query is copying rarely run job results to a separate much smaller table every day, so we can
	// query 3 months without spending a fortune. If this proves to work, we will work out a system of processing
	// this as generically as we can, but it will be difficult.
	// TODO: remove and pass junit table in
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
	for _, variant := range sortedKeys(allJobVariants.Variants) {
		v := param.Cleanse(variant) // should be clean anyway, but just to make sure
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
	commonParams := []bigquery.QueryParameter{
		{
			Name:  "IgnoredJobs",
			Value: ignoredJobsRegexp,
		},
		{
			Name:  "TestId",
			Value: c.TestID,
		},
	}

	for _, key := range sortedKeys(c.IncludeVariants) {
		// only add in include variants that aren't part of the requested or cross-compared variants

		if _, ok := c.RequestedVariants[key]; ok {
			continue
		}
		if slices.Contains(c.VariantCrossCompare, key) {
			continue
		}

		group := param.Cleanse(key)
		paramName := "IncludeVariants" + group
		queryString += fmt.Sprintf(` AND jv_%s.variant_value IN UNNEST(@%s)`, group, paramName)
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  paramName,
			Value: c.IncludeVariants[key],
		})
	}

	for _, group := range sortedKeys(c.RequestedVariants) {
		group = param.Cleanse(group) // should be clean anyway, but just to make sure
		paramName := "IncludeVariantValue" + group
		queryString += fmt.Sprintf(` AND jv_%s.variant_value = @%s`, group, paramName)
		commonParams = append(commonParams, bigquery.QueryParameter{
			Name:  paramName,
			Value: c.RequestedVariants[group],
		})
	}
	if isSample {
		queryString += filterByCrossCompareVariants(c.VariantCrossCompare, c.CompareVariants, &commonParams)
	} else {
		queryString += filterByCrossCompareVariants(c.VariantCrossCompare, c.IncludeVariants, &commonParams)
	}
	return queryString, groupString, commonParams
}

func fetchTestStatusResults(ctx context.Context, query *bigquery.Query) (map[string]crtype.TestStatus, []error) {
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
	cacheOption              cache.RequestOptions
	allJobVariants           crtype.JobVariants
	BaseRelease              string
	BaseStart                time.Time
	BaseEnd                  time.Time
	ComponentReportGenerator *componentReportGenerator
}

func newBaseTestDetailsQueryGenerator(c *componentReportGenerator, allJobVariants crtype.JobVariants,
	baseRelease string, baseStart time.Time, baseEnd time.Time) *baseTestDetailsQueryGenerator {
	return &baseTestDetailsQueryGenerator{
		allJobVariants: allJobVariants,
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
}

func (b *baseTestDetailsQueryGenerator) queryTestStatus(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {
	commonQuery, groupByQuery, queryParameters := getTestDetailsQuery(b.ComponentReportGenerator, b.allJobVariants, false)
	baseString := commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.ComponentReportGenerator.client.BQ.Query(baseString + groupByQuery)

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

	baseStatus, errs := b.ComponentReportGenerator.fetchJobRunTestStatusResults(ctx, baseQuery)
	return crtype.JobRunTestReportStatus{BaseStatus: baseStatus}, errs
}

// sampleTestDetailsQueryGenerator generates the query we use for the sample on the test details page.
type sampleTestDetailsQueryGenerator struct {
	allJobVariants           crtype.JobVariants
	ComponentReportGenerator *componentReportGenerator
}

func newSampleTestDetailsQueryGenerator(c *componentReportGenerator, allJobVariants crtype.JobVariants) *sampleTestDetailsQueryGenerator {
	return &sampleTestDetailsQueryGenerator{
		allJobVariants:           allJobVariants,
		ComponentReportGenerator: c,
	}
}

func (s *sampleTestDetailsQueryGenerator) queryTestStatus(ctx context.Context) (crtype.JobRunTestReportStatus, []error) {

	commonQuery, groupByQuery, queryParameters := getTestDetailsQuery(s.ComponentReportGenerator, s.allJobVariants, true)

	sampleString := commonQuery + ` AND branch = @SampleRelease`
	if s.ComponentReportGenerator.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND org = @Org AND repo = @Repo AND pr_number = @PRNumber`
	}
	sampleQuery := s.ComponentReportGenerator.client.BQ.Query(sampleString + groupByQuery)
	sampleQuery.Parameters = append(sampleQuery.Parameters, queryParameters...)
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

	sampleStatus, errs := s.ComponentReportGenerator.fetchJobRunTestStatusResults(ctx, sampleQuery)

	return crtype.JobRunTestReportStatus{SampleStatus: sampleStatus}, errs
}
