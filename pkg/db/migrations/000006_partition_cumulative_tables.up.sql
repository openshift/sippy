-- TRT-2741: Create partitioned summary tables
--
-- Creates 2 partitioned tables using nested LIST->RANGE partitioning:
-- - Level 1: LIST partition by release
-- - Level 2: RANGE sub-partition by date (daily granularity)
--
-- Tables:
--   - test_daily_totals (partitioned daily aggregates)
--   - test_cumulative_summaries (prefix sums from test_daily_totals)
--
-- Partition creation (release partitions + daily sub-partitions) is handled
-- by the partition management system (gopar).

-- ============================================================================
-- 1. test_daily_totals
-- ============================================================================

CREATE TABLE IF NOT EXISTS test_daily_totals (
    test_id BIGINT NOT NULL,
    prow_job_id BIGINT NOT NULL,
    suite_id BIGINT NOT NULL DEFAULT 0,
    release TEXT NOT NULL,
    date DATE NOT NULL,
    successes INT NOT NULL DEFAULT 0,
    failures INT NOT NULL DEFAULT 0,
    flakes INT NOT NULL DEFAULT 0,
    runs INT NOT NULL DEFAULT 0
) PARTITION BY LIST (release);

CREATE UNIQUE INDEX IF NOT EXISTS idx_test_daily_totals_unique
    ON test_daily_totals (test_id, prow_job_id, suite_id, release, date);

CREATE INDEX IF NOT EXISTS idx_test_daily_totals_date
    ON test_daily_totals (date);

-- ============================================================================
-- 2. test_cumulative_summaries
-- ============================================================================

CREATE TABLE IF NOT EXISTS test_cumulative_summaries (
    date DATE NOT NULL,
    release TEXT NOT NULL,
    test_id BIGINT NOT NULL,
    prow_job_id BIGINT NOT NULL,
    suite_id BIGINT NOT NULL DEFAULT 0,
    prefix_sum_successes BIGINT NOT NULL DEFAULT 0,
    prefix_sum_failures BIGINT NOT NULL DEFAULT 0,
    prefix_sum_flakes BIGINT NOT NULL DEFAULT 0,
    prefix_sum_runs BIGINT NOT NULL DEFAULT 0,

    PRIMARY KEY (date, release, test_id, prow_job_id, suite_id)
) PARTITION BY LIST (release);

CREATE INDEX IF NOT EXISTS idx_test_cumulative_summaries_prow_job_id
    ON test_cumulative_summaries (prow_job_id);
