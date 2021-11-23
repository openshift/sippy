package db

import (
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
