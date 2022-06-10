package db

import (
	"fmt"

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

	if err := syncPostgresMaterializedViews(db); err != nil {
		return nil, err
	}

	if err := syncPostgresFunctions(db); err != nil {
		return nil, err
	}

	if true {
		return nil, fmt.Errorf("bailing out during dev")
	}

	return &DB{
		DB:        db,
		BatchSize: 1024,
	}, nil
}
