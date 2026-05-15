package migrate_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/db/migrate"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/partitionmanager"
	"github.com/openshift/sippy/test/e2e/db/migrate/testmigrations"
	"github.com/openshift/sippy/test/e2e/util"
)

// TestMigrations verifies that database migrations run successfully.
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

func TestPartitionManager(t *testing.T) {
	const testTable = "e2e_partitioned_test"

	dbc := util.CreateE2EPostgresConnection(t)

	t.Cleanup(func() {
		dbc.DB.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s" CASCADE`, testTable))
		dbc.DB.Exec("DROP TABLE IF EXISTS partman.partitions CASCADE")
		dbc.DB.Exec("DROP TABLE IF EXISTS partman.tenants CASCADE")
		dbc.DB.Exec("DROP TABLE IF EXISTS partman.parent_tables CASCADE")
		dbc.DB.Exec("DROP SCHEMA IF EXISTS partman CASCADE")
	})

	// Create a test partitioned table
	err := dbc.DB.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (
		date timestamp with time zone,
		value text
	) PARTITION BY RANGE (date)`, testTable)).Error
	require.NoError(t, err, "should create test partitioned table")

	t.Run("EnsurePartition creates and is idempotent", func(t *testing.T) {
		date := "2025-06-01"
		nextDay := "2025-06-02"

		err := partitionmanager.EnsurePartition(dbc.DB, testTable, date, nextDay)
		require.NoError(t, err, "should create partition")

		// Verify partition exists
		var exists bool
		err = dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM pg_tables WHERE tablename = ?)",
			fmt.Sprintf("%s_20250601", testTable),
		).Scan(&exists).Error
		require.NoError(t, err)
		assert.True(t, exists, "partition table should exist")

		// Calling again should be idempotent
		err = partitionmanager.EnsurePartition(dbc.DB, testTable, date, nextDay)
		require.NoError(t, err, "second EnsurePartition call should succeed (idempotent)")
	})

	t.Run("write and read rows through partitioned table", func(t *testing.T) {
		date := "2025-06-01"
		nextDay := "2025-06-02"

		err := partitionmanager.EnsurePartition(dbc.DB, testTable, date, nextDay)
		require.NoError(t, err)

		// Insert a row — it should be routed to the partition automatically
		ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
		err = dbc.DB.Exec(
			fmt.Sprintf(`INSERT INTO "%s" (date, value) VALUES (?, ?)`, testTable),
			ts, "hello from partition",
		).Error
		require.NoError(t, err, "should insert into partitioned table")

		// Read it back via the parent table
		var value string
		err = dbc.DB.Raw(
			fmt.Sprintf(`SELECT value FROM "%s" WHERE date = ?`, testTable), ts,
		).Scan(&value).Error
		require.NoError(t, err, "should read row from partitioned table")
		assert.Equal(t, "hello from partition", value)

		// Verify the row physically lives in the partition, not just the parent
		partName := fmt.Sprintf("%s_20250601", testTable)
		var count int64
		err = dbc.DB.Raw(
			fmt.Sprintf(`SELECT count(*) FROM "%s"`, partName),
		).Scan(&count).Error
		require.NoError(t, err)
		assert.Equal(t, int64(1), count, "row should be in the partition table")

		// Insert a second row on a different date — requires its own partition
		err = partitionmanager.EnsurePartition(dbc.DB, testTable, nextDay, "2025-06-03")
		require.NoError(t, err)

		ts2 := time.Date(2025, 6, 2, 8, 30, 0, 0, time.UTC)
		err = dbc.DB.Exec(
			fmt.Sprintf(`INSERT INTO "%s" (date, value) VALUES (?, ?)`, testTable),
			ts2, "second day",
		).Error
		require.NoError(t, err, "should insert into second partition")

		// Both rows visible from the parent table
		var total int64
		err = dbc.DB.Raw(
			fmt.Sprintf(`SELECT count(*) FROM "%s"`, testTable),
		).Scan(&total).Error
		require.NoError(t, err)
		assert.Equal(t, int64(2), total, "parent table should see rows across partitions")
	})

	t.Run("EnsurePartition rejects invalid table names", func(t *testing.T) {
		err := partitionmanager.EnsurePartition(dbc.DB, "robert'; DROP TABLE students;--", "2025-06-01", "2025-06-02")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid table name")
	})

	t.Run("EnsurePartition rejects invalid dates", func(t *testing.T) {
		err := partitionmanager.EnsurePartition(dbc.DB, testTable, "not-a-date", "2025-06-02")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid date")
	})

	t.Run("Maintain creates future partitions", func(t *testing.T) {
		ctx := context.Background()
		partitionCount := uint(3)

		// Clean up unmanaged partitions and partman metadata left by
		// prior subtests so this manager starts with a clean slate.
		dbc.DB.Exec("DROP SCHEMA IF EXISTS partman CASCADE")
		var childTables []string
		dbc.DB.Raw(
			"SELECT inhrelid::regclass::text FROM pg_inherits WHERE inhparent = ?::regclass",
			testTable,
		).Scan(&childTables)
		for _, child := range childTables {
			dbc.DB.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, child))
		}

		pm, err := partitionmanager.New(dbc.DB, time.Hour, []partitionmanager.TableConfig{
			{
				Name:              testTable,
				Schema:            "public",
				PartitionColumn:   "date",
				PartitionInterval: 24 * time.Hour,
				PartitionCount:    partitionCount,
				RetentionPeriod:   365 * 24 * time.Hour,
			},
		})
		require.NoError(t, err, "should create partition manager")

		err = pm.Maintain(ctx)
		require.NoError(t, err, "Maintain should succeed")

		// Verify future partitions were created
		today := time.Now().UTC().Truncate(24 * time.Hour)
		for i := uint(0); i < partitionCount; i++ {
			d := today.Add(time.Duration(i) * 24 * time.Hour)
			partName := fmt.Sprintf("%s_%s", testTable, d.Format("20060102"))
			var exists bool
			err = dbc.DB.Raw(
				"SELECT EXISTS(SELECT 1 FROM pg_tables WHERE tablename = ?)", partName,
			).Scan(&exists).Error
			require.NoError(t, err)
			assert.True(t, exists, "future partition %s should exist", partName)
		}
	})

	t.Run("Maintain drops partitions beyond retention", func(t *testing.T) {
		ctx := context.Background()

		// Reset: drop all child partitions and partman metadata left by
		// prior subtests so this manager starts with a clean slate.
		dbc.DB.Exec("DROP SCHEMA IF EXISTS partman CASCADE")
		var childTables []string
		dbc.DB.Raw(
			"SELECT inhrelid::regclass::text FROM pg_inherits WHERE inhparent = ?::regclass",
			testTable,
		).Scan(&childTables)
		for _, child := range childTables {
			dbc.DB.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, child))
		}

		// Create a partition well in the past (3 days ago)
		oldDate := time.Now().UTC().Add(-3 * 24 * time.Hour).Truncate(24 * time.Hour)
		oldDateStr := oldDate.Format("2006-01-02")
		oldNextDay := oldDate.Add(24 * time.Hour).Format("2006-01-02")

		err := partitionmanager.EnsurePartition(dbc.DB, testTable, oldDateStr, oldNextDay)
		require.NoError(t, err, "should create old partition")

		oldPartName := fmt.Sprintf("%s_%s", testTable, oldDate.Format("20060102"))
		var exists bool
		err = dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM pg_tables WHERE tablename = ?)", oldPartName,
		).Scan(&exists).Error
		require.NoError(t, err)
		require.True(t, exists, "old partition should exist before Maintain")

		// Create manager with 1-hour retention so the old partition is beyond cutoff
		pm, err := partitionmanager.New(dbc.DB, time.Hour, []partitionmanager.TableConfig{
			{
				Name:              testTable,
				Schema:            "public",
				PartitionColumn:   "date",
				PartitionInterval: 24 * time.Hour,
				PartitionCount:    3,
				RetentionPeriod:   1 * time.Hour,
			},
		})
		require.NoError(t, err, "should create partition manager with short retention")

		err = pm.Maintain(ctx)
		require.NoError(t, err, "Maintain should succeed")

		// The old partition (3 days ago) should have been dropped
		err = dbc.DB.Raw(
			"SELECT EXISTS(SELECT 1 FROM pg_tables WHERE tablename = ?)", oldPartName,
		).Scan(&exists).Error
		require.NoError(t, err)
		assert.False(t, exists, "old partition %s should be dropped by retention policy", oldPartName)
	})
}
