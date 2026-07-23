package postgres

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
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
func filterByDBGroupBy(variants map[string]string, dbGroupBy map[string]bool) map[string]string {
	filtered := make(map[string]string, len(dbGroupBy))
	for k, v := range variants {
		if dbGroupBy[k] {
			filtered[k] = v
		}
	}
	return filtered
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
func (p *PostgresProvider) baseMatchesGAWindow(ctx context.Context, reqOptions reqopts.RequestOptions) bool {
	var gaDate *time.Time
	err := p.dbc.DB.WithContext(ctx).
		Model(&models.ReleaseDefinition{}).
		Where("release = ? AND ga_date < CURRENT_DATE", reqOptions.BaseRelease.Name).
		Pluck("ga_date", &gaDate).Error
	if err != nil {
		log.WithError(err).WithField("release", reqOptions.BaseRelease.Name).
			Warn("failed to query GA date, falling back to prefix-sum query")
		return false
	}
	if gaDate == nil {
		return false
	}

	windowDays := int(reqOptions.BaseRelease.End.Sub(reqOptions.BaseRelease.Start).Hours() / 24)
	if !slices.Contains(utils.GAWindows, windowDays) {
		return false
	}

	gaCivil := civil.DateOf(*gaDate)
	expectedStart := utils.GAWindowStart(gaCivil, windowDays)
	return civil.DateOf(reqOptions.BaseRelease.Start) == expectedStart &&
		civil.DateOf(reqOptions.BaseRelease.End) == gaCivil
}

func (p *PostgresProvider) QueryBaseTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions) (map[string]crstatus.TestStatus, []error) {
	if p.baseMatchesGAWindow(ctx, reqOptions) {
		return p.queryBaseTestStatusGA(ctx, reqOptions)
	}
	return p.queryTestStatusPrefixSum(ctx, reqOptions,
		reqOptions.BaseRelease.Name,
		reqOptions.VariantOption.IncludeVariants,
		reqOptions.BaseRelease.Start, reqOptions.BaseRelease.End)
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
	for k, v := range reqOptions.VariantOption.CompareVariants {
		merged[k] = v
	}
	return merged
}

func (p *PostgresProvider) QuerySampleTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
	includeVariants map[string][]string,
	start, end time.Time) (map[string]crstatus.TestStatus, []error) {
	includeVariants = mergeCompareVariants(reqOptions, includeVariants)
	return p.queryTestStatusPrefixSum(ctx, reqOptions, reqOptions.SampleRelease.Name, includeVariants, start, end)
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
	includeVariants map[string][]string) (map[string][]crstatus.TestJobRunRows, []error) {

	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}

	// MATERIALIZED CTE forces the planner to resolve test_ids first, then
	// drive prow_job_run_tests via the test_id index. Without it, the global
	// work_mem=128MB setting causes the planner to choose a prow_jobs-first
	// plan that scans ~20K runs × 30 partitions and never completes.
	testIDs := make([]string, 0, len(reqOptions.TestIDOptions))
	for _, tid := range reqOptions.TestIDOptions {
		testIDs = append(testIDs, tid.TestID)
	}

	query := `WITH target_tests AS MATERIALIZED (
    SELECT test_id, suite_id, unique_id, jira_component, jira_component_id
    FROM test_ownerships
    WHERE staff_approved_obsolete = false`

	var args []any
	if len(testIDs) > 0 {
		query += ` AND unique_id IN (?)`
		args = append(args, testIDs)
	}

	query += `)
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
			query += " AND pj.variant_combination_id IN (SELECT vc.id FROM variant_combinations vc WHERE " + filterClause + ")"
			args = append(args, filterArgs...)
		}
	}

	query += " ORDER BY pjr.timestamp"

	var rows []testDetailRow
	if err := p.dbc.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, []error{fmt.Errorf("querying test details: %w", err)}
	}

	dbGroupBy := make(map[string]bool, reqOptions.VariantOption.DBGroupBy.Len())
	for _, k := range sets.List(reqOptions.VariantOption.DBGroupBy) {
		dbGroupBy[k] = true
	}

	// Batch-fetch job variants for per-test requested variant filtering
	jobIDs := map[uint]bool{}
	for _, r := range rows {
		jobIDs[r.ProwJobID] = true
	}
	ids := make([]uint, 0, len(jobIDs))
	for id := range jobIDs {
		ids = append(ids, id)
	}
	jobVariantMap, err := p.fetchJobVariantsByIDs(ids)
	if err != nil {
		return nil, []error{err}
	}

	requestedVariantsByTestID := map[string]map[string]string{}
	for _, tid := range reqOptions.TestIDOptions {
		if len(tid.RequestedVariants) > 0 {
			requestedVariantsByTestID[tid.TestID] = tid.RequestedVariants
		}
	}

	result := map[string][]crstatus.TestJobRunRows{}
	for _, row := range rows {
		variants, ok := jobVariantMap[row.ProwJobID]
		if !ok {
			continue
		}

		if rv, ok := requestedVariantsByTestID[row.TestID]; ok {
			match := true
			for k, v := range rv {
				if variants[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		filtered := filterByDBGroupBy(variants, dbGroupBy)
		key := crtest.KeyWithVariants{
			TestID:   row.TestID,
			Variants: filtered,
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
			ProwJob:         normalizedName,
			ProwJobRunID:    row.ProwJobRunID,
			ProwJobURL:      row.ProwJobURL,
			StartTime:       row.ProwJobStart,
			Count:           crtest.Count{TotalCount: 1, SuccessCount: successCount, FlakeCount: flakeCount},
			JiraComponent:   row.JiraComponent,
			JiraComponentID: jiraComponentID,
		}

		result[normalizedName] = append(result[normalizedName], entry)
	}

	return result, nil
}

func (p *PostgresProvider) QueryBaseJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions) (map[string][]crstatus.TestJobRunRows, []error) {

	return p.queryTestDetails(
		ctx,
		reqOptions.BaseRelease.Name,
		reqOptions.BaseRelease.Start, reqOptions.BaseRelease.End,
		reqOptions, reqOptions.VariantOption.IncludeVariants,
	)
}

func (p *PostgresProvider) QuerySampleJobRunTestStatus(ctx context.Context, reqOptions reqopts.RequestOptions,
	includeVariants map[string][]string,
	start, end time.Time) (map[string][]crstatus.TestJobRunRows, []error) {
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
