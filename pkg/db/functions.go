package db

import (
	"fmt"

	"gorm.io/gorm"
)

type PostgresFunction struct {
	Name       string
	Definition string
}

var PostgresFunctions = []PostgresFunction{
	{
		Name:       "job_results",
		Definition: jobResultFunction,
	},
	{
		Name:       "test_results",
		Definition: testResultFunction,
	},
}

func syncPostgresFunctions(db *gorm.DB) error {
	for _, pgFunc := range PostgresFunctions {
		dropSQL := fmt.Sprintf("DROP FUNCTION IF EXISTS %s", pgFunc.Name)
		if _, err := syncSchema(db, hashTypeFunction, pgFunc.Name, pgFunc.Definition, dropSQL, false); err != nil {
			return err
		}
	}
	return nil
}

const testResultFunction = `
CREATE FUNCTION public.test_results(start timestamp without time zone, boundary timestamp without time zone, endstamp timestamp without time zone) RETURNS TABLE(id bigint, name text, previous_successes bigint, previous_flakes bigint, previous_failures bigint, previous_runs bigint, current_successes bigint, current_flakes bigint, current_failures bigint, current_runs bigint, current_pass_percentage double precision, current_failure_percentage double precision, previous_pass_percentage double precision, previous_failure_percentage double precision, net_improvement double precision, release text)
    LANGUAGE sql
    AS $_$
WITH results AS (
  SELECT
    tests.id AS id,
    coalesce(count(case when status = 1 AND prow_job_run_tests.prow_job_run_timestamp >= $1 AND prow_job_run_tests.prow_job_run_timestamp < $2 then 1 end), 0) AS previous_successes,
    coalesce(count(case when status = 13 AND prow_job_run_tests.prow_job_run_timestamp >= $1 AND prow_job_run_tests.prow_job_run_timestamp < $2 then 1 end), 0) AS previous_flakes,
    coalesce(count(case when status = 12 AND prow_job_run_tests.prow_job_run_timestamp >= $1 AND prow_job_run_tests.prow_job_run_timestamp < $2 then 1 end), 0) AS previous_failures,
    coalesce(count(case when prow_job_run_tests.prow_job_run_timestamp >= $1 AND prow_job_run_tests.prow_job_run_timestamp < $2 then 1 end), 0) as previous_runs,
    coalesce(count(case when status = 1 AND prow_job_run_tests.prow_job_run_timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_successes,
    coalesce(count(case when status = 13 AND prow_job_run_tests.prow_job_run_timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_flakes,
    coalesce(count(case when status = 12 AND prow_job_run_tests.prow_job_run_timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_failures,
    coalesce(count(case when prow_job_run_tests.prow_job_run_timestamp BETWEEN $2 AND $3 then 1 end), 0) as current_runs,
    prow_job_run_tests.prow_job_run_release AS release
FROM prow_job_run_tests
    JOIN tests ON tests.id = prow_job_run_tests.test_id
WHERE prow_job_run_tests.prow_job_run_timestamp BETWEEN $1 AND $3
GROUP BY tests.id, prow_job_run_tests.prow_job_run_release
)
SELECT tests.id,
       tests.name,
       previous_successes,
       previous_flakes,
       previous_failures,
       previous_runs,
       current_successes,
       current_flakes,
       current_failures,
       current_runs,
       current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       current_failures * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
       previous_successes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
       (current_successes * 100.0 / NULLIF(current_runs, 0)) - (previous_successes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement,
       release
FROM results
INNER JOIN tests on tests.id = results.id
$_$;
`

const jobResultFunction = `
CREATE FUNCTION public.job_results(p_release text, p_start timestamp without time zone, p_boundary timestamp without time zone, p_endstamp timestamp without time zone) RETURNS TABLE(pj_name text, pj_variants text[], org text, repo text, average_retests_to_merge double precision, previous_passes bigint, previous_fails bigint, previous_runs bigint, previous_infra_fails bigint, current_passes bigint, current_fails bigint, current_runs bigint, current_infra_fails bigint, id bigint, created_at timestamp without time zone, updated_at timestamp without time zone, deleted_at timestamp without time zone, name text, release text, variants text[], test_grid_url text, kind text, brief_name text, current_pass_percentage real, current_projected_pass_percentage real, current_failure_percentage real, previous_pass_percentage real, previous_projected_pass_percentage real, previous_failure_percentage real, net_improvement real, open_bugs int, last_pass timestamp, current_average_duration_minutes int, previous_average_duration_minutes int)
    LANGUAGE plpgsql
    AS $fn$
BEGIN
RETURN QUERY
WITH retests AS (
    SELECT sub.id as prow_job_id, AVG(sub.cnt)::double precision as average_retests_to_merge
    FROM (
        SELECT prow_jobs.id, COUNT(*) as cnt
        FROM prow_job_runs
             INNER JOIN prow_job_run_prow_pull_requests on prow_job_run_prow_pull_requests.prow_job_run_id = prow_job_runs.id
             INNER JOIN prow_pull_requests on prow_pull_requests.id = prow_job_run_prow_pull_requests.prow_pull_request_id
             INNER JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
        WHERE prow_pull_requests.merged_at BETWEEN p_start AND p_endstamp
        AND prow_job_runs.timestamp BETWEEN p_start AND p_endstamp
        AND prow_job_runs.prow_job_release = p_release
        AND prow_job_run_prow_pull_requests.prow_job_run_release = p_release
        AND prow_job_run_prow_pull_requests.prow_job_run_timestamp BETWEEN p_start AND p_endstamp
        AND prow_job_runs.overall_result != 'S'
        AND prow_job_runs.overall_result != 'A'
        GROUP BY prow_jobs.id, prow_pull_requests.id, prow_pull_requests.link
    ) sub
    GROUP BY sub.id
),
results AS (
        select prow_jobs.name as pj_name, prow_jobs.variants as pj_variants,
                coalesce(count(case when succeeded = true AND prow_job_runs.timestamp >= p_start AND prow_job_runs.timestamp < p_boundary then 1 end), 0) as previous_passes,
                coalesce(count(case when succeeded = false AND prow_job_runs.timestamp >= p_start AND prow_job_runs.timestamp < p_boundary then 1 end), 0) as previous_fails,
                coalesce(count(case when prow_job_runs.timestamp >= p_start AND prow_job_runs.timestamp < p_boundary then 1 end), 0) as previous_runs,
                coalesce(count(case when infrastructure_failure = true AND prow_job_runs.timestamp >= p_start AND prow_job_runs.timestamp < p_boundary then 1 end), 0) as previous_infra_fails,
                coalesce(count(case when succeeded = true AND prow_job_runs.timestamp BETWEEN p_boundary AND p_endstamp then 1 end), 0) as current_passes,
                coalesce(count(case when succeeded = false AND prow_job_runs.timestamp BETWEEN p_boundary AND p_endstamp then 1 end), 0) as current_fails,
                coalesce(count(case when prow_job_runs.timestamp BETWEEN p_boundary AND p_endstamp then 1 end), 0) as current_runs,
                coalesce(count(case when infrastructure_failure = true AND prow_job_runs.timestamp BETWEEN p_boundary AND p_endstamp then 1 end), 0) as current_infra_fails,
                ROUND(coalesce(AVG(case when prow_job_runs.timestamp BETWEEN p_boundary AND p_endstamp then prow_job_runs.duration end) / 60000000000.0, 0))::int as current_average_duration_minutes,
                ROUND(coalesce(AVG(case when prow_job_runs.timestamp >= p_start AND prow_job_runs.timestamp < p_boundary then prow_job_runs.duration end) / 60000000000.0, 0))::int as previous_average_duration_minutes
        FROM prow_job_runs
        JOIN prow_jobs
                ON prow_jobs.id = prow_job_runs.prow_job_id
                AND prow_jobs.release = p_release
        WHERE prow_job_runs.prow_job_release = p_release
          AND prow_job_runs.timestamp BETWEEN p_start AND p_endstamp
        group by prow_jobs.name, prow_jobs.variants
),
lp AS (
    SELECT prow_job_runs.prow_job_id, max(prow_job_runs.timestamp)::timestamp without time zone as last_pass
    FROM prow_job_runs
    WHERE overall_result = 'S' AND prow_job_release = p_release
      AND prow_job_runs.timestamp >= p_endstamp - INTERVAL '90 days'
      AND prow_job_runs.timestamp <= p_endstamp
    GROUP BY prow_job_runs.prow_job_id
)
SELECT results.pj_name,
       results.pj_variants,
       roj.org,
       roj.repo,
       retests.average_retests_to_merge,
       results.previous_passes,
       results.previous_fails,
       results.previous_runs,
       results.previous_infra_fails,
       results.current_passes,
       results.current_fails,
       results.current_runs,
       results.current_infra_fails,
       prow_jobs.id,
       prow_jobs.created_at::timestamp without time zone,
       prow_jobs.updated_at::timestamp without time zone,
       prow_jobs.deleted_at::timestamp without time zone,
       prow_jobs.name,
       prow_jobs.release,
       prow_jobs.variants,
       prow_jobs.test_grid_url,
       prow_jobs.kind,
       REGEXP_REPLACE(results.pj_name, 'periodic-ci-openshift-(multiarch|release)-(master|main)-(ci|nightly)-[0-9]+.[0-9]+-', '') as brief_name,
       (results.current_passes * 100.0 / NULLIF(results.current_runs, 0))::real AS current_pass_percentage,
       ((results.current_passes + results.current_infra_fails) * 100.0 / NULLIF(results.current_runs, 0))::real AS current_projected_pass_percentage,
       (results.current_fails * 100.0 / NULLIF(results.current_runs, 0))::real AS current_failure_percentage,
       (results.previous_passes * 100.0 / NULLIF(results.previous_runs, 0))::real AS previous_pass_percentage,
       ((results.previous_passes + results.previous_infra_fails) * 100.0 / NULLIF(results.previous_runs, 0))::real AS previous_projected_pass_percentage,
       (results.previous_fails * 100.0 / NULLIF(results.previous_runs, 0))::real AS previous_failure_percentage,
       ((results.current_passes * 100.0 / NULLIF(results.current_runs, 0)) - (results.previous_passes * 100.0 / NULLIF(results.previous_runs, 0)))::real AS net_improvement,
       COALESCE(jb.open_bugs, 0)::int AS open_bugs,
       lp.last_pass,
       results.current_average_duration_minutes,
       results.previous_average_duration_minutes
FROM results
         JOIN prow_jobs ON prow_jobs.name = results.pj_name
                      AND prow_jobs.release = p_release
         LEFT JOIN (
             SELECT prow_pull_requests.org, prow_pull_requests.repo, prow_jobs.id
             FROM prow_job_runs
                  INNER JOIN prow_job_run_prow_pull_requests on prow_job_run_prow_pull_requests.prow_job_run_id = prow_job_runs.id
                  INNER JOIN prow_pull_requests on prow_pull_requests.id = prow_job_run_prow_pull_requests.prow_pull_request_id
                  INNER JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
             WHERE prow_job_runs.prow_job_release = p_release
               AND prow_job_runs.timestamp BETWEEN p_start AND p_endstamp
               AND prow_job_run_prow_pull_requests.prow_job_run_release = p_release
               AND prow_job_run_prow_pull_requests.prow_job_run_timestamp BETWEEN p_start AND p_endstamp
             GROUP BY prow_pull_requests.org, prow_pull_requests.repo, prow_jobs.id
         ) roj ON prow_jobs.id = roj.id
         LEFT JOIN retests ON prow_jobs.id = retests.prow_job_id
         LEFT JOIN lp ON prow_jobs.id = lp.prow_job_id
         LEFT JOIN (
             SELECT prow_jobs.id AS prow_job_id, COUNT(DISTINCT bugs.id)::int AS open_bugs
             FROM prow_jobs
             LEFT JOIN bug_jobs ON prow_jobs.id = bug_jobs.prow_job_id
             LEFT JOIN bugs ON bugs.id = bug_jobs.bug_id AND lower(bugs.status) NOT IN ('verified', 'modified', 'closed', 'on_qa')
             WHERE prow_jobs.release = p_release
             GROUP BY prow_jobs.id
         ) jb ON prow_jobs.id = jb.prow_job_id;
END;
    $fn$;
`
