-- TRT-1989: Rollback Partitioned Tables
--
-- Drops all partitioned tables created in the up migration.
-- CASCADE ensures all partitions (if any were created) are also dropped.

DROP TABLE IF EXISTS prow_job_run_prow_pull_requests CASCADE;
DROP TABLE IF EXISTS prow_job_run_annotations CASCADE;
DROP TABLE IF EXISTS prow_job_run_tests CASCADE;
DROP TABLE IF EXISTS prow_job_run_test_outputs CASCADE;
DROP TABLE IF EXISTS prow_job_runs CASCADE;
DROP TABLE IF EXISTS test_analysis_by_job_by_dates CASCADE;
