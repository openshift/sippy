-- +goose Up
-- +goose StatementBegin
CREATE MATERIALIZED VIEW public.prow_job_failed_tests_by_hour_matview AS
 SELECT date_trunc('hour'::text, prow_job_runs."timestamp") AS period,
    prow_job_runs.prow_job_id,
    tests.name AS test_name,
    count(tests.name) AS count
   FROM ((public.prow_job_runs
     JOIN public.prow_job_run_tests pjrt ON ((prow_job_runs.id = pjrt.prow_job_run_id)))
     JOIN public.tests tests ON ((pjrt.test_id = tests.id)))
  WHERE (pjrt.status = 12)
  GROUP BY tests.name, (date_trunc('hour'::text, prow_job_runs."timestamp")), prow_job_runs.prow_job_id
  WITH NO DATA;

CREATE UNIQUE INDEX idx_prow_job_failed_tests_by_hour_matview ON prow_job_failed_tests_by_hour_matview(period, prow_job_id, test_name);
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP MATERIALIZED VIEW prow_job_failed_tests_by_hour_matview;
-- +goose StatementEnd
