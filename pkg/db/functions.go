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
		if err := syncSchema(db, hashTypeFunction, pgFunc.Name, pgFunc.Definition, dropSQL); err != nil {
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
    coalesce(count(case when status = 1 AND timestamp BETWEEN $1 AND $2 then 1 end), 0) AS previous_successes,
    coalesce(count(case when status = 13 AND timestamp BETWEEN $1 AND $2 then 1 end), 0) AS previous_flakes,
    coalesce(count(case when status = 12 AND timestamp BETWEEN $1 AND $2 then 1 end), 0) AS previous_failures,
    coalesce(count(case when timestamp BETWEEN $1 AND $2 then 1 end), 0) as previous_runs,
    coalesce(count(case when status = 1 AND timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_successes,
    coalesce(count(case when status = 13 AND timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_flakes,
    coalesce(count(case when status = 12 AND timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_failures,
    coalesce(count(case when timestamp BETWEEN $2 AND $3 then 1 end), 0) as current_runs,
    prow_jobs.release
FROM prow_job_run_tests
    JOIN tests ON tests.id = prow_job_run_tests.test_id
    JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id
    JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
GROUP BY tests.id, prow_jobs.release
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
CREATE FUNCTION public.job_results(release text, start timestamp without time zone, boundary timestamp without time zone, endstamp timestamp without time zone) RETURNS TABLE(pj_name text, pj_variants text[], previous_passes bigint, previous_failures bigint, previous_runs bigint, previous_infra_fails bigint, current_passes bigint, current_fails bigint, current_runs bigint, current_infra_fails bigint, id bigint, created_at timestamp without time zone, updated_at timestamp without time zone, deleted_at timestamp without time zone, name text, release text, variants text[], test_grid_url text, kind text, brief_name text, current_pass_percentage real, current_projected_pass_percentage real, current_failure_percentage real, previous_pass_percentage real, previous_projected_pass_percentage real, previous_failure_percentage real, net_improvement real)
    LANGUAGE sql
    AS $_$
WITH results AS (
        select prow_jobs.name as pj_name, prow_jobs.variants as pj_variants,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN $2 AND $3 then 1 end), 0) as previous_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN $2 AND $3 then 1 end), 0) as previous_failures,
                coalesce(count(case when timestamp BETWEEN $2 AND $3 then 1 end), 0) as previous_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp BETWEEN $2 AND $3 then 1 end), 0) as previous_infra_fails,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN $3 AND $4 then 1 end), 0) as current_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN $3 AND $4 then 1 end), 0) as current_fails,
                coalesce(count(case when timestamp BETWEEN $3 AND $4 then 1 end), 0) as current_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp BETWEEN $3 AND $4 then 1 end), 0) as current_infra_fails
        FROM prow_job_runs
        JOIN prow_jobs
                ON prow_jobs.id = prow_job_runs.prow_job_id
                                AND prow_jobs.release = $1
                AND timestamp BETWEEN $2 AND $4
        group by prow_jobs.name, prow_jobs.variants
)
SELECT *,
       REGEXP_REPLACE(results.pj_name, 'periodic-ci-openshift-(multiarch|release)-master-(ci|nightly)-[0-9]+.[0-9]+-', '') as brief_name,
       current_passes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       (current_passes + current_infra_fails) * 100.0 / NULLIF(current_runs, 0) AS current_projected_pass_percentage,
       current_fails * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
       previous_passes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       (previous_passes + previous_infra_fails) * 100.0 / NULLIF(previous_runs, 0) AS previous_projected_pass_percentage,
       previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
       (current_passes * 100.0 / NULLIF(current_runs, 0)) - (previous_passes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
FROM results
         JOIN prow_jobs ON prow_jobs.name = results.pj_name
    $_$;
`
