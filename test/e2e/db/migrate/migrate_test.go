package migrate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/db/migrate"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/db/migrate/testmigrations"
	"github.com/openshift/sippy/test/e2e/util"
)

// TestMigrations verifies that database migrations run successfully
// and that the database connection remains usable after migration.
// This test specifically validates the fix for the issue where m.Close()
// was closing the shared gormDB connection.
func TestMigrations(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)

	t.Run("RunMigrations preserves database connection", func(t *testing.T) {
		// Run migrations
		err := migrate.RunMigrations(dbc.DB)
		require.NoError(t, err, "migrations should complete successfully")

		// Verify the database connection is still usable after migration
		// This would fail if m.Close() closed the underlying *sql.DB
		var count int64
		err = dbc.DB.Model(&models.SchemaHash{}).Count(&count).Error
		require.NoError(t, err, "database connection should still be usable after migration")

		// Verify we can read from the database
		var schemaHashes []models.SchemaHash
		err = dbc.DB.Find(&schemaHashes).Error
		require.NoError(t, err, "should be able to query database after migration")
	})

	t.Run("CurrentVersion returns valid version", func(t *testing.T) {
		// Get current migration version
		version, dirty, err := migrate.CurrentVersion(dbc.DB)
		require.NoError(t, err, "should be able to get current version")
		assert.False(t, dirty, "migration should not be in dirty state")
		assert.Greater(t, version, uint(0), "version should be greater than 0")

		// Verify the database connection is still usable after CurrentVersion
		var count int64
		err = dbc.DB.Model(&models.SchemaHash{}).Count(&count).Error
		require.NoError(t, err, "database connection should still be usable after CurrentVersion")
	})

	t.Run("ForceVersion preserves database connection", func(t *testing.T) {
		// Get current version first
		currentVersion, _, err := migrate.CurrentVersion(dbc.DB)
		require.NoError(t, err)

		// Force to the same version (no-op but exercises the code path)
		err = migrate.ForceVersion(dbc.DB, int(currentVersion)) //nolint:gosec // version fits in int
		require.NoError(t, err, "ForceVersion should succeed")

		// Verify the database connection is still usable after ForceVersion
		var count int64
		err = dbc.DB.Model(&models.SchemaHash{}).Count(&count).Error
		require.NoError(t, err, "database connection should still be usable after ForceVersion")

		// Verify version is still correct
		version, dirty, err := migrate.CurrentVersion(dbc.DB)
		require.NoError(t, err)
		assert.Equal(t, currentVersion, version, "version should be unchanged")
		assert.False(t, dirty, "migration should not be in dirty state")
	})

	t.Run("schema_migrations table exists", func(t *testing.T) {
		// Verify the schema_migrations table was created by golang-migrate
		var exists bool
		err := dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'schema_migrations')",
		).Scan(&exists).Error
		require.NoError(t, err)
		assert.True(t, exists, "schema_migrations table should exist after migration")

		// Verify we can query the schema_migrations table
		var version uint
		err = dbc.DB.Raw("SELECT version FROM schema_migrations LIMIT 1").Scan(&version).Error
		require.NoError(t, err)
		assert.Greater(t, version, uint(0), "should have at least one migration version")
	})

	t.Run("MigrateDown with isolated test migrations", func(t *testing.T) {
		const trackingTable = "e2e_schema_migrations"
		fs := testmigrations.FS

		t.Cleanup(func() {
			dbc.DB.Exec("DROP TABLE IF EXISTS e2e_test_table")
			dbc.DB.Exec("DROP TABLE IF EXISTS " + trackingTable)
		})

		// Migrate up to version 2: creates table then adds column
		err := migrate.RunMigrationsWithFS(dbc.DB, fs, trackingTable)
		require.NoError(t, err, "RunMigrationsWithFS should succeed")

		version, dirty, err := migrate.CurrentVersionWithFS(dbc.DB, fs, trackingTable)
		require.NoError(t, err)
		assert.False(t, dirty)
		assert.Equal(t, uint(2), version)

		var exists bool
		err = dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'e2e_test_table')",
		).Scan(&exists).Error
		require.NoError(t, err)
		require.True(t, exists, "e2e_test_table should exist after up migration")

		var hasColumn bool
		err = dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'e2e_test_table' AND column_name = 'description')",
		).Scan(&hasColumn).Error
		require.NoError(t, err)
		require.True(t, hasColumn, "description column should exist at version 2")

		// Step down to version 1: column dropped, table remains
		err = migrate.MigrateDownWithFS(dbc.DB, fs, trackingTable, 1)
		require.NoError(t, err, "MigrateDownWithFS step 1 should succeed")

		version, dirty, err = migrate.CurrentVersionWithFS(dbc.DB, fs, trackingTable)
		require.NoError(t, err)
		assert.False(t, dirty)
		assert.Equal(t, uint(1), version)

		err = dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'e2e_test_table' AND column_name = 'description')",
		).Scan(&hasColumn).Error
		require.NoError(t, err)
		assert.False(t, hasColumn, "description column should be gone at version 1")

		err = dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'e2e_test_table')",
		).Scan(&exists).Error
		require.NoError(t, err)
		assert.True(t, exists, "e2e_test_table should still exist at version 1")

		// Step down to version 0: table dropped
		err = migrate.MigrateDownWithFS(dbc.DB, fs, trackingTable, 1)
		require.NoError(t, err, "MigrateDownWithFS step 2 should succeed")

		err = dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'e2e_test_table')",
		).Scan(&exists).Error
		require.NoError(t, err)
		assert.False(t, exists, "e2e_test_table should be gone after full rollback")

		// Verify production connection is unaffected
		var count int64
		err = dbc.DB.Model(&models.SchemaHash{}).Count(&count).Error
		require.NoError(t, err, "database connection should still be usable after test migrations")
	})

	t.Run("multiple migration operations preserve connection", func(t *testing.T) {
		// Run multiple migration operations in sequence to ensure
		// none of them close the shared connection

		// First operation
		version1, dirty1, err := migrate.CurrentVersion(dbc.DB)
		require.NoError(t, err)
		assert.False(t, dirty1)

		// Second operation
		version2, dirty2, err := migrate.CurrentVersion(dbc.DB)
		require.NoError(t, err)
		assert.False(t, dirty2)
		assert.Equal(t, version1, version2)

		// Third operation - verify we can still run migrations
		err = migrate.RunMigrations(dbc.DB)
		require.NoError(t, err)

		// Final verification - database connection should still work
		var count int64
		err = dbc.DB.Model(&models.SchemaHash{}).Count(&count).Error
		require.NoError(t, err, "database connection should still work after multiple migration operations")
	})
}
