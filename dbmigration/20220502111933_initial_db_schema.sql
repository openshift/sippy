-- +goose Up
-- +goose StatementBegin
--
-- PostgreSQL database dump
-- obtained with:
--   podman run -ti --rm -e PGPASSWORD=PASS quay.io/enterprisedb/postgresql pg_dump \
--   -h DBHOST -U postgres -p 5432 -s sippy_openshift

-- Dumped from database version 14.1
-- Dumped by pg_dump version 14.2

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: bug_jobs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.bug_jobs (
    bug_id bigint NOT NULL,
    prow_job_id bigint NOT NULL
);


ALTER TABLE public.bug_jobs OWNER TO postgres;

--
-- Name: bug_tests; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.bug_tests (
    bug_id bigint NOT NULL,
    test_id bigint NOT NULL
);


ALTER TABLE public.bug_tests OWNER TO postgres;

--
-- Name: bugs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.bugs (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    status text,
    last_change_time timestamp with time zone,
    summary text,
    target_release text,
    version text,
    component text,
    url text,
    failure_count bigint,
    flake_count bigint
);


ALTER TABLE public.bugs OWNER TO postgres;

--
-- Name: bugs_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.bugs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.bugs_id_seq OWNER TO postgres;

--
-- Name: bugs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.bugs_id_seq OWNED BY public.bugs.id;


--
-- Name: prow_job_run_tests; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.prow_job_run_tests (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    prow_job_run_id bigint,
    test_id bigint,
    suite_id bigint,
    status bigint
);


ALTER TABLE public.prow_job_run_tests OWNER TO postgres;

--
-- Name: prow_job_runs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.prow_job_runs (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    prow_job_id bigint,
    url text,
    test_failures bigint,
    failed boolean,
    infrastructure_failure boolean,
    known_failure boolean,
    succeeded boolean,
    "timestamp" timestamp with time zone,
    overall_result text
);


ALTER TABLE public.prow_job_runs OWNER TO postgres;

--
-- Name: tests; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.tests (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text
);


ALTER TABLE public.tests OWNER TO postgres;

--
-- Name: prow_job_failed_tests_by_day_matview; Type: MATERIALIZED VIEW; Schema: public; Owner: postgres
--

CREATE MATERIALIZED VIEW public.prow_job_failed_tests_by_day_matview AS
 SELECT date_trunc('day'::text, prow_job_runs."timestamp") AS period,
    prow_job_runs.prow_job_id,
    tests.name AS test_name,
    count(tests.name) AS count
   FROM ((public.prow_job_runs
     JOIN public.prow_job_run_tests pjrt ON ((prow_job_runs.id = pjrt.prow_job_run_id)))
     JOIN public.tests tests ON ((pjrt.test_id = tests.id)))
  WHERE (pjrt.status = 12)
  GROUP BY tests.name, (date_trunc('day'::text, prow_job_runs."timestamp")), prow_job_runs.prow_job_id
  WITH NO DATA;


ALTER TABLE public.prow_job_failed_tests_by_day_matview OWNER TO postgres;

--
-- Name: prow_job_run_tests_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.prow_job_run_tests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.prow_job_run_tests_id_seq OWNER TO postgres;

--
-- Name: prow_job_run_tests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.prow_job_run_tests_id_seq OWNED BY public.prow_job_run_tests.id;


--
-- Name: prow_job_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.prow_job_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.prow_job_runs_id_seq OWNER TO postgres;

--
-- Name: prow_job_runs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.prow_job_runs_id_seq OWNED BY public.prow_job_runs.id;


--
-- Name: prow_jobs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.prow_jobs (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text,
    release text,
    variants text[],
    test_grid_url text
);


ALTER TABLE public.prow_jobs OWNER TO postgres;

--
-- Name: prow_jobs_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.prow_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.prow_jobs_id_seq OWNER TO postgres;

--
-- Name: prow_jobs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.prow_jobs_id_seq OWNED BY public.prow_jobs.id;


--
-- Name: prow_test_analysis_by_job_14d_matview; Type: MATERIALIZED VIEW; Schema: public; Owner: postgres
--

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


ALTER TABLE public.prow_test_analysis_by_job_14d_matview OWNER TO postgres;

--
-- Name: prow_test_analysis_by_variant_14d_matview; Type: MATERIALIZED VIEW; Schema: public; Owner: postgres
--

CREATE MATERIALIZED VIEW public.prow_test_analysis_by_variant_14d_matview AS
 SELECT tests.id AS test_id,
    tests.name AS test_name,
    date(prow_job_runs."timestamp") AS date,
    unnest(prow_jobs.variants) AS variant,
    prow_jobs.release,
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
  GROUP BY tests.name, tests.id, (date(prow_job_runs."timestamp")), (unnest(prow_jobs.variants)), prow_jobs.release
  WITH NO DATA;


ALTER TABLE public.prow_test_analysis_by_variant_14d_matview OWNER TO postgres;

--
-- Name: prow_test_report_2d_matview; Type: MATERIALIZED VIEW; Schema: public; Owner: postgres
--

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


ALTER TABLE public.prow_test_report_2d_matview OWNER TO postgres;

--
-- Name: prow_test_report_7d_matview; Type: MATERIALIZED VIEW; Schema: public; Owner: postgres
--

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


ALTER TABLE public.prow_test_report_7d_matview OWNER TO postgres;

--
-- Name: release_job_runs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.release_job_runs (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    release_tag_id bigint,
    name text,
    job_name text,
    kind text,
    state text,
    transition_time timestamp with time zone,
    retries bigint,
    url text,
    upgrades_from text,
    upgrades_to text,
    upgrade boolean
);


ALTER TABLE public.release_job_runs OWNER TO postgres;

--
-- Name: release_job_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.release_job_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.release_job_runs_id_seq OWNER TO postgres;

--
-- Name: release_job_runs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.release_job_runs_id_seq OWNED BY public.release_job_runs.id;


--
-- Name: release_pull_requests; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.release_pull_requests (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    url text,
    pull_request_id text,
    name text,
    description text,
    bug_url text
);


ALTER TABLE public.release_pull_requests OWNER TO postgres;

--
-- Name: release_pull_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.release_pull_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.release_pull_requests_id_seq OWNER TO postgres;

--
-- Name: release_pull_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.release_pull_requests_id_seq OWNED BY public.release_pull_requests.id;


--
-- Name: release_repositories; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.release_repositories (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text,
    release_tag_id bigint,
    repository_head text,
    diff_url text
);


ALTER TABLE public.release_repositories OWNER TO postgres;

--
-- Name: release_repositories_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.release_repositories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.release_repositories_id_seq OWNER TO postgres;

--
-- Name: release_repositories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.release_repositories_id_seq OWNED BY public.release_repositories.id;


--
-- Name: release_tag_pull_requests; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.release_tag_pull_requests (
    release_tag_id bigint NOT NULL,
    release_pull_request_id bigint NOT NULL
);


ALTER TABLE public.release_tag_pull_requests OWNER TO postgres;

--
-- Name: release_tags; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.release_tags (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    release_tag text,
    release text,
    stream text,
    architecture text,
    phase text,
    release_time timestamp with time zone,
    previous_release_tag text,
    kubernetes_version text,
    current_os_version text,
    previous_os_version text,
    current_os_url text,
    previous_os_url text,
    os_diff_url text
);


ALTER TABLE public.release_tags OWNER TO postgres;

--
-- Name: release_tags_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.release_tags_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.release_tags_id_seq OWNER TO postgres;

--
-- Name: release_tags_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.release_tags_id_seq OWNED BY public.release_tags.id;


--
-- Name: suites; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.suites (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text
);


ALTER TABLE public.suites OWNER TO postgres;

--
-- Name: suites_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.suites_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.suites_id_seq OWNER TO postgres;

--
-- Name: suites_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.suites_id_seq OWNED BY public.suites.id;


--
-- Name: tests_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.tests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.tests_id_seq OWNER TO postgres;

--
-- Name: tests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.tests_id_seq OWNED BY public.tests.id;


--
-- Name: bugs id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bugs ALTER COLUMN id SET DEFAULT nextval('public.bugs_id_seq'::regclass);


--
-- Name: prow_job_run_tests id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_job_run_tests ALTER COLUMN id SET DEFAULT nextval('public.prow_job_run_tests_id_seq'::regclass);


--
-- Name: prow_job_runs id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_job_runs ALTER COLUMN id SET DEFAULT nextval('public.prow_job_runs_id_seq'::regclass);


--
-- Name: prow_jobs id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_jobs ALTER COLUMN id SET DEFAULT nextval('public.prow_jobs_id_seq'::regclass);


--
-- Name: release_job_runs id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_job_runs ALTER COLUMN id SET DEFAULT nextval('public.release_job_runs_id_seq'::regclass);


--
-- Name: release_pull_requests id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_pull_requests ALTER COLUMN id SET DEFAULT nextval('public.release_pull_requests_id_seq'::regclass);


--
-- Name: release_repositories id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_repositories ALTER COLUMN id SET DEFAULT nextval('public.release_repositories_id_seq'::regclass);


--
-- Name: release_tags id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_tags ALTER COLUMN id SET DEFAULT nextval('public.release_tags_id_seq'::regclass);


--
-- Name: suites id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.suites ALTER COLUMN id SET DEFAULT nextval('public.suites_id_seq'::regclass);


--
-- Name: tests id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.tests ALTER COLUMN id SET DEFAULT nextval('public.tests_id_seq'::regclass);


--
-- Name: prow_job_runs prow_job_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_job_runs
    ADD CONSTRAINT prow_job_runs_pkey PRIMARY KEY (id);


--
-- Name: prow_job_runs_report_matview; Type: MATERIALIZED VIEW; Schema: public; Owner: postgres
--

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


ALTER TABLE public.prow_job_runs_report_matview OWNER TO postgres;

--
-- Name: bug_jobs bug_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bug_jobs
    ADD CONSTRAINT bug_jobs_pkey PRIMARY KEY (bug_id, prow_job_id);


--
-- Name: bug_tests bug_tests_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bug_tests
    ADD CONSTRAINT bug_tests_pkey PRIMARY KEY (bug_id, test_id);


--
-- Name: bugs bugs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bugs
    ADD CONSTRAINT bugs_pkey PRIMARY KEY (id);


--
-- Name: prow_job_run_tests prow_job_run_tests_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_job_run_tests
    ADD CONSTRAINT prow_job_run_tests_pkey PRIMARY KEY (id);


--
-- Name: prow_jobs prow_jobs_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_jobs
    ADD CONSTRAINT prow_jobs_name_key UNIQUE (name);


--
-- Name: prow_jobs prow_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_jobs
    ADD CONSTRAINT prow_jobs_pkey PRIMARY KEY (id);


--
-- Name: release_job_runs release_job_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_job_runs
    ADD CONSTRAINT release_job_runs_pkey PRIMARY KEY (id);


--
-- Name: release_pull_requests release_pull_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_pull_requests
    ADD CONSTRAINT release_pull_requests_pkey PRIMARY KEY (id);


--
-- Name: release_repositories release_repositories_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_repositories
    ADD CONSTRAINT release_repositories_pkey PRIMARY KEY (id);


--
-- Name: release_tag_pull_requests release_tag_pull_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_tag_pull_requests
    ADD CONSTRAINT release_tag_pull_requests_pkey PRIMARY KEY (release_tag_id, release_pull_request_id);


--
-- Name: release_tags release_tags_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_tags
    ADD CONSTRAINT release_tags_pkey PRIMARY KEY (id);


--
-- Name: suites suites_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.suites
    ADD CONSTRAINT suites_pkey PRIMARY KEY (id);


--
-- Name: tests tests_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.tests
    ADD CONSTRAINT tests_pkey PRIMARY KEY (id);


--
-- Name: idx_bugs_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_bugs_deleted_at ON public.bugs USING btree (deleted_at);


--
-- Name: idx_prow_job_run_tests_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_prow_job_run_tests_deleted_at ON public.prow_job_run_tests USING btree (deleted_at);


--
-- Name: idx_prow_job_runs_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_prow_job_runs_deleted_at ON public.prow_job_runs USING btree (deleted_at);


--
-- Name: idx_prow_jobs_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_prow_jobs_deleted_at ON public.prow_jobs USING btree (deleted_at);


--
-- Name: idx_release_job_runs_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_release_job_runs_deleted_at ON public.release_job_runs USING btree (deleted_at);


--
-- Name: idx_release_pull_requests_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_release_pull_requests_deleted_at ON public.release_pull_requests USING btree (deleted_at);


--
-- Name: idx_release_repositories_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_release_repositories_deleted_at ON public.release_repositories USING btree (deleted_at);


--
-- Name: idx_release_tags_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_release_tags_deleted_at ON public.release_tags USING btree (deleted_at);


--
-- Name: idx_suites_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_suites_deleted_at ON public.suites USING btree (deleted_at);


--
-- Name: idx_suites_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_suites_name ON public.suites USING btree (name);


--
-- Name: idx_tests_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_tests_deleted_at ON public.tests USING btree (deleted_at);


--
-- Name: idx_tests_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_tests_name ON public.tests USING btree (name);


--
-- Name: pr_url_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX pr_url_name ON public.release_pull_requests USING btree (url, name);


--
-- Name: test_release_by_job; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX test_release_by_job ON public.prow_test_analysis_by_job_14d_matview USING btree (test_name, release);


--
-- Name: test_release_by_variant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX test_release_by_variant ON public.prow_test_analysis_by_variant_14d_matview USING btree (test_name, release);


--
-- Name: bug_jobs fk_bug_jobs_bug; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bug_jobs
    ADD CONSTRAINT fk_bug_jobs_bug FOREIGN KEY (bug_id) REFERENCES public.bugs(id);


--
-- Name: bug_jobs fk_bug_jobs_prow_job; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bug_jobs
    ADD CONSTRAINT fk_bug_jobs_prow_job FOREIGN KEY (prow_job_id) REFERENCES public.prow_jobs(id);


--
-- Name: bug_tests fk_bug_tests_bug; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bug_tests
    ADD CONSTRAINT fk_bug_tests_bug FOREIGN KEY (bug_id) REFERENCES public.bugs(id);


--
-- Name: bug_tests fk_bug_tests_test; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bug_tests
    ADD CONSTRAINT fk_bug_tests_test FOREIGN KEY (test_id) REFERENCES public.tests(id);


--
-- Name: prow_job_run_tests fk_prow_job_run_tests_test; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_job_run_tests
    ADD CONSTRAINT fk_prow_job_run_tests_test FOREIGN KEY (test_id) REFERENCES public.tests(id);


--
-- Name: prow_job_runs fk_prow_job_runs_prow_job; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_job_runs
    ADD CONSTRAINT fk_prow_job_runs_prow_job FOREIGN KEY (prow_job_id) REFERENCES public.prow_jobs(id);


--
-- Name: prow_job_run_tests fk_prow_job_runs_tests; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.prow_job_run_tests
    ADD CONSTRAINT fk_prow_job_runs_tests FOREIGN KEY (prow_job_run_id) REFERENCES public.prow_job_runs(id);


--
-- Name: release_tag_pull_requests fk_release_tag_pull_requests_release_pull_request; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_tag_pull_requests
    ADD CONSTRAINT fk_release_tag_pull_requests_release_pull_request FOREIGN KEY (release_pull_request_id) REFERENCES public.release_pull_requests(id);


--
-- Name: release_tag_pull_requests fk_release_tag_pull_requests_release_tag; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_tag_pull_requests
    ADD CONSTRAINT fk_release_tag_pull_requests_release_tag FOREIGN KEY (release_tag_id) REFERENCES public.release_tags(id);


--
-- Name: release_job_runs fk_release_tags_job_runs; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_job_runs
    ADD CONSTRAINT fk_release_tags_job_runs FOREIGN KEY (release_tag_id) REFERENCES public.release_tags(id);


--
-- Name: release_repositories fk_release_tags_repositories; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.release_repositories
    ADD CONSTRAINT fk_release_tags_repositories FOREIGN KEY (release_tag_id) REFERENCES public.release_tags(id);


--
-- Name: job_results(text, timestamp without time zone, timestamp without time zone, timestamp without time zone); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.job_results(release text, start timestamp without time zone, boundary timestamp without time zone, endstamp timestamp without time zone) RETURNS TABLE(pj_name text, pj_variants text[], previous_passes bigint, previous_failures bigint, previous_runs bigint, previous_infra_fails bigint, current_passes bigint, current_fails bigint, current_runs bigint, current_infra_fails bigint, id bigint, created_at timestamp without time zone, updated_at timestamp without time zone, deleted_at timestamp without time zone, name text, release text, variants text[], test_grid_url text, brief_name text, current_pass_percentage real, current_projected_pass_percentage real, current_failure_percentage real, previous_pass_percentage real, previous_projected_pass_percentage real, previous_failure_percentage real, net_improvement real)
    LANGUAGE sql
    AS $_$
WITH results AS (
        select prow_jobs.name as pj_name, prow_jobs.variants as pj_variants,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN $2 AND $3 then 1 end), 0) as previous_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN $2 AND $3 then 1 end), 0) as previous_failures,
                coalesce(count(case when timestamp BETWEEN $2 AND $3 then 1 end), 0) as previous_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp BETWEEN $2 AND $3 then 1 end), 0) as previous_infra_fails,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN $3 AND $4 then 1 end), 0) as current_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN $3 AND $4 then 1 end), 0) as current_fails,
                coalesce(count(case when timestamp BETWEEN $3 AND $4 then 1 end), 0) as current_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp BETWEEN $3 AND $4 then 1 end), 0) as current_infra_fails
        FROM prow_job_runs
        JOIN prow_jobs
                ON prow_jobs.id = prow_job_runs.prow_job_id
                                AND prow_jobs.release = $1
                AND timestamp BETWEEN $2 AND $4
        group by prow_jobs.name, prow_jobs.variants
)
SELECT *,
        REGEXP_REPLACE(results.pj_name, 'periodic-ci-openshift-(multiarch|release)-master-(ci|nightly)-[0-9]+.[0-9]+-', '') as brief_name,
        current_passes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
        (current_passes + current_infra_fails) * 100.0 / NULLIF(current_runs, 0) AS current_projected_pass_percentage,
        current_fails * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
        previous_passes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
        (previous_passes + previous_infra_fails) * 100.0 / NULLIF(previous_runs, 0) AS previous_projected_pass_percentage,
        previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
        (current_passes * 100.0 / NULLIF(current_runs, 0)) - (previous_passes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
FROM results
JOIN prow_jobs ON prow_jobs.name = results.pj_name
$_$;


ALTER FUNCTION public.job_results(release text, start timestamp without time zone, boundary timestamp without time zone, endstamp timestamp without time zone) OWNER TO postgres;

--
-- Name: test_results(timestamp without time zone, timestamp without time zone, timestamp without time zone); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.test_results(start timestamp without time zone, boundary timestamp without time zone, endstamp timestamp without time zone) RETURNS TABLE(id bigint, name text, previous_successes bigint, previous_flakes bigint, previous_failures bigint, previous_runs bigint, current_successes bigint, current_flakes bigint, current_failures bigint, current_runs bigint, current_pass_percentage double precision, current_failure_percentage double precision, previous_pass_percentage double precision, previous_failure_percentage double precision, net_improvement double precision, release text)
    LANGUAGE sql
    AS $_$
WITH results AS (
  SELECT
    tests.id AS id,
    coalesce(count(case when status = 1 AND timestamp BETWEEN $1 AND $2 then 1 end), 0) AS previous_successes,
    coalesce(count(case when status = 13 AND timestamp BETWEEN $1 AND $2 then 1 end), 0) AS previous_flakes,
    coalesce(count(case when status = 12 AND timestamp BETWEEN $1 AND $2 then 1 end), 0) AS previous_failures,
    coalesce(count(case when timestamp BETWEEN $1 AND $2 then 1 end), 0) as previous_runs,
    coalesce(count(case when status = 1 AND timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_successes,
    coalesce(count(case when status = 13 AND timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_flakes,
    coalesce(count(case when status = 12 AND timestamp BETWEEN $2 AND $3 then 1 end), 0) AS current_failures,
    coalesce(count(case when timestamp BETWEEN $2 AND $3 then 1 end), 0) as current_runs,
    prow_jobs.release
FROM prow_job_run_tests
    JOIN tests ON tests.id = prow_job_run_tests.test_id
    JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id
    JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
GROUP BY tests.id, prow_jobs.release
)
SELECT tests.id,
       tests.name,
       previous_successes,
       previous_flakes,
       previous_failures,
       previous_runs,
       current_successes,
       current_flakes,
       current_failures,
       current_runs,
       current_successes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       current_failures * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
       previous_successes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
       (current_successes * 100.0 / NULLIF(current_runs, 0)) - (previous_successes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement,
       release
FROM results
INNER JOIN tests on tests.id = results.id
$_$;


ALTER FUNCTION public.test_results(start timestamp without time zone, boundary timestamp without time zone, endstamp timestamp without time zone) OWNER TO postgres;

INSERT INTO suites(created_at, updated_at, name) VALUES(NOW(), NOW(), 'openshift-tests');
INSERT INTO suites(created_at, updated_at, name) VALUES(NOW(), NOW(), 'openshift-tests-upgrade');
INSERT INTO suites(created_at, updated_at, name) VALUES(NOW(), NOW(), 'sippy');


--
-- PostgreSQL database dump complete
--

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- +goose StatementEnd
