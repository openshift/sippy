CREATE TABLE IF NOT EXISTS test_analysis_by_job_by_dates (
    date timestamp with time zone,
    test_id bigint,
    release text,
    job_name text,
    test_name text,
    runs bigint,
    passes bigint,
    flakes bigint,
    failures bigint
) PARTITION BY RANGE (date);

CREATE UNIQUE INDEX IF NOT EXISTS test_release_date
    ON test_analysis_by_job_by_dates (date, test_id, release, job_name);

CREATE INDEX IF NOT EXISTS test_analysis_name_and_job
    ON test_analysis_by_job_by_dates (test_name, job_name);
