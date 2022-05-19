-- +goose Up
-- +goose StatementBegin

-- This change adds in the test id column we should have added originally. The down migration reverts to previous schema.

DROP MATERIALIZED VIEW prow_test_report_2d_matview;
DROP MATERIALIZED VIEW prow_test_report_7d_matview;

CREATE MATERIALIZED VIEW prow_test_report_2d_matview AS
SELECT tests.id,
       tests.name,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '9 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '2 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_successes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '9 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '2 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_flakes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '9 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '2 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_failures,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_runs."timestamp" >= (now() - '9 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '2 days'::interval))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_runs,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '2 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_successes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '2 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_flakes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '2 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_failures,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_runs."timestamp" >= (now() - '2 days'::interval)) AND (prow_job_runs."timestamp" <= now())) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_runs,
       prow_jobs.variants,
       prow_jobs.release
FROM (((public.prow_job_run_tests
    JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
    JOIN public.prow_job_runs ON ((prow_job_runs.id = prow_job_run_tests.prow_job_run_id)))
         JOIN public.prow_jobs ON ((prow_job_runs.prow_job_id = prow_jobs.id)))
GROUP BY tests.id, tests.name, prow_jobs.variants, prow_jobs.release
WITH NO DATA;

CREATE MATERIALIZED VIEW public.prow_test_report_7d_matview AS
SELECT tests.id,
       tests.name,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '7 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_successes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '7 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_flakes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '7 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_failures,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '7 days'::interval))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_runs,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '7 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_successes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '7 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_flakes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '7 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_failures,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_runs."timestamp" >= (now() - '7 days'::interval)) AND (prow_job_runs."timestamp" <= now())) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_runs,
       prow_jobs.variants,
       prow_jobs.release
FROM (((public.prow_job_run_tests
    JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
    JOIN public.prow_job_runs ON ((prow_job_runs.id = prow_job_run_tests.prow_job_run_id)))
         JOIN public.prow_jobs ON ((prow_job_runs.prow_job_id = prow_jobs.id)))
GROUP BY tests.id, tests.name, prow_jobs.variants, prow_jobs.release
WITH NO DATA;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP MATERIALIZED VIEW prow_test_report_2d_matview;
DROP MATERIALIZED VIEW prow_test_report_7d_matview;

CREATE MATERIALIZED VIEW prow_test_report_2d_matview AS
SELECT tests.name,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '9 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '2 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_successes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '9 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '2 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_flakes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '9 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '2 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_failures,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_runs."timestamp" >= (now() - '9 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '2 days'::interval))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_runs,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '2 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_successes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '2 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_flakes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '2 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_failures,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_runs."timestamp" >= (now() - '2 days'::interval)) AND (prow_job_runs."timestamp" <= now())) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_runs,
       prow_jobs.variants,
       prow_jobs.release
FROM (((public.prow_job_run_tests
    JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
    JOIN public.prow_job_runs ON ((prow_job_runs.id = prow_job_run_tests.prow_job_run_id)))
         JOIN public.prow_jobs ON ((prow_job_runs.prow_job_id = prow_jobs.id)))
GROUP BY tests.name, prow_jobs.variants, prow_jobs.release
WITH NO DATA;

CREATE MATERIALIZED VIEW public.prow_test_report_7d_matview AS
SELECT tests.name,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '7 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_successes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '7 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_flakes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '7 days'::interval)))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_failures,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= (now() - '7 days'::interval))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS previous_runs,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '7 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_successes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '7 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_flakes,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '7 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_failures,
       COALESCE(count(
                        CASE
                            WHEN ((prow_job_runs."timestamp" >= (now() - '7 days'::interval)) AND (prow_job_runs."timestamp" <= now())) THEN 1
                            ELSE NULL::integer
                            END), (0)::bigint) AS current_runs,
       prow_jobs.variants,
       prow_jobs.release
FROM (((public.prow_job_run_tests
    JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
    JOIN public.prow_job_runs ON ((prow_job_runs.id = prow_job_run_tests.prow_job_run_id)))
         JOIN public.prow_jobs ON ((prow_job_runs.prow_job_id = prow_jobs.id)))
GROUP BY tests.name, prow_jobs.variants, prow_jobs.release
WITH NO DATA;

-- +goose StatementEnd
