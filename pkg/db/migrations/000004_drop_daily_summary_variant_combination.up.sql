DROP MATERIALIZED VIEW IF EXISTS prow_test_report_7d_collapsed_matview;
DROP MATERIALIZED VIEW IF EXISTS prow_test_report_2d_collapsed_matview;
DROP MATERIALIZED VIEW IF EXISTS prow_test_report_7d_matview;
DROP MATERIALIZED VIEW IF EXISTS prow_test_report_2d_matview;
ALTER TABLE test_daily_summaries DROP COLUMN IF EXISTS variant_combination_id;
