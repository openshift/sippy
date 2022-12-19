WITH current_events AS (
    SELECT
        j.release,
        t.id AS "test_id",
        md.metadata ->> 'reason' AS "reason",
        md.metadata ->> 'ns' AS "ns",
        count(md.metadata)
    FROM
        prow_job_run_test_output_metadata md,
        prow_job_run_test_outputs o,
        prow_job_run_tests rt,
        prow_job_runs r,
        prow_jobs j,
        tests t
    WHERE
        md.created_at > NOW() - INTERVAL '7 days'
        AND md.prow_job_run_test_output_id = o.id
        AND o.prow_job_run_test_id = rt.id
        AND rt.status = 12
        AND rt.prow_job_run_id = r.id
        AND r.prow_job_id = j.id
        AND rt.test_id = t.id
        AND (j.release = '4.13'
            OR j.release = 'Presubmits')
        AND t.name LIKE '%pathological%'
    GROUP BY
        t.id,
        md.metadata ->> 'ns',
        md.metadata ->> 'reason',
        j.release
    ORDER BY
        count DESC
),
previous_events AS (
    SELECT
        j.release,
        t.id AS "test_id",
        md.metadata ->> 'reason' AS "reason",
        md.metadata ->> 'ns' AS "ns",
        count(md.metadata)
    FROM
        prow_job_run_test_output_metadata md,
        prow_job_run_test_outputs o,
        prow_job_run_tests rt,
        prow_job_runs r,
        prow_jobs j,
        tests t
    WHERE
        md.created_at < NOW() - INTERVAL '7 days'
        AND md.created_at >= NOW() - INTERVAL '14 days'
        AND md.prow_job_run_test_output_id = o.id
        AND o.prow_job_run_test_id = rt.id
        AND rt.status = 12
        AND rt.prow_job_run_id = r.id
        AND r.prow_job_id = j.id
        AND rt.test_id = t.id
        AND (j.release = '4.13'
            OR j.release = 'Presubmits')
        AND t.name LIKE '%pathological%'
    GROUP BY
        t.id,
        md.metadata ->> 'ns',
        md.metadata ->> 'reason',
        j.release
    ORDER BY
        count DESC
)
SELECT
    previous_events.release,
    previous_events.reason,
    previous_events.ns,
    previous_events.count AS previous_count,
    current_events.count AS current_count,
    round((100.0 * (current_events.count::float - previous_events.count::float) / previous_events.count::float)::numeric,2) AS percent_change
FROM
    current_events
    INNER JOIN previous_events ON previous_events.reason = current_events.reason
        AND previous_events.ns = current_events.ns
    ORDER BY
        release,
        percent_change DESC,
        current_count DESC;
