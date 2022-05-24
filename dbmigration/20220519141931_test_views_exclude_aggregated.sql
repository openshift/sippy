-- +goose Up
-- +goose StatementBegin
DROP MATERIALIZED VIEW prow_test_report_7d_matview;

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
  WHERE NOT 'aggregated' = ANY(prow_jobs.variants)
  GROUP BY tests.name, prow_jobs.variants, prow_jobs.release
  WITH NO DATA;

DROP MATERIALIZED VIEW prow_test_report_2d_matview;

CREATE MATERIALIZED VIEW public.prow_test_report_2d_matview AS
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
  WHERE NOT 'aggregated' = ANY(prow_jobs.variants)
  GROUP BY tests.name, prow_jobs.variants, prow_jobs.release
  WITH NO DATA;

DROP MATERIALIZED VIEW prow_test_analysis_by_job_14d_matview;

CREATE MATERIALIZED VIEW public.prow_test_analysis_by_job_14d_matview AS
 SELECT tests.id AS test_id,
    tests.name AS test_name,
    date(prow_job_runs."timestamp") AS date,
    prow_jobs.release,
    prow_jobs.name AS job_name,
    COALESCE(count(
        CASE
            WHEN ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= now())) THEN 1
            ELSE NULL::integer
        END), (0)::bigint) AS runs,
    COALESCE(count(
        CASE
            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
            ELSE NULL::integer
        END), (0)::bigint) AS passes,
    COALESCE(count(
        CASE
            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
            ELSE NULL::integer
        END), (0)::bigint) AS flakes,
    COALESCE(count(
        CASE
            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
            ELSE NULL::integer
        END), (0)::bigint) AS failures
   FROM (((public.prow_job_run_tests
     JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
     JOIN public.prow_job_runs ON ((prow_job_runs.id = prow_job_run_tests.prow_job_run_id)))
     JOIN public.prow_jobs ON ((prow_jobs.id = prow_job_runs.prow_job_id)))
  WHERE (prow_job_runs."timestamp" > (now() - '14 days'::interval))
  AND NOT 'aggregated' = ANY(prow_jobs.variants)
  GROUP BY tests.name, tests.id, (date(prow_job_runs."timestamp")), prow_jobs.release, prow_jobs.name
  WITH NO DATA;

CREATE INDEX test_release_by_job ON public.prow_test_analysis_by_job_14d_matview USING btree (test_name, release);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP MATERIALIZED VIEW prow_test_report_7d_matview;

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

DROP MATERIALIZED VIEW prow_test_report_2d_matview;

CREATE MATERIALIZED VIEW public.prow_test_report_2d_matview AS
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

DROP MATERIALIZED VIEW prow_test_analysis_by_job_14d_matview;

CREATE MATERIALIZED VIEW public.prow_test_analysis_by_job_14d_matview AS
 SELECT tests.id AS test_id,
    tests.name AS test_name,
    date(prow_job_runs."timestamp") AS date,
    prow_jobs.release,
    prow_jobs.name AS job_name,
    COALESCE(count(
        CASE
            WHEN ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= now())) THEN 1
            ELSE NULL::integer
        END), (0)::bigint) AS runs,
    COALESCE(count(
        CASE
            WHEN ((prow_job_run_tests.status = 1) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
            ELSE NULL::integer
        END), (0)::bigint) AS passes,
    COALESCE(count(
        CASE
            WHEN ((prow_job_run_tests.status = 13) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
            ELSE NULL::integer
        END), (0)::bigint) AS flakes,
    COALESCE(count(
        CASE
            WHEN ((prow_job_run_tests.status = 12) AND ((prow_job_runs."timestamp" >= (now() - '14 days'::interval)) AND (prow_job_runs."timestamp" <= now()))) THEN 1
            ELSE NULL::integer
        END), (0)::bigint) AS failures
   FROM (((public.prow_job_run_tests
     JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
     JOIN public.prow_job_runs ON ((prow_job_runs.id = prow_job_run_tests.prow_job_run_id)))
     JOIN public.prow_jobs ON ((prow_jobs.id = prow_job_runs.prow_job_id)))
  WHERE (prow_job_runs."timestamp" > (now() - '14 days'::interval))
  GROUP BY tests.name, tests.id, (date(prow_job_runs."timestamp")), prow_jobs.release, prow_jobs.name
  WITH NO DATA;

CREATE INDEX test_release_by_job ON public.prow_test_analysis_by_job_14d_matview USING btree (test_name, release);

-- +goose StatementEnd
