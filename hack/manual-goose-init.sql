-- One time sql to be applied to the prod sippy db to mimic that is has been setup with goose.
CREATE TABLE public.goose_db_version (
    id integer NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp without time zone DEFAULT now()
);

ALTER TABLE public.goose_db_version OWNER TO postgres;

CREATE SEQUENCE public.goose_db_version_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER TABLE public.goose_db_version_id_seq OWNER TO postgres;

ALTER SEQUENCE public.goose_db_version_id_seq OWNED BY public.goose_db_version.id;
ALTER TABLE ONLY public.goose_db_version ALTER COLUMN id SET DEFAULT nextval('public.goose_db_version_id_seq'::regclass);

INSERT INTO goose_db_version(version_id, is_applied, tstamp) VALUES(0, 't', NOW());
INSERT INTO goose_db_version(version_id, is_applied, tstamp) VALUES(20220502111933, 't', NOW());
