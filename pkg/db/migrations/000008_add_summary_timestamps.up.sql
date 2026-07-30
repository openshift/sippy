-- TRT-2821: Add timestamp columns to summary tables
--
-- Adds MIN/MAX timestamp columns to test_daily_totals for tracking when
-- failures and successes occurred within each day. Adds running-maximum
-- columns to test_cumulative_summaries for efficient lookback queries.
--
-- ALTER TABLE on a partitioned parent propagates to all existing partitions.

ALTER TABLE test_daily_totals
  ADD COLUMN IF NOT EXISTS first_failure_timestamp TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_failure_timestamp TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS first_success_timestamp TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_success_timestamp TIMESTAMPTZ;

ALTER TABLE test_cumulative_summaries
  ADD COLUMN IF NOT EXISTS prefix_max_last_failure TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS prefix_max_last_success TIMESTAMPTZ;
