package postgres

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
)

var _ dataprovider.DataProvider = &PostgresProvider{}

// PostgresProvider implements dataprovider.DataProvider using PostgreSQL.
// Designed for local development and testing — not optimized for production scale.
type PostgresProvider struct {
	dbc   *db.DB
	cache cache.Cache
}

func NewPostgresProvider(dbc *db.DB, c cache.Cache) *PostgresProvider {
	if c == nil {
		c = &noOpCache{}
	}
	return &PostgresProvider{dbc: dbc, cache: c}
}

// noOpCache never stores or returns data; no Redis needed for local dev.
type noOpCache struct{}

func (n *noOpCache) Get(_ context.Context, _ string, _ time.Duration) ([]byte, error) {
	return nil, fmt.Errorf("cache miss")
}
func (n *noOpCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }

func (p *PostgresProvider) Cache() cache.Cache {
	return p.cache
}

// --- Variant helpers ---

// parseVariants splits a pq.StringArray like ["Platform:aws", "Upgrade:none"] into a map.
func parseVariants(variants pq.StringArray) map[string]string {
	result := make(map[string]string, len(variants))
	for _, v := range variants {
		if k, val, ok := strings.Cut(v, ":"); ok {
			result[k] = val
		}
	}
	return result
}

// filterByDBGroupBy returns a copy of the variant map keeping only keys in dbGroupBy.
func filterByDBGroupBy(variants map[string]string, dbGroupBy sets.Set[string]) map[string]string {
	filtered := make(map[string]string, dbGroupBy.Len())
	for k, v := range variants {
		if dbGroupBy.Has(k) {
			filtered[k] = v
		}
	}
	return filtered
}

// matchRequestedVariants checks whether a row's job variants satisfy the
// requested variant filters for the given testID, and returns the filtered
// key. Returns ok=false if the row should be skipped.
func matchRequestedVariants(
	testID string,
	variants map[string]string,
	requestedVariants map[string]map[string]string,
	dbGroupBy sets.Set[string],
) (crtest.KeyWithVariants, bool) {
	if rv, ok := requestedVariants[testID]; ok {
		for k, v := range rv {
			if variants[k] != v {
				return crtest.KeyWithVariants{}, false
			}
		}
	}
	filtered := filterByDBGroupBy(variants, dbGroupBy)
	return crtest.KeyWithVariants{
		TestID:   testID,
		Variants: filtered,
	}, true
}

// buildRequestedVariantsMap builds a testID -> requested variants lookup
// from the request options.
func buildRequestedVariantsMap(testIDOptions []reqopts.TestIdentification) map[string]map[string]string {
	m := map[string]map[string]string{}
	for _, tid := range testIDOptions {
		if len(tid.RequestedVariants) > 0 {
			m[tid.TestID] = tid.RequestedVariants
		}
	}
	return m
}

// matchesIncludeVariants checks if a variant map passes the include filter.
func matchesIncludeVariants(variants map[string]string, includeVariants map[string][]string) bool {
	for key, allowed := range includeVariants {
		val, exists := variants[key]
		if !exists {
			return false
		}
		if !slices.Contains(allowed, val) {
			return false
		}
	}
	return true
}

// --- MetadataQuerier ---

func (p *PostgresProvider) QueryJobVariants(ctx context.Context, _ reqopts.RequestOptions) (crtest.JobVariants, []error) {
	variants := crtest.JobVariants{Variants: map[string][]string{}}

	var pairs []string
	err := p.dbc.DB.WithContext(ctx).Raw(`SELECT DISTINCT unnest(variants) AS pair FROM prow_jobs WHERE deleted_at IS NULL`).
		Pluck("pair", &pairs).Error
	if err != nil {
		return variants, []error{fmt.Errorf("querying job variants: %w", err)}
	}

	grouped := map[string]map[string]bool{}
	for _, pair := range pairs {
		k, v, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		if grouped[k] == nil {
			grouped[k] = map[string]bool{}
		}
		grouped[k][v] = true
	}

	for k, vals := range grouped {
		sorted := make([]string, 0, len(vals))
		for v := range vals {
			sorted = append(sorted, v)
		}
		sort.Strings(sorted)
		variants.Variants[k] = sorted
	}
	return variants, nil
}

func (p *PostgresProvider) QueryReleases(ctx context.Context) ([]v1.Release, error) {
	return api.GetReleasesFromDB(ctx, p.dbc)
}

func (p *PostgresProvider) QueryReleaseDates(ctx context.Context, reqOptions reqopts.RequestOptions) ([]crtest.ReleaseTimeRange, []error) {
	timeRanges, err := api.GetReleaseDatesFromDB(ctx, p.dbc, reqOptions)
	if err != nil {
		return nil, []error{err}
	}
	return timeRanges, nil
}

func (p *PostgresProvider) QueryUniqueVariantValues(ctx context.Context, _ reqopts.RequestOptions, field string, nested bool) ([]string, error) {
	if nested {
		// Return all variant key names
		var pairs []string
		err := p.dbc.DB.WithContext(ctx).Raw(`
			SELECT DISTINCT unnest(variants) AS pair FROM prow_jobs
			WHERE deleted_at IS NULL
		`).Pluck("pair", &pairs).Error
		if err != nil {
			return nil, err
		}
		keys := map[string]bool{}
		for _, pair := range pairs {
			if k, _, ok := strings.Cut(pair, ":"); ok {
				keys[k] = true
			}
		}
		result := make([]string, 0, len(keys))
		for k := range keys {
			result = append(result, k)
		}
		sort.Strings(result)
		return result, nil
	}

	// Map BQ column names to variant key names
	fieldMap := map[string]string{
		"platform": "Platform",
		"network":  "Network",
		"arch":     "Architecture",
		"upgrade":  "Upgrade",
	}
	variantKey, ok := fieldMap[field]
	if !ok {
		return []string{}, nil
	}

	var pairs []string
	err := p.dbc.DB.WithContext(ctx).Raw(`
		SELECT DISTINCT unnest(variants) AS pair FROM prow_jobs
		WHERE deleted_at IS NULL
	`).Pluck("pair", &pairs).Error
	if err != nil {
		return nil, err
	}

	vals := map[string]bool{}
	for _, pair := range pairs {
		if k, v, ok := strings.Cut(pair, ":"); ok && k == variantKey {
			vals[v] = true
		}
	}
	result := make([]string, 0, len(vals))
	for v := range vals {
		result = append(result, v)
	}
	sort.Strings(result)
	return result, nil
}

// fetchJobVariantsByIDs loads ProwJob variant maps for the given job IDs.
func (p *PostgresProvider) fetchJobVariantsByIDs(ids []uint) (map[uint]map[string]string, error) {
	if len(ids) == 0 {
		return map[uint]map[string]string{}, nil
	}

	type jobRow struct {
		ID       uint           `gorm:"column:id"`
		Variants pq.StringArray `gorm:"column:variants;type:text[]"`
	}

	var jobRows []jobRow
	if err := p.dbc.DB.Raw(`SELECT id, variants FROM prow_jobs WHERE id IN (?)`, ids).Scan(&jobRows).Error; err != nil {
		return nil, fmt.Errorf("fetching job variants: %w", err)
	}

	result := make(map[uint]map[string]string, len(jobRows))
	for _, jr := range jobRows {
		result[jr.ID] = parseVariants(jr.Variants)
	}
	return result, nil
}

// baseMatchesGAWindow returns true when the base release dates align with a
// pre-computed GA window in prow_ga_raw_test_data.
func (p *PostgresProvider) baseMatchesGAWindow(ctx context.Context, release string, baseRange query.DateRange) bool {
	var rd models.ReleaseDefinition
	err := p.dbc.DB.WithContext(ctx).
		Select("ga_date").
		Where("release = ?", release).
		First(&rd).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.WithError(err).WithField("release", release).
				Warn("failed to query GA date, falling back to prefix-sum query")
		}
		return false
	}
	if rd.GADate == nil {
		return false
	}

	gaDate := *rd.GADate
	if gaDate.After(civil.DateOf(time.Now().UTC())) {
		return false
	}
	if baseRange.End != utils.GAWindowEnd(gaDate) {
		return false
	}

	windowDays := gaDate.DaysSince(baseRange.Start)
	return slices.Contains(utils.GAWindows, windowDays)
}

func (p *PostgresProvider) QueryBaseTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions) (map[string]crstatus.TestStatus, []error) {
	baseRange := query.DateRange{
		Start: civil.DateOf(reqOptions.BaseRelease.Start),
		End:   civil.DateOf(reqOptions.BaseRelease.End).AddDays(1),
	}
	if p.baseMatchesGAWindow(ctx, reqOptions.BaseRelease.Name, baseRange) {
		return p.queryBaseTestStatusGA(ctx, reqOptions, baseRange)
	}
	return p.queryTestStatusPrefixSum(ctx, reqOptions,
		reqOptions.BaseRelease.Name,
		nil,
		reqOptions.VariantOption.IncludeVariants,
		baseRange)
}

// mergeCompareVariants returns a copy of includeVariants with CompareVariants
// merged in for cross-compare views. For cross-compare, IncludeVariants holds
// base-side values (e.g. Topology:[ha]) while CompareVariants holds sample-side
// values (e.g. Topology:[single]). Sample queries need the merged set.
func mergeCompareVariants(reqOptions reqopts.RequestOptions, includeVariants map[string][]string) map[string][]string {
	if len(reqOptions.VariantOption.VariantCrossCompare) == 0 {
		return includeVariants
	}
	merged := make(map[string][]string, len(includeVariants))
	for k, v := range includeVariants {
		merged[k] = v
	}
	for _, k := range reqOptions.VariantOption.VariantCrossCompare {
		if v, ok := reqOptions.VariantOption.CompareVariants[k]; ok {
			merged[k] = v
		}
	}
	return merged
}

func (p *PostgresProvider) QueryTestStatus(
	ctx context.Context,
	reqOptions reqopts.RequestOptions,
) (baseStatus, sampleStatus map[string]crstatus.TestStatus, errs []error) {
	return p.queryCombinedTestStatus(ctx, reqOptions)
}

// --- TestDetailsQuerier ---

type testDetailRow struct {
	TestID          string    `gorm:"column:test_id"`
	TestName        string    `gorm:"column:test_name"`
	ProwJobName     string    `gorm:"column:prowjob_name"`
	ProwJobRunID    string    `gorm:"column:prowjob_run_id"`
	ProwJobURL      string    `gorm:"column:prowjob_url"`
	ProwJobStart    time.Time `gorm:"column:prowjob_start"`
	ProwJobID       uint      `gorm:"column:prow_job_id"`
	Status          int       `gorm:"column:status"`
	JiraComponent   string    `gorm:"column:jira_component"`
	JiraComponentID *uint     `gorm:"column:jira_component_id"`
}

func (p *PostgresProvider) queryTestDetails(ctx context.Context, release string, start, end time.Time,
	reqOptions reqopts.RequestOptions,
	includeVariants map[string][]string) (map[string][]crstatus.TestDetailsSummary, []error) {

	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}

	// MATERIALIZED CTE forces the planner to resolve test_ids first, then
	// drive prow_job_run_tests via the test_id index. Without it, the global
	// work_mem=128MB setting causes the planner to choose a prow_jobs-first
	// plan that scans ~20K runs × 30 partitions and never completes.
	testIDs := make([]string, 0, len(reqOptions.TestIDOptions))
	for _, tid := range reqOptions.TestIDOptions {
		if tid.TestID == "" {
			continue
		}
		testIDs = append(testIDs, tid.TestID)
	}

	sqlQuery := `WITH target_tests AS MATERIALIZED (
    SELECT test_id, suite_id, unique_id, jira_component, jira_component_id
    FROM test_ownerships
    WHERE staff_approved_obsolete = false`

	var args []any
	if len(testIDs) > 0 {
		sqlQuery += ` AND unique_id IN (?)`
		args = append(args, testIDs)
	}

	sqlQuery += `)
SELECT
    tt.unique_id AS test_id,
    t.name AS test_name,
    pj.name AS prowjob_name,
    CAST(pjr.id AS TEXT) AS prowjob_run_id,
    COALESCE(pjr.url, '') AS prowjob_url,
    pjr.timestamp AS prowjob_start,
    pj.id AS prow_job_id,
    pjrt.status,
    COALESCE(tt.jira_component, '') AS jira_component,
    tt.jira_component_id
FROM target_tests tt
JOIN prow_job_run_tests pjrt ON pjrt.test_id = tt.test_id
    AND (tt.suite_id = pjrt.suite_id OR (tt.suite_id IS NULL AND pjrt.suite_id IS NULL))
JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id
    AND pjr.prow_job_release = pjrt.prow_job_run_release
    AND pjr.timestamp = pjrt.prow_job_run_timestamp
JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
JOIN tests t ON t.id = pjrt.test_id
WHERE pj.release = ?
    AND pjr.timestamp >= ? AND pjr.timestamp < ?
    AND pjr.prow_job_release = ?
    AND pjrt.prow_job_run_release = ?
    AND pjrt.prow_job_run_timestamp >= ? AND pjrt.prow_job_run_timestamp < ?
    AND pjrt.deleted_at IS NULL AND pjr.deleted_at IS NULL AND pj.deleted_at IS NULL
    AND (pjr.labels IS NULL OR NOT pjr.labels @> ARRAY['InfraFailure'])`

	args = append(args, release, start, end, release, release, start, end)

	if len(includeVariants) > 0 {
		filterClause, filterArgs := buildVariantFilterClause(includeVariants)
		if filterClause != "" {
			sqlQuery += " AND pj.variant_combination_id IN (SELECT vc.id FROM variant_combinations vc WHERE " + filterClause + ")"
			args = append(args, filterArgs...)
		}
	}

	sqlQuery += " ORDER BY pjr.timestamp"

	var rows []testDetailRow
	if err := p.dbc.DB.WithContext(ctx).Raw(sqlQuery, args...).Scan(&rows).Error; err != nil {
		return nil, []error{fmt.Errorf("querying test details: %w", err)}
	}

	dbGroupBy := reqOptions.VariantOption.DBGroupBy

	// Batch-fetch job variants for per-test requested variant filtering
	jobIDs := sets.New[uint]()
	for _, r := range rows {
		jobIDs.Insert(r.ProwJobID)
	}
	ids := jobIDs.UnsortedList()
	jobVariantMap, err := p.fetchJobVariantsByIDs(ids)
	if err != nil {
		return nil, []error{err}
	}

	requestedVariantsByTestID := buildRequestedVariantsMap(reqOptions.TestIDOptions)

	result := map[string][]crstatus.TestJobRunRows{}
	for _, row := range rows {
		variants, ok := jobVariantMap[row.ProwJobID]
		if !ok {
			continue
		}

		key, matched := matchRequestedVariants(row.TestID, variants, requestedVariantsByTestID, dbGroupBy)
		if !matched {
			continue
		}

		successCount := 0
		flakeCount := 0
		if row.Status == 1 || row.Status == 13 {
			successCount = 1
		}
		if row.Status == 13 {
			flakeCount = 1
		}

		var jiraComponentID *big.Rat
		if row.JiraComponentID != nil {
			jiraComponentID = new(big.Rat).SetUint64(uint64(*row.JiraComponentID))
		}

		normalizedName := utils.NormalizeProwJobName(row.ProwJobName)
		entry := crstatus.TestJobRunRows{
			TestKey:         key,
			TestKeyStr:      key.Encode(),
			TestName:        row.TestName,
			ProwJob:         row.ProwJobName,
			ProwJobRunID:    row.ProwJobRunID,
			ProwJobURL:      row.ProwJobURL,
			StartTime:       row.ProwJobStart,
			Count:           crtest.Count{TotalCount: 1, SuccessCount: successCount, FlakeCount: flakeCount},
			JiraComponent:   row.JiraComponent,
			JiraComponentID: jiraComponentID,
		}

		result[normalizedName] = append(result[normalizedName], entry)
	}

	return crstatus.SummarizeTestJobRuns(result), nil
}

func (p *PostgresProvider) QueryBaseJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions) (map[string][]crstatus.TestDetailsSummary, []error) {
	result, errs := p.queryTestDetails(
		ctx,
		reqOptions.BaseRelease.Name,
		reqOptions.BaseRelease.Start, reqOptions.BaseRelease.End,
		reqOptions, reqOptions.VariantOption.IncludeVariants,
	)
	if len(errs) > 0 {
		return result, errs
	}
	if len(result) > 0 {
		return result, nil
	}
	log.WithField("release", reqOptions.BaseRelease.Name).
		Info("no per-run base test details found, falling back to aggregate tables")
	return p.queryBaseAggregateTestDetails(ctx, reqOptions)
}

type aggregateTestDetailRow struct {
	TestID          string `gorm:"column:test_id"`
	TestName        string `gorm:"column:test_name"`
	ProwJobName     string `gorm:"column:prowjob_name"`
	ProwJobID       uint   `gorm:"column:prow_job_id"`
	JiraComponent   string `gorm:"column:jira_component"`
	JiraComponentID *uint  `gorm:"column:jira_component_id"`
	TotalCount      int    `gorm:"column:total_count"`
	SuccessCount    int    `gorm:"column:success_count"`
	FlakeCount      int    `gorm:"column:flake_count"`
}

// queryBaseAggregateTestDetails queries aggregate tables (test_cumulative_summaries
// or prow_ga_raw_test_data) as a fallback when prow_job_run_tests has no data for
// the base release. Returns per-job aggregate stats as TestDetailsSummary entries
// with no individual JobRuns, because per-run identifiers do not exist in these tables.
func (p *PostgresProvider) queryBaseAggregateTestDetails(ctx context.Context, reqOptions reqopts.RequestOptions) (map[string][]crstatus.TestDetailsSummary, []error) {
	includeVariants := reqOptions.VariantOption.IncludeVariants
	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}
	includeVariants = mergeRequestedVariants(includeVariants, reqOptions)

	testIDs := make([]string, 0, len(reqOptions.TestIDOptions))
	for _, tid := range reqOptions.TestIDOptions {
		if tid.TestID == "" {
			continue
		}
		testIDs = append(testIDs, tid.TestID)
	}

	cte := `WITH target_tests AS MATERIALIZED (
    SELECT test_id, suite_id, unique_id, jira_component, jira_component_id
    FROM test_ownerships
    WHERE staff_approved_obsolete = false`

	var cteArgs []any
	if len(testIDs) > 0 {
		cte += ` AND unique_id IN (?)`
		cteArgs = append(cteArgs, testIDs)
	}
	cte += ")"

	baseRange := query.DateRange{
		Start: civil.DateOf(reqOptions.BaseRelease.Start),
		End:   civil.DateOf(reqOptions.BaseRelease.End).AddDays(1),
	}

	var sqlQuery string
	var queryArgs []any

	if p.baseMatchesGAWindow(ctx, reqOptions.BaseRelease.Name, baseRange) {
		windowDays := baseRange.End.AddDays(-1).DaysSince(baseRange.Start)
		sqlQuery, queryArgs = p.buildAggregateGAQuery(cte, cteArgs, reqOptions.BaseRelease.Name, windowDays, includeVariants)
	} else {
		var err error
		sqlQuery, queryArgs, err = p.buildAggregatePrefixSumQuery(cte, cteArgs, reqOptions.BaseRelease.Name, baseRange, includeVariants)
		if err != nil {
			return nil, []error{err}
		}
	}

	var rows []aggregateTestDetailRow
	if err := p.dbc.DB.WithContext(ctx).Raw(sqlQuery, queryArgs...).Scan(&rows).Error; err != nil {
		return nil, []error{fmt.Errorf("querying aggregate test details: %w", err)}
	}

	return p.processAggregateRows(rows, reqOptions)
}

func (p *PostgresProvider) buildAggregatePrefixSumQuery(cte string, cteArgs []any, release string, dateRange query.DateRange, includeVariants map[string][]string) (string, []any, error) {
	if err := query.ResolveDateRanges(p.dbc, release, &dateRange); err != nil {
		return "", nil, fmt.Errorf("resolving date ranges: %w", err)
	}
	lookupEnd := dateRange.End.AddDays(-1)
	lookupStart := dateRange.Start.AddDays(-1)

	sqlQuery := cte + `
SELECT
    tt.unique_id AS test_id,
    t.name AS test_name,
    pj.name AS prowjob_name,
    pj.id AS prow_job_id,
    COALESCE(tt.jira_component, '') AS jira_component,
    tt.jira_component_id,
    SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)) AS total_count,
    SUM(e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0)) AS success_count,
    SUM(e.prefix_sum_flakes - COALESCE(s.prefix_sum_flakes, 0)) AS flake_count
FROM target_tests tt
JOIN test_cumulative_summaries e ON e.test_id = tt.test_id
    AND (e.suite_id = tt.suite_id OR (tt.suite_id IS NULL AND e.suite_id = 0))
LEFT JOIN test_cumulative_summaries s
    ON s.release = e.release AND s.test_id = e.test_id
    AND s.prow_job_id = e.prow_job_id AND s.suite_id = e.suite_id
    AND s.date = ?
JOIN prow_jobs pj ON pj.id = e.prow_job_id AND pj.deleted_at IS NULL
JOIN tests t ON t.id = tt.test_id
WHERE e.release = ? AND e.date = ?`

	args := make([]any, 0, len(cteArgs)+10)
	args = append(args, cteArgs...)
	args = append(args, lookupStart, release, lookupEnd)

	if len(includeVariants) > 0 {
		filterClause, filterArgs := buildVariantFilterClause(includeVariants)
		if filterClause != "" {
			sqlQuery += " AND pj.variant_combination_id IN (SELECT vc.id FROM variant_combinations vc WHERE " + filterClause + ")"
			args = append(args, filterArgs...)
		}
	}

	sqlQuery += `
GROUP BY tt.unique_id, t.name, pj.name, pj.id, tt.jira_component, tt.jira_component_id
HAVING SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)) > 0`

	return sqlQuery, args, nil
}

func (p *PostgresProvider) buildAggregateGAQuery(cte string, cteArgs []any, release string, windowDays int, includeVariants map[string][]string) (string, []any) {
	sqlQuery := cte + `
SELECT
    tt.unique_id AS test_id,
    t.name AS test_name,
    pj.name AS prowjob_name,
    pj.id AS prow_job_id,
    COALESCE(tt.jira_component, '') AS jira_component,
    tt.jira_component_id,
    SUM(e.runs) AS total_count,
    SUM(e.passes) AS success_count,
    SUM(e.flakes) AS flake_count
FROM target_tests tt
JOIN prow_ga_raw_test_data e ON e.test_id = tt.test_id
    AND (e.suite_id = tt.suite_id OR (tt.suite_id IS NULL AND e.suite_id = 0))
JOIN prow_jobs pj ON pj.id = e.prow_job_id AND pj.deleted_at IS NULL
JOIN tests t ON t.id = tt.test_id
WHERE e.release = ? AND e.window_days = ?`

	args := make([]any, 0, len(cteArgs)+10)
	args = append(args, cteArgs...)
	args = append(args, release, windowDays)

	if len(includeVariants) > 0 {
		filterClause, filterArgs := buildVariantFilterClause(includeVariants)
		if filterClause != "" {
			sqlQuery += " AND pj.variant_combination_id IN (SELECT vc.id FROM variant_combinations vc WHERE " + filterClause + ")"
			args = append(args, filterArgs...)
		}
	}

	sqlQuery += `
GROUP BY tt.unique_id, t.name, pj.name, pj.id, tt.jira_component, tt.jira_component_id
HAVING SUM(e.runs) > 0`

	return sqlQuery, args
}

func (p *PostgresProvider) processAggregateRows(rows []aggregateTestDetailRow, reqOptions reqopts.RequestOptions) (map[string][]crstatus.TestDetailsSummary, []error) {
	dbGroupBy := reqOptions.VariantOption.DBGroupBy

	jobIDs := sets.New[uint]()
	for _, r := range rows {
		jobIDs.Insert(r.ProwJobID)
	}
	jobVariantMap, err := p.fetchJobVariantsByIDs(jobIDs.UnsortedList())
	if err != nil {
		return nil, []error{err}
	}

	requestedVariantsByTestID := buildRequestedVariantsMap(reqOptions.TestIDOptions)

	result := map[string][]crstatus.TestDetailsSummary{}
	for _, row := range rows {
		variants, ok := jobVariantMap[row.ProwJobID]
		if !ok {
			continue
		}

		key, matched := matchRequestedVariants(row.TestID, variants, requestedVariantsByTestID, dbGroupBy)
		if !matched {
			continue
		}

		var jiraComponentID *big.Rat
		if row.JiraComponentID != nil {
			jiraComponentID = new(big.Rat).SetUint64(uint64(*row.JiraComponentID))
		}

		normalizedName := utils.NormalizeProwJobName(row.ProwJobName)
		entry := crstatus.TestDetailsSummary{
			TestKey:         key,
			TestKeyStr:      key.Encode(),
			ProwJob:         normalizedName,
			TestName:        row.TestName,
			Stats:           crtest.Count{TotalCount: row.TotalCount, SuccessCount: row.SuccessCount, FlakeCount: row.FlakeCount}.ToTestStats(false),
			JiraComponent:   row.JiraComponent,
			JiraComponentID: jiraComponentID,
		}

		result[normalizedName] = append(result[normalizedName], entry)
	}

	return result, nil
}

func (p *PostgresProvider) QuerySampleJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
	includeVariants map[string][]string,
	start, end time.Time) (map[string][]crstatus.TestDetailsSummary, []error) {
	return p.queryTestDetails(
		ctx,
		reqOptions.SampleRelease.Name,
		start, end,
		reqOptions, mergeCompareVariants(reqOptions, includeVariants),
	)
}

// --- JobQuerier ---

func (p *PostgresProvider) QueryJobRuns(ctx context.Context, reqOptions reqopts.RequestOptions,
	release string, start, end time.Time) (map[string]dataprovider.JobRunStats, error) {

	type jobRunRow struct {
		JobName    string `gorm:"column:job_name"`
		TotalRuns  int    `gorm:"column:total_runs"`
		Successful int    `gorm:"column:successful_runs"`
	}

	var rows []jobRunRow
	err := p.dbc.DB.WithContext(ctx).Raw(`
		SELECT
			pj.name AS job_name,
			COUNT(DISTINCT pjr.id) AS total_runs,
			COUNT(DISTINCT CASE WHEN pjr.succeeded THEN pjr.id END) AS successful_runs
		FROM prow_jobs pj
		JOIN prow_job_runs pjr ON pjr.prow_job_id = pj.id
		WHERE pj.release = ?
			AND pjr.timestamp >= ? AND pjr.timestamp < ?
			AND pjr.prow_job_release = ?
			AND pj.deleted_at IS NULL AND pjr.deleted_at IS NULL
			AND (pj.name LIKE 'periodic-%%' OR pj.name LIKE 'release-%%' OR pj.name LIKE 'aggregator-%%')
		GROUP BY pj.name
		ORDER BY pj.name
	`, release, start, end, release).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("querying job runs: %w", err)
	}

	// Apply variant filtering in Go
	includeVariants := reqOptions.VariantOption.IncludeVariants
	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}

	// Fetch variants for all jobs
	jobNames := make([]string, 0, len(rows))
	for _, r := range rows {
		jobNames = append(jobNames, r.JobName)
	}
	jobVariantMap := map[string]map[string]string{}
	if len(jobNames) > 0 {
		type jvRow struct {
			Name     string         `gorm:"column:name"`
			Variants pq.StringArray `gorm:"column:variants;type:text[]"`
		}
		var jvRows []jvRow
		if err := p.dbc.DB.WithContext(ctx).Raw(`SELECT name, variants FROM prow_jobs WHERE name IN (?) AND deleted_at IS NULL`, jobNames).Scan(&jvRows).Error; err != nil {
			return nil, fmt.Errorf("fetching job variants: %w", err)
		}
		for _, jr := range jvRows {
			jobVariantMap[jr.Name] = parseVariants(jr.Variants)
		}
	}

	results := map[string]dataprovider.JobRunStats{}
	for _, row := range rows {
		if variants, ok := jobVariantMap[row.JobName]; ok {
			if !matchesIncludeVariants(variants, includeVariants) {
				continue
			}
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

func (p *PostgresProvider) QueryJobVariantValues(ctx context.Context, _ reqopts.RequestOptions, jobNames []string,
	variantKeys []string) (map[string]map[string]string, error) {

	if len(jobNames) == 0 {
		return map[string]map[string]string{}, nil
	}

	type jvRow struct {
		Name     string         `gorm:"column:name"`
		Variants pq.StringArray `gorm:"column:variants;type:text[]"`
	}

	var rows []jvRow
	if err := p.dbc.DB.WithContext(ctx).Raw(`SELECT name, variants FROM prow_jobs WHERE name IN (?) AND deleted_at IS NULL`, jobNames).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying job variant values: %w", err)
	}

	keyFilter := map[string]bool{}
	for _, k := range variantKeys {
		keyFilter[k] = true
	}

	results := map[string]map[string]string{}
	for _, row := range rows {
		parsed := parseVariants(row.Variants)
		if len(keyFilter) > 0 {
			filtered := map[string]string{}
			for k, v := range parsed {
				if keyFilter[k] {
					filtered[k] = v
				}
			}
			results[row.Name] = filtered
		} else {
			results[row.Name] = parsed
		}
	}
	return results, nil
}

func (p *PostgresProvider) LookupJobVariants(ctx context.Context, _ reqopts.RequestOptions, jobName string) (map[string]string, error) {
	type jvRow struct {
		Variants pq.StringArray `gorm:"column:variants;type:text[]"`
	}

	var row jvRow
	err := p.dbc.DB.WithContext(ctx).Raw(`SELECT variants FROM prow_jobs WHERE name = ? AND deleted_at IS NULL LIMIT 1`, jobName).Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("looking up job variants: %w", err)
	}
	return parseVariants(row.Variants), nil
}
