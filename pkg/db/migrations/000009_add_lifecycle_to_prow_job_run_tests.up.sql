-- TRT-2752: Add lifecycle column to prow_job_run_tests
--
-- Records the per-test lifecycle value ("blocking" or "informing") parsed from
-- JUnit XML at load time. Propagates to all partitions automatically.
-- Existing rows default to 'blocking', matching BQ COALESCE behavior.

ALTER TABLE prow_job_run_tests
    ADD COLUMN IF NOT EXISTS lifecycle TEXT NOT NULL DEFAULT 'blocking';
