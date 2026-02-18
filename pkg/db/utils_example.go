package db

import (
	"time"

	log "github.com/sirupsen/logrus"
)

// ExampleVerifyTablesHaveSameColumns demonstrates how to verify that two tables have identical columns
//
// This is useful for:
// - Verifying partition tables match the parent table structure
// - Ensuring schema consistency before data migration
// - Validating table clones or backups
//
// Usage:
//
//	err := dbc.VerifyTablesHaveSameColumns("source_table", "target_table", DefaultColumnVerificationOptions())
//	if err != nil {
//	    log.WithError(err).Error("tables have different schemas")
//	}
func ExampleVerifyTablesHaveSameColumns(dbc *DB, sourceTable, targetTable string) {
	log.WithFields(log.Fields{
		"source": sourceTable,
		"target": targetTable,
	}).Info("verifying tables have identical columns")

	// Use default options to verify all aspects: names, types, nullable, defaults, and order
	err := dbc.VerifyTablesHaveSameColumns(sourceTable, targetTable, DefaultColumnVerificationOptions())
	if err != nil {
		log.WithError(err).Error("table schema verification failed")
		return
	}

	log.Info("tables have identical column definitions")
}

// ExampleVerifyPartitionMatchesParent demonstrates verifying a partition matches its parent table
//
// This is particularly useful when:
// - Creating new partitions
// - Reattaching detached partitions
// - Validating partition structure after schema changes
//
// Usage:
//
//	parentTable := "test_analysis_by_job_by_dates"
//	partition := "test_analysis_by_job_by_dates_2024_01_15"
//	ExampleVerifyPartitionMatchesParent(dbc, parentTable, partition)
func ExampleVerifyPartitionMatchesParent(dbc *DB, parentTable, partition string) {
	log.WithFields(log.Fields{
		"parent":    parentTable,
		"partition": partition,
	}).Info("verifying partition matches parent table structure")

	// Use default options to ensure partition fully matches parent table
	err := dbc.VerifyTablesHaveSameColumns(parentTable, partition, DefaultColumnVerificationOptions())
	if err != nil {
		log.WithError(err).Error("partition schema does not match parent table")
		log.Error("this partition may have been created with an old schema or manually modified")
		return
	}

	log.Info("partition structure matches parent table - safe to attach")
}

// ExampleVerifyBeforeMigration demonstrates verification before data migration
//
// Before migrating data from one table to another, it's critical to ensure
// the schemas match to avoid data loss or type conversion errors.
//
// Usage:
//
//	ExampleVerifyBeforeMigration(dbc, "old_table", "new_table")
func ExampleVerifyBeforeMigration(dbc *DB, sourceTable, targetTable string) {
	log.Info("preparing data migration")

	// Step 1: Verify schemas match
	// For data migration, we only need column names and types to match
	// Nullable and default constraints don't affect the data copy
	err := dbc.VerifyTablesHaveSameColumns(sourceTable, targetTable, DataMigrationColumnVerificationOptions())
	if err != nil {
		log.WithError(err).Error("cannot migrate: schema mismatch detected")
		log.Error("resolve schema differences before proceeding with migration")
		return
	}

	log.Info("schema verification passed - safe to proceed with migration")

	// Step 2: Proceed with migration
	// (migration code would go here)
}

// ExampleVerifyMultipleTables demonstrates checking multiple tables against a reference
//
// This is useful for:
// - Verifying all partitions match the parent table
// - Checking multiple replicas or shards have identical schemas
// - Validating a set of tables after schema updates
//
// Usage:
//
//	ExampleVerifyMultipleTables(dbc, "parent_table", []string{"partition_1", "partition_2", "partition_3"})
func ExampleVerifyMultipleTables(dbc *DB, referenceTable string, tablesToCheck []string) {
	log.WithFields(log.Fields{
		"reference": referenceTable,
		"count":     len(tablesToCheck),
	}).Info("verifying multiple tables against reference")

	var failures []string
	for _, table := range tablesToCheck {
		// Use default options to fully verify schema consistency
		err := dbc.VerifyTablesHaveSameColumns(referenceTable, table, DefaultColumnVerificationOptions())
		if err != nil {
			log.WithError(err).WithField("table", table).Error("schema mismatch detected")
			failures = append(failures, table)
		} else {
			log.WithField("table", table).Debug("schema matches reference")
		}
	}

	if len(failures) > 0 {
		log.WithFields(log.Fields{
			"total":    len(tablesToCheck),
			"failures": len(failures),
			"failed":   failures,
		}).Error("schema verification completed with failures")
	} else {
		log.WithField("count", len(tablesToCheck)).Info("all tables match reference schema")
	}
}

// ExampleMigrateTableData demonstrates basic table data migration
//
// This function:
// - Verifies schemas match before migration
// - Copies all data from source to target
// - Supports dry-run mode for safety
// - Verifies migration success
//
// Usage:
//
//	rowsMigrated, err := dbc.MigrateTableData("old_table", "new_table", false)
func ExampleMigrateTableData(dbc *DB, sourceTable, targetTable string) {
	log.WithFields(log.Fields{
		"source": sourceTable,
		"target": targetTable,
	}).Info("preparing table migration")

	// Step 1: Dry run first to verify and preview
	log.Info("performing dry run")
	_, err := dbc.MigrateTableData(sourceTable, targetTable, true)
	if err != nil {
		log.WithError(err).Error("dry run failed - cannot proceed with migration")
		return
	}

	log.Info("dry run successful - proceeding with actual migration")

	// Step 2: Perform actual migration
	rowsMigrated, err := dbc.MigrateTableData(sourceTable, targetTable, false)
	if err != nil {
		log.WithError(err).Error("migration failed")
		return
	}

	log.WithField("rows", rowsMigrated).Info("migration completed successfully")
}

// ExampleMigratePartitionData demonstrates migrating data from a detached partition to a new table
//
// Use case: You have a detached partition with old data that needs to be migrated
// to a new table structure or archive table.
//
// Usage:
//
//	ExampleMigratePartitionData(dbc, "test_table_2024_01_15", "archive_table")
func ExampleMigratePartitionData(dbc *DB, detachedPartition, archiveTable string) {
	log.WithFields(log.Fields{
		"partition": detachedPartition,
		"archive":   archiveTable,
	}).Info("migrating detached partition to archive")

	// Verify the partition is actually detached (optional safety check)
	// This would use functions from pkg/db/partitions if available

	// Migrate the data
	rowsMigrated, err := dbc.MigrateTableData(detachedPartition, archiveTable, false)
	if err != nil {
		log.WithError(err).Error("partition migration failed")
		return
	}

	log.WithFields(log.Fields{
		"partition": detachedPartition,
		"archive":   archiveTable,
		"rows":      rowsMigrated,
	}).Info("partition data migrated to archive - safe to drop partition")
}

// ExampleMigrateWithBackup demonstrates migrating data with a backup strategy
//
// Best practice: Create a backup before migration in case something goes wrong
//
// Usage:
//
//	ExampleMigrateWithBackup(dbc, "source_table", "target_table", "backup_table")
func ExampleMigrateWithBackup(dbc *DB, sourceTable, targetTable, backupTable string) {
	log.Info("migration with backup strategy")

	// Step 1: Create backup of target table
	log.WithField("backup", backupTable).Info("creating backup of target table")
	_, err := dbc.MigrateTableData(targetTable, backupTable, false)
	if err != nil {
		log.WithError(err).Error("backup creation failed - aborting migration")
		return
	}

	log.Info("backup created successfully")

	// Step 2: Perform migration
	log.Info("performing migration")
	rowsMigrated, err := dbc.MigrateTableData(sourceTable, targetTable, false)
	if err != nil {
		log.WithError(err).Error("migration failed - restore from backup if needed")
		log.WithField("backup", backupTable).Info("backup table is available for restoration")
		return
	}

	log.WithField("rows", rowsMigrated).Info("migration completed successfully")
	log.WithField("backup", backupTable).Info("backup table can be dropped if no longer needed")
}

// ExampleBatchMigratePartitions demonstrates migrating multiple partitions
//
// Use case: You have multiple detached partitions that need to be migrated
// to an archive table or consolidated into a single table.
//
// Usage:
//
//	partitions := []string{"table_2024_01_15", "table_2024_01_16", "table_2024_01_17"}
//	ExampleBatchMigratePartitions(dbc, partitions, "archive_table")
func ExampleBatchMigratePartitions(dbc *DB, partitions []string, targetTable string) {
	log.WithFields(log.Fields{
		"partitions": len(partitions),
		"target":     targetTable,
	}).Info("batch migrating partitions")

	var totalRows int64
	var successCount int
	var failures []string

	for _, partition := range partitions {
		log.WithField("partition", partition).Info("migrating partition")

		rows, err := dbc.MigrateTableData(partition, targetTable, false)
		if err != nil {
			log.WithError(err).WithField("partition", partition).Error("partition migration failed")
			failures = append(failures, partition)
			continue
		}

		totalRows += rows
		successCount++
		log.WithFields(log.Fields{
			"partition": partition,
			"rows":      rows,
		}).Info("partition migrated successfully")
	}

	log.WithFields(log.Fields{
		"total_partitions": len(partitions),
		"successful":       successCount,
		"failed":           len(failures),
		"total_rows":       totalRows,
	}).Info("batch migration completed")

	if len(failures) > 0 {
		log.WithField("failed_partitions", failures).Warn("some partitions failed to migrate")
	}
}

// ExampleMigrateAndVerify demonstrates migration with comprehensive verification
//
// This example shows best practices for production migrations:
// - Dry run first
// - Verify schemas
// - Perform migration
// - Verify row counts
// - Log all steps
//
// Usage:
//
//	ExampleMigrateAndVerify(dbc, "source_table", "target_table")
func ExampleMigrateAndVerify(dbc *DB, sourceTable, targetTable string) {
	log.Info("production migration workflow")

	// Step 1: Verify schemas match
	log.Info("step 1: verifying schema compatibility")
	// For migration, we only need column names and types to match
	if err := dbc.VerifyTablesHaveSameColumns(sourceTable, targetTable, DataMigrationColumnVerificationOptions()); err != nil {
		log.WithError(err).Error("schema verification failed")
		return
	}
	log.Info("schema verification passed")

	// Step 2: Get pre-migration counts
	log.Info("step 2: getting pre-migration row counts")
	sourceCount, err := dbc.GetTableRowCount(sourceTable)
	if err != nil {
		log.WithError(err).Error("failed to get source count")
		return
	}
	targetCountBefore, err := dbc.GetTableRowCount(targetTable)
	if err != nil {
		log.WithError(err).Error("failed to get target count")
		return
	}

	log.WithFields(log.Fields{
		"source_rows": sourceCount,
		"target_rows": targetCountBefore,
	}).Info("pre-migration row counts")

	// Step 3: Dry run
	log.Info("step 3: performing dry run")
	_, err = dbc.MigrateTableData(sourceTable, targetTable, true)
	if err != nil {
		log.WithError(err).Error("dry run failed")
		return
	}
	log.Info("dry run successful")

	// Step 4: Actual migration
	log.Info("step 4: performing actual migration")
	rowsMigrated, err := dbc.MigrateTableData(sourceTable, targetTable, false)
	if err != nil {
		log.WithError(err).Error("migration failed")
		return
	}

	// Step 5: Verify results
	log.Info("step 5: verifying migration results")
	targetCountAfter, err := dbc.GetTableRowCount(targetTable)
	if err != nil {
		log.WithError(err).Error("failed to verify final count")
		return
	}

	expectedCount := targetCountBefore + sourceCount
	if targetCountAfter != expectedCount {
		log.WithFields(log.Fields{
			"expected": expectedCount,
			"actual":   targetCountAfter,
		}).Error("row count mismatch detected!")
		return
	}

	log.WithFields(log.Fields{
		"source_table":  sourceTable,
		"target_table":  targetTable,
		"rows_migrated": rowsMigrated,
		"target_before": targetCountBefore,
		"target_after":  targetCountAfter,
		"verification":  "passed",
	}).Info("migration completed and verified successfully")
}

// ExampleSyncIdentityColumn demonstrates synchronizing an IDENTITY column sequence
//
// This is useful after migrating data to a table with IDENTITY columns,
// ensuring the sequence starts at the correct value.
//
// Usage:
//
//	ExampleSyncIdentityColumn(dbc, "my_table", "id")
func ExampleSyncIdentityColumn(dbc *DB, tableName, columnName string) {
	log.WithFields(log.Fields{
		"table":  tableName,
		"column": columnName,
	}).Info("synchronizing identity column")

	// Sync the identity sequence to match the current max value
	err := dbc.SyncIdentityColumn(tableName, columnName)
	if err != nil {
		log.WithError(err).Error("failed to sync identity column")
		return
	}

	log.Info("identity column synchronized successfully")
}

// ExampleMigrateToPartitionedTable demonstrates the complete workflow for
// migrating from a non-partitioned table to a partitioned table
//
// Usage:
//
//	ExampleMigrateToPartitionedTable(dbc, "orders", "orders_partitioned")
func ExampleMigrateToPartitionedTable(dbc *DB, sourceTable, partitionedTable string) {
	log.Info("Complete workflow: Migrating to partitioned table")

	// Assume partitioned table was created using CreatePartitionedTable
	// and partitions were created using CreateMissingPartitions

	// Step 1: Migrate the data
	log.Info("Step 1: Migrating data")
	rows, err := dbc.MigrateTableData(sourceTable, partitionedTable, false)
	if err != nil {
		log.WithError(err).Error("data migration failed")
		return
	}

	log.WithField("rows", rows).Info("data migrated successfully")

	// Step 2: Sync the IDENTITY column
	log.Info("Step 2: Synchronizing IDENTITY sequence")
	err = dbc.SyncIdentityColumn(partitionedTable, "id")
	if err != nil {
		log.WithError(err).Error("failed to sync identity column")
		return
	}

	// Step 3: Verify row counts match
	log.Info("Step 3: Verifying row counts")
	sourceCount, _ := dbc.GetTableRowCount(sourceTable)
	targetCount, _ := dbc.GetTableRowCount(partitionedTable)

	if sourceCount != targetCount {
		log.WithFields(log.Fields{
			"source": sourceCount,
			"target": targetCount,
		}).Error("row count mismatch!")
		return
	}

	log.WithFields(log.Fields{
		"source_table":      sourceTable,
		"partitioned_table": partitionedTable,
		"rows":              rows,
	}).Info("migration to partitioned table completed successfully")

	// Next steps (manual):
	// 1. Test the partitioned table thoroughly
	// 2. Update application to use new table
	// 3. After verification, drop the old table
}

// ExampleMigrateTableDataRange demonstrates migrating data for a specific date range
//
// This is useful when:
// - Migrating data incrementally in smaller batches
// - Testing migrations with a subset of data
// - Moving specific time periods to archive tables
// - Migrating data to date-partitioned tables partition by partition
//
// Usage:
//
//	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
//	endDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
//	ExampleMigrateTableDataRange(dbc, "orders", "orders_archive", "created_at", startDate, endDate)
func ExampleMigrateTableDataRange(dbc *DB, sourceTable, targetTable, dateColumn string, startDate, endDate time.Time) {
	log.WithFields(log.Fields{
		"source":      sourceTable,
		"target":      targetTable,
		"date_column": dateColumn,
		"start_date":  startDate.Format("2006-01-02"),
		"end_date":    endDate.Format("2006-01-02"),
	}).Info("migrating data for date range")

	// Step 1: Dry run first to verify and preview
	log.Info("performing dry run")
	_, err := dbc.MigrateTableDataRange(sourceTable, targetTable, dateColumn, startDate, endDate, true)
	if err != nil {
		log.WithError(err).Error("dry run failed - cannot proceed with migration")
		return
	}

	log.Info("dry run successful - proceeding with actual migration")

	// Step 2: Perform actual migration
	rowsMigrated, err := dbc.MigrateTableDataRange(sourceTable, targetTable, dateColumn, startDate, endDate, false)
	if err != nil {
		log.WithError(err).Error("migration failed")
		return
	}

	log.WithFields(log.Fields{
		"rows":       rowsMigrated,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
	}).Info("migration completed successfully")
}

// ExampleIncrementalMigrationByMonth demonstrates migrating data month by month
//
// This approach is useful for:
// - Large tables where migrating all at once would be too slow
// - Reducing lock contention by migrating in smaller batches
// - Being able to pause and resume migrations
// - Easier rollback if issues are detected
//
// Usage:
//
//	ExampleIncrementalMigrationByMonth(dbc, "large_table", "large_table_new", "created_at", 2024)
func ExampleIncrementalMigrationByMonth(dbc *DB, sourceTable, targetTable, dateColumn string, year int) {
	log.WithFields(log.Fields{
		"source": sourceTable,
		"target": targetTable,
		"year":   year,
	}).Info("starting incremental migration by month")

	var totalMigrated int64
	var failedMonths []string

	// Migrate data month by month
	for month := 1; month <= 12; month++ {
		startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		endDate := startDate.AddDate(0, 1, 0) // First day of next month

		log.WithFields(log.Fields{
			"month":      time.Month(month).String(),
			"start_date": startDate.Format("2006-01-02"),
			"end_date":   endDate.Format("2006-01-02"),
		}).Info("migrating month")

		rows, err := dbc.MigrateTableDataRange(sourceTable, targetTable, dateColumn, startDate, endDate, false)
		if err != nil {
			log.WithError(err).WithField("month", time.Month(month).String()).Error("month migration failed")
			failedMonths = append(failedMonths, time.Month(month).String())
			continue
		}

		totalMigrated += rows
		log.WithFields(log.Fields{
			"month": time.Month(month).String(),
			"rows":  rows,
		}).Info("month migrated successfully")
	}

	log.WithFields(log.Fields{
		"total_rows":    totalMigrated,
		"total_months":  12,
		"failed_months": len(failedMonths),
	}).Info("incremental migration completed")

	if len(failedMonths) > 0 {
		log.WithField("failed_months", failedMonths).Warn("some months failed to migrate")
	}
}

// ExampleMigrateToPartitionByDateRange demonstrates migrating data to a specific partition
//
// This workflow is useful when:
// - You have a non-partitioned table and want to migrate to a partitioned structure
// - You want to populate partitions incrementally
// - You're backfilling historical data into partitions
//
// Important: MigrateTableDataRange automatically verifies that all necessary partitions
// exist for the date range being migrated. If the target table is RANGE partitioned and
// partitions are missing, the function will return an error before attempting migration.
//
// Usage:
//
//	ExampleMigrateToPartitionByDateRange(dbc, "orders", "orders_partitioned", "order_date")
func ExampleMigrateToPartitionByDateRange(dbc *DB, sourceTable, partitionedTable, dateColumn string) {
	log.Info("migrating data to partitioned table by date range")

	// Example: Migrate January 2024 data to the partition
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	// Step 1: Migrate the data for this date range
	// The function will automatically verify that partitions exist for all dates
	// in the range [2024-01-01, 2024-02-01) before attempting the migration
	log.WithFields(log.Fields{
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
	}).Info("migrating date range to partition")

	rows, err := dbc.MigrateTableDataRange(sourceTable, partitionedTable, dateColumn, startDate, endDate, false)
	if err != nil {
		log.WithError(err).Error("migration failed")
		return
	}

	log.WithField("rows", rows).Info("data migrated to partition")

	// Step 2: Verify the data landed in the expected partition
	// This would use partition-specific queries to verify
	log.Info("verifying data distribution across partitions")

	// Step 3: Repeat for other date ranges as needed
	log.Info("migration to partition completed - repeat for additional date ranges as needed")
}

// ExampleGetPartitionStrategy demonstrates checking if a table is partitioned
//
// This is useful before performing operations that differ between partitioned
// and non-partitioned tables.
//
// Usage:
//
//	ExampleGetPartitionStrategy(dbc, "orders")
func ExampleGetPartitionStrategy(dbc *DB, tableName string) {
	log.WithField("table", tableName).Info("checking partition strategy")

	strategy, err := dbc.GetPartitionStrategy(tableName)
	if err != nil {
		log.WithError(err).Error("failed to check partition strategy")
		return
	}

	if strategy == "" {
		log.Info("table is not partitioned")
		// Proceed with normal table operations
	} else {
		log.WithField("strategy", strategy).Info("table is partitioned")

		switch strategy {
		case PartitionStrategyRange:
			log.Info("table uses RANGE partitioning - can use date-based partition operations")
		case PartitionStrategyList:
			log.Info("table uses LIST partitioning - partitioned by discrete values")
		case PartitionStrategyHash:
			log.Info("table uses HASH partitioning - partitioned by hash function")
		default:
			log.Warn("unknown partition strategy")
		}
	}
}

// ExampleVerifyPartitionCoverage demonstrates verifying partition coverage before migration
//
// This workflow ensures all necessary partitions exist before attempting a data migration,
// preventing runtime failures due to missing partitions.
//
// Usage:
//
//	ExampleVerifyPartitionCoverage(dbc, "orders", startDate, endDate)
func ExampleVerifyPartitionCoverage(dbc *DB, tableName string, startDate, endDate time.Time) {
	log.WithFields(log.Fields{
		"table":      tableName,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
	}).Info("verifying partition coverage")

	// Verify that all necessary partitions exist
	err := dbc.VerifyPartitionCoverage(tableName, startDate, endDate)
	if err != nil {
		log.WithError(err).Error("partition coverage verification failed")
		log.Error("missing partitions detected - cannot proceed with migration")
		log.Info("create missing partitions using partitions.CreateMissingPartitions before retrying")
		return
	}

	log.Info("partition coverage verified - all required partitions exist")
	log.Info("safe to proceed with data migration")
}

// ExampleCheckAndCreatePartitions demonstrates checking for missing partitions and creating them
//
// This workflow combines partition verification with automatic creation of missing partitions.
//
// Note: This example shows the pattern but doesn't import the partitions package
// to avoid circular dependencies in the example file.
//
// Usage:
//
//	ExampleCheckAndCreatePartitions(dbc, "orders", startDate, endDate)
func ExampleCheckAndCreatePartitions(dbc *DB, tableName string, startDate, endDate time.Time) {
	log.Info("checking partition coverage and creating missing partitions")

	// Step 1: Check if partitions exist
	err := dbc.VerifyPartitionCoverage(tableName, startDate, endDate)
	if err != nil {
		log.WithError(err).Warn("missing partitions detected")

		// Step 2: In actual usage, you would create missing partitions using:
		// import "github.com/openshift/sippy/pkg/db/partitions"
		// count, err := partitions.CreateMissingPartitions(dbc, tableName, startDate, endDate, false)

		log.Info("would create missing partitions here using partitions.CreateMissingPartitions")
		return
	}

	log.Info("all partitions exist - ready for operations")
}
