package db

import (
	"database/sql"
	"fmt"

	"github.com/openshift/sippy/pkg/db/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"k8s.io/klog"
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

	if err := db.AutoMigrate(&models.ReleaseTag{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.PullRequest{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.Repository{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.JobRun{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.ProwJob{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.ProwJobRun{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.Test{}); err != nil {
		return nil, err
	}

	/*
		if err := db.AutoMigrate(&models.ProwJobRunTest{}); err != nil {
			return nil, err
		}
	*/

	if err = createPostgresMaterializedViews(db); err != nil {
		return nil, err
	}

	return &DB{
		DB:        db,
		BatchSize: 1024,
	}, nil
}

// LastID returns the last ID (highest) from a table.
func (db *DB) LastID(table string) int {
	var lastID int
	if res := db.DB.Table(table).Order("id desc").Limit(1).Select("id").Scan(&lastID); res.Error != nil {
		klog.V(1).Infof("error retrieving last id from %q: %s", table, res.Error)
		lastID = 0
	} else {
		klog.V(1).Infof("retrieved last id of %d from %q", lastID, table)
	}

	return lastID
}

func createPostgresMaterializedViews(db *gorm.DB) error {
	for _, pmv := range PostgresMatViews {

		// TODO: temporary, just for developing this
		db.Exec("DROP MATERIALIZED VIEW IF EXISTS ?", pmv.Name)

		var count int64
		if res := db.Raw("SELECT COUNT(*) FROM pg_matviews WHERE matviewname = ?", pmv.Name).Count(&count); res.Error != nil {
			return res.Error
		}
		if count == 0 {
			klog.Infof("creating missing materialized view: %s", pmv.Name)
			if res := db.Exec(
				fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS %s", pmv.Name, pmv.Definition)); res.Error != nil {
				klog.Errorf("error creating materialized view %s: %v", pmv.Name, res.Error)
				return res.Error
			}
		}
	}
	return nil
}

type PostgresMaterializedView struct {
	Name       string
	Definition string
	NamedArgs  []sql.NamedArg
}

var PostgresMatViews = []PostgresMaterializedView{
	/*
		{
			name:       "prow_job_report_7d_matview",
			definition: jobReportMatView,
			namedArgs: []sql.NamedArg{
				sql.Named("start", "INTERVAL 14 DAY"),
				sql.Named("boundary", "INTERVAL 7 DAY"),
			},
		},

	*/
	{
		Name:       "prow_test_report_7d_matview",
		Definition: testReportMatView,
		/*
			namedArgs: []sql.NamedArg{
				sql.Named("start", "INTERVAL 14 DAY"),
				sql.Named("boundary", "INTERVAL 7 DAY"),
			},
		*/
	},
}

// jobReportMatView is a postgresql materialized view showing current vs previous period
// results.
// TODO: unused right now
/*
const jobReportMatView = `
WITH results AS (
        select prow_jobs.name as pj_name,
				prow_jobs.variants as pj_variants,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN NOW() - INTERVAL @start AND NOW() - INTERVAL @boundary then 1 end), 0) as previous_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN NOW() - INTERVAL @start AND NOW() - INTERVAL @boundary then 1 end), 0) as previous_failures,
                coalesce(count(case when timestamp BETWEEN NOW() - INTERVAL @start AND NOW() - INTERVAL @boundary then 1 end), 0) as previous_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp BETWEEN NOW() - INTERVAL @start AND NOW() - INTERVAL @boundary then 1 end), 0) as previous_infra_fails,
                coalesce(count(case when succeeded = true AND timestamp > NOW() - INTERVAL @boundary then 1 end), 0) as current_passes,
                coalesce(count(case when succeeded = false AND timestamp > NOW() - INTERVAL @boundary then 1 end), 0) as current_fails,
                coalesce(count(case when timestamp > NOW() - INTERVAL @boundary then 1 end), 0) as current_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp > NOW() - INTERVAL @boundary then 1 end), 0) as current_infra_fails
        FROM prow_job_runs
        JOIN prow_jobs
                ON prow_jobs.id = prow_job_runs.prow_job_id
                and timestamp BETWEEN NOW() - INTERVAL @start AND NOW()
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
JOIN prow_jobs ON prow_jobs.name = results.pj_name;`
*/

const testReportMatView = `
WITH results AS (
    SELECT tests.name                                                                                    AS testname,
           coalesce(count(case
                              when status = 1 AND
                                   timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY' then 1 end),
                    0)                                                                                   AS previous_passes,
           coalesce(count(case
                              when status = 13 AND
                                   timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY' then 1 end),
                    0)                                                                                   AS previous_flakes,
           coalesce(count(case
                              when status = 12 AND
                                   timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY' then 1 end),
                    0)                                                                                   AS previous_failures,
           coalesce(
                   count(case when timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY' then 1 end),
                   0)                                                                                    as previous_runs,
           coalesce(count(case when status = 1 AND timestamp BETWEEN NOW() - INTERVAL '7 DAY' AND NOW() then 1 end),
                    0)                                                                                   AS current_passes,
           coalesce(count(case when status = 13 AND timestamp BETWEEN NOW() - INTERVAL '7 DAY' AND NOW() then 1 end),
                    0)                                                                                   AS current_flakes,
           coalesce(count(case when status = 12 AND timestamp BETWEEN NOW() - INTERVAL '7 DAY' AND NOW() then 1 end),
                    0)                                                                                   AS current_failures,
           coalesce(count(case when timestamp BETWEEN NOW() - INTERVAL '7 DAY' AND NOW() then 1 end), 0) as current_runs
    FROM prow_job_run_tests
             JOIN tests ON tests.id = prow_job_run_tests.test_id
             JOIN prow_job_runs on prow_job_runs.id = prow_job_run_tests.prow_job_run_id
    GROUP BY tests.name
)
SELECT *,
       current_passes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       current_failures * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
       previous_passes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
       (current_passes * 100.0 / NULLIF(current_runs, 0)) - (previous_passes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
FROM results;
`
