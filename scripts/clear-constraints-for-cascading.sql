/* This drops the fk constraints on tables where
 * we need to re-add them with OnDelete CASCADES. This is a one time
 * operation that should be done manually on a database created prior to
 * October 17, 2022. */
ALTER TABLE "prow_job_runs" DROP CONSTRAINT "fk_prow_job_runs_prow_job";
ALTER TABLE "prow_job_run_tests" DROP CONSTRAINT "fk_prow_job_runs_tests";
ALTER TABLE "prow_job_run_tests" DROP CONSTRAINT "fk_prow_job_runs_tests";
ALTER TABLE "prow_job_run_test_outputs" DROP CONSTRAINT "fk_prow_job_run_tests_prow_job_run_test_output";
ALTER TABLE "prow_job_run_prow_pull_requests" DROP CONSTRAINT "fk_prow_job_run_prow_pull_requests_prow_job_run";
ALTER TABLE "prow_job_run_prow_pull_requests" DROP CONSTRAINT "fk_prow_job_run_prow_pull_requests_prow_pull_request";

ALTER TABLE "release_tag_pull_requests" DROP CONSTRAINT "fk_release_tag_pull_requests_release_tag";
ALTER TABLE "release_job_runs" DROP CONSTRAINT "fk_release_tags_job_runs";
ALTER TABLE "release_repositories" DROP CONSTRAINT "fk_release_tags_repositories";

ALTER TABLE "bug_jobs" DROP CONSTRAINT "fk_bug_jobs_bug";
ALTER TABLE "bug_tests" DROP CONSTRAINT "fk_bug_tests_bug";

