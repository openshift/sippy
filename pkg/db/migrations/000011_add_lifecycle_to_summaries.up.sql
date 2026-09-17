-- TRT-2752: Add lifecycle as a key column in summary tables.
-- Separates blocking/informing test results in daily and cumulative
-- aggregations so downstream queries (CR, test reports) can filter by
-- lifecycle. Existing rows default to 'blocking'.
--
-- Index rebuilds: both tables need unique indexes that include lifecycle
-- for ON CONFLICT upserts. On large databases, build the new indexes
-- out-of-band first with CREATE INDEX CONCURRENTLY (outside a
-- transaction), then this migration's IF NOT EXISTS makes them no-ops.

ALTER TABLE test_daily_totals
    ADD COLUMN IF NOT EXISTS lifecycle TEXT NOT NULL DEFAULT 'blocking';

ALTER TABLE test_cumulative_summaries
    ADD COLUMN IF NOT EXISTS lifecycle TEXT NOT NULL DEFAULT 'blocking';

CREATE UNIQUE INDEX IF NOT EXISTS idx_test_daily_totals_key
    ON test_daily_totals (release, date, test_id, suite_id, lifecycle, prow_job_id)
    INCLUDE (successes, failures, flakes, runs,
             first_failure_timestamp, last_failure_timestamp,
             first_success_timestamp, last_success_timestamp);

CREATE UNIQUE INDEX IF NOT EXISTS idx_test_cumulative_summaries_key
    ON test_cumulative_summaries (release, date, test_id, suite_id, lifecycle, prow_job_id)
    INCLUDE (prefix_sum_successes, prefix_sum_failures, prefix_sum_flakes, prefix_sum_runs,
             prefix_max_last_failure, prefix_max_last_success);

DROP INDEX IF EXISTS idx_test_daily_totals_unique;

ALTER TABLE test_cumulative_summaries
    DROP CONSTRAINT IF EXISTS test_cumulative_summaries_pkey;
