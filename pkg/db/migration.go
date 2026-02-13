package db

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// MigrateToPartitionedTable creates a new partitioned table from a GORM model,
// creates daily partitions covering the source table's date range, and migrates
// data from the source table up to the specified date.  Keeping the migrateUpTo
// date behind the current date allows follow-up Update and Finalize migration
// to fill in data incrementally picking up from the previous migrateUpTo date.
//
// Parameters:
//   - model: GORM model struct pointer defining the target schema (e.g., &models.ProwJobRunTest{})
//   - sourceTable: name of the existing non-partitioned table to migrate from
//   - dateColumn: the column used for partitioning and date-range migration
//   - migrateUpTo: migrate data with dateColumn < migrateUpTo
//   - dryRun: if true, logs all steps without executing
//
// The target table name is derived as sourceTable + "_partitioned".
//
// Steps performed:
//  1. Create the partitioned table from the model
//  2. Create daily partitions from the earliest data date through migrateUpTo + 7 days
//  3. Migrate data from sourceTable where dateColumn >= earliest and dateColumn < migrateUpTo
//  4. Sync identity columns so new inserts get correct IDs
func (dbc *DB) MigrateToPartitionedTable(model interface{}, sourceTable, dateColumn string, migrateUpTo time.Time, dryRun bool) error {
	targetTable := sourceTable + "_partitioned"

	l := log.WithFields(log.Fields{
		"source":        sourceTable,
		"target":        targetTable,
		"date_column":   dateColumn,
		"migrate_up_to": migrateUpTo.Format("2006-01-02"),
		"dry_run":       dryRun,
	})

	l.Info("starting migration to partitioned table")

	// Step 1: Create the partitioned table
	l.Info("step 1: creating partitioned table")
	config := NewRangePartitionConfig(dateColumn)
	sql, err := dbc.CreatePartitionedTable(model, targetTable, config, dryRun)
	if err != nil {
		return fmt.Errorf("failed to create partitioned table: %w", err)
	}
	l.WithField("sql", sql).Info("partitioned table created")

	// Step 2: Determine date range and create partitions
	l.Info("step 2: determining date range for partitions")
	var minDate time.Time
	query := fmt.Sprintf("SELECT MIN(%s) FROM %s", dateColumn, sourceTable)
	result := dbc.DB.Raw(query).Scan(&minDate)
	if result.Error != nil {
		return fmt.Errorf("failed to get min %s from %s: %w", dateColumn, sourceTable, result.Error)
	}

	if minDate.IsZero() {
		return fmt.Errorf("source table %s is empty or %s has no values", sourceTable, dateColumn)
	}

	// Create partitions from earliest data through migrateUpTo + 7 days buffer
	partitionEnd := migrateUpTo.AddDate(0, 0, 7)
	partitionStart := minDate.UTC().Truncate(24 * time.Hour)

	l.WithFields(log.Fields{
		"partition_start": partitionStart.Format("2006-01-02"),
		"partition_end":   partitionEnd.Format("2006-01-02"),
	}).Info("creating partitions")

	created, err := dbc.CreateMissingPartitions(targetTable, partitionStart.AddDate(0, 0, -1), partitionEnd, dryRun)
	if err != nil {
		return fmt.Errorf("failed to create partitions: %w", err)
	}
	l.WithField("partitions_created", created).Info("partitions created")

	// can't go any further if we haven't created the table
	if !dryRun {
		// Step 3: Migrate data
		l.Info("step 3: migrating data")
		rows, err := dbc.MigrateTableDataRange(sourceTable, targetTable, dateColumn, partitionStart, migrateUpTo, nil, dryRun)
		if err != nil {
			return fmt.Errorf("failed to migrate data: %w", err)
		}
		l.WithField("rows_migrated", rows).Info("data migration complete")

		// Step 4: Sync identity column
		l.Info("step 4: syncing identity column")
		if err := dbc.SyncIdentityColumn(targetTable, "id"); err != nil {
			return fmt.Errorf("failed to sync identity column: %w", err)
		}
		l.Info("identity column synced")
	}

	l.Info("migration to partitioned table complete")
	return nil
}

// UpdatePartitionedTableMigration migrates additional data from the source table to
// the partitioned table created by MigrateToPartitionedTable. Call this one or more
// times to incrementally catch up before calling FinalizePartitionedTableMigration.
//
// Parameters:
//   - sourceTable: name of the original non-partitioned table
//   - dateColumn: the column used for partitioning and date-range migration
//   - migrateUpTo: migrate data with dateColumn < migrateUpTo
//   - dryRun: if true, logs all steps without executing
//
// Steps performed:
//  1. Find the newest date already migrated in the partitioned table
//  2. Verify partitions exist for the range; create any that are missing
//  3. Migrate data from sourceTable where dateColumn > newest migrated and dateColumn < migrateUpTo
//  4. Sync identity column so new inserts get correct IDs
func (dbc *DB) UpdatePartitionedTableMigration(sourceTable, dateColumn string, migrateUpTo time.Time, dryRun bool) error {
	targetTable := sourceTable + "_partitioned"

	l := log.WithFields(log.Fields{
		"source":        sourceTable,
		"target":        targetTable,
		"date_column":   dateColumn,
		"migrate_up_to": migrateUpTo.Format("2006-01-02"),
		"dry_run":       dryRun,
	})

	l.Info("starting incremental migration update")

	// Step 1: Find the newest date already migrated
	l.Info("step 1: finding newest migrated date")
	var maxDate time.Time
	query := fmt.Sprintf("SELECT COALESCE(MAX(%s), '0001-01-01'::timestamp) FROM %s", dateColumn, targetTable)
	result := dbc.DB.Raw(query).Scan(&maxDate)
	if result.Error != nil {
		return fmt.Errorf("failed to get max %s from %s: %w", dateColumn, targetTable, result.Error)
	}

	if maxDate.IsZero() || maxDate.Year() == 1 {
		return fmt.Errorf("partitioned table %s has no data; run MigrateToPartitionedTable first", targetTable)
	}

	if !migrateUpTo.After(maxDate) {
		l.WithFields(log.Fields{
			"newest_migrated": maxDate.Format("2006-01-02 15:04:05"),
			"migrate_up_to":   migrateUpTo.Format("2006-01-02"),
		}).Info("no new data to migrate; migrateUpTo is not after newest migrated date")
		return nil
	}

	l.WithFields(log.Fields{
		"migrate_from":  maxDate.Format("2006-01-02 15:04:05"),
		"migrate_up_to": migrateUpTo.Format("2006-01-02"),
	}).Info("determined migration range")

	// Step 2: Verify and create missing partitions
	l.Info("step 2: verifying partitions")
	partitionStart := maxDate.UTC().Truncate(24 * time.Hour)
	partitionEnd := migrateUpTo.AddDate(0, 0, 7)

	created, err := dbc.CreateMissingPartitions(targetTable, partitionStart, partitionEnd, dryRun)
	if err != nil {
		return fmt.Errorf("failed to create missing partitions: %w", err)
	}
	l.WithField("partitions_created", created).Info("partition verification complete")

	// Step 3: Migrate data from newest migrated through migrateUpTo
	l.Info("step 3: migrating new data")
	rows, err := dbc.MigrateTableDataRange(sourceTable, targetTable, dateColumn, maxDate, migrateUpTo, nil, dryRun)
	if err != nil {
		return fmt.Errorf("failed to migrate data: %w", err)
	}
	l.WithField("rows_migrated", rows).Info("data migration complete")

	// Step 4: Sync identity column
	if !dryRun {
		l.Info("step 4: syncing identity column")
		if err := dbc.SyncIdentityColumn(targetTable, "id"); err != nil {
			return fmt.Errorf("failed to sync identity column: %w", err)
		}
		l.Info("identity column synced")
	}

	l.Info("incremental migration update complete")
	return nil
}

// FinalizePartitionedTableMigration migrates any remaining data from the source table
// to the partitioned table, syncs identity columns, swaps the tables so the partitioned
// table takes the original name, and handles foreign keys.
//
// Parameters:
//   - sourceTable: name of the original non-partitioned table
//   - dateColumn: the column used for partitioning and date-range migration
//   - migrateUpTo: migrate data with dateColumn < migrateUpTo
//   - moveForeignKeys: if true, foreign keys are moved from the old table to the new
//     partitioned table; if false, foreign keys are dropped (necessary when the
//     partitioned table cannot support the FK constraints)
//   - dryRun: if true, logs all steps without executing
//
// Assumes MigrateToPartitionedTable was previously called to create sourceTable + "_partitioned".
//
// Steps performed:
//  1. Determine the newest date already migrated in the partitioned table
//  2. Create any missing partitions up to migrateUpTo + 7 days
//  3. Migrate remaining data from sourceTable where dateColumn >= newest migrated and dateColumn < migrateUpTo
//  4. Sync identity columns
//  5. Swap tables: sourceTable → sourceTable_old, sourceTable_partitioned → sourceTable
//  6. Move or drop foreign keys from sourceTable_old
func (dbc *DB) FinalizePartitionedTableMigration(sourceTable, dateColumn string, migrateUpTo time.Time, moveForeignKeys, dryRun bool) error {
	partitionedTable := sourceTable + "_partitioned"
	oldTable := sourceTable + "_old"

	l := log.WithFields(log.Fields{
		"source":            sourceTable,
		"partitioned_table": partitionedTable,
		"date_column":       dateColumn,
		"migrate_up_to":     migrateUpTo.Format("2006-01-02"),
		"move_foreign_keys": moveForeignKeys,
		"dry_run":           dryRun,
	})

	l.Info("starting finalization of partitioned table migration")

	// Step 1: Find the newest date already migrated
	l.Info("step 1: checking for new data to migrate")
	var maxDate time.Time
	query := fmt.Sprintf("SELECT COALESCE(MAX(%s), '0001-01-01'::timestamp) FROM %s", dateColumn, partitionedTable)
	result := dbc.DB.Raw(query).Scan(&maxDate)
	if result.Error != nil {
		return fmt.Errorf("failed to get max %s from %s: %w", dateColumn, partitionedTable, result.Error)
	}

	migrateFrom := maxDate
	if maxDate.IsZero() || maxDate.Year() == 1 {
		return fmt.Errorf("partitioned table %s has no data; run MigrateToPartitionedTable first", partitionedTable)
	}

	l.WithFields(log.Fields{
		"newest_migrated": migrateFrom.Format("2006-01-02 15:04:05"),
		"migrate_up_to":   migrateUpTo.Format("2006-01-02"),
	}).Info("determined migration range")

	// Step 2: Create any missing partitions
	l.Info("step 2: creating missing partitions")
	partitionEnd := migrateUpTo.AddDate(0, 0, 7)
	partitionStart := migrateFrom.UTC().Truncate(24 * time.Hour)

	created, err := dbc.CreateMissingPartitions(partitionedTable, partitionStart, partitionEnd, dryRun)
	if err != nil {
		return fmt.Errorf("failed to create partitions: %w", err)
	}
	l.WithField("partitions_created", created).Info("partitions created")

	// Step 3: Migrate remaining data
	l.Info("step 3: migrating remaining data")
	rows, err := dbc.MigrateTableDataRange(sourceTable, partitionedTable, dateColumn, migrateFrom, migrateUpTo, nil, dryRun)
	if err != nil {
		return fmt.Errorf("failed to migrate remaining data: %w", err)
	}
	l.WithField("rows_migrated", rows).Info("remaining data migration complete")

	// Step 4: Sync identity column
	l.Info("step 4: syncing identity column")
	if !dryRun {
		if err := dbc.SyncIdentityColumn(partitionedTable, "id"); err != nil {
			return fmt.Errorf("failed to sync identity column: %w", err)
		}
		l.Info("identity column synced")
	}

	// Step 5: Swap tables atomically
	l.Info("step 5: swapping tables")
	renames := []TableRename{
		{From: sourceTable, To: oldTable},
		{From: partitionedTable, To: sourceTable},
	}

	count, err := dbc.RenameTables(renames, true, true, true, true, dryRun)
	if err != nil {
		return fmt.Errorf("failed to swap tables: %w", err)
	}
	l.WithField("renames", count).Info("tables swapped")

	// Step 6: Move or drop foreign keys
	fkTarget := sourceTable
	if !moveForeignKeys {
		fkTarget = ""
	}
	if moveForeignKeys {
		l.Info("step 6: moving foreign keys")
	} else {
		l.Info("step 6: dropping foreign keys")
	}
	moved, err := dbc.MoveForeignKeys(oldTable, fkTarget, dryRun)
	if err != nil {
		return fmt.Errorf("failed to process foreign keys: %w", err)
	}
	if moveForeignKeys {
		l.WithField("fks_moved", moved).Info("foreign keys moved")
	} else {
		l.WithField("fks_dropped", moved).Info("foreign keys dropped")
	}

	l.Info("partitioned table migration finalized successfully")
	return nil
}

// AnalyzePartitioningImpact examines the foreign key relationships for a table and
// logs the impact of migrating it to a partitioned table. For each FK, it reports:
//   - Direction (inbound or outbound)
//   - Whether the related table is already partitioned
//   - Whether the FK can be preserved, must be expanded, or must be dropped
//   - Whether the source table has the necessary partition key columns for expansion
//
// This is a read-only analysis function that makes no changes to the database.
func (dbc *DB) AnalyzePartitioningImpact(tableName string) error {
	l := log.WithField("table", tableName)
	l.Info("analyzing partitioning impact")

	// Check if the table is already partitioned
	tablePartCols, _ := dbc.getPartitionColumns(tableName)
	if len(tablePartCols) > 0 {
		l.WithField("partition_columns", tablePartCols).Info("table is already partitioned")
	}

	// Get all FK relationships
	fks, err := dbc.GetFKRelationships(tableName)
	if err != nil {
		return fmt.Errorf("failed to get FK relationships: %w", err)
	}

	if len(fks) == 0 {
		l.Info("no foreign key relationships found")
		return nil
	}

	l.WithField("count", len(fks)).Info("found foreign key relationships")

	for _, fk := range fks {
		fields := log.Fields{
			"constraint":         fk.ConstraintName,
			"source_table":       fk.SourceTable,
			"source_columns":     fk.SourceColumns,
			"referenced_table":   fk.ReferencedTable,
			"referenced_columns": fk.ReferencedColumns,
		}

		if fk.SourceTable == tableName {
			// Outbound FK: this table references another table
			fields["direction"] = "outbound"

			refPartCols, _ := dbc.getPartitionColumns(fk.ReferencedTable)
			fields["referenced_table_partitioned"] = len(refPartCols) > 0

			if len(refPartCols) == 0 {
				// Referenced table is not partitioned — FK can be preserved
				fields["action"] = "KEEP"
				fields["reason"] = "referenced table is not partitioned; outbound FKs from partitioned tables are allowed"
			} else {
				// Referenced table is partitioned — FK requires unique constraint
				// on the referenced columns which must include partition key
				fields["action"] = "DROP"
				fields["reason"] = "referenced table is partitioned; unique constraints on partitioned tables must include partition key, making single-column FK references problematic"
				fields["referenced_partition_columns"] = refPartCols
			}

			l.WithFields(fields).Info("outbound FK analysis")
		} else {
			// Inbound FK: another table references this table
			fields["direction"] = "inbound"

			srcPartCols, _ := dbc.getPartitionColumns(fk.SourceTable)
			fields["source_table_partitioned"] = len(srcPartCols) > 0

			// When this table becomes partitioned, its PK will include the partition
			// key. Inbound FKs must reference the full PK (or a unique constraint
			// that includes the partition key).
			srcColumns, err := dbc.GetTableColumns(fk.SourceTable)
			if err != nil {
				l.WithError(err).WithFields(fields).Warn("could not inspect source table columns")
				continue
			}

			srcColSet := make(map[string]bool)
			for _, col := range srcColumns {
				srcColSet[col.ColumnName] = true
			}

			// Assume partitioning on created_at (most common case) — check if source has it
			hasCreatedAt := srcColSet["created_at"]
			fields["source_has_created_at"] = hasCreatedAt

			if len(srcPartCols) > 0 {
				// Source table is also partitioned
				fields["action"] = "DROP"
				fields["reason"] = "both tables are partitioned; FK constraints between partitioned tables are not supported"
			} else if !hasCreatedAt {
				// Source table lacks the partition key column — FK must be dropped
				fields["action"] = "DROP"
				fields["reason"] = fmt.Sprintf("source table %s lacks created_at column needed to reference partitioned %s; FK cannot be expanded",
					fk.SourceTable, tableName)
			} else {
				// Source table has the partition key — FK could be expanded
				fields["action"] = "EXPAND or DROP"
				fields["reason"] = fmt.Sprintf("source table %s has created_at column; FK could be expanded to include partition key, "+
					"but this requires a UNIQUE constraint on %s(%s, created_at) and adds complexity",
					fk.SourceTable, tableName, fk.ReferencedColumns)
			}

			l.WithFields(fields).Info("inbound FK analysis")
		}
	}

	l.Info("partitioning impact analysis complete")
	return nil
}

// DeleteTestsByName deletes all data associated with tests whose name matches the
// given SQL LIKE pattern. This handles the full dependency chain across partitioned
// tables where FK cascades are not available.
//
// Deletion order (leaf to root):
//  1. prow_job_run_test_outputs (references prow_job_run_tests)
//  2. test_analysis_by_job_by_dates (references tests)
//  3. prow_job_run_tests (references tests)
//  4. bug_tests (join table between bugs and tests)
//  5. test_ownerships (references tests)
//  6. tests
//
// The namePattern uses SQL LIKE syntax (e.g., "%shouldn't use random names%").
func (dbc *DB) DeleteTestsByName(namePattern string, dryRun bool) (int64, error) {
	if strings.TrimSpace(namePattern) == "" {
		return 0, fmt.Errorf("namePattern cannot be empty")
	}

	l := log.WithFields(log.Fields{
		"name_pattern": namePattern,
		"dry_run":      dryRun,
	})

	// Preview matching tests
	var matchCount int64
	if err := dbc.DB.Raw("SELECT COUNT(*) FROM tests WHERE name LIKE ?", namePattern).Scan(&matchCount).Error; err != nil {
		return 0, fmt.Errorf("failed to count matching tests: %w", err)
	}

	if matchCount == 0 {
		l.Info("no tests match the pattern")
		return 0, nil
	}

	l.WithField("matching_tests", matchCount).Info("found tests matching pattern")

	if dryRun {
		type tableCount struct {
			name  string
			query string
		}
		counts := []tableCount{
			{"prow_job_run_test_outputs", "SELECT COUNT(*) FROM prow_job_run_test_outputs WHERE prow_job_run_test_id IN (SELECT pjrt.id FROM prow_job_run_tests pjrt JOIN tests t ON t.id = pjrt.test_id WHERE t.name LIKE ?)"},
			{"test_analysis_by_job_by_dates", "SELECT COUNT(*) FROM test_analysis_by_job_by_dates WHERE test_id IN (SELECT id FROM tests WHERE name LIKE ?)"},
			{"prow_job_run_tests", "SELECT COUNT(*) FROM prow_job_run_tests WHERE test_id IN (SELECT id FROM tests WHERE name LIKE ?)"},
			{"bug_tests", "SELECT COUNT(*) FROM bug_tests WHERE test_id IN (SELECT id FROM tests WHERE name LIKE ?)"},
			{"test_ownerships", "SELECT COUNT(*) FROM test_ownerships WHERE test_id IN (SELECT id FROM tests WHERE name LIKE ?)"},
			{"tests", "SELECT COUNT(*) FROM tests WHERE name LIKE ?"},
		}

		for _, tc := range counts {
			var c int64
			if err := dbc.DB.Raw(tc.query, namePattern).Scan(&c).Error; err != nil {
				l.WithError(err).WithField("table", tc.name).Warn("failed to count rows")
				continue
			}
			l.WithFields(log.Fields{"table": tc.name, "rows": c}).Info("would delete")
		}

		return matchCount, nil
	}

	// Execute deletes in a transaction
	tx := dbc.DB.Begin()
	if tx.Error != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	type deleteStep struct {
		name string
		sql  string
	}

	steps := []deleteStep{
		{
			"prow_job_run_test_outputs",
			"DELETE FROM prow_job_run_test_outputs WHERE prow_job_run_test_id IN (SELECT pjrt.id FROM prow_job_run_tests pjrt JOIN tests t ON t.id = pjrt.test_id WHERE t.name LIKE ?)",
		},
		{
			"test_analysis_by_job_by_dates",
			"DELETE FROM test_analysis_by_job_by_dates WHERE test_id IN (SELECT id FROM tests WHERE name LIKE ?)",
		},
		{
			"prow_job_run_tests",
			"DELETE FROM prow_job_run_tests WHERE test_id IN (SELECT id FROM tests WHERE name LIKE ?)",
		},
		{
			"bug_tests",
			"DELETE FROM bug_tests WHERE test_id IN (SELECT id FROM tests WHERE name LIKE ?)",
		},
		{
			"test_ownerships",
			"DELETE FROM test_ownerships WHERE test_id IN (SELECT id FROM tests WHERE name LIKE ?)",
		},
		{
			"tests",
			"DELETE FROM tests WHERE name LIKE ?",
		},
	}

	var totalDeleted int64
	for _, step := range steps {
		result := tx.Exec(step.sql, namePattern)
		if result.Error != nil {
			return 0, fmt.Errorf("failed to delete from %s: %w", step.name, result.Error)
		}
		l.WithFields(log.Fields{"table": step.name, "rows": result.RowsAffected}).Info("deleted rows")
		totalDeleted += result.RowsAffected
	}

	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.WithField("total_deleted", totalDeleted).Info("test deletion complete")
	return matchCount, nil
}
