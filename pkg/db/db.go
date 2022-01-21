package db

import (
	"fmt"
	"strings"

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

	if err := db.AutoMigrate(&models.ProwJobRunTest{}); err != nil {
		return nil, err
	}

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
		db.Exec(fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", pmv.Name))

		var count int64
		if res := db.Raw("SELECT COUNT(*) FROM pg_matviews WHERE matviewname = ?", pmv.Name).Count(&count); res.Error != nil {
			return res.Error
		}
		if count == 0 {
			klog.Infof("creating missing materialized view: %s", pmv.Name)

			vd := pmv.Definition
			for k, v := range pmv.ReplaceStrings {
				vd = strings.ReplaceAll(vd, k, v)
			}

			if res := db.Exec(
				fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS %s", pmv.Name, vd)); res.Error != nil {
				klog.Errorf("error creating materialized view %s: %v", pmv.Name, res.Error)
				return res.Error
			}
		}
	}
	return nil
}

type PostgresMaterializedView struct {
	// Name is the name of the materialized view in postgres.
	Name string
	// Definition is the material view definition.
	Definition string
	// ReplaceStrings is a map of strings we want to replace in the create view statement, allowing for re-use.
	ReplaceStrings map[string]string
}

var PostgresMatViews = []PostgresMaterializedView{
	{
		Name:       "prow_test_report_7d_matview",
		Definition: testReportMatView,
		ReplaceStrings: map[string]string{
			"|||START|||":    "NOW() - INTERVAL '14 DAY'",
			"|||BOUNDARY|||": "NOW() - INTERVAL '7 DAY'",
			"|||END|||":      "NOW()",
		},
	},
	{
		Name:       "prow_test_report_2d_matview",
		Definition: testReportMatView,
		ReplaceStrings: map[string]string{
			"|||START|||":    "NOW() - INTERVAL '9 DAY'",
			"|||BOUNDARY|||": "NOW() - INTERVAL '2 DAY'",
			"|||END|||":      "NOW()",
		},
	},
}

const testReportMatView = `
SELECT 
	tests.name AS name,
	coalesce(count(case when status = 1 AND timestamp BETWEEN |||START||| AND |||BOUNDARY||| then 1 end), 0) AS previous_successes,
    coalesce(count(case when status = 13 AND timestamp BETWEEN |||START||| AND |||BOUNDARY||| then 1 end), 0) AS previous_flakes,
    coalesce(count(case when status = 12 AND timestamp BETWEEN |||START||| AND |||BOUNDARY||| then 1 end), 0) AS previous_failures,
    coalesce(count(case when timestamp BETWEEN |||START||| AND |||BOUNDARY||| then 1 end), 0) as previous_runs,
    coalesce(count(case when status = 1 AND timestamp BETWEEN |||BOUNDARY||| AND |||END||| then 1 end), 0) AS current_successes,
    coalesce(count(case when status = 13 AND timestamp BETWEEN |||BOUNDARY||| AND |||END||| then 1 end), 0) AS current_flakes,
    coalesce(count(case when status = 12 AND timestamp BETWEEN |||BOUNDARY||| AND |||END||| then 1 end), 0) AS current_failures,
    coalesce(count(case when timestamp BETWEEN |||BOUNDARY||| AND |||END||| then 1 end), 0) as current_runs,
    prow_jobs.variants
FROM prow_job_run_tests
    JOIN tests ON tests.id = prow_job_run_tests.test_id
    JOIN prow_job_runs ON prow_job_runs.id = prow_job_run_tests.prow_job_run_id
    JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
GROUP BY tests.name, prow_jobs.variants
`
