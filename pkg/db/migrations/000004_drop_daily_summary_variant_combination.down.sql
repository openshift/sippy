TRUNCATE test_daily_summaries;
ALTER TABLE test_daily_summaries ADD COLUMN IF NOT EXISTS variant_combination_id BIGINT;
