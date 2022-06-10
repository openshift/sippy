package db

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type PostgresFunction struct {
	Name       string
	Definition string
}

var PostgresFunctions = []PostgresFunction{
	{
		Name:       "job_results",
		Definition: jobResultFunction,
	},
}

func syncPostgresFunctions(db *gorm.DB) error {
	for _, pmv := range PostgresFunctions {
		vlog := log.WithFields(log.Fields{"function": pmv.Name})
		// Generate our schema and calculate hash.
		hash := sha256.Sum256([]byte(pmv.Definition))
		hashStr := base64.URLEncoding.EncodeToString(hash[:])
		vlog.WithField("hash", string(hashStr)).Info("generated SHA256 hash")

		currSchemaHash := models.SchemaHash{}
		res := db.Where("type = ? AND name = ?", hashTypeFunction, pmv.Name).Find(&currSchemaHash)
		if res.Error != nil {
			vlog.WithError(res.Error).Error("error looking up schema hash")
		}
		var updateRequired bool
		if currSchemaHash.ID == 0 {
			vlog.Info("no current hash in db, function will be created/replaced")
			currSchemaHash = models.SchemaHash{
				Type: hashTypeFunction,
				Name: pmv.Name,
				Hash: hashStr,
			}
			updateRequired = true
		} else if currSchemaHash.Hash != hashStr {
			vlog.WithField("oldHash", currSchemaHash.Hash).Info("function schema has has changed, recreating")
			updateRequired = true
		}

		if updateRequired {
			vlog.Info("function update required")

			if res := db.Exec(fmt.Sprintf("DROP FUNCTION IF EXISTS %s", pmv.Name)); res.Error != nil {
				log.WithError(res.Error).Error("error dropping postgres function")
				return res.Error
			}

			if res := db.Exec(pmv.Definition); res.Error != nil {
				log.WithError(res.Error).Error("error creating postgres function")
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
		} else {
			vlog.Info("no schema update required")
		}
	}
	return nil
}

const jobResultFunction = `
CREATE FUNCTION public.job_results(release text, start timestamp without time zone, boundary timestamp without time zone, endstamp timestamp without time zone) RETURNS TABLE(pj_name text, pj_variants text[], previous_passes bigint, previous_failures bigint, previous_runs bigint, previous_infra_fails bigint, current_passes bigint, current_fails bigint, current_runs bigint, current_infra_fails bigint, id bigint, created_at timestamp without time zone, updated_at timestamp without time zone, deleted_at timestamp without time zone, name text, release text, variants text[], test_grid_url text, kind text, brief_name text, current_pass_percentage real, current_projected_pass_percentage real, current_failure_percentage real, previous_pass_percentage real, previous_projected_pass_percentage real, previous_failure_percentage real, net_improvement real)
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
`
