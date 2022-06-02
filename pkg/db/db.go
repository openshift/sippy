package db

import (
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

	return &DB{
		DB:        db,
		BatchSize: 1024,
	}, nil
}

var PostgresMatViews = []string{
	"prow_test_report_7d_matview",
	"prow_test_analysis_by_variant_14d_matview",
	"prow_test_analysis_by_job_14d_matview",
	"prow_test_report_2d_matview",
	"prow_job_runs_report_matview",
	"prow_job_failed_tests_by_day_matview",
	"prow_job_failed_tests_by_hour_matview",
}
