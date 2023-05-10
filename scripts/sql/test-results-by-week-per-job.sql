CREATE OR REPLACE FUNCTION pg_temp.weekly_test_results_by_job(test_substring TEXT, job_name TEXT) RETURNS TABLE (
    name TEXT,
    week_start TIMESTAMP WITH TIME ZONE,
    current_successes BIGINT,
    current_flakes BIGINT,
    current_failures BIGINT,
    current_runs BIGINT,
    current_working_percentage NUMERIC(5,2)
) AS $$
BEGIN
    RETURN QUERY
    WITH results AS (
        SELECT
            tests.name,
            DATE_TRUNC('week', pjr.timestamp) AS week_start,
            COALESCE(count(
                    CASE WHEN pjrt.status = 1 THEN
                        1
                    ELSE
                        NULL::integer
                    END), 0::bigint) AS current_successes,
            COALESCE(count(
                    CASE WHEN pjrt.status = 13 THEN
                        1
                    ELSE
                        NULL::integer
                    END), 0::bigint) AS current_flakes,
            COALESCE(count(
                    CASE WHEN pjrt.status = 12 THEN
                        1
                    ELSE
                        NULL::integer
                    END), 0::bigint) AS current_failures,
            count(*) AS current_runs
        FROM
            prow_job_run_tests pjrt
            INNER JOIN prow_job_runs pjr ON pjr.id = pjrt.prow_job_run_id
            INNER JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
            INNER JOIN tests ON pjrt.test_id = tests.id
        WHERE
            tests.name LIKE test_substring
            AND pj.name = job_name
        GROUP BY
            tests.name,
            week_start
        ORDER BY
            week_start, tests.name ASC
    )
    SELECT
        *,
        ROUND((results.current_successes + results.current_flakes) * 100.0 / NULLIF(results.current_runs, 0), 2) AS current_working_percentage
    FROM
        results;
END;
$$ LANGUAGE plpgsql;

--- Example, TODO: replace parameters
SELECT * FROM pg_temp.weekly_test_results_by_job('%CPU Partitioning%', 'periodic-ci-openshift-release-master-nightly-4.14-e2e-aws-ovn-cpu-partitioning');
