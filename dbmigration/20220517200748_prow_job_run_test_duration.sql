-- +goose Up
-- +goose StatementBegin
ALTER TABLE prow_job_run_tests ADD COLUMN duration double precision;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE prow_job_run_tests DROP COLUMN duration;
-- +goose StatementEnd
