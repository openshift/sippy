package postgres

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db"
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

// variantMapToSlice converts a map to sorted "Key:Value" strings.
func variantMapToSlice(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+":"+v)
	}
	sort.Strings(result)
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

func (p *PostgresProvider) QueryJobVariants(_ context.Context) (crtest.JobVariants, []error) {
	variants := crtest.JobVariants{Variants: map[string][]string{}}

	var pairs []string
	err := p.dbc.DB.Raw(`SELECT DISTINCT unnest(variants) AS pair FROM prow_jobs WHERE deleted_at IS NULL`).
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

// releaseMetadata holds hardcoded release info for known releases.
// This avoids needing a releases table — we derive release names from prow_jobs
// and fill in metadata from this map.
var releaseMetadata = map[string]struct {
	previousRelease string
	gaOffsetDays    int // 0 = no GA date (in development)
}{
	"4.17": {previousRelease: "4.16", gaOffsetDays: -540},
	"4.18": {previousRelease: "4.17", gaOffsetDays: -395},
	"4.19": {previousRelease: "4.18", gaOffsetDays: -289},
	"4.20": {previousRelease: "4.19", gaOffsetDays: -163},
	"4.21": {previousRelease: "4.20", gaOffsetDays: -58},
	"4.22": {previousRelease: "4.21"},
	"5.0":  {previousRelease: "4.22"},
}

func (p *PostgresProvider) QueryReleases(_ context.Context) ([]v1.Release, error) {
	var releaseNames []string
	err := p.dbc.DB.Raw(`SELECT DISTINCT release FROM prow_jobs WHERE deleted_at IS NULL ORDER BY release DESC`).
		Pluck("release", &releaseNames).Error
	if err != nil {
		return nil, fmt.Errorf("querying releases: %w", err)
	}

	caps := map[v1.ReleaseCapability]bool{
		v1.ComponentReadinessCap: true,
		v1.FeatureGatesCap:       true,
		v1.MetricsCap:            true,
		v1.PayloadTagsCap:        true,
		v1.SippyClassicCap:       true,
	}

	now := time.Now().UTC()
	var releases []v1.Release
	for _, name := range releaseNames {
		rel := v1.Release{
			Release:      name,
			Capabilities: caps,
		}
		if meta, ok := releaseMetadata[name]; ok {
			rel.PreviousRelease = meta.previousRelease
			if meta.gaOffsetDays != 0 {
				ga := now.AddDate(0, 0, meta.gaOffsetDays)
				rel.GADate = &ga
			}
		}
		releases = append(releases, rel)
	}
	return releases, nil
}

func (p *PostgresProvider) QueryReleaseDates(_ context.Context, _ reqopts.RequestOptions) ([]crtest.ReleaseTimeRange, []error) {
	// Derive time ranges from actual data in the DB rather than hardcoded GA dates.
	// This ensures fallback queries find data where it actually exists.
	type releaseRange struct {
		Release string
		Start   time.Time
		End     time.Time
	}
	var ranges []releaseRange
	err := p.dbc.DB.Raw(`
		SELECT pj.release,
		       MIN(pjr.timestamp) AS start,
		       MAX(pjr.timestamp) AS end
		FROM prow_job_runs pjr
		JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
		WHERE pj.deleted_at IS NULL AND pjr.deleted_at IS NULL
		GROUP BY pj.release
		ORDER BY pj.release DESC
	`).Scan(&ranges).Error
	if err != nil {
		return nil, []error{fmt.Errorf("querying release dates: %w", err)}
	}

	var dates []crtest.ReleaseTimeRange
	for _, r := range ranges {
		start := r.Start
		end := r.End
		dates = append(dates, crtest.ReleaseTimeRange{
			Release: r.Release,
			Start:   &start,
			End:     &end,
		})
	}
	return dates, nil
}

func (p *PostgresProvider) QueryUniqueVariantValues(_ context.Context, field string, nested bool) ([]string, error) {
	if nested {
		// Return all variant key names
		var pairs []string
		err := p.dbc.DB.Raw(`
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
	err := p.dbc.DB.Raw(`
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

// --- TestStatusQuerier ---

// testStatusRow is the result of the aggregation query.
type testStatusRow struct {
	TestID       string         `gorm:"column:test_id"`
	TestName     string         `gorm:"column:test_name"`
	TestSuite    string         `gorm:"column:test_suite"`
	Component    string         `gorm:"column:component"`
	Capabilities pq.StringArray `gorm:"column:capabilities;type:text[]"`
	ProwJobID    uint           `gorm:"column:prow_job_id"`
	TotalCount   int            `gorm:"column:total_count"`
	SuccessCount int            `gorm:"column:success_count"`
	FlakeCount   int            `gorm:"column:flake_count"`
	LastFailure  *time.Time     `gorm:"column:last_failure"`
}

const testStatusQuery = `
WITH deduped AS (
    SELECT DISTINCT ON (pjrt.prow_job_run_id, pjrt.test_id, pjrt.suite_id)
        pjrt.test_id, pjrt.suite_id, pjrt.status,
        pjr.timestamp, pj.id AS prow_job_id
    FROM prow_job_run_tests pjrt
    JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id
    JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
    WHERE pj.release = ?
      AND pjr.timestamp >= ? AND pjr.timestamp < ?
      AND pjrt.deleted_at IS NULL AND pjr.deleted_at IS NULL AND pj.deleted_at IS NULL
      AND (pjr.labels IS NULL OR NOT pjr.labels @> ARRAY['InfraFailure'])
    ORDER BY pjrt.prow_job_run_id, pjrt.test_id, pjrt.suite_id,
        CASE WHEN pjrt.status = 13 THEN 0 WHEN pjrt.status = 1 THEN 1 ELSE 2 END
)
SELECT
    tow.unique_id AS test_id,
    t.name AS test_name,
    COALESCE(s.name, '') AS test_suite,
    tow.component,
    tow.capabilities,
    d.prow_job_id,
    COUNT(*) AS total_count,
    SUM(CASE WHEN d.status IN (1, 13) THEN 1 ELSE 0 END) AS success_count,
    SUM(CASE WHEN d.status = 13 THEN 1 ELSE 0 END) AS flake_count,
    MAX(CASE WHEN d.status NOT IN (1, 13) THEN d.timestamp ELSE NULL END) AS last_failure
FROM deduped d
JOIN tests t ON t.id = d.test_id
JOIN test_ownerships tow ON tow.test_id = d.test_id
    AND (tow.suite_id = d.suite_id OR (tow.suite_id IS NULL AND d.suite_id IS NULL))
LEFT JOIN suites s ON s.id = d.suite_id
WHERE tow.staff_approved_obsolete = false
GROUP BY tow.unique_id, t.name, s.name, tow.component, tow.capabilities, d.prow_job_id
`

func (p *PostgresProvider) queryTestStatus(release string, start, end time.Time,
	_ crtest.JobVariants, includeVariants map[string][]string,
	dbGroupBy map[string]bool) (map[string]crstatus.TestStatus, []error) {

	var rows []testStatusRow
	if err := p.dbc.DB.Raw(testStatusQuery, release, start, end).Scan(&rows).Error; err != nil {
		return nil, []error{fmt.Errorf("querying test status: %w", err)}
	}

	// Batch-fetch all ProwJob variants we need
	jobVariantMap := p.fetchJobVariants(rows)

	result := map[string]crstatus.TestStatus{}
	for _, row := range rows {
		variants, ok := jobVariantMap[row.ProwJobID]
		if !ok {
			continue
		}

		if !matchesIncludeVariants(variants, includeVariants) {
			continue
		}

		filtered := filterByDBGroupBy(variants, dbGroupBy)
		key := crtest.KeyWithVariants{
			TestID:   row.TestID,
			Variants: filtered,
		}
		keyStr := key.KeyOrDie()

		existing, exists := result[keyStr]
		if exists {
			// Merge counts for same test+variant combo from different job runs
			existing.Count.TotalCount += row.TotalCount
			existing.Count.SuccessCount += row.SuccessCount
			existing.Count.FlakeCount += row.FlakeCount
			if row.LastFailure != nil && (existing.LastFailure.IsZero() || row.LastFailure.After(existing.LastFailure)) {
				existing.LastFailure = *row.LastFailure
			}
			result[keyStr] = existing
		} else {
			ts := crstatus.TestStatus{
				TestName:     row.TestName,
				TestSuite:    row.TestSuite,
				Component:    row.Component,
				Capabilities: row.Capabilities,
				Variants:     variantMapToSlice(filtered),
				Count: crtest.Count{
					TotalCount:   row.TotalCount,
					SuccessCount: row.SuccessCount,
					FlakeCount:   row.FlakeCount,
				},
			}
			if row.LastFailure != nil {
				ts.LastFailure = *row.LastFailure
			}
			result[keyStr] = ts
		}
	}

	return result, nil
}

// fetchJobVariants loads and caches ProwJob variant maps for the given rows.
func (p *PostgresProvider) fetchJobVariants(rows []testStatusRow) map[uint]map[string]string {
	jobIDs := map[uint]bool{}
	for _, r := range rows {
		jobIDs[r.ProwJobID] = true
	}

	ids := make([]uint, 0, len(jobIDs))
	for id := range jobIDs {
		ids = append(ids, id)
	}

	type jobRow struct {
		ID       uint           `gorm:"column:id"`
		Variants pq.StringArray `gorm:"column:variants;type:text[]"`
	}

	var jobRows []jobRow
	if err := p.dbc.DB.Raw(`SELECT id, variants FROM prow_jobs WHERE id IN (?)`, ids).Scan(&jobRows).Error; err != nil {
		log.WithError(err).Error("error fetching job variants")
		return map[uint]map[string]string{}
	}

	result := make(map[uint]map[string]string, len(jobRows))
	for _, jr := range jobRows {
		result[jr.ID] = parseVariants(jr.Variants)
	}
	return result
}

func (p *PostgresProvider) QueryBaseTestStatus(_ context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants) (map[string]crstatus.TestStatus, []error) {

	dbGroupBy := make(map[string]bool, reqOptions.VariantOption.DBGroupBy.Len())
	for _, k := range reqOptions.VariantOption.DBGroupBy.List() {
		dbGroupBy[k] = true
	}

	includeVariants := reqOptions.VariantOption.IncludeVariants
	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}

	return p.queryTestStatus(
		reqOptions.BaseRelease.Name,
		reqOptions.BaseRelease.Start,
		reqOptions.BaseRelease.End,
		allJobVariants,
		includeVariants,
		dbGroupBy,
	)
}

func (p *PostgresProvider) QuerySampleTestStatus(_ context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	_ string) (map[string]crstatus.TestStatus, []error) {

	dbGroupBy := make(map[string]bool, reqOptions.VariantOption.DBGroupBy.Len())
	for _, k := range reqOptions.VariantOption.DBGroupBy.List() {
		dbGroupBy[k] = true
	}

	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}

	return p.queryTestStatus(
		reqOptions.SampleRelease.Name,
		start, end,
		allJobVariants,
		includeVariants,
		dbGroupBy,
	)
}

// --- TestDetailsQuerier ---

type testDetailRow struct {
	TestID          string         `gorm:"column:test_id"`
	TestName        string         `gorm:"column:test_name"`
	ProwJobName     string         `gorm:"column:prowjob_name"`
	ProwJobRunID    string         `gorm:"column:prowjob_run_id"`
	ProwJobURL      string         `gorm:"column:prowjob_url"`
	ProwJobStart    time.Time      `gorm:"column:prowjob_start"`
	ProwJobID       uint           `gorm:"column:prow_job_id"`
	Status          int            `gorm:"column:status"`
	JiraComponent   string         `gorm:"column:jira_component"`
	JiraComponentID *uint          `gorm:"column:jira_component_id"`
	Capabilities    pq.StringArray `gorm:"column:capabilities;type:text[]"`
}

const testDetailQuery = `
SELECT
    tow.unique_id AS test_id,
    t.name AS test_name,
    pj.name AS prowjob_name,
    CAST(pjr.id AS TEXT) AS prowjob_run_id,
    COALESCE(pjr.url, '') AS prowjob_url,
    pjr.timestamp AS prowjob_start,
    pj.id AS prow_job_id,
    pjrt.status,
    COALESCE(tow.jira_component, '') AS jira_component,
    tow.jira_component_id,
    tow.capabilities
FROM prow_job_run_tests pjrt
JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id
JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
JOIN tests t ON t.id = pjrt.test_id
JOIN test_ownerships tow ON tow.test_id = pjrt.test_id
    AND (tow.suite_id = pjrt.suite_id OR (tow.suite_id IS NULL AND pjrt.suite_id IS NULL))
WHERE pj.release = ?
    AND pjr.timestamp >= ? AND pjr.timestamp < ?
    AND pjrt.deleted_at IS NULL AND pjr.deleted_at IS NULL AND pj.deleted_at IS NULL
    AND tow.staff_approved_obsolete = false
    AND (pjr.labels IS NULL OR NOT pjr.labels @> ARRAY['InfraFailure'])
ORDER BY pjr.timestamp
`

func (p *PostgresProvider) queryTestDetails(release string, start, end time.Time,
	reqOptions reqopts.RequestOptions, _ crtest.JobVariants,
	includeVariants map[string][]string) (map[string][]crstatus.TestJobRunRows, []error) {

	var rows []testDetailRow
	if err := p.dbc.DB.Raw(testDetailQuery, release, start, end).Scan(&rows).Error; err != nil {
		return nil, []error{fmt.Errorf("querying test details: %w", err)}
	}

	dbGroupBy := make(map[string]bool, reqOptions.VariantOption.DBGroupBy.Len())
	for _, k := range reqOptions.VariantOption.DBGroupBy.List() {
		dbGroupBy[k] = true
	}

	if includeVariants == nil {
		includeVariants = map[string][]string{}
	}

	// Batch-fetch job variants
	jobIDs := map[uint]bool{}
	for _, r := range rows {
		jobIDs[r.ProwJobID] = true
	}
	ids := make([]uint, 0, len(jobIDs))
	for id := range jobIDs {
		ids = append(ids, id)
	}
	type jobRow struct {
		ID       uint           `gorm:"column:id"`
		Variants pq.StringArray `gorm:"column:variants;type:text[]"`
	}
	var jobRows []jobRow
	if len(ids) > 0 {
		if err := p.dbc.DB.Raw(`SELECT id, variants FROM prow_jobs WHERE id IN (?)`, ids).Scan(&jobRows).Error; err != nil {
			return nil, []error{fmt.Errorf("fetching job variants: %w", err)}
		}
	}
	jobVariantMap := make(map[uint]map[string]string, len(jobRows))
	for _, jr := range jobRows {
		jobVariantMap[jr.ID] = parseVariants(jr.Variants)
	}

	// Filter test IDs if specified
	// Build test ID filter and per-test requested variant filters
	testIDFilter := map[string]bool{}
	requestedVariantsByTestID := map[string]map[string]string{}
	for _, tid := range reqOptions.TestIDOptions {
		testIDFilter[tid.TestID] = true
		if len(tid.RequestedVariants) > 0 {
			requestedVariantsByTestID[tid.TestID] = tid.RequestedVariants
		}
	}

	result := map[string][]crstatus.TestJobRunRows{}
	for _, row := range rows {
		if len(testIDFilter) > 0 && !testIDFilter[row.TestID] {
			continue
		}

		variants, ok := jobVariantMap[row.ProwJobID]
		if !ok {
			continue
		}
		if !matchesIncludeVariants(variants, includeVariants) {
			continue
		}

		// Filter by requested variants (exact match for specific test+variant combo)
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

		entry := crstatus.TestJobRunRows{
			TestKey:         key,
			TestKeyStr:      key.KeyOrDie(),
			TestName:        row.TestName,
			ProwJob:         utils.NormalizeProwJobName(row.ProwJobName),
			ProwJobRunID:    row.ProwJobRunID,
			ProwJobURL:      row.ProwJobURL,
			StartTime:       row.ProwJobStart,
			Count:           crtest.Count{TotalCount: 1, SuccessCount: successCount, FlakeCount: flakeCount},
			JiraComponent:   row.JiraComponent,
			JiraComponentID: jiraComponentID,
		}

		normalizedName := utils.NormalizeProwJobName(row.ProwJobName)
		result[normalizedName] = append(result[normalizedName], entry)
	}

	return result, nil
}

func (p *PostgresProvider) QueryBaseJobRunTestStatus(_ context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants) (map[string][]crstatus.TestJobRunRows, []error) {

	return p.queryTestDetails(
		reqOptions.BaseRelease.Name,
		reqOptions.BaseRelease.Start, reqOptions.BaseRelease.End,
		reqOptions, allJobVariants, reqOptions.VariantOption.IncludeVariants,
	)
}

func (p *PostgresProvider) QuerySampleJobRunTestStatus(_ context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	includeVariants map[string][]string,
	start, end time.Time,
	_ string) (map[string][]crstatus.TestJobRunRows, []error) {

	return p.queryTestDetails(
		reqOptions.SampleRelease.Name,
		start, end,
		reqOptions, allJobVariants, includeVariants,
	)
}

// --- JobQuerier ---

func (p *PostgresProvider) QueryJobRuns(_ context.Context, reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	release string, start, end time.Time) (map[string]dataprovider.JobRunStats, error) {

	type jobRunRow struct {
		JobName    string `gorm:"column:job_name"`
		TotalRuns  int    `gorm:"column:total_runs"`
		Successful int    `gorm:"column:successful_runs"`
	}

	var rows []jobRunRow
	err := p.dbc.DB.Raw(`
		SELECT
			pj.name AS job_name,
			COUNT(DISTINCT pjr.id) AS total_runs,
			COUNT(DISTINCT CASE WHEN pjr.succeeded THEN pjr.id END) AS successful_runs
		FROM prow_jobs pj
		JOIN prow_job_runs pjr ON pjr.prow_job_id = pj.id
		WHERE pj.release = ?
			AND pjr.timestamp >= ? AND pjr.timestamp < ?
			AND pj.deleted_at IS NULL AND pjr.deleted_at IS NULL
		GROUP BY pj.name
		ORDER BY pj.name
	`, release, start, end).Scan(&rows).Error
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
		if err := p.dbc.DB.Raw(`SELECT name, variants FROM prow_jobs WHERE name IN (?) AND deleted_at IS NULL`, jobNames).Scan(&jvRows).Error; err != nil {
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

func (p *PostgresProvider) QueryJobVariantValues(_ context.Context, jobNames []string,
	variantKeys []string) (map[string]map[string]string, error) {

	if len(jobNames) == 0 {
		return map[string]map[string]string{}, nil
	}

	type jvRow struct {
		Name     string         `gorm:"column:name"`
		Variants pq.StringArray `gorm:"column:variants;type:text[]"`
	}

	var rows []jvRow
	if err := p.dbc.DB.Raw(`SELECT name, variants FROM prow_jobs WHERE name IN (?) AND deleted_at IS NULL`, jobNames).Scan(&rows).Error; err != nil {
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

func (p *PostgresProvider) LookupJobVariants(_ context.Context, jobName string) (map[string]string, error) {
	type jvRow struct {
		Variants pq.StringArray `gorm:"column:variants;type:text[]"`
	}

	var row jvRow
	err := p.dbc.DB.Raw(`SELECT variants FROM prow_jobs WHERE name = ? AND deleted_at IS NULL LIMIT 1`, jobName).Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("looking up job variants: %w", err)
	}
	return parseVariants(row.Variants), nil
}
