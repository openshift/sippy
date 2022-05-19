-- +goose Up
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_prow_job_runs_report_matview ON prow_job_runs_report_matview(id);
CREATE UNIQUE INDEX idx_prow_job_failed_tests_by_day_matview ON prow_job_failed_tests_by_day_matview(period, prow_job_id, test_name);
CREATE UNIQUE INDEX idx_prow_test_analysis_by_job_14d_matview ON prow_test_analysis_by_job_14d_matview(test_id, date, job_name);
CREATE UNIQUE INDEX idx_prow_test_analysis_by_variant_14d_matview ON prow_test_analysis_by_variant_14d_matview(test_id, date, variant, release);
CREATE UNIQUE INDEX idx_prow_test_report_2d_matview ON prow_test_report_2d_matview(id, release, variants);
CREATE UNIQUE INDEX idx_prow_test_report_7d_matview ON prow_test_report_7d_matview(id, release, variants);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_prow_job_runs_report_matview;
DROP INDEX idx_prow_job_failed_tests_by_day_matview;
DROP INDEX idx_prow_test_analysis_by_job_14d_matview;
DROP INDEX idx_prow_test_analysis_by_variant_14d_matview;
DROP INDEX idx_prow_test_report_2d_matview;
DROP INDEX idx_prow_test_report_7d_matview;
-- +goose StatementEnd
