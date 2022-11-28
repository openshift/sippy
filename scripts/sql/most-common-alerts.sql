SELECT
    j.release,
    t.id AS "test_id",
    md.metadata->>'alert' AS "alert",
    md.metadata->>'namespace' AS "namespace",
    md.metadata->>'result' AS "result",
    count(md.metadata->>'alert') AS "alert_count"
FROM
    prow_job_run_test_output_metadata md,
	prow_job_run_test_outputs o,
	prow_job_run_tests rt,
	prow_job_runs r,
	prow_jobs j,
	tests t
WHERE
    md.created_at > NOW() - INTERVAL '14 days' AND
    md.metadata->>'result' != 'allow' AND
	md.prow_job_run_test_output_id = o.id AND
	o.prow_job_run_test_id = rt.id AND
	(rt.status = 12 OR rt.status = 13) AND
	rt.prow_job_run_id = r.id AND
	r.prow_job_id = j.id AND
	(j.release = '4.13' OR j.release = 'Presubmits') AND
	rt.test_id = t.id AND (
		t.name LIKE '%unexpected alerts in firing or pending%' OR
		t.name LIKE '%during or after upgrade%'
	)
GROUP BY
    t.id, md.metadata->>'alert', md.metadata->>'namespace', md.metadata->>'result', j.release
ORDER BY
    alert_count DESC
LIMIT 30;

