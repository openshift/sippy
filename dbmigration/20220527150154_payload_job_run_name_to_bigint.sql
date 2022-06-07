-- +goose Up
-- +goose StatementBegin
UPDATE release_job_runs SET name='0' WHERE name='.';
ALTER TABLE release_job_runs ALTER COLUMN name TYPE bigint USING (name::bigint);
ALTER TABLE release_job_runs RENAME COLUMN name to prow_job_run_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE release_job_runs RENAME COLUMN prow_job_run_id to name;
ALTER TABLE release_job_runs ALTER COLUMN name TYPE text USING (name::text);
UPDATE release_job_runs SET name='.' WHERE name='0';
-- +goose StatementEnd
