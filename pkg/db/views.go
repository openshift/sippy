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
		Name:         "prow_test_report_7d_matview",
		Definition:   testReportMatView,
		IndexColumns: []string{"release", "name", "id", "variant_combination_id", "suite_name"},
		ReplaceStrings: map[string]string{
			"|||START|||":    "|||TIMENOW||| - INTERVAL '14 DAY'",
			"|||BOUNDARY|||": "|||TIMENOW||| - INTERVAL '7 DAY'",
			"|||END|||":      "|||TIMENOW|||",
		},
	},
	{
		Name:         "prow_test_report_2d_matview",
		Definition:   testReportMatView,
		IndexColumns: []string{"release", "name", "id", "variant_combination_id", "suite_name"},
		RefreshPhase: 1, // avoid CPU overload from refreshing concurrently with the 7d matview
		ReplaceStrings: map[string]string{
			"|||START|||":    "|||TIMENOW||| - INTERVAL '9 DAY'",
			"|||BOUNDARY|||": "|||TIMENOW||| - INTERVAL '2 DAY'",
			"|||END|||":      "|||TIMENOW|||",
		},
	},
	{
		Name:              "prow_job_runs_report_matview",
		Definition:        jobRunsReportMatView,
		IndexColumns:      []string{"id"},
		AdditionalIndexes: []string{"release, timestamp DESC"},
	},
	{
		Name:         "prow_job_failed_tests_by_day_matview",
		Definition:   prowJobFailedTestsMatView,
		IndexColumns: []string{"period", "prow_job_id", "test_name"},
		ReplaceStrings: map[string]string{
			"|||BY|||": "day",
		},
	},
	{
		Name:         "prow_job_failed_tests_by_hour_matview",
		Definition:   prowJobFailedTestsMatView,
		IndexColumns: []string{"period", "prow_job_id", "test_name"},
		ReplaceStrings: map[string]string{
			"|||BY|||": "hour",
		},
	},
	{
		Name:         "prow_test_report_7d_collapsed_matview",
		Definition:   testReportCollapsedMatView,
		IndexColumns: []string{"release", "id", "suite_name", "jira_component", "jira_component_id"},
		RefreshPhase: 2, // reads from prow_test_report_7d_matview, which refreshes in phase 0
		ReplaceStrings: map[string]string{
			"|||SOURCE|||": "prow_test_report_7d_matview",
		},
	},
	{
		Name:         "prow_test_report_2d_collapsed_matview",
		Definition:   testReportCollapsedMatView,
		IndexColumns: []string{"release", "id", "suite_name", "jira_component", "jira_component_id"},
		RefreshPhase: 2, // reads from prow_test_report_2d_matview, which refreshes in phase 1
		ReplaceStrings: map[string]string{
			"|||SOURCE|||": "prow_test_report_2d_matview",
		},
	},
	{
		// TODO: this probably doesn't need to be a matview anymore since we only keep 3 months of data,
		// metrics show this refreshing in .6s a lot of the time, occasionally up to 5s.
		Name:           "payload_test_failures_14d_matview",
		Definition:     payloadTestFailuresMatView,
		IndexColumns:   []string{"release", "architecture", "stream", "prow_job_run_id", "test_id", "suite_id"},
		ReplaceStrings: map[string]string{},
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
const testReportMatView = `
SELECT base.*,
    COALESCE(base.current_successes * 100.0 / NULLIF(base.current_runs, 0), 0) AS current_pass_percentage,
    COALESCE(base.current_failures * 100.0 / NULLIF(base.current_runs, 0), 0) AS current_failure_percentage,
    COALESCE(base.current_flakes * 100.0 / NULLIF(base.current_runs, 0), 0) AS current_flake_percentage,
    COALESCE((base.current_successes + base.current_flakes) * 100.0 / NULLIF(base.current_runs, 0), 0) AS current_working_percentage,
    COALESCE(base.previous_successes * 100.0 / NULLIF(base.previous_runs, 0), 0) AS previous_pass_percentage,
    COALESCE(base.previous_failures * 100.0 / NULLIF(base.previous_runs, 0), 0) AS previous_failure_percentage,
    COALESCE(base.previous_flakes * 100.0 / NULLIF(base.previous_runs, 0), 0) AS previous_flake_percentage,
    COALESCE((base.previous_successes + base.previous_flakes) * 100.0 / NULLIF(base.previous_runs, 0), 0) AS previous_working_percentage,
    AVG((base.current_successes + base.current_flakes) * 100.0 / NULLIF(base.current_runs, 0)) OVER w AS working_average,
    STDDEV((base.current_successes + base.current_flakes) * 100.0 / NULLIF(base.current_runs, 0)) OVER w AS working_standard_deviation,
    AVG(base.current_successes * 100.0 / NULLIF(base.current_runs, 0)) OVER w AS passing_average,
    STDDEV(base.current_successes * 100.0 / NULLIF(base.current_runs, 0)) OVER w AS passing_standard_deviation,
    AVG(base.current_flakes * 100.0 / NULLIF(base.current_runs, 0)) OVER w AS flake_average,
    STDDEV(base.current_flakes * 100.0 / NULLIF(base.current_runs, 0)) OVER w AS flake_standard_deviation
FROM (
    WITH open_bugs AS (
      SELECT
        test_id,
        COUNT(DISTINCT bugs.id) AS open_bugs
      FROM
        bug_tests
        INNER JOIN tests ON tests.id = bug_tests.test_id
        INNER JOIN bugs ON bug_tests.bug_id = bugs.id
      WHERE
        LOWER(bugs.status) <> 'closed'
      GROUP BY
        test_id
    ),
    pre_agg AS (
      SELECT
        variant_combination_id,
        test_id,
        suite_id,
        release AS prow_job_run_release,
        COALESCE(SUM(successes) FILTER (WHERE summary_date >= |||START||| AND summary_date < |||BOUNDARY|||), 0) AS previous_successes,
        COALESCE(SUM(flakes)    FILTER (WHERE summary_date >= |||START||| AND summary_date < |||BOUNDARY|||), 0) AS previous_flakes,
        COALESCE(SUM(failures)  FILTER (WHERE summary_date >= |||START||| AND summary_date < |||BOUNDARY|||), 0) AS previous_failures,
        COALESCE(SUM(runs)      FILTER (WHERE summary_date >= |||START||| AND summary_date < |||BOUNDARY|||), 0) AS previous_runs,
        COALESCE(SUM(successes) FILTER (WHERE summary_date >= |||BOUNDARY||| AND summary_date <= |||END|||), 0) AS current_successes,
        COALESCE(SUM(flakes)    FILTER (WHERE summary_date >= |||BOUNDARY||| AND summary_date <= |||END|||), 0) AS current_flakes,
        COALESCE(SUM(failures)  FILTER (WHERE summary_date >= |||BOUNDARY||| AND summary_date <= |||END|||), 0) AS current_failures,
        COALESCE(SUM(runs)      FILTER (WHERE summary_date >= |||BOUNDARY||| AND summary_date <= |||END|||), 0) AS current_runs
      FROM
        test_daily_summaries
      WHERE
        summary_date >= |||START||| AND summary_date <= |||END|||
      GROUP BY
        variant_combination_id, test_id, suite_id, release
    )
    SELECT
        tests.id,
        tests.name,
        suites.name AS suite_name,
        jira_components.name AS jira_component,
        jira_components.id AS jira_component_id,
        pre_agg.previous_successes::bigint AS previous_successes,
        pre_agg.previous_flakes::bigint AS previous_flakes,
        pre_agg.previous_failures::bigint AS previous_failures,
        pre_agg.previous_runs::bigint AS previous_runs,
        pre_agg.current_successes::bigint AS current_successes,
        pre_agg.current_flakes::bigint AS current_flakes,
        pre_agg.current_failures::bigint AS current_failures,
        pre_agg.current_runs::bigint AS current_runs,
        open_bugs.open_bugs AS open_bugs,
        vc.variants,
        pre_agg.variant_combination_id,
        pre_agg.prow_job_run_release AS release
    FROM
        pre_agg
        JOIN tests ON tests.id = pre_agg.test_id
        LEFT JOIN open_bugs ON pre_agg.test_id = open_bugs.test_id
        LEFT JOIN suites ON suites.id = pre_agg.suite_id
        LEFT JOIN test_ownerships ON (tests.id = test_ownerships.test_id AND pre_agg.suite_id = test_ownerships.suite_id)
        LEFT JOIN jira_components ON test_ownerships.jira_component = jira_components.name
        LEFT JOIN variant_combinations vc ON pre_agg.variant_combination_id = vc.id
) AS base
WINDOW w AS (PARTITION BY base.id, base.suite_name, base.release)
`

// CollapsedVariantExclusions lists the variant values that are pre-excluded in
// the collapsed matview. The API checks incoming variant filters against this
// list to decide whether the collapsed matview can be used.
var CollapsedVariantExclusions = []string{"never-stable", "aggregated"}

var testReportCollapsedMatView = buildCollapsedMatViewSQL()

func buildCollapsedMatViewSQL() string {
	var clauses []string
	for _, v := range CollapsedVariantExclusions {
		clauses = append(clauses, fmt.Sprintf("NOT ('%s' = any(variants))", v))
	}
	return `
SELECT suite_name, name, id, jira_component, jira_component_id, release,
    SUM(current_runs)::bigint AS current_runs,
    SUM(current_successes)::bigint AS current_successes,
    SUM(current_failures)::bigint AS current_failures,
    SUM(current_flakes)::bigint AS current_flakes,
    SUM(previous_runs)::bigint AS previous_runs,
    SUM(previous_successes)::bigint AS previous_successes,
    SUM(previous_failures)::bigint AS previous_failures,
    SUM(previous_flakes)::bigint AS previous_flakes,
    (array_agg(open_bugs))[1] AS open_bugs
FROM |||SOURCE|||
WHERE ` + strings.Join(clauses, "\n  AND ") + `
GROUP BY suite_name, name, id, jira_component, jira_component_id, release
`
}

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

const prowJobFailedTestsMatView = `
SELECT date_trunc('|||BY|||'::text, pjrt.prow_job_run_timestamp) AS period,
   pjrt.prow_job_id,
   tests.name AS test_name,
   count(tests.name) AS count
FROM prow_job_run_tests pjrt
   JOIN tests tests ON pjrt.test_id = tests.id
WHERE pjrt.status = 12
GROUP BY tests.name, (date_trunc('|||BY|||'::text, pjrt.prow_job_run_timestamp)), pjrt.prow_job_id
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
    AND pjrt.status = 12
    AND t.id = pjrt.test_id
    AND pjr.id = pjrt.prow_job_run_id
    AND pj.id = pjr.prow_job_id
ORDER BY pjrt.id DESC
`
