package bigquery

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/param"
	"github.com/openshift/sippy/pkg/util/sets"
)

var _ dataprovider.DataProvider = &BigQueryProvider{}

// BigQueryProvider implements dataprovider.DataProvider using Google BigQuery
// as the backing data store, wrapping the existing query generators.
type BigQueryProvider struct {
	client                     *bqcachedclient.Client
	variantJunitTableOverrides []configv1.VariantJunitTableOverride
}

func NewBigQueryProvider(client *bqcachedclient.Client, overrides []configv1.VariantJunitTableOverride) *BigQueryProvider {
	return &BigQueryProvider{client: client, variantJunitTableOverrides: overrides}
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

	fLog := log.WithField("func", "QuerySampleTestStatus")

	// Filter out overridden variants from the default query
	filteredVariants, skipDefault := copyIncludeVariantsAndRemoveOverrides(p.variantJunitTableOverrides, -1, includeVariants)

	type result struct {
		status map[string]crstatus.TestStatus
		errs   []error
	}
	resultCh := make(chan result)
	var wg sync.WaitGroup

	// Run default query (unless all variants were overridden)
	if !skipDefault {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
				fLog.Infof("running default sample query with includeVariants: %+v", filteredVariants)
				s, e := p.querySampleTestStatusForTable(ctx, reqOptions, allJobVariants, filteredVariants, start, end, DefaultJunitTable)
				resultCh <- result{s, e}
			}
		}()
	}

	// Run override queries for applicable variants
	for i, or := range p.variantJunitTableOverrides {
		if !containsOverriddenVariant(includeVariants, or.VariantName, or.VariantValue) {
			continue
		}
		index, override := i, or
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
				overrideVariants, skip := copyIncludeVariantsAndRemoveOverrides(p.variantJunitTableOverrides, index, includeVariants)
				if skip {
					fLog.Infof("skipping override sample query as all values for a variant were overridden")
					return
				}
				overrideEnd := end
				overrideStart, err := util.ParseCRReleaseTime([]v1.Release{}, "", override.RelativeStart,
					true, &overrideEnd, reqOptions.CacheOption.CRTimeRoundingFactor)
				if err != nil {
					resultCh <- result{nil, []error{err}}
					return
				}
				fLog.Infof("running override sample query for %+v with includeVariants: %+v", override, overrideVariants)
				s, e := p.querySampleTestStatusForTable(ctx, reqOptions, allJobVariants, overrideVariants, overrideStart, overrideEnd, override.TableName)
				resultCh <- result{s, e}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	merged := make(map[string]crstatus.TestStatus)
	var allErrs []error
	for r := range resultCh {
		allErrs = append(allErrs, r.errs...)
		for k, v := range r.status {
			merged[k] = v
		}
	}
	if len(allErrs) > 0 {
		return nil, allErrs
	}
	return merged, nil
}

func (p *BigQueryProvider) querySampleTestStatusForTable(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	junitTable string) (map[string]crstatus.TestStatus, []error) {

	generator := NewSampleQueryGenerator(p.client, reqOptions, allJobVariants, includeVariants, start, end, junitTable)
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
		apiPkg.NewCacheSpec(generator, "BaseJobRunTestStatus~", &reqOptions.BaseRelease.End),
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

	fLog := log.WithField("func", "QuerySampleJobRunTestStatus")

	filteredVariants, skipDefault := copyIncludeVariantsAndRemoveOverrides(p.variantJunitTableOverrides, -1, includeVariants)

	type result struct {
		status map[string][]crstatus.TestJobRunRows
		errs   []error
	}
	resultCh := make(chan result)
	var wg sync.WaitGroup

	if !skipDefault {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
				fLog.Infof("running default sample job run query with includeVariants: %+v", filteredVariants)
				s, e := p.querySampleJobRunTestStatusForTable(ctx, reqOptions, allJobVariants, filteredVariants, start, end, DefaultJunitTable)
				resultCh <- result{s, e}
			}
		}()
	}

	for i, or := range p.variantJunitTableOverrides {
		if !containsOverriddenVariant(includeVariants, or.VariantName, or.VariantValue) {
			continue
		}
		index, override := i, or
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
				overrideVariants, skip := copyIncludeVariantsAndRemoveOverrides(p.variantJunitTableOverrides, index, includeVariants)
				if skip {
					fLog.Infof("skipping override sample job run query as all values for a variant were overridden")
					return
				}
				overrideEnd := end
				overrideStart, err := util.ParseCRReleaseTime([]v1.Release{}, "", override.RelativeStart,
					true, &overrideEnd, reqOptions.CacheOption.CRTimeRoundingFactor)
				if err != nil {
					resultCh <- result{nil, []error{err}}
					return
				}
				fLog.Infof("running override sample job run query for %+v with includeVariants: %+v", override, overrideVariants)
				s, e := p.querySampleJobRunTestStatusForTable(ctx, reqOptions, allJobVariants, overrideVariants, overrideStart, overrideEnd, override.TableName)
				resultCh <- result{s, e}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	merged := make(map[string][]crstatus.TestJobRunRows)
	var allErrs []error
	for r := range resultCh {
		allErrs = append(allErrs, r.errs...)
		for k, v := range r.status {
			merged[k] = v
		}
	}
	if len(allErrs) > 0 {
		return nil, allErrs
	}
	return merged, nil
}

func (p *BigQueryProvider) querySampleJobRunTestStatusForTable(ctx context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	junitTable string) (map[string][]crstatus.TestJobRunRows, []error) {

	generator := NewSampleTestDetailsQueryGenerator(p.client, reqOptions, allJobVariants, includeVariants, start, end, junitTable)
	result, errs := apiPkg.GetDataFromCacheOrGenerate[crstatus.TestJobRunStatuses](
		ctx, p.client.Cache, reqOptions.CacheOption,
		apiPkg.NewCacheSpec(generator, "SampleJobRunTestStatus~", &end),
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

// containsOverriddenVariant checks whether the given variant key/value pair
// is present in the includeVariants map (i.e. the request actually includes
// data for this overridden variant).
func containsOverriddenVariant(includeVariants map[string][]string, key, value string) bool {
	for k, v := range includeVariants {
		if k != key {
			continue
		}
		for _, vv := range v {
			if vv == value {
				return true
			}
		}
	}
	return false
}

// copyIncludeVariantsAndRemoveOverrides copies includeVariants and removes
// overridden variant values. For the default query (currOverride=-1), all
// overridden values are removed. For an override query at index i, only
// other overrides' values are removed. Returns true if the query should be
// skipped because all values for a variant were removed.
func copyIncludeVariantsAndRemoveOverrides(
	overrides []configv1.VariantJunitTableOverride,
	currOverride int,
	includeVariants map[string][]string) (map[string][]string, bool) {

	cp := make(map[string][]string)
	for key, values := range includeVariants {
		var newSlice []string
		for _, v := range values {
			if !shouldSkipVariant(overrides, currOverride, key, v) {
				newSlice = append(newSlice, v)
			}
		}
		if len(newSlice) == 0 {
			return cp, true
		}
		cp[key] = newSlice
	}
	return cp, false
}

func shouldSkipVariant(overrides []configv1.VariantJunitTableOverride, currOverride int, key, value string) bool {
	for i, override := range overrides {
		if i == currOverride {
			return false
		}
		if override.VariantName == key && override.VariantValue == value {
			return true
		}
	}
	return false
}
