-- TRT-2676: Create test_daily_summaries table
--
-- Pre-aggregates prow_job_run_tests data into daily buckets per
-- (test_id, prow_job_id, suite_id, release, date). The test report
-- matviews read from this table instead of scanning 100M+ raw rows,
-- reducing refresh time from ~30 minutes to ~5.5 minutes.
--
-- Flat (non-partitioned) — benchmarks showed this is faster than
-- release-partitioned for the all-release matview query.

CREATE TABLE IF NOT EXISTS test_daily_summaries (
    test_id BIGINT NOT NULL,
    prow_job_id BIGINT NOT NULL,
    suite_id BIGINT NOT NULL DEFAULT 0,
    release TEXT NOT NULL,
    summary_date DATE NOT NULL,
    successes INT NOT NULL DEFAULT 0,
    failures INT NOT NULL DEFAULT 0,
    flakes INT NOT NULL DEFAULT 0,
    runs INT NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_test_daily_summaries_unique
    ON test_daily_summaries (test_id, prow_job_id, suite_id, release, summary_date);

CREATE INDEX IF NOT EXISTS idx_test_daily_summaries_date
    ON test_daily_summaries (summary_date);
