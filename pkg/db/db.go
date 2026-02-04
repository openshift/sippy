package db

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

type SchemaHashType string

const (
	hashTypeMatView      SchemaHashType = "matview"
	hashTypeView         SchemaHashType = "view"
	hashTypeMatViewIndex SchemaHashType = "matview_index"
	hashTypeFunction     SchemaHashType = "function"
)

type DB struct {
	DB *gorm.DB

	// BatchSize is used for how many insertions we should do at once. Postgres supports
	// a maximum of 2^16 records per insert.
	BatchSize int
}

// log2LogrusWriter bridges gorm logging to logrus logging.
// All messages will come through at DEBUG level.
type log2LogrusWriter struct {
	entry *log.Entry
}

func (w log2LogrusWriter) Printf(msg string, args ...interface{}) {
	w.entry.Debugf(msg, args...)
}

func New(dsn string, logLevel gormlogger.LogLevel) (*DB, error) {
	gormLogger := gormlogger.New(
		log2LogrusWriter{entry: log.WithField("source", "gorm")},
		gormlogger.Config{
			SlowThreshold:             2 * time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, err
	}
	return &DB{
		DB:        db,
		BatchSize: 1024,
	}, nil
}

func (d *DB) UpdateSchema(reportEnd *time.Time) error {
	// List of all models to migrate
	modelsToMigrate := []interface{}{
		&models.ReleaseTag{},
		&models.ReleasePullRequest{},
		&models.ReleaseRepository{},
		&models.ReleaseJobRun{},
		&models.ProwJob{},
		&models.ProwJobRun{},
		&models.Test{},
		&models.Suite{},
		&models.ProwJobRunTest{},
		&models.ProwJobRunTestOutput{},
		&models.APISnapshot{},
		&models.Bug{},
		&models.ProwPullRequest{},
		&models.SchemaHash{},
		&models.PullRequestComment{},
		&models.JiraIncident{},
		&models.JiraComponent{},
		&models.TestOwnership{},
		&models.FeatureGate{},
		&models.TestRegression{},
		&models.Triage{},
		&models.AuditLog{},
		&models.ChatRating{},
		&models.ChatConversation{},
		&jobrunscan.Label{},
		&jobrunscan.Symptom{},
	}

	// Migrate each model
	for _, model := range modelsToMigrate {
		if err := d.DB.AutoMigrate(model); err != nil {
			return err
		}
	}

	//TODO(sgoeddel): This migration logic can be removed once we have a migration that drops the view column from test_regressions
	if d.DB.Migrator().HasColumn(&models.TestRegression{}, "view") {
		if err := d.DB.Migrator().DropColumn(&models.TestRegression{}, "view"); err != nil {
			return err
		}
	}

	if err := createAuditLogIndexes(d.DB); err != nil {
		return err
	}

	if err := populateTestSuitesInDB(d.DB); err != nil {
		return err
	}

	if err := syncPostgresMaterializedViews(d.DB, reportEnd); err != nil {
		return err
	}

	if err := syncPartitionedTables(d.DB); err != nil {
		return err
	}

	if err := syncPostgresViews(d.DB, reportEnd); err != nil {
		return err
	}

	return syncPostgresFunctions(d.DB)
}

// syncSchema will update generic db resources if their schema has changed. (functions, materialized views, indexes)
// This is useful for resources that cannot be updated incrementally with goose, and can cause conflict / last write
// wins problems with concurrent development.
//
// desiredSchema should be the full SQL command we would issue to create the resource fresh. It will be hashed and
// compared to a pre-existing value in the db of the given name and type, if any exists. If none exists, or the hashes
// have changed, the resource will be recreated.
//
// dropSQL is the full SQL command we will run if we detect that the resource needs updating. It should include
// "IF EXISTS" as it will be attempted even when no previous resource exists. (i.e. new databases)
//
// This function does not check for existence of the resource in the db, thus if you ever delete something manually, it will
// not be recreated until you also delete the corresponding row from schema_hashes.
//
// returns true if schema change was detected
func syncSchema(db *gorm.DB, hashType SchemaHashType, name, desiredSchema, dropSQL string, forceUpdate bool) (bool, error) {

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
	switch {
	case currSchemaHash.ID == 0:
		vlog.Debug("no current schema hash in db, creating")
		updateRequired = true
		currSchemaHash = models.SchemaHash{
			Type: string(hashType),
			Name: name,
			Hash: hashStr,
		}
	case currSchemaHash.Hash != hashStr:
		vlog.WithField("oldHash", currSchemaHash.Hash).Debug("schema hash has changed, recreating")
		currSchemaHash.Hash = hashStr
		updateRequired = true
	case forceUpdate:
		vlog.Debug("schema hash has not changed but a force update was requested, recreating")
		updateRequired = true
	}

	if updateRequired {
		if res := db.Exec(dropSQL); res.Error != nil {
			vlog.WithError(res.Error).Error("error dropping")
			return updateRequired, res.Error
		}

		vlog.Info("creating with latest schema")

		if res := db.Exec(desiredSchema); res.Error != nil {
			log.WithError(res.Error).Error("error creating")
			return updateRequired, res.Error
		}

		if currSchemaHash.ID == 0 {
			if res := db.Create(&currSchemaHash); res.Error != nil {
				vlog.WithError(res.Error).Error("error creating schema hash")
				return updateRequired, res.Error
			}
		} else {
			if res := db.Save(&currSchemaHash); res.Error != nil {
				vlog.WithError(res.Error).Error("error updating schema hash")
				return updateRequired, res.Error
			}
		}
		vlog.Info("schema hash updated")
	} else {
		vlog.Debug("no schema update required")
	}
	return updateRequired, nil
}

// createAuditLogIndexes creates GIN indexes for JSONB columns in audit_logs table
// for efficient JSON querying operations.
func createAuditLogIndexes(db *gorm.DB) error {
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_new_data_gin ON audit_logs USING GIN (new_data)").Error; err != nil {
		return fmt.Errorf("failed to create GIN index on audit_logs.new_data: %w", err)
	}

	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_old_data_gin ON audit_logs USING GIN (old_data)").Error; err != nil {
		return fmt.Errorf("failed to create GIN index on audit_logs.old_data: %w", err)
	}

	return nil
}

func ParseGormLogLevel(logLevel string) (gormlogger.LogLevel, error) {
	switch logLevel {
	case "info":
		return gormlogger.Info, nil
	case "warn":
		return gormlogger.Warn, nil
	case "error":
		return gormlogger.Error, nil
	case "silent":
		return gormlogger.Silent, nil
	default:
		return gormlogger.Info, fmt.Errorf("Unknown gorm LogLevel: %s", logLevel)
	}
}
