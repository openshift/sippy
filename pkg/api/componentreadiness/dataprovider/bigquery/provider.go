package bigquery

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	apiPkg "github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	apiCache "github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/util/param"
	"github.com/openshift/sippy/pkg/util/sets"
)

var _ dataprovider.DataProvider = &BigQueryProvider{}

// BigQueryProvider implements dataprovider.DataProvider using Google BigQuery
// as the backing data store, wrapping the existing query generators.
type BigQueryProvider struct {
	client *bqcachedclient.Client
}

func NewBigQueryProvider(client *bqcachedclient.Client) *BigQueryProvider {
	return &BigQueryProvider{client: client}
}

// Client returns the underlying BigQuery client for callers that still need direct access
// during the migration period.
func (p *BigQueryProvider) Client() *bqcachedclient.Client {
	return p.client
}

func (p *BigQueryProvider) Cache() apiCache.Cache {
	return p.client.Cache
}

// --- TestStatusQuerier ---

func (p *BigQueryProvider) QueryBaseTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants) (map[string]crstatus.TestStatus, []error) {

	generator := NewBaseQueryGenerator(p.client, reqOptions, allJobVariants)
	result, errs := apiPkg.GetDataFromCacheOrGenerate[crstatus.ReportTestStatus](
		ctx, p.client.Cache, reqOptions.CacheOption,
		apiPkg.NewCacheSpec(generator, "BaseTestStatus~", &reqOptions.BaseRelease.End),
		generator.QueryTestStatus, crstatus.ReportTestStatus{})
	if len(errs) > 0 {
		return nil, errs
	}
	return result.BaseStatus, nil
}

func (p *BigQueryProvider) QuerySampleTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time) (map[string]crstatus.TestStatus, []error) {

	generator := NewSampleQueryGenerator(p.client, reqOptions, allJobVariants, includeVariants, start, end)
	result, errs := apiPkg.GetDataFromCacheOrGenerate[crstatus.ReportTestStatus](
		ctx, p.client.Cache, reqOptions.CacheOption,
		apiPkg.NewCacheSpec(generator, "SampleTestStatus~", &reqOptions.SampleRelease.End),
		generator.QueryTestStatus, crstatus.ReportTestStatus{})
	if len(errs) > 0 {
		return nil, errs
	}
	return result.SampleStatus, nil
}

// --- TestDetailsQuerier ---

func (p *BigQueryProvider) QueryBaseJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants) (map[string][]crstatus.TestJobRunRows, []error) {

	generator := NewBaseTestDetailsQueryGenerator(
		log.WithField("func", "QueryBaseJobRunTestStatus"),
		p.client, reqOptions, allJobVariants,
		reqOptions.BaseRelease.Name, reqOptions.BaseRelease.Start, reqOptions.BaseRelease.End,
		reqOptions.TestIDOptions)

	result, errs := apiPkg.GetDataFromCacheOrGenerate[crstatus.TestJobRunStatuses](
		ctx, p.client.Cache, reqOptions.CacheOption,
		apiPkg.NewCacheSpec(generator, "BaseJobRunTestStatusV2~", &reqOptions.BaseRelease.End),
		generator.QueryTestStatus, crstatus.TestJobRunStatuses{})
	if len(errs) > 0 {
		return nil, errs
	}
	return result.BaseStatus, nil
}

func (p *BigQueryProvider) QuerySampleJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time) (map[string][]crstatus.TestJobRunRows, []error) {

	generator := NewSampleTestDetailsQueryGenerator(p.client, reqOptions, allJobVariants, includeVariants, start, end)
	result, errs := apiPkg.GetDataFromCacheOrGenerate[crstatus.TestJobRunStatuses](
		ctx, p.client.Cache, reqOptions.CacheOption,
		apiPkg.NewCacheSpec(generator, "SampleJobRunTestStatusV2~", &end),
		generator.QueryTestStatus, crstatus.TestJobRunStatuses{})
	if len(errs) > 0 {
		return nil, errs
	}
	return result.SampleStatus, nil
}

// --- MetadataQuerier ---

func (p *BigQueryProvider) QueryJobVariants(ctx context.Context) (crtest.JobVariants, []error) {
	variants := crtest.JobVariants{Variants: map[string][]string{}}
	queryString := fmt.Sprintf(`SELECT variant_name, ARRAY_AGG(DISTINCT variant_value ORDER BY variant_value) AS variant_values
					FROM
						%s.job_variants
					WHERE
						variant_value!=""
					GROUP BY
						variant_name`, p.client.Dataset)
	q := p.client.Query(ctx, bqlabel.CRJobVariants, queryString)
	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Errorf("error querying variants from bigquery")
		return variants, []error{err}
	}

	floatVariants := sets.NewString("FromRelease", "FromReleaseMajor", "FromReleaseMinor", "Release", "ReleaseMajor", "ReleaseMinor")
	for {
		row := crstatus.JobVariant{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error fetching variants from bigquery")
			return variants, []error{err}
		}

		if floatVariants.Has(row.VariantName) {
			sort.Slice(row.VariantValues, func(i, j int) bool {
				iStrings := strings.Split(row.VariantValues[i], ".")
				jStrings := strings.Split(row.VariantValues[j], ".")
				for idx, iString := range iStrings {
					if idx >= len(jStrings) {
						return false
					}
					if iValue, err := strconv.ParseInt(iString, 10, 32); err == nil {
						if jValue, err := strconv.ParseInt(jStrings[idx], 10, 32); err == nil {
							if iValue != jValue {
								return iValue < jValue
							}
						}
					}
				}
				return len(iStrings) < len(jStrings)
			})
		}
		variants.Variants[row.VariantName] = row.VariantValues
	}
	return variants, nil
}

func (p *BigQueryProvider) QueryReleaseDates(ctx context.Context, reqOptions reqopts.RequestOptions) ([]crtest.ReleaseTimeRange, []error) {
	return GetReleaseDatesFromBigQuery(ctx, p.client, reqOptions)
}

func (p *BigQueryProvider) QueryReleases(ctx context.Context) ([]v1.Release, error) {
	return apiPkg.GetReleasesFromBigQuery(ctx, p.client)
}

func (p *BigQueryProvider) QueryUniqueVariantValues(ctx context.Context, field string, nested bool) ([]string, error) {
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
						modified_time > DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 60 DAY)
					ORDER BY
						name`, field, p.client.Dataset, unnest)

	q := p.client.Query(ctx, bqlabel.CRJunitColumnCount, queryString)
	return getSingleColumnResultToSlice(ctx, q)
}

// --- JobQuerier ---

func (p *BigQueryProvider) QueryJobRuns(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	release string, start, end time.Time) (map[string]dataprovider.JobRunStats, error) {

	joinVariants := ""
	for _, v := range sortedKeys(allJobVariants.Variants) {
		cleanV := param.Cleanse(v)
		joinVariants += fmt.Sprintf(
			"LEFT JOIN %s.job_variants jv_%s ON jobs.prowjob_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			p.client.Dataset, cleanV, cleanV, cleanV, v)
	}

	variantFilters := ""
	var params []bigquery.QueryParameter

	includeVariants := reqOptions.VariantOption.IncludeVariants
	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}
	for _, group := range sortedKeys(includeVariants) {
		cleanGroup := param.Cleanse(group)
		paramName := fmt.Sprintf("variantGroup_%s", cleanGroup)
		variantFilters += fmt.Sprintf(" AND (jv_%s.variant_value IN UNNEST(@%s))", cleanGroup, paramName)
		params = append(params, bigquery.QueryParameter{
			Name:  paramName,
			Value: includeVariants[group],
		})
	}

	queryString := fmt.Sprintf(`
		SELECT
			jobs.prowjob_job_name AS job_name,
			COUNT(DISTINCT jobs.prowjob_build_id) AS total_runs,
			COUNT(DISTINCT IF(jobs.prowjob_state = 'success', jobs.prowjob_build_id, NULL)) AS successful_runs
		FROM %s.jobs jobs
		%s
		WHERE jobs.prowjob_start >= DATETIME(@From)
			AND jobs.prowjob_start < DATETIME(@To)
			AND jv_Release.variant_value = @Release
			AND (jobs.prowjob_job_name LIKE 'periodic-%%' OR jobs.prowjob_job_name LIKE 'release-%%' OR jobs.prowjob_job_name LIKE 'aggregator-%%')
			%s
		GROUP BY jobs.prowjob_job_name
		ORDER BY jobs.prowjob_job_name
	`, p.client.Dataset, joinVariants, variantFilters)

	params = append(params,
		bigquery.QueryParameter{Name: "From", Value: start},
		bigquery.QueryParameter{Name: "To", Value: end},
		bigquery.QueryParameter{Name: "Release", Value: release},
	)

	q := p.client.Query(ctx, bqlabel.CRViewJobs, queryString)
	q.Parameters = params

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("error executing view jobs query: %w", err)
	}

	type jobRunRow struct {
		JobName    string `bigquery:"job_name"`
		TotalRuns  int    `bigquery:"total_runs"`
		Successful int    `bigquery:"successful_runs"`
	}

	results := map[string]dataprovider.JobRunStats{}
	for {
		var row jobRunRow
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading view jobs row: %w", err)
		}
		passRate := 0.0
		if row.TotalRuns > 0 {
			passRate = float64(row.Successful) / float64(row.TotalRuns) * 100
		}
		results[row.JobName] = dataprovider.JobRunStats{
			JobName:        row.JobName,
			TotalRuns:      row.TotalRuns,
			SuccessfulRuns: row.Successful,
			PassRate:       passRate,
		}
	}
	return results, nil
}

func (p *BigQueryProvider) QueryJobVariantValues(ctx context.Context, jobNames []string,
	variantKeys []string) (map[string]map[string]string, error) {
	if len(jobNames) == 0 {
		return map[string]map[string]string{}, nil
	}

	queryString := fmt.Sprintf(`
		SELECT job_name, variant_name, variant_value
		FROM %s.job_variants
		WHERE job_name IN UNNEST(@JobNames)
			AND variant_name IN UNNEST(@VariantNames)
	`, p.client.Dataset)

	q := p.client.Query(ctx, bqlabel.CRViewJobs, queryString)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "JobNames", Value: jobNames},
		{Name: "VariantNames", Value: variantKeys},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("error querying job variant values: %w", err)
	}

	type variantRow struct {
		JobName      string `bigquery:"job_name"`
		VariantName  string `bigquery:"variant_name"`
		VariantValue string `bigquery:"variant_value"`
	}

	results := map[string]map[string]string{}
	for {
		var row variantRow
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading job variant row: %w", err)
		}
		if results[row.JobName] == nil {
			results[row.JobName] = map[string]string{}
		}
		results[row.JobName][row.VariantName] = row.VariantValue
	}
	return results, nil
}

func (p *BigQueryProvider) LookupJobVariants(ctx context.Context, jobName string) (map[string]string, error) {
	queryString := fmt.Sprintf(`
		SELECT variant_name, variant_value
		FROM %s.job_variants
		WHERE job_name = @JobName
	`, p.client.Dataset)

	q := p.client.Query(ctx, bqlabel.CRViewJobs, queryString)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "JobName", Value: jobName},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("error querying job variants: %w", err)
	}

	type row struct {
		VariantName  string `bigquery:"variant_name"`
		VariantValue string `bigquery:"variant_value"`
	}

	variants := map[string]string{}
	for {
		var r row
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading variant row: %w", err)
		}
		variants[r.VariantName] = r.VariantValue
	}
	return variants, nil
}

// --- SpotCheckQuerier ---

// QuerySpotCheckJobRuns queries the jobs table (not junit) for rare-tier periodic/release
// jobs, grouping by the requested variant columns (e.g. Platform, Architecture, Network).
// Returns aggregated pass/fail counts per variant group so the middleware can create
// synthetic test results without needing individual test case data.
func (p *BigQueryProvider) QuerySpotCheckJobRuns(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	jobNameSubstrings []string,
	start, end time.Time) ([]dataprovider.SpotCheckGroup, error) {

	columnGroupByVariants := reqOptions.VariantOption.ColumnGroupBy
	if len(columnGroupByVariants) == 0 {
		columnGroupByVariants = sets.NewString("Platform", "Architecture", "Network")
	}

	var selectVariants, joinVariants string
	var groupByParts []string
	for _, v := range columnGroupByVariants.List() {
		cleanV := param.Cleanse(v)
		joinVariants += fmt.Sprintf(
			"LEFT JOIN %s.job_variants jv_%s ON jobs.prowjob_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			p.client.Dataset, cleanV, cleanV, cleanV, v)
		selectVariants += fmt.Sprintf("jv_%s.variant_value AS variant_%s,\n", cleanV, cleanV)
		groupByParts = append(groupByParts, fmt.Sprintf("jv_%s.variant_value", cleanV))
	}
	groupByVariants := strings.Join(groupByParts, ", ")

	// Always join Release and JobTier
	joinVariants += fmt.Sprintf(
		"LEFT JOIN %s.job_variants jv_Release ON jobs.prowjob_job_name = jv_Release.job_name AND jv_Release.variant_name = 'Release'\n",
		p.client.Dataset)
	joinVariants += fmt.Sprintf(
		"LEFT JOIN %s.job_variants jv_JobTier ON jobs.prowjob_job_name = jv_JobTier.job_name AND jv_JobTier.variant_name = 'JobTier'\n",
		p.client.Dataset)

	// Track which variant groups already have JOINs
	joinedVariants := sets.NewString(columnGroupByVariants.List()...)
	joinedVariants.Insert("Release", "JobTier")

	// Build include variant filters (Platform, Architecture, etc.) but skip JobTier
	variantFilters := ""
	var params []bigquery.QueryParameter
	includeVariants := reqOptions.VariantOption.IncludeVariants
	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}
	for _, group := range sortedKeys(includeVariants) {
		if group == "JobTier" {
			continue
		}
		cleanGroup := param.Cleanse(group)
		if !joinedVariants.Has(group) {
			joinVariants += fmt.Sprintf(
				"LEFT JOIN %s.job_variants jv_%s ON jobs.prowjob_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
				p.client.Dataset, cleanGroup, cleanGroup, cleanGroup, group)
			joinedVariants.Insert(group)
		}
		paramName := fmt.Sprintf("variantGroup_%s", cleanGroup)
		variantFilters += fmt.Sprintf(" AND (jv_%s.variant_value IN UNNEST(@%s))", cleanGroup, paramName)
		params = append(params, bigquery.QueryParameter{
			Name:  paramName,
			Value: includeVariants[group],
		})
	}

	jobNameFilters := ""
	for i, sub := range jobNameSubstrings {
		paramName := fmt.Sprintf("jobNameSub_%d", i)
		jobNameFilters += fmt.Sprintf(" AND LOWER(jobs.prowjob_job_name) LIKE CONCAT('%%', @%s, '%%')", paramName)
		params = append(params, bigquery.QueryParameter{
			Name:  paramName,
			Value: strings.ToLower(sub),
		})
	}

	queryString := fmt.Sprintf(`
		SELECT
			%s
			COUNT(DISTINCT jobs.prowjob_build_id) AS total_runs,
			COUNT(DISTINCT IF(jobs.prowjob_state = 'success', jobs.prowjob_build_id, NULL)) AS successful_runs,
			ARRAY_AGG(DISTINCT jobs.prowjob_job_name) AS job_names
		FROM %s.jobs jobs
		%s
		WHERE jobs.prowjob_start >= DATETIME(@From)
			AND jobs.prowjob_start < DATETIME(@To)
			AND jv_Release.variant_value = @Release
			AND jv_JobTier.variant_value = 'rare'
			AND (jobs.prowjob_job_name LIKE 'periodic-%%' OR jobs.prowjob_job_name LIKE 'release-%%')
			%s
			%s
		GROUP BY %s
	`, selectVariants, p.client.Dataset, joinVariants, variantFilters, jobNameFilters,
		groupByVariants)

	params = append(params,
		bigquery.QueryParameter{Name: "From", Value: start},
		bigquery.QueryParameter{Name: "To", Value: end},
		bigquery.QueryParameter{Name: "Release", Value: reqOptions.SampleRelease.Name},
	)

	q := p.client.Query(ctx, bqlabel.CRSpotCheck, queryString)
	q.Parameters = params

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("error executing spot check query: %w", err)
	}

	var results []dataprovider.SpotCheckGroup
	for {
		var rawRow map[string]bigquery.Value
		err := it.Next(&rawRow)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading spot check row: %w", err)
		}

		group := dataprovider.SpotCheckGroup{
			Variants: map[string]string{},
		}
		for _, v := range columnGroupByVariants.List() {
			cleanV := param.Cleanse(v)
			if val, ok := rawRow["variant_"+cleanV]; ok && val != nil {
				group.Variants[v] = val.(string)
			}
		}
		if val, ok := rawRow["total_runs"]; ok && val != nil {
			group.TotalRuns = int(val.(int64))
		}
		if val, ok := rawRow["successful_runs"]; ok && val != nil {
			group.SuccessfulRuns = int(val.(int64))
		}
		if val, ok := rawRow["job_names"]; ok && val != nil {
			for _, jn := range val.([]bigquery.Value) {
				group.JobNames = append(group.JobNames, jn.(string))
			}
		}
		results = append(results, group)
	}

	log.Infof("spot check query returned %d groups", len(results))
	return results, nil
}

// QuerySpotCheckJobRunDetails returns individual job runs matching the given variant
// filters, used to populate the test details drill-down page for a specific spot-check
// synthetic test.
func (p *BigQueryProvider) QuerySpotCheckJobRunDetails(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	variants map[string]string,
	jobNameSubstrings []string,
	start, end time.Time) ([]dataprovider.JobRunDetail, error) {

	joinVariants := fmt.Sprintf(
		"LEFT JOIN %s.job_variants jv_Release ON jobs.prowjob_job_name = jv_Release.job_name AND jv_Release.variant_name = 'Release'\n",
		p.client.Dataset)
	joinVariants += fmt.Sprintf(
		"LEFT JOIN %s.job_variants jv_JobTier ON jobs.prowjob_job_name = jv_JobTier.job_name AND jv_JobTier.variant_name = 'JobTier'\n",
		p.client.Dataset)

	variantFilters := ""
	var params []bigquery.QueryParameter
	for k, v := range variants {
		cleanK := param.Cleanse(k)
		joinVariants += fmt.Sprintf(
			"LEFT JOIN %s.job_variants jv_%s ON jobs.prowjob_job_name = jv_%s.job_name AND jv_%s.variant_name = '%s'\n",
			p.client.Dataset, cleanK, cleanK, cleanK, k)
		paramName := fmt.Sprintf("variant_%s", cleanK)
		variantFilters += fmt.Sprintf(" AND jv_%s.variant_value = @%s", cleanK, paramName)
		params = append(params, bigquery.QueryParameter{
			Name:  paramName,
			Value: v,
		})
	}

	jobNameFilters := ""
	for i, sub := range jobNameSubstrings {
		paramName := fmt.Sprintf("jobNameSub_%d", i)
		jobNameFilters += fmt.Sprintf(" AND LOWER(jobs.prowjob_job_name) LIKE CONCAT('%%', @%s, '%%')", paramName)
		params = append(params, bigquery.QueryParameter{
			Name:  paramName,
			Value: strings.ToLower(sub),
		})
	}

	queryString := fmt.Sprintf(`
		SELECT
			jobs.prowjob_job_name AS job_name,
			jobs.prowjob_build_id AS run_id,
			jobs.prowjob_url AS url,
			TIMESTAMP(jobs.prowjob_start) AS start_time,
			(jobs.prowjob_state = 'success') AS success
		FROM %s.jobs jobs
		%s
		WHERE jobs.prowjob_start >= DATETIME(@From)
			AND jobs.prowjob_start < DATETIME(@To)
			AND jv_Release.variant_value = @Release
			AND jv_JobTier.variant_value = 'rare'
			AND (jobs.prowjob_job_name LIKE 'periodic-%%' OR jobs.prowjob_job_name LIKE 'release-%%')
			%s
			%s
		ORDER BY jobs.prowjob_start DESC
	`, p.client.Dataset, joinVariants, variantFilters, jobNameFilters)

	params = append(params,
		bigquery.QueryParameter{Name: "From", Value: start},
		bigquery.QueryParameter{Name: "To", Value: end},
		bigquery.QueryParameter{Name: "Release", Value: reqOptions.SampleRelease.Name},
	)

	q := p.client.Query(ctx, bqlabel.CRSpotCheckDetails, queryString)
	q.Parameters = params

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("error executing spot check details query: %w", err)
	}

	type detailRow struct {
		JobName   string    `bigquery:"job_name"`
		RunID     string    `bigquery:"run_id"`
		URL       string    `bigquery:"url"`
		StartTime time.Time `bigquery:"start_time"`
		Success   bool      `bigquery:"success"`
	}

	var results []dataprovider.JobRunDetail
	for {
		var row detailRow
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading spot check detail row: %w", err)
		}
		results = append(results, dataprovider.JobRunDetail{
			JobName:   row.JobName,
			RunID:     row.RunID,
			URL:       row.URL,
			StartTime: row.StartTime,
			Success:   row.Success,
		})
	}

	return results, nil
}

// --- Helpers ---

func getSingleColumnResultToSlice(ctx context.Context, q *bigquery.Query) ([]string, error) {
	names := []string{}
	it, err := q.Read(ctx)
	if err != nil {
		log.WithError(err).Error("error querying from bigquery")
		return names, err
	}
	for {
		row := struct{ Name string }{}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing row from bigquery")
			return names, err
		}
		names = append(names, row.Name)
	}
	return names, nil
}
