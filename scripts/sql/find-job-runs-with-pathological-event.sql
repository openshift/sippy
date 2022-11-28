SELECT
    r.timestamp,
    md.metadata->>'reason' as "reason",
    md.metadata->>'ns' as "ns",
    r.test_failures,
    r.url
FROM
    prow_job_run_test_output_metadata md,
    prow_job_run_test_outputs o,
    prow_job_run_tests rt,
    prow_job_runs r,
    prow_jobs j,
    tests t
WHERE
    md.created_at > NOW() - INTERVAL '14 days' AND
    md.prow_job_run_test_output_id = o.id AND
    o.prow_job_run_test_id = rt.id AND
    rt.status = 12 AND
    rt.prow_job_run_id = r.id AND
    r.prow_job_id = j.id AND
    rt.test_id = t.id AND
    (j.release = '4.13' OR j.release = 'Presubmits') AND
    t.name LIKE '%pathological%' AND
    /* replace with the alert you're interested in */
    md.metadata->>'reason' = 'Unable' AND
    md.metadata->>'ns' = 'default'
ORDER BY r.timestamp DESC
LIMIT 30;
