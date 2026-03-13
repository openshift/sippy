package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
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

// DataMigrationColumnVerificationOptions returns options suitable for data migrations
// (only checks column names and types, not constraints or defaults)
func DataMigrationColumnVerificationOptions() ColumnVerificationOptions {
	return ColumnVerificationOptions{
		CheckNullable: false,
		CheckDefaults: false,
		CheckOrder:    false,
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

// GetTableColumns retrieves column information for a table from pg_catalog
// Uses format_type() to preserve precise type definitions including:
// - Length modifiers: varchar(64) vs varchar(255)
// - Precision/scale: numeric(8,2) vs numeric(20,10)
// - Enum type names: user_role instead of USER-DEFINED
// - Array types: integer[] vs integer
func (dbc *DB) GetTableColumns(tableName string) ([]ColumnInfo, error) {
	var columns []ColumnInfo

	// Use pg_catalog to get precise type information including modifiers
	// format_type() preserves varchar(64) vs varchar(255), numeric(8,2) vs numeric(20,10), etc.
	query := `
		SELECT
			a.attname AS column_name,
			format_type(a.atttypid, a.atttypmod) AS data_type,
			CASE WHEN a.attnotnull THEN 'NO' ELSE 'YES' END AS is_nullable,
			pg_get_expr(d.adbin, d.adrelid) AS column_default,
			a.attnum AS ordinal_position
		FROM pg_catalog.pg_attribute a
		JOIN pg_catalog.pg_class c ON a.attrelid = c.oid
		JOIN pg_catalog.pg_namespace n ON c.relnamespace = n.oid
		LEFT JOIN pg_catalog.pg_attrdef d ON a.attrelid = d.adrelid AND a.attnum = d.adnum
		WHERE c.relname = @table_name
			AND n.nspname = 'public'
			AND a.attnum > 0
			AND NOT a.attisdropped
		ORDER BY a.attnum
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
// Preserves type modifiers (length, precision, scale) while normalizing base type names
// Examples:
//   - "character varying(64)" -> "varchar(64)"
//   - "integer" -> "int"
//   - "timestamp without time zone" -> "timestamp"
func normalizeDataType(dataType string) string {
	dataType = strings.ToLower(strings.TrimSpace(dataType))

	// Map common type variations to standard forms (preserving any modifiers)
	// Check for types with modifiers first (e.g., "character varying(64)")
	replacements := map[string]string{
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

	// Try to replace the base type name while preserving modifiers
	for old, newType := range replacements {
		if suffix, found := strings.CutPrefix(dataType, old); found {
			// Replace the prefix and keep everything after (modifiers, array brackets, etc.)
			return newType + suffix
		}
	}

	return dataType
}

func quoteIdentifierList(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, pq.QuoteIdentifier(n))
	}
	return strings.Join(quoted, ", ")
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
// - omitColumns: List of column names to omit from migration (e.g., ["id"] to use target's auto-increment)
// - dryRun: If true, only verifies schemas and reports what would be migrated without actually copying data
//
// Returns:
// - rowsMigrated: The number of rows successfully migrated (0 if dryRun is true)
// - error: Any error encountered during migration
func (dbc *DB) MigrateTableData(sourceTable, targetTable string, omitColumns []string, dryRun bool) (int64, error) {
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

	// Create a map of columns to omit for quick lookup
	omitMap := make(map[string]bool)
	for _, col := range omitColumns {
		omitMap[col] = true
	}

	// Build column list, excluding omitted columns
	var columnNames []string
	for _, col := range columns {
		if !omitMap[col.ColumnName] {
			columnNames = append(columnNames, col.ColumnName)
		}
	}

	if len(columnNames) == 0 {
		return 0, fmt.Errorf("no columns to migrate after omitting %v", omitColumns)
	}

	// Step 5: Perform the migration using INSERT INTO ... SELECT
	// This is done in a single statement for efficiency and atomicity
	columnList := quoteIdentifierList(columnNames)
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s FROM %s",
		pq.QuoteIdentifier(targetTable),
		columnList,
		columnList,
		pq.QuoteIdentifier(sourceTable),
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
// - omitColumns: List of column names to omit from migration (e.g., ["id"] to use target's auto-increment)
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
//	rows, err := dbc.MigrateTableDataRange("old_table", "new_table", "created_at", startDate, endDate, nil, false)
func (dbc *DB) MigrateTableDataRange(sourceTable, targetTable, dateColumn string, startDate, endDate time.Time, omitColumns []string, dryRun bool) (int64, error) {
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
		pq.QuoteIdentifier(sourceTable), pq.QuoteIdentifier(dateColumn), pq.QuoteIdentifier(dateColumn))
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

	// Create a map of columns to omit for quick lookup
	omitMap := make(map[string]bool)
	for _, col := range omitColumns {
		omitMap[col] = true
	}

	// Build column list, excluding omitted columns
	var columnNames []string
	for _, col := range columns {
		if !omitMap[col.ColumnName] {
			columnNames = append(columnNames, col.ColumnName)
		}
	}

	if len(columnNames) == 0 {
		return 0, fmt.Errorf("no columns to migrate after omitting %v", omitColumns)
	}

	// Step 6: Perform the migration using INSERT INTO ... SELECT ... WHERE
	// This is done in a single statement for efficiency and atomicity
	columnList := quoteIdentifierList(columnNames)
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) SELECT %s FROM %s WHERE %s >= @start_date AND %s < @end_date",
		pq.QuoteIdentifier(targetTable),
		columnList,
		columnList,
		pq.QuoteIdentifier(sourceTable),
		pq.QuoteIdentifier(dateColumn),
		pq.QuoteIdentifier(dateColumn))

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

	// Query only attached partitions using pg_inherits
	// Detached partitions won't appear in pg_inherits
	query := `
		WITH attached_partitions AS (
			SELECT c.relname AS tablename
			FROM pg_inherits i
			JOIN pg_class c ON i.inhrelid = c.oid
			JOIN pg_class p ON i.inhparent = p.oid
			WHERE p.relname = @table_name
		)
		SELECT
			tablename AS partition_name,
			TO_DATE(SUBSTRING(tablename FROM '_(\d{4}_\d{2}_\d{2})$'), 'YYYY_MM_DD') AS partition_date
		FROM pg_tables
		WHERE schemaname = 'public'
			AND tablename IN (SELECT tablename FROM attached_partitions)
			AND tablename ~ @regex_pattern
		ORDER BY partition_date
	`

	regexPattern := tableName + "_\\d{4}_\\d{2}_\\d{2}$"

	result := dbc.DB.Raw(query,
		sql.Named("table_name", tableName),
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

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", pq.QuoteIdentifier(tableName))
	result := dbc.DB.Raw(query).Scan(&count)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count rows in table %s: %w", tableName, result.Error)
	}

	return count, nil
}

// SequenceInfo represents information about a sequence associated with a table column
type SequenceInfo struct {
	SequenceName string
	TableName    string
	ColumnName   string
}

// PartitionTableInfo represents information about a table partition
type PartitionTableInfo struct {
	PartitionName string
	ParentTable   string
}

// ConstraintInfo represents information about a table constraint
type ConstraintInfo struct {
	ConstraintName string
	TableName      string
	ConstraintType string // 'p'=primary key, 'f'=foreign key, 'u'=unique, 'c'=check, 'x'=exclusion
	Definition     string // Full constraint definition
}

// GetTableConstraints returns all constraints for a table
// This includes primary keys, foreign keys, unique constraints, check constraints, and exclusion constraints
//
// Constraint types:
//   - 'p' = Primary key
//   - 'f' = Foreign key
//   - 'u' = Unique
//   - 'c' = Check
//   - 'x' = Exclusion
//
// Example:
//
//	constraints, err := dbc.GetTableConstraints("orders")
//	if err != nil {
//	    log.WithError(err).Error("failed to get constraints")
//	}
//	for _, c := range constraints {
//	    log.WithFields(log.Fields{
//	        "constraint": c.ConstraintName,
//	        "type":       c.ConstraintType,
//	    }).Info("found constraint")
//	}
func (dbc *DB) GetTableConstraints(tableName string) ([]ConstraintInfo, error) {
	var constraints []ConstraintInfo

	query := `
		SELECT
			con.conname AS constraint_name,
			t.relname AS table_name,
			con.contype AS constraint_type,
			pg_get_constraintdef(con.oid) AS definition
		FROM pg_constraint con
		JOIN pg_class t ON con.conrelid = t.oid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE t.relname = @table_name
			AND n.nspname = 'public'
		ORDER BY con.contype, con.conname
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&constraints)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get constraints for table %s: %w", tableName, result.Error)
	}

	return constraints, nil
}

// IndexInfo represents metadata about a table index
type IndexInfo struct {
	IndexName  string
	TableName  string
	Definition string // Index definition (CREATE INDEX statement)
	IsPrimary  bool   // true if this is a primary key index
	IsUnique   bool   // true if this is a unique index
}

// GetTableIndexes returns all indexes for a table
// This includes indexes created explicitly and indexes backing constraints (primary keys, unique constraints)
//
// Note: Indexes backing constraints may have the same name as the constraint,
// but they are separate objects. Renaming a constraint does NOT rename the index.
//
// Example:
//
//	indexes, err := dbc.GetTableIndexes("orders")
//	if err != nil {
//	    log.WithError(err).Error("failed to get indexes")
//	}
//	for _, idx := range indexes {
//	    log.WithFields(log.Fields{
//	        "index":      idx.IndexName,
//	        "is_primary": idx.IsPrimary,
//	        "is_unique":  idx.IsUnique,
//	    }).Info("found index")
//	}
func (dbc *DB) GetTableIndexes(tableName string) ([]IndexInfo, error) {
	var indexes []IndexInfo

	query := `
		SELECT
			i.indexname AS index_name,
			i.tablename AS table_name,
			i.indexdef AS definition,
			ix.indisprimary AS is_primary,
			ix.indisunique AS is_unique
		FROM pg_indexes i
		JOIN pg_class c ON c.relname = i.indexname
		JOIN pg_index ix ON ix.indexrelid = c.oid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE i.tablename = @table_name
			AND i.schemaname = 'public'
			AND n.nspname = 'public'
		ORDER BY i.indexname
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&indexes)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get indexes for table %s: %w", tableName, result.Error)
	}

	return indexes, nil
}

// GetTablePartitions returns all partitions of a partitioned table
// Uses PostgreSQL's partition inheritance system to find child partitions
func (dbc *DB) GetTablePartitions(tableName string) ([]PartitionTableInfo, error) {
	var partitions []PartitionTableInfo

	query := `
		SELECT
			child.relname AS partition_name,
			parent.relname AS parent_table
		FROM pg_inherits
		JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
		JOIN pg_class child ON pg_inherits.inhrelid = child.oid
		JOIN pg_namespace nmsp_parent ON nmsp_parent.oid = parent.relnamespace
		JOIN pg_namespace nmsp_child ON nmsp_child.oid = child.relnamespace
		WHERE parent.relname = @table_name
			AND nmsp_parent.nspname = 'public'
			AND nmsp_child.nspname = 'public'
		ORDER BY child.relname
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&partitions)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get partitions for table %s: %w", tableName, result.Error)
	}

	return partitions, nil
}

// SequenceMetadata represents detailed metadata about how a sequence is linked to a column
type SequenceMetadata struct {
	SequenceName     string
	TableName        string
	ColumnName       string
	DependencyType   string // 'a' = auto (SERIAL), 'i' = internal (IDENTITY)
	IsIdentityColumn bool   // true if column uses GENERATED AS IDENTITY
	SequenceOwner    string // Table.Column that owns this sequence
}

// GetSequenceMetadata returns detailed metadata about how a sequence is linked to a column
// This shows the internal PostgreSQL mechanisms that link IDENTITY/SERIAL columns to sequences:
//
// For IDENTITY columns, PostgreSQL uses:
//  1. pg_depend: Creates an internal dependency (deptype='i') linking sequence to column
//  2. pg_attribute.attidentity: Marks column as identity ('d' or 'a')
//  3. pg_sequence: Stores sequence ownership information
//
// For SERIAL columns, PostgreSQL uses:
//  1. pg_depend: Creates an auto dependency (deptype='a') linking sequence to column
//  2. Column default: Uses nextval('sequence_name')
//
// When you rename a sequence using ALTER SEQUENCE...RENAME:
//   - PostgreSQL automatically updates pg_depend (OID-based, not name-based)
//   - For SERIAL: You must also update the column default expression (name-based!)
//   - For IDENTITY: No additional updates needed (uses OID internally)
//
// This is why our RenameTables function just renames sequences - PostgreSQL handles the rest
// for IDENTITY columns, but SERIAL columns may have stale defaults if renamed outside ALTER TABLE.
//
// Example:
//
//	metadata, err := dbc.GetSequenceMetadata("orders")
//	for _, m := range metadata {
//	    log.WithFields(log.Fields{
//	        "sequence":     m.SequenceName,
//	        "column":       m.ColumnName,
//	        "dep_type":     m.DependencyType,
//	        "is_identity":  m.IsIdentityColumn,
//	    }).Info("sequence linkage")
//	}
func (dbc *DB) GetSequenceMetadata(tableName string) ([]SequenceMetadata, error) {
	var metadata []SequenceMetadata

	query := `
		SELECT
			s.relname AS sequence_name,
			t.relname AS table_name,
			a.attname AS column_name,
			d.deptype AS dependency_type,
			CASE WHEN a.attidentity IN ('a', 'd') THEN true ELSE false END AS is_identity_column,
			t.relname || '.' || a.attname AS sequence_owner
		FROM pg_class s
		JOIN pg_depend d ON d.objid = s.oid
		JOIN pg_class t ON d.refobjid = t.oid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
		JOIN pg_namespace n ON n.oid = s.relnamespace
		WHERE s.relkind = 'S'
			AND t.relname = @table_name
			AND n.nspname = 'public'
			AND d.deptype IN ('a', 'i')
		ORDER BY a.attnum
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&metadata)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get sequence metadata for table %s: %w", tableName, result.Error)
	}

	return metadata, nil
}

// GetTableSequences returns all sequences owned by columns in the specified table
// This includes sequences from:
//   - SERIAL/BIGSERIAL columns (dependency type 'a' - auto)
//   - IDENTITY columns (dependency type 'i' - internal, e.g., GENERATED BY DEFAULT AS IDENTITY)
//
// Example:
//
//	sequences, err := dbc.GetTableSequences("orders")
//	if err != nil {
//	    log.WithError(err).Error("failed to get sequences")
//	}
//	for _, seq := range sequences {
//	    log.WithFields(log.Fields{
//	        "sequence": seq.SequenceName,
//	        "table":    seq.TableName,
//	        "column":   seq.ColumnName,
//	    }).Info("found sequence")
//	}
func (dbc *DB) GetTableSequences(tableName string) ([]SequenceInfo, error) {
	var sequences []SequenceInfo

	query := `
		SELECT
			s.relname AS sequence_name,
			t.relname AS table_name,
			a.attname AS column_name
		FROM pg_class s
		JOIN pg_depend d ON d.objid = s.oid
		JOIN pg_class t ON d.refobjid = t.oid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
		JOIN pg_namespace n ON n.oid = s.relnamespace
		WHERE s.relkind = 'S'
			AND t.relname = @table_name
			AND n.nspname = 'public'
			AND d.deptype IN ('a', 'i')
		ORDER BY a.attnum
	`

	result := dbc.DB.Raw(query, sql.Named("table_name", tableName)).Scan(&sequences)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get sequences for table %s: %w", tableName, result.Error)
	}

	return sequences, nil
}

// ListAllTableSequences returns all sequences owned by table columns in the public schema
// This includes sequences from:
//   - SERIAL/BIGSERIAL columns (dependency type 'a' - auto)
//   - IDENTITY columns (dependency type 'i' - internal, e.g., GENERATED BY DEFAULT AS IDENTITY)
//
// This is useful for:
// - Auditing sequence ownership across the entire database
// - Understanding which tables use auto-increment columns
// - Finding sequences that may need to be renamed or synced
// - Database documentation and inventory
//
// # Returns a map where keys are table names and values are lists of sequences
//
// Example:
//
//	allSequences, err := dbc.ListAllTableSequences()
//	if err != nil {
//	    log.WithError(err).Error("failed to list sequences")
//	}
//	for tableName, sequences := range allSequences {
//	    log.WithFields(log.Fields{
//	        "table": tableName,
//	        "count": len(sequences),
//	    }).Info("table sequences")
//	    for _, seq := range sequences {
//	        log.WithFields(log.Fields{
//	            "sequence": seq.SequenceName,
//	            "column":   seq.ColumnName,
//	        }).Debug("sequence detail")
//	    }
//	}
func (dbc *DB) ListAllTableSequences() (map[string][]SequenceInfo, error) {
	var allSequences []SequenceInfo

	query := `
		SELECT
			s.relname AS sequence_name,
			t.relname AS table_name,
			a.attname AS column_name
		FROM pg_class s
		JOIN pg_depend d ON d.objid = s.oid
		JOIN pg_class t ON d.refobjid = t.oid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
		JOIN pg_namespace n ON n.oid = s.relnamespace
		WHERE s.relkind = 'S'
			AND n.nspname = 'public'
			AND d.deptype IN ('a', 'i')
		ORDER BY t.relname, a.attnum
	`

	result := dbc.DB.Raw(query).Scan(&allSequences)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list all table sequences: %w", result.Error)
	}

	// Group sequences by table name
	sequencesByTable := make(map[string][]SequenceInfo)
	for _, seq := range allSequences {
		sequencesByTable[seq.TableName] = append(sequencesByTable[seq.TableName], seq)
	}

	log.WithFields(log.Fields{
		"tables":    len(sequencesByTable),
		"sequences": len(allSequences),
	}).Info("listed all table sequences")

	return sequencesByTable, nil
}

// TableRename represents a single table rename operation
type TableRename struct {
	From string // Source table name
	To   string // Target table name
}

// RenameTables renames multiple tables atomically in a single transaction
// This function is useful for:
// - Swapping partitioned tables with non-partitioned tables
// - Renaming related tables together to maintain consistency
// - Performing atomic schema migrations
//
// Parameters:
//   - tableRenames: Ordered list of table renames to execute (executed in the order provided)
//   - renameSequences: If true, also renames sequences owned by table columns (e.g., SERIAL, IDENTITY)
//   - renamePartitions: If true, also renames child partitions of partitioned tables
//   - renameConstraints: If true, also renames table constraints (primary keys, foreign keys, unique, check)
//   - renameIndexes: If true, also renames table indexes (including those backing constraints)
//   - dryRun: If true, only validates the operation without executing it
//
// Returns:
//   - renamedCount: Number of tables successfully renamed (0 if dryRun is true)
//   - error: Any error encountered during the operation
//
// Example:
//
//	renames := []db.TableRename{
//	    {From: "orders_old", To: "orders_backup"},      // Execute first
//	    {From: "orders_new", To: "orders"},             // Execute second
//	    {From: "orders_archive", To: "orders_old"},     // Execute third
//	}
//	count, err := dbc.RenameTables(renames, true, true, true, true, false)
//	if err != nil {
//	    log.WithError(err).Error("table rename failed")
//	}
//
// Important Notes:
//   - All renames are executed in a single transaction - either all succeed or all fail
//   - The function validates that all source tables exist before attempting renames
//   - The function checks for conflicts (target table already exists)
//   - Views, indexes, and foreign keys are automatically updated by PostgreSQL
//   - Renaming is extremely fast - PostgreSQL only updates metadata, not data
//   - When renameSequences=true, sequences follow naming pattern: newtablename_columnname_seq
//   - Sequences owned by SERIAL, BIGSERIAL, and IDENTITY columns will be renamed
//   - When renamePartitions=true, child partitions follow naming pattern: newtablename_suffix
//   - Partition renaming extracts suffix from old name and applies to new table name
//   - When renamePartitions=true AND renameSequences/Constraints/Indexes=true, partition sequences/constraints/indexes are also renamed
//   - When renameConstraints=true, constraints follow naming pattern: newtablename_suffix
//   - Constraint renaming applies to primary keys, foreign keys, unique, check, and exclusion constraints
//   - When renameIndexes=true, indexes follow naming pattern: newtablename_suffix
//   - Index renaming applies to all indexes including those backing constraints
//   - Indexes with the same name as constraints are skipped (they're renamed automatically with the constraint)
//   - Renames are executed in the order provided - caller is responsible for dependency ordering
//   - For table swaps (A->B, B->C), ensure B->C comes before A->B in the array
func (dbc *DB) RenameTables(tableRenames []TableRename, renameSequences bool, renamePartitions bool, renameConstraints bool, renameIndexes bool, dryRun bool) (int, error) {
	if len(tableRenames) == 0 {
		return 0, fmt.Errorf("no tables to rename")
	}

	log.WithFields(log.Fields{
		"count":   len(tableRenames),
		"dry_run": dryRun,
	}).Info("starting table rename operation")

	// Convert to map for easier lookups during validation and discovery
	tableRenameMap := make(map[string]string)
	var sourceNames []string
	var targetNames []string
	for _, rename := range tableRenames {
		if rename.From == "" || rename.To == "" {
			return 0, fmt.Errorf("invalid rename: both From and To must be specified")
		}
		if _, exists := tableRenameMap[rename.From]; exists {
			return 0, fmt.Errorf("duplicate source table: %s", rename.From)
		}
		tableRenameMap[rename.From] = rename.To
		sourceNames = append(sourceNames, rename.From)
		targetNames = append(targetNames, rename.To)
	}

	// Step 1: Validate all source tables exist and check for conflicts

	// Check that all source tables exist
	for source := range tableRenameMap {
		var exists bool
		query := `
			SELECT EXISTS (
				SELECT 1 FROM pg_tables
				WHERE schemaname = 'public' AND tablename = @table_name
			)
		`
		result := dbc.DB.Raw(query, sql.Named("table_name", source)).Scan(&exists)
		if result.Error != nil {
			return 0, fmt.Errorf("failed to check if table %s exists: %w", source, result.Error)
		}
		if !exists {
			return 0, fmt.Errorf("source table %s does not exist", source)
		}
	}

	// Check for conflicts - ensure no target tables already exist
	// (unless they're also being renamed as part of this operation)
	for source, target := range tableRenameMap {
		// Skip check if this target is also a source (table swap scenario)
		if _, isAlsoSource := tableRenameMap[target]; isAlsoSource {
			log.WithFields(log.Fields{
				"source": source,
				"target": target,
			}).Debug("target is also a source - table swap detected")
			continue
		}

		var exists bool
		query := `
			SELECT EXISTS (
				SELECT 1 FROM pg_tables
				WHERE schemaname = 'public' AND tablename = @table_name
			)
		`
		result := dbc.DB.Raw(query, sql.Named("table_name", target)).Scan(&exists)
		if result.Error != nil {
			return 0, fmt.Errorf("failed to check if target table %s exists: %w", target, result.Error)
		}
		if exists {
			return 0, fmt.Errorf("target table %s already exists (conflict with rename from %s)", target, source)
		}
	}

	log.WithFields(log.Fields{
		"sources": sourceNames,
		"targets": targetNames,
	}).Info("validation passed - all source tables exist and no conflicts detected")

	// Step 2: Find sequences that need to be renamed (if requested)
	sequenceRenames := make(map[string]string)
	if renameSequences {
		for source, target := range tableRenameMap {
			sequences, err := dbc.GetTableSequences(source)
			if err != nil {
				return 0, fmt.Errorf("failed to get sequences for table %s: %w", source, err)
			}

			for _, seq := range sequences {
				// Generate new sequence name following PostgreSQL convention
				// old: oldtable_columnname_seq -> new: newtable_columnname_seq
				newSeqName := fmt.Sprintf("%s_%s_seq", target, seq.ColumnName)
				sequenceRenames[seq.SequenceName] = newSeqName

				log.WithFields(log.Fields{
					"table":        source,
					"column":       seq.ColumnName,
					"old_sequence": seq.SequenceName,
					"new_sequence": newSeqName,
				}).Debug("will rename sequence")
			}
		}

		if len(sequenceRenames) > 0 {
			log.WithField("count", len(sequenceRenames)).Info("found sequences to rename")
		}
	}

	// Step 2b: Find partitions that need to be renamed (if requested)
	partitionRenames := make(map[string]string)
	if renamePartitions {
		for source, target := range tableRenameMap {
			partitions, err := dbc.GetTablePartitions(source)
			if err != nil {
				return 0, fmt.Errorf("failed to get partitions for table %s: %w", source, err)
			}

			for _, part := range partitions {
				// Extract suffix from old partition name
				// old: oldtable_2024_01_01 -> suffix: _2024_01_01
				// new: newtable_2024_01_01
				suffix := strings.TrimPrefix(part.PartitionName, source)
				if suffix == part.PartitionName {
					// Partition name doesn't start with parent table name - skip
					log.WithFields(log.Fields{
						"partition": part.PartitionName,
						"parent":    source,
					}).Warn("partition name doesn't start with parent table name - skipping")
					continue
				}

				newPartName := target + suffix
				partitionRenames[part.PartitionName] = newPartName

				log.WithFields(log.Fields{
					"parent":        source,
					"old_partition": part.PartitionName,
					"new_partition": newPartName,
					"suffix":        suffix,
				}).Debug("will rename partition")
			}
		}

		if len(partitionRenames) > 0 {
			log.WithField("count", len(partitionRenames)).Info("found partitions to rename")

			// Also find sequences/constraints/indexes for partition tables
			// This allows renaming them when the partition is renamed
			if renameSequences {
				for oldPartName, newPartName := range partitionRenames {
					partSeqs, err := dbc.GetTableSequences(oldPartName)
					if err != nil {
						return 0, fmt.Errorf("failed to get sequences for partition %s: %w", oldPartName, err)
					}

					for _, seq := range partSeqs {
						newSeqName := fmt.Sprintf("%s_%s_seq", newPartName, seq.ColumnName)
						sequenceRenames[seq.SequenceName] = newSeqName

						log.WithFields(log.Fields{
							"partition":    oldPartName,
							"column":       seq.ColumnName,
							"old_sequence": seq.SequenceName,
							"new_sequence": newSeqName,
						}).Debug("will rename partition sequence")
					}
				}
			}
		}
	}

	// Step 2c: Find constraints that need to be renamed (if requested)
	constraintRenames := make(map[string]map[string]string) // map[tableName]map[oldConstraint]newConstraint
	if renameConstraints {
		for source, target := range tableRenameMap {
			constraints, err := dbc.GetTableConstraints(source)
			if err != nil {
				return 0, fmt.Errorf("failed to get constraints for table %s: %w", source, err)
			}

			for _, cons := range constraints {
				// Extract suffix from old constraint name if it starts with the table name
				// old: oldtable_pkey -> suffix: _pkey
				// new: newtable_pkey
				suffix := strings.TrimPrefix(cons.ConstraintName, source)
				if suffix == cons.ConstraintName {
					// Constraint name doesn't start with table name - skip
					log.WithFields(log.Fields{
						"constraint": cons.ConstraintName,
						"table":      source,
					}).Debug("constraint name doesn't start with table name - skipping")
					continue
				}

				newConsName := target + suffix

				// Initialize map for this table if needed
				if constraintRenames[source] == nil {
					constraintRenames[source] = make(map[string]string)
				}
				constraintRenames[source][cons.ConstraintName] = newConsName

				log.WithFields(log.Fields{
					"table":          source,
					"old_constraint": cons.ConstraintName,
					"new_constraint": newConsName,
					"type":           cons.ConstraintType,
					"suffix":         suffix,
				}).Debug("will rename constraint")
			}
		}

		totalConstraints := 0
		for _, consMap := range constraintRenames {
			totalConstraints += len(consMap)
		}
		if totalConstraints > 0 {
			log.WithField("count", totalConstraints).Info("found constraints to rename")
		}

		// Also find constraints for partition tables
		if renamePartitions && len(partitionRenames) > 0 {
			for oldPartName, newPartName := range partitionRenames {
				partCons, err := dbc.GetTableConstraints(oldPartName)
				if err != nil {
					return 0, fmt.Errorf("failed to get constraints for partition %s: %w", oldPartName, err)
				}

				for _, cons := range partCons {
					suffix := strings.TrimPrefix(cons.ConstraintName, oldPartName)
					if suffix == cons.ConstraintName {
						log.WithFields(log.Fields{
							"constraint": cons.ConstraintName,
							"partition":  oldPartName,
						}).Debug("constraint name doesn't start with partition name - skipping")
						continue
					}

					newConsName := newPartName + suffix

					if constraintRenames[oldPartName] == nil {
						constraintRenames[oldPartName] = make(map[string]string)
					}
					constraintRenames[oldPartName][cons.ConstraintName] = newConsName

					log.WithFields(log.Fields{
						"partition":      oldPartName,
						"old_constraint": cons.ConstraintName,
						"new_constraint": newConsName,
						"type":           cons.ConstraintType,
						"suffix":         suffix,
					}).Debug("will rename partition constraint")
				}
			}
		}
	}

	// Step 2d: Find indexes that need to be renamed (if requested)
	indexRenames := make(map[string]map[string]string) // map[tableName]map[oldIndex]newIndex
	if renameIndexes {
		for source, target := range tableRenameMap {
			indexes, err := dbc.GetTableIndexes(source)
			if err != nil {
				return 0, fmt.Errorf("failed to get indexes for table %s: %w", source, err)
			}

			for _, idx := range indexes {
				// Extract suffix from old index name if it starts with the table name
				// old: oldtable_pkey -> suffix: _pkey
				// new: newtable_pkey
				suffix := strings.TrimPrefix(idx.IndexName, source)
				if suffix == idx.IndexName {
					// Index name doesn't start with table name - skip
					log.WithFields(log.Fields{
						"index": idx.IndexName,
						"table": source,
					}).Debug("index name doesn't start with table name - skipping")
					continue
				}

				newIdxName := target + suffix

				// Initialize map for this table if needed
				if indexRenames[source] == nil {
					indexRenames[source] = make(map[string]string)
				}
				indexRenames[source][idx.IndexName] = newIdxName

				log.WithFields(log.Fields{
					"table":      source,
					"old_index":  idx.IndexName,
					"new_index":  newIdxName,
					"is_unique":  idx.IsUnique,
					"is_primary": idx.IsPrimary,
					"suffix":     suffix,
				}).Debug("will rename index")
			}
		}

		totalIndexes := 0
		for _, idxMap := range indexRenames {
			totalIndexes += len(idxMap)
		}
		if totalIndexes > 0 {
			log.WithField("count", totalIndexes).Info("found indexes to rename")
		}

		// Also find indexes for partition tables
		if renamePartitions && len(partitionRenames) > 0 {
			for oldPartName, newPartName := range partitionRenames {
				partIdxs, err := dbc.GetTableIndexes(oldPartName)
				if err != nil {
					return 0, fmt.Errorf("failed to get indexes for partition %s: %w", oldPartName, err)
				}

				for _, idx := range partIdxs {
					suffix := strings.TrimPrefix(idx.IndexName, oldPartName)
					if suffix == idx.IndexName {
						log.WithFields(log.Fields{
							"index":     idx.IndexName,
							"partition": oldPartName,
						}).Debug("index name doesn't start with partition name - skipping")
						continue
					}

					newIdxName := newPartName + suffix

					if indexRenames[oldPartName] == nil {
						indexRenames[oldPartName] = make(map[string]string)
					}
					indexRenames[oldPartName][idx.IndexName] = newIdxName

					log.WithFields(log.Fields{
						"partition":  oldPartName,
						"old_index":  idx.IndexName,
						"new_index":  newIdxName,
						"is_unique":  idx.IsUnique,
						"is_primary": idx.IsPrimary,
						"suffix":     suffix,
					}).Debug("will rename partition index")
				}
			}
		}
	}

	// Step 3: Dry run - report what would be renamed
	if dryRun {
		log.Info("[DRY RUN] would rename the following tables:")
		for _, rename := range tableRenames {
			log.WithFields(log.Fields{
				"from": rename.From,
				"to":   rename.To,
			}).Info("[DRY RUN] table rename")
		}

		if len(partitionRenames) > 0 {
			log.Info("[DRY RUN] would rename the following partitions:")
			for oldPart, newPart := range partitionRenames {
				log.WithFields(log.Fields{
					"from": oldPart,
					"to":   newPart,
				}).Info("[DRY RUN] partition rename")
			}
		}

		if len(sequenceRenames) > 0 {
			log.Info("[DRY RUN] would rename the following sequences:")
			for oldSeq, newSeq := range sequenceRenames {
				log.WithFields(log.Fields{
					"from": oldSeq,
					"to":   newSeq,
				}).Info("[DRY RUN] sequence rename")
			}
		}

		totalConstraints := 0
		for _, consMap := range constraintRenames {
			totalConstraints += len(consMap)
		}
		if totalConstraints > 0 {
			log.Info("[DRY RUN] would rename the following constraints:")
			for tableName, consMap := range constraintRenames {
				for oldCons, newCons := range consMap {
					log.WithFields(log.Fields{
						"table": tableName,
						"from":  oldCons,
						"to":    newCons,
					}).Info("[DRY RUN] constraint rename")
				}
			}
		}

		totalIndexes := 0
		for _, idxMap := range indexRenames {
			totalIndexes += len(idxMap)
		}
		if totalIndexes > 0 {
			log.Info("[DRY RUN] would rename the following indexes:")
			for tableName, idxMap := range indexRenames {
				for oldIdx, newIdx := range idxMap {
					log.WithFields(log.Fields{
						"table": tableName,
						"from":  oldIdx,
						"to":    newIdx,
					}).Info("[DRY RUN] index rename")
				}
			}
		}

		return 0, nil
	}

	// Step 4: Execute all renames in a single transaction
	tx := dbc.DB.Begin()
	if tx.Error != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Use defer to handle rollback on error
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	// Execute each table rename in the order provided
	renamedCount := 0
	for _, rename := range tableRenames {
		renameSQL := fmt.Sprintf("ALTER TABLE %s RENAME TO %s", pq.QuoteIdentifier(rename.From), pq.QuoteIdentifier(rename.To))

		log.WithFields(log.Fields{
			"from": rename.From,
			"to":   rename.To,
		}).Info("renaming table")

		result := tx.Exec(renameSQL)
		if result.Error != nil {
			return 0, fmt.Errorf("failed to rename table %s to %s: %w", rename.From, rename.To, result.Error)
		}

		renamedCount++
	}

	// Execute each partition rename
	partitionsRenamed := 0
	for oldPart, newPart := range partitionRenames {
		renameSQL := fmt.Sprintf("ALTER TABLE %s RENAME TO %s", pq.QuoteIdentifier(oldPart), pq.QuoteIdentifier(newPart))

		log.WithFields(log.Fields{
			"from": oldPart,
			"to":   newPart,
		}).Info("renaming partition")

		result := tx.Exec(renameSQL)
		if result.Error != nil {
			return 0, fmt.Errorf("failed to rename partition %s to %s: %w", oldPart, newPart, result.Error)
		}

		partitionsRenamed++
	}

	// Execute each sequence rename
	// Sequences are renamed in the order discovered (matching table rename order)
	sequencesRenamed := 0

	for oldSeq, newSeq := range sequenceRenames {
		renameSQL := fmt.Sprintf("ALTER SEQUENCE %s RENAME TO %s", pq.QuoteIdentifier(oldSeq), pq.QuoteIdentifier(newSeq))

		log.WithFields(log.Fields{
			"from": oldSeq,
			"to":   newSeq,
		}).Info("renaming sequence")

		result := tx.Exec(renameSQL)
		if result.Error != nil {
			return 0, fmt.Errorf("failed to rename sequence %s to %s: %w", oldSeq, newSeq, result.Error)
		}

		sequencesRenamed++
	}

	// Execute each constraint rename
	constraintsRenamed := 0

	for tableName, consMap := range constraintRenames {
		// Get the new table name (in case table or partition was renamed)
		newTableName := tableName
		if renamed, exists := tableRenameMap[tableName]; exists {
			newTableName = renamed
		} else if renamed, exists := partitionRenames[tableName]; exists {
			newTableName = renamed
		}

		for oldCons, newCons := range consMap {
			renameSQL := fmt.Sprintf("ALTER TABLE %s RENAME CONSTRAINT %s TO %s", pq.QuoteIdentifier(newTableName), pq.QuoteIdentifier(oldCons), pq.QuoteIdentifier(newCons))

			log.WithFields(log.Fields{
				"table": newTableName,
				"from":  oldCons,
				"to":    newCons,
			}).Info("renaming constraint")

			result := tx.Exec(renameSQL)
			if result.Error != nil {
				return 0, fmt.Errorf("failed to rename constraint %s to %s on table %s: %w", oldCons, newCons, newTableName, result.Error)
			}

			constraintsRenamed++
		}
	}

	// Build a set of constraint names that were renamed
	// (to skip indexes with the same name, as they're renamed automatically with the constraint)
	renamedConstraintNames := make(map[string]bool)
	for _, consMap := range constraintRenames {
		for oldCons := range consMap {
			renamedConstraintNames[oldCons] = true
		}
	}

	// Execute each index rename
	indexesRenamed := 0

	for tableName, idxMap := range indexRenames {
		for oldIdx, newIdx := range idxMap {
			// Skip if this index has the same name as a constraint we renamed
			// PostgreSQL automatically renames the backing index when renaming PRIMARY KEY or UNIQUE constraints
			if renamedConstraintNames[oldIdx] {
				log.WithFields(log.Fields{
					"table": tableName,
					"index": oldIdx,
				}).Debug("skipping index - already renamed as part of constraint rename")
				continue
			}

			renameSQL := fmt.Sprintf("ALTER INDEX %s RENAME TO %s", pq.QuoteIdentifier(oldIdx), pq.QuoteIdentifier(newIdx))

			log.WithFields(log.Fields{
				"table": tableName,
				"from":  oldIdx,
				"to":    newIdx,
			}).Info("renaming index")

			result := tx.Exec(renameSQL)
			if result.Error != nil {
				return 0, fmt.Errorf("failed to rename index %s to %s: %w", oldIdx, newIdx, result.Error)
			}

			indexesRenamed++
		}
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true

	log.WithFields(log.Fields{
		"renamed_tables":      renamedCount,
		"renamed_partitions":  partitionsRenamed,
		"renamed_sequences":   sequencesRenamed,
		"renamed_constraints": constraintsRenamed,
		"renamed_indexes":     indexesRenamed,
	}).Info("rename operation completed successfully")

	return renamedCount, nil
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
	query := fmt.Sprintf("SELECT MAX(%s) FROM %s", pq.QuoteIdentifier(columnName), pq.QuoteIdentifier(tableName))
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
	alterSQL := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s RESTART WITH %d", pq.QuoteIdentifier(tableName), pq.QuoteIdentifier(columnName), nextValue)
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
