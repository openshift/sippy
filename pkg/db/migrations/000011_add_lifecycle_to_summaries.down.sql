DROP INDEX IF EXISTS idx_test_cumulative_summaries_key;

DROP INDEX IF EXISTS idx_test_daily_totals_key;

ALTER TABLE test_cumulative_summaries
    DROP COLUMN IF EXISTS lifecycle;

ALTER TABLE test_daily_totals
    DROP COLUMN IF EXISTS lifecycle;

ALTER TABLE test_cumulative_summaries
    ADD PRIMARY KEY (date, release, test_id, prow_job_id, suite_id);

CREATE UNIQUE INDEX idx_test_daily_totals_unique
    ON test_daily_totals (test_id, prow_job_id, suite_id, release, date);
