-- Recreate the table that was dropped. No data is restored.
CREATE TABLE IF NOT EXISTS test_daily_summaries (
    test_id       BIGINT  NOT NULL,
    prow_job_id   BIGINT  NOT NULL,
    suite_id      BIGINT  NOT NULL DEFAULT 0,
    release       TEXT    NOT NULL,
    summary_date  DATE    NOT NULL,
    successes     INTEGER NOT NULL DEFAULT 0,
    failures      INTEGER NOT NULL DEFAULT 0,
    flakes        INTEGER NOT NULL DEFAULT 0,
    runs          INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_test_daily_summaries_unique
    ON test_daily_summaries (test_id, prow_job_id, suite_id, release, summary_date);

CREATE INDEX IF NOT EXISTS idx_test_daily_summaries_date
    ON test_daily_summaries (summary_date);
