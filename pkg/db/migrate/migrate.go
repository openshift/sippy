package migrate

import (
	"database/sql"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	mpg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/migrations"
)

const baselineVersion = 1

type logAdapter struct{}

func (l *logAdapter) Printf(format string, v ...interface{}) {
	log.Infof(format, v...)
}

func (l *logAdapter) Verbose() bool {
	return log.IsLevelEnabled(log.DebugLevel)
}

func newMigrate(gormDB *gorm.DB) (*migrate.Migrate, error) {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get *sql.DB from GORM: %w", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to create migration source: %w", err)
	}

	driver, err := mpg.WithInstance(sqlDB, &mpg.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	m.Log = &logAdapter{}
	return m, nil
}

func tableExists(sqlDB *sql.DB, table string) (bool, error) {
	var exists bool
	err := sqlDB.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table).
		Scan(&exists)
	return exists, err
}

// RunMigrations runs all pending versioned migrations. On existing databases
// that predate golang-migrate, it detects the baseline and stamps the version
// without re-running DDL.
func RunMigrations(gormDB *gorm.DB) error {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get *sql.DB from GORM: %w", err)
	}

	hasSchemaMigrations, err := tableExists(sqlDB, "schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to check for schema_migrations table: %w", err)
	}

	if !hasSchemaMigrations {
		hasExistingTable, err := tableExists(sqlDB, "test_analysis_by_job_by_dates")
		if err != nil {
			return fmt.Errorf("failed to check for test_analysis_by_job_by_dates table: %w", err)
		}

		if hasExistingTable {
			log.Info("existing database detected, stamping baseline migration version")
			m, err := newMigrate(gormDB)
			if err != nil {
				return err
			}
			if err := m.Force(baselineVersion); err != nil {
				return fmt.Errorf("failed to stamp baseline version: %w", err)
			}
			srcErr, dbErr := m.Close()
			if srcErr != nil {
				return srcErr
			}
			if dbErr != nil {
				return dbErr
			}
		}
	}

	m, err := newMigrate(gormDB)
	if err != nil {
		return err
	}
	defer func() {
		m.Close()
	}()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to read migration version: %w", err)
	}
	log.WithFields(log.Fields{"version": version, "dirty": dirty}).Info("migrations complete")
	return nil
}

// CurrentVersion returns the current migration version and dirty flag.
func CurrentVersion(gormDB *gorm.DB) (uint, bool, error) {
	m, err := newMigrate(gormDB)
	if err != nil {
		return 0, false, err
	}
	defer func() {
		m.Close()
	}()
	return m.Version()
}

// ForceVersion sets the migration version without running any migrations.
// Use this to recover from a dirty state.
func ForceVersion(gormDB *gorm.DB, version int) error {
	m, err := newMigrate(gormDB)
	if err != nil {
		return err
	}
	defer func() {
		m.Close()
	}()
	return m.Force(version)
}
