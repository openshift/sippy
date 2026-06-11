-- Verify that the optimized testReportMatView (pre-aggregation CTE,
-- no prow_job_runs join) produces identical results to the original query.
--
-- Uses the 7d matview parameters: 14d lookback, 7d boundary, NOW() end.
-- Run against the prod-like database:
--   podman exec sippy-postgres psql -U postgres -d prodlike -f scripts/sql/verify_matview_optimization.sql

BEGIN;

-- Pin a single timestamp so both queries use the exact same window.
SET LOCAL timezone = 'UTC';
DO $$ BEGIN RAISE NOTICE 'Report end time: %', NOW(); END $$;

CREATE TEMPORARY TABLE old_results ON COMMIT DROP AS
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
)
SELECT
    tests.id,
    tests.name,
    suites.name AS suite_name,
    jira_components.name AS jira_component,
    jira_components.id AS jira_component_id,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 1  AND prow_job_runs."timestamp" BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY') AS previous_successes,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 13 AND prow_job_runs."timestamp" BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY') AS previous_flakes,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 12 AND prow_job_runs."timestamp" BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY') AS previous_failures,
    COUNT(*) FILTER (WHERE prow_job_runs."timestamp" BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY') AS previous_runs,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 1  AND prow_job_runs."timestamp" BETWEEN NOW() - INTERVAL '7 DAY' AND NOW()) AS current_successes,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 13 AND prow_job_runs."timestamp" BETWEEN NOW() - INTERVAL '7 DAY' AND NOW()) AS current_flakes,
    COUNT(*) FILTER (WHERE prow_job_run_tests.status = 12 AND prow_job_runs."timestamp" BETWEEN NOW() - INTERVAL '7 DAY' AND NOW()) AS current_failures,
    COUNT(*) FILTER (WHERE prow_job_runs."timestamp" BETWEEN NOW() - INTERVAL '7 DAY' AND NOW()) AS current_runs,
    open_bugs.open_bugs AS open_bugs,
    prow_jobs.variants,
    prow_jobs.release
FROM
    prow_job_run_tests
    JOIN tests ON tests.id = prow_job_run_tests.test_id
    LEFT JOIN open_bugs ON prow_job_run_tests.test_id = open_bugs.test_id
    LEFT JOIN suites ON suites.id = prow_job_run_tests.suite_id
    LEFT JOIN test_ownerships ON (tests.id = test_ownerships.test_id AND prow_job_run_tests.suite_id = test_ownerships.suite_id)
    LEFT JOIN jira_components ON test_ownerships.jira_component = jira_components.name
    JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id
    JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
WHERE
    prow_job_run_tests.created_at >= NOW() - INTERVAL '14 DAY' AND prow_job_runs.timestamp >= NOW() - INTERVAL '14 DAY'
GROUP BY
    tests.id, tests.name, jira_components.name, jira_components.id, suites.name, open_bugs.open_bugs, prow_jobs.variants, prow_jobs.release;

CREATE TEMPORARY TABLE new_results ON COMMIT DROP AS
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
    prow_job_id, test_id, suite_id,
    COUNT(*) FILTER (WHERE status = 1  AND prow_job_run_timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY') AS previous_successes,
    COUNT(*) FILTER (WHERE status = 13 AND prow_job_run_timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY') AS previous_flakes,
    COUNT(*) FILTER (WHERE status = 12 AND prow_job_run_timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY') AS previous_failures,
    COUNT(*) FILTER (WHERE prow_job_run_timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY') AS previous_runs,
    COUNT(*) FILTER (WHERE status = 1  AND prow_job_run_timestamp BETWEEN NOW() - INTERVAL '7 DAY' AND NOW()) AS current_successes,
    COUNT(*) FILTER (WHERE status = 13 AND prow_job_run_timestamp BETWEEN NOW() - INTERVAL '7 DAY' AND NOW()) AS current_flakes,
    COUNT(*) FILTER (WHERE status = 12 AND prow_job_run_timestamp BETWEEN NOW() - INTERVAL '7 DAY' AND NOW()) AS current_failures,
    COUNT(*) FILTER (WHERE prow_job_run_timestamp BETWEEN NOW() - INTERVAL '7 DAY' AND NOW()) AS current_runs
  FROM prow_job_run_tests
  WHERE prow_job_run_timestamp >= NOW() - INTERVAL '14 DAY'
  GROUP BY prow_job_id, test_id, suite_id
)
SELECT
    tests.id,
    tests.name,
    suites.name AS suite_name,
    jira_components.name AS jira_component,
    jira_components.id AS jira_component_id,
    SUM(pre_agg.previous_successes) AS previous_successes,
    SUM(pre_agg.previous_flakes) AS previous_flakes,
    SUM(pre_agg.previous_failures) AS previous_failures,
    SUM(pre_agg.previous_runs) AS previous_runs,
    SUM(pre_agg.current_successes) AS current_successes,
    SUM(pre_agg.current_flakes) AS current_flakes,
    SUM(pre_agg.current_failures) AS current_failures,
    SUM(pre_agg.current_runs) AS current_runs,
    open_bugs.open_bugs AS open_bugs,
    prow_jobs.variants,
    prow_jobs.release
FROM
    pre_agg
    JOIN tests ON tests.id = pre_agg.test_id
    LEFT JOIN open_bugs ON pre_agg.test_id = open_bugs.test_id
    LEFT JOIN suites ON suites.id = pre_agg.suite_id
    LEFT JOIN test_ownerships ON (tests.id = test_ownerships.test_id AND pre_agg.suite_id = test_ownerships.suite_id)
    LEFT JOIN jira_components ON test_ownerships.jira_component = jira_components.name
    JOIN prow_jobs ON pre_agg.prow_job_id = prow_jobs.id
GROUP BY
    tests.id, tests.name, jira_components.name, jira_components.id, suites.name, open_bugs.open_bugs, prow_jobs.variants, prow_jobs.release;

-- Compare row counts
SELECT
    (SELECT COUNT(*) FROM old_results) AS old_count,
    (SELECT COUNT(*) FROM new_results) AS new_count;

-- Rows in old but not new
SELECT 'IN OLD ONLY' AS diff_direction, * FROM (
    SELECT * FROM old_results EXCEPT SELECT * FROM new_results
) t LIMIT 20;

-- Rows in new but not old
SELECT 'IN NEW ONLY' AS diff_direction, * FROM (
    SELECT * FROM new_results EXCEPT SELECT * FROM old_results
) t LIMIT 20;

COMMIT;
