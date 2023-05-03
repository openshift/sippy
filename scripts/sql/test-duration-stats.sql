--- Creates a temporary function for calculating the percentile of test durations by an array of 3-tuple variants.
CREATE OR REPLACE FUNCTION pg_temp.calculate_percentiles(testName TEXT) RETURNS TABLE (
    platform TEXT,
    cni TEXT,
    upgrade TEXT,
    percentile_50 NUMERIC,
    percentile_99 NUMERIC,
    percentile_100 NUMERIC
) AS $$
DECLARE
    variant_sets TEXT[][] := '{{"aws","ovn","upgrade-minor"},{"aws","sdn","upgrade-minor"},{"aws","ovn","upgrade-micro"},{"aws","sdn","upgrade-micro"},{"azure","ovn","upgrade-minor"},{"azure","sdn","upgrade-minor"},{"azure","ovn","upgrade-micro"},{"azure","sdn","upgrade-micro"},{"gcp","ovn","upgrade-minor"},{"gcp","sdn","upgrade-minor"},{"gcp","ovn","upgrade-micro"},{"gcp","sdn","upgrade-micro"},{"metal-ipi","ovn","upgrade-minor"},{"metal-ipi","sdn","upgrade-minor"},{"metal-ipi","ovn","upgrade-micro"},{"metal-ipi","sdn","upgrade-micro"},{"vsphere-ipi","ovn","upgrade-minor"},{"vsphere-ipi","sdn","upgrade-minor"},{"vsphere-ipi","ovn","upgrade-micro"},{"vsphere-ipi","sdn","upgrade-micro"}}';
BEGIN
    FOR i IN 1..array_length(variant_sets, 1) LOOP
        SELECT
            variant_sets[i][1] as platform,
            variant_sets[i][2] as cni,
            variant_sets[i][3] as upgrade,
            (percentile_disc(0.5) within group (order by pjt.duration asc)) / 60 as percentile_50,
            (percentile_disc(0.99) within group (order by pjt.duration asc)) / 60  as percentile_99,
            (percentile_disc(1.0) within group (order by pjt.duration asc)) / 60 as percentile_100
        INTO
            platform,
            cni,
            upgrade,
            percentile_50,
            percentile_99,
            percentile_100
        FROM
            prow_job_run_tests pjt
            INNER JOIN prow_job_runs pjr ON pjr.id = pjt.prow_job_run_id
            INNER JOIN prow_jobs pj ON pj.id = pjr.prow_job_id
        WHERE
            pjt.status = 1
            AND pjr.timestamp > current_date - interval '30' day
            AND pjt.test_id = (SELECT id FROM tests WHERE name = testName)
            AND pj.release = '4.14'
            AND variant_sets[i][1] = ANY(pj.variants)
            AND variant_sets[i][2] = ANY(pj.variants)
            AND variant_sets[i][3] = ANY(pj.variants)
            AND 'amd64' = ANY(pj.variants)
            AND 'single-node' != ALL(pj.variants)
        GROUP BY pj.variants;        RETURN NEXT;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Example:
SELECT * from pg_temp.calculate_percentiles('Cluster upgrade.[sig-cluster-lifecycle] Cluster completes upgrade');
