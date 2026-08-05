-- idx_test_daily_totals_date exists only to make an unscoped
-- SELECT MAX(date) FROM test_daily_totals fast across ~3,700 partitions.
-- Nothing queries MAX(date) without a release filter anymore
-- (dailysummary.MaxSummaryDate and cumulativesummary.MaxDailySummaryDate
-- were the last two callers), so this index is now pure write overhead:
-- every insert into test_daily_totals maintains it for no reader.
DROP INDEX IF EXISTS idx_test_daily_totals_date;
