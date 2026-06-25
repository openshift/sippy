CREATE TABLE IF NOT EXISTS test_analysis_by_job_by_dates (
    date TIMESTAMP WITH TIME ZONE,
    test_id BIGINT,
    release TEXT NOT NULL,
    job_name TEXT,
    test_name TEXT,
    runs BIGINT,
    passes BIGINT,
    flakes BIGINT,
    failures BIGINT
) PARTITION BY LIST (release);

CREATE UNIQUE INDEX IF NOT EXISTS idx_test_analysis_test_release_date
    ON test_analysis_by_job_by_dates (date, test_id, release, job_name);

CREATE INDEX IF NOT EXISTS idx_test_analysis_name_and_job
    ON test_analysis_by_job_by_dates (test_name, job_name);
