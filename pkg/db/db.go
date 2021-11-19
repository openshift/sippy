package db

import (
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	DB *gorm.DB
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

	if err := db.AutoMigrate(&v1.JobRunResult{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&v1.JobResult{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&v1.TestResult{}); err != nil {
		return nil, err
	}

	return &DB{
		DB: db,
	}, nil
}
