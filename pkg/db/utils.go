package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// ColumnInfo represents metadata about a database column
type ColumnInfo struct {
	ColumnName    string
	DataType      string
	IsNullable    string
	ColumnDefault sql.NullString
	OrdinalPos    int
}

// PartitionStrategy defines the partitioning strategy type
type PartitionStrategy string

const (
	// PartitionStrategyRange partitions by value ranges (e.g., date ranges)
	PartitionStrategyRange PartitionStrategy = "RANGE"
	// PartitionStrategyList partitions by discrete value lists
	PartitionStrategyList PartitionStrategy = "LIST"
	// PartitionStrategyHash partitions by hash of partition key
	PartitionStrategyHash PartitionStrategy = "HASH"
)

// ColumnVerificationOptions controls which aspects of column definitions to verify
type ColumnVerificationOptions struct {
	// CheckNullable verifies that columns have matching nullable constraints
	CheckNullable bool
	// CheckDefaults verifies that columns have matching default values
	CheckDefaults bool
	// CheckOrder verifies that columns are in the same ordinal position
	CheckOrder bool
}

// DefaultColumnVerificationOptions returns options with all checks enabled
func DefaultColumnVerificationOptions() ColumnVerificationOptions {
	return ColumnVerificationOptions{
		CheckNullable: true,
		CheckDefaults: true,
		CheckOrder:    true,
	}
}

// DataMigrationColumnVerificationOptions returns options suitable for data migrations
// (only checks column names and types, not constraints or defaults)
func DataMigrationColumnVerificationOptions() ColumnVerificationOptions {
	return ColumnVerificationOptions{
		CheckNullable: false,
		CheckDefaults: false,
		CheckOrder:    true,
	}
}

// VerifyTablesHaveSameColumns verifies that two tables have identical column definitions
// Returns nil if the tables have the same columns, or an error describing the differences
//
// This function checks column names and data types by default. Use options parameter
// to control whether nullable constraints, default values, and column order are verified.
func (dbc *DB) VerifyTablesHaveSameColumns(table1, table2 string, opts ColumnVerificationOptions) error {
	log.WithFields(log.Fields{
		"table1": table1,
		"table2": table2,
	}).Debug("verifying tables have same columns")

	// Get columns for both tables
	cols1, err := dbc.GetTableColumns(table1)
	if err != nil {
		return fmt.Errorf("failed to get columns for table %s: %w", table1, err)
	}

	cols2, err := dbc.GetTableColumns(table2)
	if err != nil {
		return fmt.Errorf("failed to get columns for table %s: %w", table2, err)
	}

	// Check if column counts match
	if len(cols1) != len(cols2) {
		return fmt.Errorf("column count mismatch: %s has %d columns, %s has %d columns",
			table1, len(cols1), table2, len(cols2))
	}

	// Create maps for easier comparison
	cols1Map := make(map[string]ColumnInfo)
	for _, col := range cols1 {
		cols1Map[col.ColumnName] = col
	}

	cols2Map := make(map[string]ColumnInfo)
	for _, col := range cols2 {
		cols2Map[col.ColumnName] = col
	}

	// Check for missing columns
	var missingInTable2 []string
	for colName := range cols1Map {
		if _, exists := cols2Map[colName]; !exists {
			missingInTable2 = append(missingInTable2, colName)
		}
	}

	var missingInTable1 []string
	for colName := range cols2Map {
		if _, exists := cols1Map[colName]; !exists {
			missingInTable1 = append(missingInTable1, colName)
		}
	}

	if len(missingInTable1) > 0 || len(missingInTable2) > 0 {
		var errMsg strings.Builder
		errMsg.WriteString("column name mismatch:")
		if len(missingInTable2) > 0 {
			errMsg.WriteString(fmt.Sprintf(" columns in %s but not in %s: %v;",
				table1, table2, missingInTable2))
		}
		if len(missingInTable1) > 0 {
			errMsg.WriteString(fmt.Sprintf(" columns in %s but not in %s: %v",
				table2, table1, missingInTable1))
		}
		return errors.New(errMsg.String())
	}

	// Compare column definitions for matching columns
	var differences []string
	for colName, col1 := range cols1Map {
		col2 := cols2Map[colName]

		// Normalize data types for comparison
		type1 := normalizeDataType(col1.DataType)
		type2 := normalizeDataType(col2.DataType)

		if !strings.EqualFold(type1, type2) {
			differences = append(differences,
				fmt.Sprintf("column %s: type mismatch (%s: %s vs %s: %s)",
					colName, table1, col1.DataType, table2, col2.DataType))
		}

		// Optional: Check nullable constraints
		if opts.CheckNullable && col1.IsNullable != col2.IsNullable {
			differences = append(differences,
				fmt.Sprintf("column %s: nullable mismatch (%s: %s vs %s: %s)",
					colName, table1, col1.IsNullable, table2, col2.IsNullable))
		}

		// Optional: Compare defaults
		if opts.CheckDefaults {
			default1 := ""
			if col1.ColumnDefault.Valid {
				default1 = col1.ColumnDefault.String
			}
			default2 := ""
			if col2.ColumnDefault.Valid {
				default2 = col2.ColumnDefault.String
			}

			if default1 != default2 {
				differences = append(differences,
					fmt.Sprintf("column %s: default mismatch (%s: %q vs %s: %q)",
						colName, table1, default1, table2, default2))
			}
		}

		// Optional: Check ordinal position (column order)
		if opts.CheckOrder && col1.OrdinalPos != col2.OrdinalPos {
			differences = append(differences,
				fmt.Sprintf("column %s: position mismatch (%s: pos %d vs %s: pos %d)",
					colName, table1, col1.OrdinalPos, table2, col2.OrdinalPos))
		}
	}

	if len(differences) > 0 {
		return fmt.Errorf("column definition mismatches:\n  - %s",
			strings.Join(differences, "\n  - "))
	}

	log.WithFields(log.Fields{
		"table1": table1,
		"table2": table2,
		"count":  len(cols1),
	}).Info("tables have identical columns")

	return nil
}

// GetTableColumns retrieves column information for a table from information_schema
func (dbc *DB) GetTableColumns(tableName string) ([]ColumnInfo, error) {
	var columns []ColumnInfo

	query := `
		SELECT
			column_name,
			data_type,
			is_nullable,
			column_default,
			ordinal_position
		FROM information_schema.columns
		WHERE table_schema = 'public'
			AND table_name = @table_name
		ORDER BY ordinal_position
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&columns)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to query columns for table %s: %w", tableName, result.Error)
	}

	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s does not exist or has no columns", tableName)
	}

	return columns, nil
}

// normalizeDataType normalizes PostgreSQL data type names for comparison
func normalizeDataType(dataType string) string {
	dataType = strings.ToLower(strings.TrimSpace(dataType))

	// Map common type variations to standard forms
	typeMap := map[string]string{
		"character varying":           "varchar",
		"integer":                     "int",
		"int4":                        "int",
		"int8":                        "bigint",
		"bigserial":                   "bigint",
		"serial":                      "int",
		"timestamp without time zone": "timestamp",
		"timestamp with time zone":    "timestamptz",
		"double precision":            "float8",
		"boolean":                     "bool",
	}

	if normalized, exists := typeMap[dataType]; exists {
		return normalized
	}

	return dataType
}

// MigrateTableData migrates all data from sourceTable to targetTable after verifying schemas match
// This function performs the following steps:
// 1. Verifies that both tables have identical column definitions
// 2. Checks row counts in both tables
// 3. Copies all data from source to target using INSERT INTO ... SELECT
// 4. Verifies row counts after migration
//
// Parameters:
// - sourceTable: The table to copy data from
// - targetTable: The table to copy data to
// - dryRun: If true, only verifies schemas and reports what would be migrated without actually copying data
//
// Returns:
// - rowsMigrated: The number of rows successfully migrated (0 if dryRun is true)
// - error: Any error encountered during migration
func (dbc *DB) MigrateTableData(sourceTable, targetTable string, dryRun bool) (int64, error) {
	log.WithFields(log.Fields{
		"source":  sourceTable,
		"target":  targetTable,
		"dry_run": dryRun,
	}).Info("starting table data migration")

	// Step 1: Verify schemas match
	// For data migration, we only need to verify column names and types
	// Nullable constraints and defaults don't affect the migration itself
	if err := dbc.VerifyTablesHaveSameColumns(sourceTable, targetTable, DataMigrationColumnVerificationOptions()); err != nil {
		return 0, fmt.Errorf("schema verification failed: %w", err)
	}

	log.Info("schema verification passed - tables have identical column definitions")

	// Step 2: Get row counts before migration
	sourceCount, err := dbc.GetTableRowCount(sourceTable)
	if err != nil {
		return 0, fmt.Errorf("failed to get source table row count: %w", err)
	}

	targetCountBefore, err := dbc.GetTableRowCount(targetTable)
	if err != nil {
		return 0, fmt.Errorf("failed to get target table row count: %w", err)
	}

	log.WithFields(log.Fields{
		"source_rows": sourceCount,
		"target_rows": targetCountBefore,
	}).Info("row counts before migration")

	if sourceCount == 0 {
		log.Warn("source table is empty - nothing to migrate")
		return 0, nil
	}

	// Step 3: Dry run - report what would be migrated
	if dryRun {
		log.WithFields(log.Fields{
			"source_table":   sourceTable,
			"target_table":   targetTable,
			"rows_to_copy":   sourceCount,
			"target_current": targetCountBefore,
		}).Info("[DRY RUN] would migrate data")
		return 0, nil
	}

	// Step 4: Get column names for the INSERT statement
	columns, err := dbc.GetTableColumns(sourceTable)
	if err != nil {
		return 0, fmt.Errorf("failed to get column list: %w", err)
	}

	var columnNames []string
	for _, col := range columns {
		columnNames = append(columnNames, col.ColumnName)
	}

	// Step 5: Perform the migration using INSERT INTO ... SELECT
	// This is done in a single statement for efficiency and atomicity
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s FROM %s",
		targetTable,
		strings.Join(columnNames, ", "),
		strings.Join(columnNames, ", "),
		sourceTable,
	)

	log.WithFields(log.Fields{
		"source": sourceTable,
		"target": targetTable,
		"rows":   sourceCount,
	}).Info("migrating data")

	result := dbc.DB.Exec(insertSQL)
	if result.Error != nil {
		return 0, fmt.Errorf("data migration failed: %w", result.Error)
	}

	rowsAffected := result.RowsAffected

	// Step 6: Verify migration success
	targetCountAfter, err := dbc.GetTableRowCount(targetTable)
	if err != nil {
		return rowsAffected, fmt.Errorf("migration completed but failed to verify: %w", err)
	}

	expectedCount := targetCountBefore + sourceCount
	if targetCountAfter != expectedCount {
		log.WithFields(log.Fields{
			"expected": expectedCount,
			"actual":   targetCountAfter,
			"source":   sourceCount,
			"target":   targetCountBefore,
		}).Warn("row count mismatch after migration")
	}

	log.WithFields(log.Fields{
		"source_table":        sourceTable,
		"target_table":        targetTable,
		"rows_migrated":       rowsAffected,
		"target_count_before": targetCountBefore,
		"target_count_after":  targetCountAfter,
	}).Info("data migration completed successfully")

	return rowsAffected, nil
}

// MigrateTableDataRange migrates data within a specific date range from sourceTable to targetTable
// This function performs the following steps:
// 1. Verifies that both tables have identical column definitions
// 2. Checks if target table is partitioned and verifies partition coverage for the date range
// 3. Counts rows in the date range
// 4. Copies data within the date range from source to target using INSERT INTO ... SELECT ... WHERE
// 5. Verifies row counts after migration
//
// If the target table is RANGE partitioned, the function automatically verifies that all necessary
// partitions exist for the date range being migrated. This prevents migration failures due to missing partitions.
//
// Parameters:
// - sourceTable: The table to copy data from
// - targetTable: The table to copy data to
// - dateColumn: The column name to filter by date range (e.g., "created_at")
// - startDate: Start of date range (inclusive)
// - endDate: End of date range (exclusive)
// - dryRun: If true, only verifies schemas and reports what would be migrated without actually copying data
//
// Returns:
// - rowsMigrated: The number of rows successfully migrated (0 if dryRun is true)
// - error: Any error encountered during migration
//
// Example:
//
//	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
//	endDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
//	rows, err := dbc.MigrateTableDataRange("old_table", "new_table", "created_at", startDate, endDate, false)
func (dbc *DB) MigrateTableDataRange(sourceTable, targetTable, dateColumn string, startDate, endDate time.Time, dryRun bool) (int64, error) {
	log.WithFields(log.Fields{
		"source":      sourceTable,
		"target":      targetTable,
		"date_column": dateColumn,
		"start_date":  startDate.Format("2006-01-02"),
		"end_date":    endDate.Format("2006-01-02"),
		"dry_run":     dryRun,
	}).Info("starting table data migration for date range")

	// Validate date range
	if endDate.Before(startDate) {
		return 0, fmt.Errorf("end date (%s) cannot be before start date (%s)",
			endDate.Format("2006-01-02"), startDate.Format("2006-01-02"))
	}

	// Step 1: Verify schemas match
	// For data migration, we only need to verify column names and types
	// Nullable constraints and defaults don't affect the migration itself
	if err := dbc.VerifyTablesHaveSameColumns(sourceTable, targetTable, DataMigrationColumnVerificationOptions()); err != nil {
		return 0, fmt.Errorf("schema verification failed: %w", err)
	}

	log.Info("schema verification passed - tables have identical column definitions")

	// Step 2: Check if target table is partitioned and verify partition coverage
	partitionStrategy, err := dbc.GetPartitionStrategy(targetTable)
	if err != nil {
		return 0, fmt.Errorf("failed to check if target table is partitioned: %w", err)
	}

	if partitionStrategy != "" {
		log.WithFields(log.Fields{
			"table":    targetTable,
			"strategy": partitionStrategy,
		}).Info("target table is partitioned - verifying partition coverage")

		// For RANGE partitioned tables, verify that partitions exist for the date range
		if partitionStrategy == PartitionStrategyRange {
			if err := dbc.VerifyPartitionCoverage(targetTable, startDate, endDate); err != nil {
				return 0, fmt.Errorf("partition coverage verification failed: %w", err)
			}
			log.Info("partition coverage verified - all required partitions exist")
		} else {
			log.WithField("strategy", partitionStrategy).Warn("target table uses non-RANGE partitioning - skipping partition coverage check")
		}
	}

	// Step 3: Count rows in the date range in source table
	var sourceCount int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s >= @start_date AND %s < @end_date",
		sourceTable, dateColumn, dateColumn)
	result := dbc.DB.Raw(countQuery, sql.Named("start_date", startDate), sql.Named("end_date", endDate)).Scan(&sourceCount)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count rows in date range: %w", result.Error)
	}

	// Get total target row count before migration
	targetCountBefore, err := dbc.GetTableRowCount(targetTable)
	if err != nil {
		return 0, fmt.Errorf("failed to get target table row count: %w", err)
	}

	log.WithFields(log.Fields{
		"source_rows_in_range": sourceCount,
		"target_rows":          targetCountBefore,
		"start_date":           startDate.Format("2006-01-02"),
		"end_date":             endDate.Format("2006-01-02"),
	}).Info("row counts before migration")

	if sourceCount == 0 {
		log.Warn("no rows in date range - nothing to migrate")
		return 0, nil
	}

	// Step 4: Dry run - report what would be migrated
	if dryRun {
		log.WithFields(log.Fields{
			"source_table":   sourceTable,
			"target_table":   targetTable,
			"rows_to_copy":   sourceCount,
			"target_current": targetCountBefore,
			"date_range":     fmt.Sprintf("%s to %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02")),
		}).Info("[DRY RUN] would migrate data")
		return 0, nil
	}

	// Step 5: Get column names for the INSERT statement
	columns, err := dbc.GetTableColumns(sourceTable)
	if err != nil {
		return 0, fmt.Errorf("failed to get column list: %w", err)
	}

	var columnNames []string
	for _, col := range columns {
		columnNames = append(columnNames, col.ColumnName)
	}

	// Step 6: Perform the migration using INSERT INTO ... SELECT ... WHERE
	// This is done in a single statement for efficiency and atomicity
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s FROM %s WHERE %s >= @start_date AND %s < @end_date",
		targetTable,
		strings.Join(columnNames, ", "),
		strings.Join(columnNames, ", "),
		sourceTable,
		dateColumn,
		dateColumn,
	)

	log.WithFields(log.Fields{
		"source":     sourceTable,
		"target":     targetTable,
		"rows":       sourceCount,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
	}).Info("migrating data in date range")

	result = dbc.DB.Exec(insertSQL, sql.Named("start_date", startDate), sql.Named("end_date", endDate))
	if result.Error != nil {
		return 0, fmt.Errorf("data migration failed: %w", result.Error)
	}

	rowsAffected := result.RowsAffected

	// Step 7: Verify migration success
	targetCountAfter, err := dbc.GetTableRowCount(targetTable)
	if err != nil {
		return rowsAffected, fmt.Errorf("migration completed but failed to verify: %w", err)
	}

	expectedCount := targetCountBefore + sourceCount
	if targetCountAfter != expectedCount {
		log.WithFields(log.Fields{
			"expected":             expectedCount,
			"actual":               targetCountAfter,
			"source_in_range":      sourceCount,
			"target_before":        targetCountBefore,
			"rows_actually_copied": rowsAffected,
		}).Warn("row count mismatch after migration")
	}

	log.WithFields(log.Fields{
		"source_table":        sourceTable,
		"target_table":        targetTable,
		"rows_migrated":       rowsAffected,
		"target_count_before": targetCountBefore,
		"target_count_after":  targetCountAfter,
		"start_date":          startDate.Format("2006-01-02"),
		"end_date":            endDate.Format("2006-01-02"),
	}).Info("data migration completed successfully")

	return rowsAffected, nil
}

// GetPartitionStrategy checks if a table is partitioned and returns its partition strategy
// Returns empty string ("") if table is not partitioned
// Returns PartitionStrategyRange, PartitionStrategyList, PartitionStrategyHash, or "UNKNOWN" if partitioned
//
// Example:
//
//	strategy, err := dbc.GetPartitionStrategy("orders")
//	if err != nil {
//	    return err
//	}
//	if strategy == PartitionStrategyRange {
//	    // Handle RANGE partitioned table
//	}
func (dbc *DB) GetPartitionStrategy(tableName string) (PartitionStrategy, error) {
	var strategy string

	query := `
		SELECT
			CASE pp.partstrat
				WHEN 'r' THEN 'RANGE'
				WHEN 'l' THEN 'LIST'
				WHEN 'h' THEN 'HASH'
				ELSE 'UNKNOWN'
			END AS partition_strategy
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_partitioned_table pp ON pp.partrelid = c.oid
		WHERE n.nspname = 'public'
			AND c.relname = @table_name
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&strategy)
	if result.Error != nil {
		return "", fmt.Errorf("failed to check partition strategy: %w", result.Error)
	}

	// If no rows returned, table is not partitioned
	if result.RowsAffected == 0 {
		return "", nil
	}

	return PartitionStrategy(strategy), nil
}

// partitionDateInfo holds date range information for a partition
type partitionDateInfo struct {
	PartitionName string
	PartitionDate time.Time
}

// getPartitionsInDateRange returns all partitions that cover a date range
// Assumes daily partitions with naming convention: tablename_YYYY_MM_DD
func (dbc *DB) getPartitionsInDateRange(tableName string, startDate, endDate time.Time) ([]partitionDateInfo, error) {
	var partitions []partitionDateInfo

	// Prepare patterns in Go code since named parameters can't be concatenated in SQL
	likePattern := tableName + "_%"
	regexPattern := tableName + "_\\d{4}_\\d{2}_\\d{2}$"

	query := `
		SELECT
			tablename AS partition_name,
			TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date
		FROM pg_tables
		WHERE schemaname = 'public'
			AND tablename LIKE @like_pattern
			AND tablename ~ @regex_pattern
		ORDER BY partition_date
	`

	result := dbc.DB.Raw(query,
		sql.Named("like_pattern", likePattern),
		sql.Named("regex_pattern", regexPattern),
	).Scan(&partitions)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to query partitions: %w", result.Error)
	}

	// Filter to only partitions in the date range
	var filtered []partitionDateInfo
	for _, p := range partitions {
		if (p.PartitionDate.Equal(startDate) || p.PartitionDate.After(startDate)) && p.PartitionDate.Before(endDate) {
			filtered = append(filtered, p)
		}
	}

	return filtered, nil
}

// VerifyPartitionCoverage verifies that all necessary partitions exist for a date range
// Assumes daily partitions with naming convention: tablename_YYYY_MM_DD
//
// This function is useful before migrating data to partitioned tables to ensure
// all required partitions exist, preventing INSERT failures.
func (dbc *DB) VerifyPartitionCoverage(tableName string, startDate, endDate time.Time) error {
	partitions, err := dbc.getPartitionsInDateRange(tableName, startDate, endDate)
	if err != nil {
		return fmt.Errorf("failed to get partitions: %w", err)
	}

	// Create a map of existing partition dates for quick lookup
	existingDates := make(map[string]bool)
	for _, p := range partitions {
		dateStr := p.PartitionDate.Format("2006-01-02")
		existingDates[dateStr] = true
	}

	// Check that we have a partition for each day in the range
	var missingDates []string
	currentDate := startDate
	for currentDate.Before(endDate) {
		dateStr := currentDate.Format("2006-01-02")
		if !existingDates[dateStr] {
			missingDates = append(missingDates, dateStr)
		}
		currentDate = currentDate.AddDate(0, 0, 1) // Move to next day
	}

	if len(missingDates) > 0 {
		return fmt.Errorf("missing partitions for dates: %v", missingDates)
	}

	log.WithFields(log.Fields{
		"table":           tableName,
		"partition_count": len(partitions),
		"start_date":      startDate.Format("2006-01-02"),
		"end_date":        endDate.Format("2006-01-02"),
	}).Info("verified partition coverage for date range")

	return nil
}

// GetTableRowCount returns the number of rows in a table
// This is useful for:
// - Verifying table size before operations
// - Comparing source and target tables during migration
// - Monitoring table growth
func (dbc *DB) GetTableRowCount(tableName string) (int64, error) {
	var count int64

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	result := dbc.DB.Raw(query).Scan(&count)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count rows in table %s: %w", tableName, result.Error)
	}

	return count, nil
}

// SyncIdentityColumn synchronizes the IDENTITY sequence for a column to match the current maximum value
// This is useful after migrating data to a partitioned table that uses IDENTITY columns
//
// NOTE: PostgreSQL does not have a SYNC IDENTITY command. Instead, this function uses
// ALTER TABLE ... ALTER COLUMN ... RESTART WITH, which is the standard PostgreSQL syntax
// for resetting an IDENTITY column's sequence to a specific value.
//
// Parameters:
//   - tableName: Name of the table containing the IDENTITY column
//   - columnName: Name of the IDENTITY column to sync (typically "id")
//
// The function executes: ALTER TABLE table_name ALTER COLUMN column_name RESTART WITH (max_value + 1)
// where max_value is the current maximum value in the column.
//
// Use cases:
//   - After migrating data from a non-partitioned table to a partitioned table
//   - After bulk inserting data with explicit IDs
//   - When IDENTITY sequence is out of sync with actual data
//
// Example:
//
//	err := dbc.SyncIdentityColumn("my_table", "id")
//	if err != nil {
//	    log.WithError(err).Error("failed to sync identity column")
//	}
func (dbc *DB) SyncIdentityColumn(tableName, columnName string) error {
	log.WithFields(log.Fields{
		"table":  tableName,
		"column": columnName,
	}).Info("synchronizing identity column")

	// Get the current maximum value
	var maxValue sql.NullInt64
	query := fmt.Sprintf("SELECT MAX(%s) FROM %s", columnName, tableName)
	result := dbc.DB.Raw(query).Scan(&maxValue)
	if result.Error != nil {
		return fmt.Errorf("failed to get max value for %s.%s: %w", tableName, columnName, result.Error)
	}

	// If table is empty or column has all NULL values, start at 1
	nextValue := int64(1)
	if maxValue.Valid {
		nextValue = maxValue.Int64 + 1
	}

	log.WithFields(log.Fields{
		"table":      tableName,
		"column":     columnName,
		"max_value":  maxValue.Int64,
		"next_value": nextValue,
	}).Debug("restarting identity sequence")

	// Restart the identity sequence
	// NOTE: PostgreSQL requires "RESTART WITH" for IDENTITY columns, not "SYNC IDENTITY"
	// This is the standard way to synchronize an IDENTITY sequence in PostgreSQL
	alterSQL := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s RESTART WITH %d", tableName, columnName, nextValue)
	result = dbc.DB.Exec(alterSQL)
	if result.Error != nil {
		return fmt.Errorf("failed to sync identity for %s.%s: %w", tableName, columnName, result.Error)
	}

	log.WithFields(log.Fields{
		"table":      tableName,
		"column":     columnName,
		"next_value": nextValue,
	}).Info("identity column synchronized successfully")

	return nil
}
