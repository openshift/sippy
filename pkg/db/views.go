package db

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

const replaceTimeNow = "|||TIMENOW|||"
const timestampFormat = "2006-01-02 15:04:05"

// TODO: for historical sippy we need to specify the pinnedDate and not use NOW
var PostgresMatViews = []PostgresView{
	{
		Name:              "prow_job_runs_report_matview",
		Definition:        jobRunsReportMatView,
		IndexColumns:      []string{"id"},
		AdditionalIndexes: []string{"release, timestamp DESC"},
	},
	{
		// TODO: this probably doesn't need to be a matview anymore since we only keep 3 months of data,
		// metrics show this refreshing in .6s a lot of the time, occasionally up to 5s.
		Name:           "payload_test_failures_14d_matview",
		Definition:     payloadTestFailuresMatView,
		IndexColumns:   []string{"release", "architecture", "stream", "prow_job_run_id", "test_id", "suite_id"},
		ReplaceStrings: map[string]string{},
	},
	{
		Name:         "prow_ga_test_statuses_matview",
		Definition:   gaTestStatusMatView,
		IndexColumns: []string{"release", "window_days", "test_id", "suite_id", "variant_combination_id"},
		RefreshPhase: 1,
	},
}

// PostgresViews are regular, non-materialized views:
var PostgresViews = []PostgresView{
	{
		Name:       "prow_test_analysis_by_variant_14d_view",
		Definition: testAnalysisByVariantView,
	},
}

type PostgresView struct {
	// Name is the name of the materialized view in postgres.
	Name string
	// Definition is the material view definition.
	Definition string
	// ReplaceStrings is a map of strings we want to replace in the create view statement, allowing for re-use.
	ReplaceStrings map[string]string
	// IndexColumns are the columns to create a unique index for. Will be named idx_[Name] and automatically
	// replaced if changes are made to these values. IndexColumns are required as we need them defined to be able to
	// refresh materialized views concurrently. (avoiding locking reads for several minutes while we update)
	IndexColumns []string
	// AdditionalIndexes are non-unique indexes to create on the materialized view for query performance.
	// Each entry is a raw column expression (e.g. "release, timestamp DESC") and will be named
	// idx_[Name]_[sequence].
	AdditionalIndexes []string
	// RefreshPhase controls the order in which matviews are refreshed. All matviews
	// in phase 0 refresh first (concurrently), then all in phase 1, and so on.
	// Use this when a matview reads from another matview and needs it to be up-to-date.
	// The default zero value means phase 0.
	RefreshPhase int
}

// RefreshByPhase groups matviews by RefreshPhase and calls refreshFn for each
// phase in order. All matviews in a phase are passed to refreshFn together
// (the caller is responsible for concurrent execution within a phase).
func RefreshByPhase(matviews []PostgresView, refreshFn func([]PostgresView)) {
	sorted := make([]PostgresView, len(matviews))
	copy(sorted, matviews)
	slices.SortFunc(sorted, func(a, b PostgresView) int {
		return a.RefreshPhase - b.RefreshPhase
	})

	for i := 0; i < len(sorted); {
		phase := sorted[i].RefreshPhase
		j := i
		for j < len(sorted) && sorted[j].RefreshPhase == phase {
			j++
		}
		refreshFn(sorted[i:j])
		i = j
	}
}

func syncPostgresMaterializedViews(db *gorm.DB, reportEnd *time.Time) error {

	// initialize outside our loop
	reportEndFmt := "NOW()"

	if reportEnd != nil {
		reportEndFmt = "TO_TIMESTAMP('" + reportEnd.UTC().Format(timestampFormat) + "', 'YYYY-MM-DD HH24:MI:SS')"
	}

	for _, pmv := range PostgresMatViews {
		// Sync materialized view:
		viewDef := pmv.Definition
		for k, v := range pmv.ReplaceStrings {
			viewDef = strings.ReplaceAll(viewDef, k, v)
		}

		// This has to occur after the replaceAll above as they might contain the REPLACE_TIME_NOW constant as well
		viewDef = strings.ReplaceAll(viewDef, replaceTimeNow, reportEndFmt)

		// CASCADE is safe here: dependent matviews (e.g. collapsed matviews) are
		// all in PostgresMatViews and will be detected as missing and recreated
		// by this same sync loop.
		dropSQL := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s CASCADE", pmv.Name)
		schema := fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS %s WITH NO DATA", pmv.Name, viewDef)
		matViewUpdated, err := syncSchema(db, hashTypeMatView, pmv.Name, schema, dropSQL, false)
		if err != nil {
			return err
		}

		// Sync index for the materialized view:
		indexName := fmt.Sprintf("idx_%s", pmv.Name)
		index := fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s(%s)", indexName, pmv.Name, strings.Join(pmv.IndexColumns, ","))
		dropSQL = fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
		if _, err := syncSchema(db, hashTypeMatViewIndex, indexName, index, dropSQL, matViewUpdated); err != nil {
			return err
		}

		for i, cols := range pmv.AdditionalIndexes {
			idxName := fmt.Sprintf("idx_%s_%d", pmv.Name, i)
			idxSQL := fmt.Sprintf("CREATE INDEX %s ON %s(%s)", idxName, pmv.Name, cols)
			dropSQL = fmt.Sprintf("DROP INDEX IF EXISTS %s", idxName)
			if _, err := syncSchema(db, hashTypeMatViewIndex, idxName, idxSQL, dropSQL, matViewUpdated); err != nil {
				return err
			}
		}
	}

	return nil
}

func syncPostgresViews(db *gorm.DB, reportEnd *time.Time) error {

	// initialize outside our loop
	reportEndFmt := "NOW()"

	if reportEnd != nil {
		reportEndFmt = "TO_TIMESTAMP('" + reportEnd.UTC().Format(timestampFormat) + "', 'YYYY-MM-DD HH24:MI:SS')"
	}

	for _, pmv := range PostgresViews {
		// Sync view:
		viewDef := pmv.Definition
		for k, v := range pmv.ReplaceStrings {
			viewDef = strings.ReplaceAll(viewDef, k, v)
		}

		// This has to occur after the replaceAll above as they might contain the REPLACE_TIME_NOW constant as well
		viewDef = strings.ReplaceAll(viewDef, replaceTimeNow, reportEndFmt)

		dropSQL := fmt.Sprintf("DROP VIEW IF EXISTS %s", pmv.Name)
		schema := fmt.Sprintf("CREATE OR REPLACE VIEW %s AS %s", pmv.Name, viewDef)
		_, err := syncSchema(db, hashTypeView, pmv.Name, schema, dropSQL, false)
		if err != nil {
			return err
		}
	}

	return nil
}

// jobRunsReportMatView limits all data to a 90-day window. This is intentional:
// prow_job_run_tests is heavily partitioned and scanning beyond 90 days is expensive
// with no consumer needing older per-test failure/flake details in this view.
const jobRunsReportMatView = `
WITH test_results AS (
	SELECT prow_job_run_tests.prow_job_run_id,
		prow_job_run_tests.prow_job_run_release,
		count(tests.id)       FILTER (WHERE prow_job_run_tests.status = 12) AS failed_test_count,
		array_agg(tests.name) FILTER (WHERE prow_job_run_tests.status = 12) AS failed_test_names,
		count(tests.id)       FILTER (WHERE prow_job_run_tests.status = 13) AS flaked_test_count,
		array_agg(tests.name) FILTER (WHERE prow_job_run_tests.status = 13) AS flaked_test_names
	FROM prow_job_run_tests
		JOIN tests ON tests.id = prow_job_run_tests.test_id
	WHERE prow_job_run_tests.status IN (12, 13)
		AND prow_job_run_tests.prow_job_run_timestamp >= |||TIMENOW||| - interval '90 days'
	GROUP BY prow_job_run_tests.prow_job_run_id, prow_job_run_tests.prow_job_run_release
),
pull_requests AS (
	SELECT
		DISTINCT ON(prow_job_runs.id)
		prow_job_runs.id as id,
		prow_pull_requests.link,
		prow_pull_requests.sha,
		prow_pull_requests.org,
		prow_pull_requests.author,
		prow_pull_requests.repo
        FROM
                prow_pull_requests
        INNER JOIN
                prow_job_run_prow_pull_requests ON prow_job_run_prow_pull_requests.prow_pull_request_id = prow_pull_requests.id
        INNER JOIN
                prow_job_runs ON prow_job_run_prow_pull_requests.prow_job_run_id = prow_job_runs.id
        WHERE prow_job_runs."timestamp" >= |||TIMENOW||| - interval '90 days'
        GROUP BY prow_job_runs.id, prow_pull_requests.link, prow_pull_requests.sha, prow_pull_requests.org, prow_pull_requests.repo, prow_pull_requests.author
)
SELECT prow_job_runs.id,
   prow_jobs.release,
   prow_jobs.name,
   prow_jobs.name AS job,
   prow_jobs.variants,
   regexp_replace(prow_jobs.name, 'periodic-ci-openshift-(multiarch|release)-master-(ci|nightly)-[0-9]+.[0-9]+-'::text, ''::text) AS brief_name,
   prow_job_runs.overall_result,
   prow_job_runs.url AS test_grid_url,
   prow_job_runs.url,
   prow_job_runs.succeeded,
   prow_job_runs.infrastructure_failure,
   prow_job_runs.known_failure,
   (EXTRACT(epoch FROM (prow_job_runs."timestamp" AT TIME ZONE 'utc'::text)) * 1000::numeric)::bigint AS "timestamp",
   prow_job_runs.id AS prow_id,
   prow_job_runs.cluster AS cluster,
   prow_job_runs.labels as labels,
   test_results.flaked_test_names AS flaked_test_names,
   test_results.flaked_test_count AS test_flakes,
   test_results.failed_test_names AS failed_test_names,
   test_results.failed_test_count AS test_failures,
   pull_requests.link as pull_request_link,
   pull_requests.sha as pull_request_sha,
   pull_requests.org as pull_request_org,
   pull_requests.repo as pull_request_repo,
   pull_requests.author as pull_request_author
FROM prow_job_runs
   LEFT JOIN test_results ON test_results.prow_job_run_id = prow_job_runs.id
       AND test_results.prow_job_run_release = prow_job_runs.prow_job_release
   LEFT JOIN pull_requests ON pull_requests.id = prow_job_runs.id
   JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
WHERE prow_job_runs."timestamp" >= |||TIMENOW||| - interval '90 days'
`
const testAnalysisByVariantView = `
SELECT
	byjob.test_id AS test_id,
	byjob.test_name AS test_name,
	byjob.date AS date,
	unnest(prow_jobs.variants) AS variant,
	prow_jobs.release,
	SUM(runs) as runs,
	SUM(passes) as passes,
	SUM(flakes) as flakes,
	SUM(failures) as failures
FROM
	test_analysis_by_job_by_dates byjob
	JOIN tests ON tests.id = byjob.test_id
	JOIN prow_jobs ON prow_jobs.name = byjob.job_name
WHERE
	byjob.date >= (|||TIMENOW||| - '15 days'::interval)
GROUP BY
	tests.name, tests.id, byjob.test_id, byjob.test_name, date, unnest(prow_jobs.variants), prow_jobs.release
`

const testAnalysisByJobMatView = `
SELECT
    tests.id AS test_id,
    tests.name AS test_name,
    date(prow_job_run_tests.prow_job_run_timestamp) AS date,
    prow_job_run_tests.prow_job_run_release AS release,
    prow_jobs.name AS job_name,
    COUNT(*) FILTER (WHERE prow_job_run_tests.prow_job_run_timestamp >= (|||TIMENOW||| - '14 days'::interval) AND prow_job_run_tests.prow_job_run_timestamp <= |||TIMENOW|||) AS runs,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 1 AND prow_job_run_tests.prow_job_run_timestamp >= (|||TIMENOW||| - '14 days'::interval) AND prow_job_run_tests.prow_job_run_timestamp <= |||TIMENOW|||) AS passes,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 13 AND prow_job_run_tests.prow_job_run_timestamp >= (|||TIMENOW||| - '14 days'::interval) AND prow_job_run_tests.prow_job_run_timestamp <= |||TIMENOW|||) AS flakes,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 12 AND prow_job_run_tests.prow_job_run_timestamp >= (|||TIMENOW||| - '14 days'::interval) AND prow_job_run_tests.prow_job_run_timestamp <= |||TIMENOW|||) AS failures
FROM
    prow_job_run_tests
    JOIN tests ON tests.id = prow_job_run_tests.test_id
    JOIN prow_jobs ON prow_jobs.id = prow_job_run_tests.prow_job_id
WHERE
    prow_job_run_tests.prow_job_run_timestamp > (|||TIMENOW||| - '14 days'::interval)
GROUP BY
    tests.name, tests.id, date(prow_job_run_tests.prow_job_run_timestamp), prow_job_run_tests.prow_job_run_release, prow_jobs.name
`

// TODO: remove distinct once bug fixed re dupes in release_job_runs
const payloadTestFailuresMatView = `
SELECT DISTINCT
       rt.release,
       rt.architecture,
       rt.stream,
	   rt.release_tag,
       pjrt.id, 
       pjrt.test_id,
       pjrt.suite_id,
       pjrt.status,
       t.name,
       pjrt.prow_job_run_id as prow_job_run_id,
       pjr.url as prow_job_run_url,
       pj.name as prow_job_name
FROM
     release_tags rt,
     release_job_runs rjr,
     prow_job_run_tests pjrt,
     tests t,
     prow_jobs pj,
     prow_job_runs pjr
WHERE
    rt.release_time > (|||TIMENOW||| - '14 days'::interval)
    AND pjrt.prow_job_run_timestamp > (|||TIMENOW||| - '14 days'::interval)
    AND pjr.timestamp > (|||TIMENOW||| - '14 days'::interval)
    AND rjr.release_tag_id = rt.id
    AND rjr.kind = 'Blocking'
    AND rjr.State = 'Failed'
    AND pjrt.prow_job_run_id = rjr.prow_job_run_id
    AND pjrt.prow_job_run_release = rt.release
    AND pjrt.prow_job_run_timestamp = pjr.timestamp
    AND pjrt.status = 12
    AND t.id = pjrt.test_id
    AND pjr.id = pjrt.prow_job_run_id
    AND pj.id = pjr.prow_job_id
ORDER BY pjrt.id DESC
`

const gaTestStatusMatView = `
SELECT
    raw.test_id,
    raw.suite_id,
    pj.variant_combination_id,
    raw.release,
    raw.window_days,
    SUM(raw.runs)::int AS total_count,
    SUM(raw.passes + raw.flakes)::int AS success_count,
    SUM(raw.flakes)::int AS flake_count
FROM prow_ga_raw_test_data raw
JOIN prow_jobs pj ON pj.id = raw.prow_job_id AND pj.deleted_at IS NULL AND pj.variant_combination_id IS NOT NULL
GROUP BY raw.test_id, raw.suite_id, pj.variant_combination_id, raw.release, raw.window_days
`
