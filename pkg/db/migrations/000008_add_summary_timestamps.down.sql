ALTER TABLE test_cumulative_summaries
  DROP COLUMN IF EXISTS prefix_max_last_failure,
  DROP COLUMN IF EXISTS prefix_max_last_success;

ALTER TABLE test_daily_totals
  DROP COLUMN IF EXISTS first_failure_timestamp,
  DROP COLUMN IF EXISTS last_failure_timestamp,
  DROP COLUMN IF EXISTS first_success_timestamp,
  DROP COLUMN IF EXISTS last_success_timestamp;
