-- +goose Up
-- +goose StatementBegin
DROP MATERIALIZED VIEW public.prow_job_runs_report_matview;
CREATE MATERIALIZED VIEW public.prow_job_runs_report_matview AS
 WITH failed_test_results AS (
         SELECT prow_job_run_tests.prow_job_run_id,
            ARRAY_AGG(tests.id) AS test_ids,
            COUNT(tests.id) AS test_count,
            ARRAY_AGG(tests.name) AS test_names
           FROM (public.prow_job_run_tests
             JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
          WHERE (prow_job_run_tests.status = 12)
          GROUP BY prow_job_run_id
        ), flaked_test_results AS (
         SELECT prow_job_run_tests.prow_job_run_id,
            ARRAY_AGG(tests.id) AS test_ids,
            COUNT(tests.id) AS test_count,
            ARRAY_AGG(tests.name) AS test_names
           FROM (public.prow_job_run_tests
             JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
          WHERE (prow_job_run_tests.status = 13)
          GROUP BY prow_job_run_id
        )
 SELECT prow_job_runs.id,
    prow_jobs.release,
    prow_jobs.name,
    prow_jobs.name AS job,
    prow_jobs.variants,
    regexp_replace(prow_jobs.name, 'periodic-ci-openshift-(multiarch|release)-master-(ci|nightly)-[0-9]+.[0-9]+-'::text, ''::text) AS brief_name,
    prow_job_runs.overall_result,
    prow_job_runs.url AS test_grid_url,
    prow_job_runs.url,
    prow_job_runs.succeeded,
    prow_job_runs.infrastructure_failure,
    prow_job_runs.known_failure,
    ((EXTRACT(epoch FROM (prow_job_runs."timestamp" AT TIME ZONE 'utc'::text)) * (1000)::numeric))::bigint AS "timestamp",
    prow_job_runs.id AS prow_id,
    flaked_test_results.test_names AS flaked_test_names,
    flaked_test_results.test_count AS test_flakes,
    failed_test_results.test_names AS failed_test_names,
    failed_test_results.test_count AS test_failures
   FROM (((public.prow_job_runs
     JOIN failed_test_results ON ((failed_test_results.prow_job_run_id = prow_job_runs.id)))
     JOIN flaked_test_results ON ((flaked_test_results.prow_job_run_id = prow_job_runs.id)))
     JOIN public.prow_jobs ON ((prow_job_runs.prow_job_id = prow_jobs.id)))
   WITH NO DATA;
CREATE UNIQUE INDEX idx_prow_job_runs_report_matview ON prow_job_runs_report_matview(id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP MATERIALIZED VIEW public.prow_job_runs_report_matview;

CREATE MATERIALIZED VIEW public.prow_job_runs_report_matview AS
 WITH failed_test_results AS (
         SELECT prow_job_run_tests.prow_job_run_id,
            tests.id,
            tests.name,
            prow_job_run_tests.status
           FROM (public.prow_job_run_tests
             JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
          WHERE (prow_job_run_tests.status = 12)
        ), flaked_test_results AS (
         SELECT prow_job_run_tests.prow_job_run_id,
            tests.id,
            tests.name,
            prow_job_run_tests.status
           FROM (public.prow_job_run_tests
             JOIN public.tests ON ((tests.id = prow_job_run_tests.test_id)))
          WHERE (prow_job_run_tests.status = 13)
        )
 SELECT prow_job_runs.id,
    prow_jobs.release,
    prow_jobs.name,
    prow_jobs.name AS job,
    prow_jobs.variants,
    regexp_replace(prow_jobs.name, 'periodic-ci-openshift-(multiarch|release)-master-(ci|nightly)-[0-9]+.[0-9]+-'::text, ''::text) AS brief_name,
    prow_job_runs.overall_result,
    prow_job_runs.url AS test_grid_url,
    prow_job_runs.url,
    prow_job_runs.succeeded,
    prow_job_runs.infrastructure_failure,
    prow_job_runs.known_failure,
    ((EXTRACT(epoch FROM (prow_job_runs."timestamp" AT TIME ZONE 'utc'::text)) * (1000)::numeric))::bigint AS "timestamp",
    prow_job_runs.id AS prow_id,
    array_remove(array_agg(flaked_test_results.name), NULL::text) AS flaked_test_names,
    count(flaked_test_results.name) AS test_flakes,
    array_remove(array_agg(failed_test_results.name), NULL::text) AS failed_test_names,
    count(failed_test_results.name) AS test_failures
   FROM (((public.prow_job_runs
     LEFT JOIN failed_test_results ON ((failed_test_results.prow_job_run_id = prow_job_runs.id)))
     LEFT JOIN flaked_test_results ON ((flaked_test_results.prow_job_run_id = prow_job_runs.id)))
     JOIN public.prow_jobs ON ((prow_job_runs.prow_job_id = prow_jobs.id)))
  GROUP BY prow_job_runs.id, prow_jobs.name, prow_jobs.variants, prow_jobs.release
  WITH NO DATA;

CREATE UNIQUE INDEX idx_prow_job_runs_report_matview ON prow_job_runs_report_matview(id);
-- +goose StatementEnd
