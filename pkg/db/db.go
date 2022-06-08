package db

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	DB *gorm.DB

	// BatchSize is used for how many insertions we should do at once. Postgres supports
	// a maximum of 2^16 records per insert.
	BatchSize int
}

func New(dsn string) (*DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	if err := createPostgresMaterializedViews(db); err != nil {
		return nil, err
	}

	if err := createPostgresFunctions(db); err != nil {
		return nil, err
	}
	return &DB{
		DB:        db,
		BatchSize: 1024,
	}, nil
}

type PostgresMaterializedView struct {
	// Name is the name of the materialized view in postgres.
	Name string
	// Definition is the material view definition.
	Definition string
	// ReplaceStrings is a map of strings we want to replace in the create view statement, allowing for re-use.
	ReplaceStrings map[string]string
	// IndexColumns are the columns to create a unique index for. Will be named idx_[matviewname] and automatically
	// replaced if changes are made to these values. IndexColumns are required as we need them defined to be able to
	// refresh materialized views concurrently. (avoiding locking reads for several minutes while we update)
	IndexColumns []string
}

const (
	hashTypeMatView      = "matview"
	hashTypeMatViewIndex = "matview_index"
	hashTypeFunction     = "function"
)

func createPostgresMaterializedViews(db *gorm.DB) error {
	for _, pmv := range PostgresMatViews {
		vlog := log.WithFields(log.Fields{"view": pmv.Name})

		// Generate our materialized view schema and calculate hash:
		vd := pmv.Definition
		for k, v := range pmv.ReplaceStrings {
			vd = strings.ReplaceAll(vd, k, v)
		}
		hash := sha256.Sum256([]byte(vd))
		hashStr := base64.URLEncoding.EncodeToString(hash[:])
		vlog.WithField("hash", string(hashStr)).Info("generated SHA256 hash")

		// If we have no recorded hash or the hash doesn't match, delete the current matview, recreate, and store new hash:
		currSchemaHash := models.SchemaHash{}
		res := db.Where("type = ? AND name = ?", hashTypeMatView, pmv.Name).Find(&currSchemaHash)
		if res.Error != nil {
			vlog.WithError(res.Error).Error("error looking up schema hash")
		}
		// If true we will delete the existing view if there is one, and recreate with latest schema, then store the new hash
		var viewUpdateRequired bool
		if currSchemaHash.ID == 0 {
			vlog.Info("no current hash in db, view will be created")
			viewUpdateRequired = true
			currSchemaHash = models.SchemaHash{
				Type: hashTypeMatView,
				Name: pmv.Name,
				Hash: hashStr,
			}
		}

		// Check if the view exists at all:
		var count int64
		if res := db.Raw("SELECT COUNT(*) FROM pg_matviews WHERE matviewname = ?", pmv.Name).Count(&count); res.Error != nil {
			return res.Error
		}
		if count == 0 {
			vlog.Info("view does not exist, creating")
			viewUpdateRequired = true
		}
		if currSchemaHash.Hash != hashStr {
			vlog.WithField("oldHash", currSchemaHash.Hash).Info("view schema has has changed, recreating")
		}

		if viewUpdateRequired {
			if count > 0 {
				vlog.Info("dropping existing view")
				if res := db.Exec(fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", pmv.Name)); res.Error != nil {
					vlog.WithError(res.Error).Error("error dropping materialized view")
					return res.Error
				}
			}

			vlog.Info("creating view with latest schema")

			// TODO: remove WITH NO DATA
			// Create includes an implicit refresh, but then the view will appear populate and our initialization
			// code will not attempt a refresh in that case.
			if res := db.Exec(
				fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS %s WITH NO DATA", pmv.Name, vd)); res.Error != nil {
				log.WithError(res.Error).Error("error creating materialized view")
				return res.Error
			}

			if currSchemaHash.ID == 0 {
				if res := db.Create(&currSchemaHash); res.Error != nil {
					vlog.WithError(res.Error).Error("error creating schema hash")
				}
			} else {
				if res := db.Save(&currSchemaHash); res.Error != nil {
					vlog.WithError(res.Error).Error("error updating schema hash")
				}
			}
			vlog.Info("schema hash updated")
		}

		// TODO indicies
	}
	if true {
		return fmt.Errorf("oops")
	}

	return nil
}

var PostgresMatViews = []PostgresMaterializedView{
	{
		Name:         "prow_test_report_7d_matview",
		Definition:   testReportMatView,
		IndexColumns: []string{"id", "release", "variants"},
		ReplaceStrings: map[string]string{
			"|||START|||":    "NOW() - INTERVAL '14 DAY'",
			"|||BOUNDARY|||": "NOW() - INTERVAL '7 DAY'",
			"|||END|||":      "NOW()",
		},
	},
	{
		Name:         "prow_test_report_2d_matview",
		Definition:   testReportMatView,
		IndexColumns: []string{"id", "release", "variants"},
		ReplaceStrings: map[string]string{
			"|||START|||":    "NOW() - INTERVAL '9 DAY'",
			"|||BOUNDARY|||": "NOW() - INTERVAL '2 DAY'",
			"|||END|||":      "NOW()",
		},
	},
	{
		Name:         "prow_test_analysis_by_variant_14d_matview",
		Definition:   testAnalysisByVariantMatView,
		IndexColumns: []string{"test_id", "date", "variant", "release"},
	},
	{
		Name:         "prow_test_analysis_by_job_14d_matview",
		Definition:   testAnalysisByJobMatView,
		IndexColumns: []string{"test_id", "date", "job_name"},
	},
	{
		Name:         "prow_job_runs_report_matview",
		Definition:   jobRunsReportMatView,
		IndexColumns: []string{"id"},
	},
	{
		Name:         "prow_job_failed_tests_by_day_matview",
		Definition:   prowJobFailedTestsMatView,
		IndexColumns: []string{"period", "prow_job_id", "test_name"},
		ReplaceStrings: map[string]string{
			"|||BY|||": "day",
		},
	},
	{
		Name:         "prow_job_failed_tests_by_hour_matview",
		Definition:   prowJobFailedTestsMatView,
		IndexColumns: []string{"period", "prow_job_id", "test_name"},
		ReplaceStrings: map[string]string{
			"|||BY|||": "hour",
		},
	},
}

const jobRunsReportMatView = `
WITH failed_test_results AS (
	SELECT prow_job_run_tests.prow_job_run_id,
		array_agg(tests.id) AS test_ids,
		count(tests.id) AS test_count,
		array_agg(tests.name) AS test_names
	FROM prow_job_run_tests
		JOIN tests ON tests.id = prow_job_run_tests.test_id
	WHERE prow_job_run_tests.status = 12
	GROUP BY prow_job_run_tests.prow_job_run_id
), flaked_test_results AS (
	SELECT prow_job_run_tests.prow_job_run_id,
		array_agg(tests.id) AS test_ids,
		count(tests.id) AS test_count,
		array_agg(tests.name) AS test_names
	FROM prow_job_run_tests
		JOIN tests ON tests.id = prow_job_run_tests.test_id
	WHERE prow_job_run_tests.status = 13
	GROUP BY prow_job_run_tests.prow_job_run_id
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
   (EXTRACT(epoch FROM (prow_job_runs."timestamp" AT TIME ZONE 'utc'::text)) * 1000::numeric)::bigint AS "timestamp",
   prow_job_runs.id AS prow_id,
   flaked_test_results.test_names AS flaked_test_names,
   flaked_test_results.test_count AS test_flakes,
   failed_test_results.test_names AS failed_test_names,
   failed_test_results.test_count AS test_failures
FROM prow_job_runs
   LEFT JOIN failed_test_results ON failed_test_results.prow_job_run_id = prow_job_runs.id
   LEFT JOIN flaked_test_results ON flaked_test_results.prow_job_run_id = prow_job_runs.id
   JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
`

const testReportMatView = `
SELECT tests.id,
   tests.name,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 1 AND prow_job_runs."timestamp" BETWEEN |||START||| AND |||BOUNDARY||| THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS previous_successes,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 13 AND prow_job_runs."timestamp" BETWEEN |||START||| AND |||BOUNDARY||| THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS previous_flakes,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 12 AND prow_job_runs."timestamp" BETWEEN |||START||| AND |||BOUNDARY||| THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS previous_failures,
   COALESCE(count(
       CASE
           WHEN prow_job_runs."timestamp" BETWEEN |||START||| AND |||BOUNDARY||| THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS previous_runs,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 1 AND prow_job_runs."timestamp" BETWEEN |||BOUNDARY||| AND |||END||| THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS current_successes,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 13 AND prow_job_runs."timestamp" BETWEEN |||BOUNDARY||| AND |||END||| THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS current_flakes,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 12 AND prow_job_runs."timestamp" BETWEEN |||BOUNDARY||| AND |||END||| THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS current_failures,
   COALESCE(count(
       CASE
           WHEN prow_job_runs."timestamp" BETWEEN |||BOUNDARY||| AND |||END||| THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS current_runs,
   prow_jobs.variants,
   prow_jobs.release
FROM prow_job_run_tests
   JOIN tests ON tests.id = prow_job_run_tests.test_id
   JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id
   JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
WHERE NOT ('aggregated'::text = ANY (prow_jobs.variants))
GROUP BY tests.id, tests.name, prow_jobs.variants, prow_jobs.release
`

const testAnalysisByVariantMatView = `
SELECT tests.id AS test_id,
   tests.name AS test_name,
   date(prow_job_runs."timestamp") AS date,
   unnest(prow_jobs.variants) AS variant,
   prow_jobs.release,
   COALESCE(count(
       CASE
           WHEN prow_job_runs."timestamp" >= (now() - '14 days'::interval) AND prow_job_runs."timestamp" <= now() THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS runs,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 1 AND prow_job_runs."timestamp" >= (now() - '14 days'::interval) AND prow_job_runs."timestamp" <= now() THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS passes,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 13 AND prow_job_runs."timestamp" >= (now() - '14 days'::interval) AND prow_job_runs."timestamp" <= now() THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS flakes,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 12 AND prow_job_runs."timestamp" >= (now() - '14 days'::interval) AND prow_job_runs."timestamp" <= now() THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS failures
FROM prow_job_run_tests
    JOIN tests ON tests.id = prow_job_run_tests.test_id
	JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id
	JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id
WHERE prow_job_runs."timestamp" > (now() - '14 days'::interval)
GROUP BY tests.name, tests.id, (date(prow_job_runs."timestamp")), (unnest(prow_jobs.variants)), prow_jobs.release
`

const testAnalysisByJobMatView = `
SELECT tests.id AS test_id,
   tests.name AS test_name,
   date(prow_job_runs."timestamp") AS date,
   prow_jobs.release,
   prow_jobs.name AS job_name,
   COALESCE(count(
       CASE
           WHEN prow_job_runs."timestamp" >= (now() - '14 days'::interval) AND prow_job_runs."timestamp" <= now() THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS runs,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 1 AND prow_job_runs."timestamp" >= (now() - '14 days'::interval) AND prow_job_runs."timestamp" <= now() THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS passes,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 13 AND prow_job_runs."timestamp" >= (now() - '14 days'::interval) AND prow_job_runs."timestamp" <= now() THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS flakes,
   COALESCE(count(
       CASE
           WHEN prow_job_run_tests.status = 12 AND prow_job_runs."timestamp" >= (now() - '14 days'::interval) AND prow_job_runs."timestamp" <= now() THEN 1
           ELSE NULL::integer
       END), 0::bigint) AS failures
FROM prow_job_run_tests
    JOIN tests ON tests.id = prow_job_run_tests.test_id
    JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id
    JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id
WHERE prow_job_runs."timestamp" > (now() - '14 days'::interval) AND NOT ('aggregated'::text = ANY (prow_jobs.variants))
GROUP BY tests.name, tests.id, (date(prow_job_runs."timestamp")), prow_jobs.release, prow_jobs.name
`

const prowJobFailedTestsMatView = `
SELECT date_trunc('|||BY|||'::text, prow_job_runs."timestamp") AS period,
   prow_job_runs.prow_job_id,
   tests.name AS test_name,
   count(tests.name) AS count
FROM prow_job_runs
   JOIN prow_job_run_tests pjrt ON prow_job_runs.id = pjrt.prow_job_run_id
   JOIN tests tests ON pjrt.test_id = tests.id
WHERE pjrt.status = 12
GROUP BY tests.name, (date_trunc('|||BY|||'::text, prow_job_runs."timestamp")), prow_job_runs.prow_job_id
`

func createPostgresFunctions(db *gorm.DB) error {
	if res := db.Exec(jobResultFunction); res.Error != nil {
		log.WithError(res.Error).Error("error creating postgres function")
		return res.Error
	}
	return nil
}

const jobResultFunction = `
create or replace function job_results(release text, start timestamp, boundary timestamp, endstamp timestamp)
  returns table (pj_name text,
        pj_variants text[],                                       
        previous_passes bigint,                                                                                                                                            
        previous_failures bigint,                                                                                                                                             
        previous_runs bigint,                                                                                                                       
        previous_infra_fails bigint,                                                                                                                                                         
        current_passes bigint,                                                                                                                                            
        current_fails bigint,                                                                                                                                                     
        current_runs bigint,                                                                                                                       
        current_infra_fails bigint,                                                                                                                                                        
        id bigint,
        created_at timestamp,
        updated_at timestamp,                                               
        deleted_at timestamp,
        name text,                                                                            
        release text,                              
        variants text[],
        test_grid_url text,
        brief_name text,                                                                                                                  
        current_pass_percentage real,                                               
        current_projected_pass_percentage REAL,
        current_failure_percentage real,                                              
        previous_pass_percentage real,                                                 
        previous_projected_pass_percentage real,
        previous_failure_percentage real,                                                   
        net_improvement real)                                                                                                       
as          
$body$                                            
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
$body$
language sql;
`
