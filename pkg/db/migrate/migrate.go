package migrate

import (
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	mpg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/migrations"
)

const baselineVersion = 1

type logAdapter struct{}

func (l *logAdapter) Printf(format string, v ...any) {
	log.Infof(format, v...)
}

func (l *logAdapter) Verbose() bool {
	return log.IsLevelEnabled(log.DebugLevel)
}

// NewMigrateWithFS creates a migrate instance from an arbitrary fs.FS and
// optional custom migrations tracking table. Pass an empty migrationsTable
// to use the default "schema_migrations".
func NewMigrateWithFS(gormDB *gorm.DB, fsys fs.FS, migrationsTable string) (*migrate.Migrate, func() error, error) {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get *sql.DB from GORM: %w", err)
	}

	source, err := iofs.New(fsys, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create migration source: %w", err)
	}

	cfg := &mpg.Config{}
	if migrationsTable != "" {
		cfg.MigrationsTable = migrationsTable
	}

	driver, err := mpg.WithInstance(sqlDB, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	m.Log = &logAdapter{}

	cleanup := func() error {
		return source.Close()
	}

	return m, cleanup, nil
}

func newMigrate(gormDB *gorm.DB) (*migrate.Migrate, func() error, error) {
	return NewMigrateWithFS(gormDB, migrations.FS, "")
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
			m, cleanup, err := newMigrate(gormDB)
			if err != nil {
				return err
			}
			defer cleanup()
			if err := m.Force(baselineVersion); err != nil {
				return fmt.Errorf("failed to stamp baseline version: %w", err)
			}
		}
	}

	m, cleanup, err := newMigrate(gormDB)
	if err != nil {
		return err
	}
	defer cleanup()

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

// CurrentVersionWithFS returns the current migration version and dirty flag
// for the given migration source and tracking table.
func CurrentVersionWithFS(gormDB *gorm.DB, fsys fs.FS, migrationsTable string) (uint, bool, error) {
	m, cleanup, err := NewMigrateWithFS(gormDB, fsys, migrationsTable)
	if err != nil {
		return 0, false, err
	}
	defer cleanup()
	return m.Version()
}

// CurrentVersion returns the current migration version and dirty flag.
func CurrentVersion(gormDB *gorm.DB) (uint, bool, error) {
	return CurrentVersionWithFS(gormDB, migrations.FS, "")
}

// MigrateDownWithFS rolls back the given number of migration steps
// for the given migration source and tracking table.
func MigrateDownWithFS(gormDB *gorm.DB, fsys fs.FS, migrationsTable string, steps int) error {
	m, cleanup, err := NewMigrateWithFS(gormDB, fsys, migrationsTable)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := m.Steps(-steps); err != nil {
		return fmt.Errorf("migrate down failed: %w", err)
	}
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to read migration version: %w", err)
	}
	log.WithFields(log.Fields{"version": version, "dirty": dirty}).Info("migrate down complete")
	return nil
}

// MigrateDown rolls back the given number of migration steps.
func MigrateDown(gormDB *gorm.DB, steps int) error {
	return MigrateDownWithFS(gormDB, migrations.FS, "", steps)
}

// RunMigrationsWithFS runs all pending versioned migrations from the given
// migration source, tracking state in the specified table.
func RunMigrationsWithFS(gormDB *gorm.DB, fsys fs.FS, migrationsTable string) error {
	m, cleanup, err := NewMigrateWithFS(gormDB, fsys, migrationsTable)
	if err != nil {
		return err
	}
	defer cleanup()

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

// ForceVersion sets the migration version without running any migrations.
// Use this to recover from a dirty state.
func ForceVersion(gormDB *gorm.DB, version int) error {
	m, cleanup, err := newMigrate(gormDB)
	if err != nil {
		return err
	}
	defer cleanup()
	return m.Force(version)
}
