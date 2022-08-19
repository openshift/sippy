package db

import (
	"crypto/sha256"
	"encoding/base64"

	log "github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/openshift/sippy/pkg/db/models"
)

type SchemaHashType string

const (
	hashTypeMatView  SchemaHashType = "matview"
	hashTypeFunction SchemaHashType = "function"
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

	if err := db.AutoMigrate(&models.ReleasePullRequest{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.ReleaseRepository{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.ReleaseJobRun{}); err != nil {
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

	if err := db.AutoMigrate(&models.Suite{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.ProwJobRunTest{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.ProwJobRunTestOutput{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.Bug{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.ProwPullRequest{}); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.SchemaHash{}); err != nil {
		return nil, err
	}

	// TODO: in the future, we should add an implied migration. If we see a new suite needs to be created,
	// scan all test names for any starting with that prefix, and if found merge all records into a new or modified test
	// with the prefix stripped. This is not necessary today, but in future as new suites are added, there'll be a good
	// change this happens without thinking to update sippy.
	if err := populateTestSuitesInDB(db); err != nil {
		return nil, err
	}

	if err := syncPostgresMaterializedViews(db); err != nil {
		return nil, err
	}

	if err := syncPostgresFunctions(db); err != nil {
		return nil, err
	}

	return &DB{
		DB:        db,
		BatchSize: 1024,
	}, nil
}

// syncSchema will update generic db resources if their schema has changed. (functions, materialized views, indexes)
// This is useful for resources that cannot be updated incrementally with goose, and can cause conflict / last write
// wins problems with concurrent development.
//
// desiredSchema should be the full SQL command we would issue to create the resource fresh. It will be hashed and
//   compared to a pre-existing value in the db of the given name and type, if any exists. If none exists, or the hashes
//   have changed, the resource will be recreated.
// dropSQL is the full SQL command we will run if we detect that the resource needs updating. It should include
//   "IF EXISTS" as it will be attempted even when no previous resource exists. (i.e. new databases)
// indexes is a pointer to a SQL command we will run if present to create the desired indexes. This only happens if we detect
//   the desiredSchema needed an update.
// dropIndexes is a pointer to a SQL command we will run to drop indexes prior to creating them. This only happens if we detect
//   the desiredSchema needed an update.
//
// This function does not check for existence of the resource in the db, thus if you ever delete something manually, it will
// not be recreated until you also delete the corresponding row from schema_hashes.
func syncSchema(db *gorm.DB, hashType SchemaHashType, name, desiredSchema, dropSQL string, indexes, dropIndexes *string) error {

	// Calculate hash of our schema to see if anything has changed.
	hash := sha256.Sum256([]byte(desiredSchema))
	hashStr := base64.URLEncoding.EncodeToString(hash[:])
	vlog := log.WithFields(log.Fields{"name": name, "type": hashType})
	vlog.WithField("hash", hashStr).Debug("generated SHA256 hash")

	currSchemaHash := models.SchemaHash{}
	res := db.Where("type = ? AND name = ?", hashType, name).Find(&currSchemaHash)
	if res.Error != nil {
		vlog.WithError(res.Error).Error("error looking up schema hash")
	}

	var updateRequired bool
	if currSchemaHash.ID == 0 {
		vlog.Debug("no current schema hash in db, creating")
		updateRequired = true
		currSchemaHash = models.SchemaHash{
			Type: string(hashType),
			Name: name,
			Hash: hashStr,
		}
	} else if currSchemaHash.Hash != hashStr {
		vlog.WithField("oldHash", currSchemaHash.Hash).Debug("schema hash has has changed, recreating")
		currSchemaHash.Hash = hashStr
		updateRequired = true
	}

	if updateRequired {
		if res := db.Exec(dropSQL); res.Error != nil {
			vlog.WithError(res.Error).Error("error dropping")
			return res.Error
		}

		vlog.Info("creating with latest schema")

		if res := db.Exec(desiredSchema); res.Error != nil {
			log.WithError(res.Error).Error("error creating")
			return res.Error
		}

		if currSchemaHash.ID == 0 {
			if res := db.Create(&currSchemaHash); res.Error != nil {
				vlog.WithError(res.Error).Error("error creating schema hash")
				return res.Error
			}
		} else {
			if res := db.Save(&currSchemaHash); res.Error != nil {
				vlog.WithError(res.Error).Error("error updating schema hash")
				return res.Error
			}
		}
		vlog.Info("schema hash updated")

		if dropIndexes != nil {
			vlog.Info("dropping indexes")
			if res := db.Exec(*dropIndexes); res.Error != nil {
				vlog.WithError(res.Error).Error("error dropping indexes")
				return res.Error
			}
		}

		if indexes != nil {
			vlog.Info("creating indexes")
			if res := db.Exec(*indexes); res.Error != nil {
				log.WithError(res.Error).Error("error creating indexes")
				return res.Error
			}
		}
	} else {
		vlog.Debug("no schema update required")
	}
	return nil
}
