DROP VIEW IF EXISTS prow_test_analysis_by_variant_14d_view;
DROP TABLE IF EXISTS test_analysis_by_job_by_dates;

-- Drop any detached partitions that are no longer children of the parent
-- table. The partition lifecycle detaches old partitions before dropping
-- them; those detached tables survive the parent DROP TABLE above.
DO $$
DECLARE
    tbl TEXT;
BEGIN
    FOR tbl IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
          AND tablename LIKE 'test_analysis_by_job_by_dates_%'
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I', tbl);
    END LOOP;
END;
$$;
