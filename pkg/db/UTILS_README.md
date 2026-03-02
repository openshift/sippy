# Database Utilities

This package provides utility functions for database operations including schema verification and data migration.

## Overview

The utilities in `utils.go` provide safe, validated operations for working with database tables, particularly useful for:
- Schema migration and validation
- Data migration between tables
- Partition management workflows
- Table consolidation and archival

## Functions

### VerifyTablesHaveSameColumns

Verifies that two tables have identical column definitions with configurable verification options.

```go
// Full verification (default) - checks all aspects
err := dbc.VerifyTablesHaveSameColumns("source_table", "target_table", DefaultColumnVerificationOptions())
if err != nil {
    log.WithError(err).Error("tables have different schemas")
}

// Data migration verification - only checks names and types
err := dbc.VerifyTablesHaveSameColumns("source_table", "target_table", DataMigrationColumnVerificationOptions())
if err != nil {
    log.WithError(err).Error("incompatible schemas for migration")
}
```

**Verification Options:**

| Option | DefaultColumnVerificationOptions | DataMigrationColumnVerificationOptions |
|--------|----------------------------------|---------------------------------------|
| Column names | ✓ | ✓ |
| Data types | ✓ | ✓ |
| NOT NULL constraints | ✓ | ✗ |
| DEFAULT values | ✓ | ✗ |
| Column ordering | ✓ | ✓ |

**Custom Options:**
```go
opts := ColumnVerificationOptions{
    CheckNullable: true,   // Verify NOT NULL constraints match
    CheckDefaults: false,  // Skip default value comparison
    CheckOrder:    true,   // Verify column order matches
}
err := dbc.VerifyTablesHaveSameColumns("table1", "table2", opts)
```

**Always Compared:**
- Column names (always required to match)
- Data types (with normalization, always required to match)

**Optionally Compared:**
- NOT NULL constraints (controlled by `CheckNullable`)
- DEFAULT values (controlled by `CheckDefaults`)
- Column ordering/position (controlled by `CheckOrder`)

**Returns:**
- `nil` if tables have compatible schemas
- Error with detailed description of differences

**Use Cases:**
- **Full verification** (`DefaultColumnVerificationOptions`):
  - Verifying partitions match parent table exactly
  - Ensuring replicas have identical structures
  - Validating table clones or backups

- **Migration verification** (`DataMigrationColumnVerificationOptions`):
  - Pre-migration schema validation
  - Verifying data can be copied between tables
  - Checking compatibility for INSERT INTO ... SELECT operations

**Why Skip Nullable/Defaults for Migrations?**

When migrating data with `INSERT INTO target SELECT * FROM source`, PostgreSQL only requires that:
- Column names exist in both tables
- Data types are compatible

Nullable constraints and default values don't affect the data copy itself, so checking them is optional for migrations.

---

### MigrateTableData

Migrates all data from one table to another after verifying schemas match.

```go
// Dry run first
rowsMigrated, err := dbc.MigrateTableData("source_table", "target_table", true)

// Actual migration
rowsMigrated, err := dbc.MigrateTableData("source_table", "target_table", false)
if err != nil {
    log.WithError(err).Error("migration failed")
}
```

**Process:**
1. Verifies schemas match using `VerifyTablesHaveSameColumns`
2. Checks row counts in both tables
3. Performs `INSERT INTO target SELECT * FROM source`
4. Verifies row counts after migration
5. Logs all steps with detailed metrics

**Parameters:**
- `sourceTable` - Table to copy data from
- `targetTable` - Table to copy data to
- `dryRun` - If true, only verifies without copying data

**Returns:**
- `rowsMigrated` - Number of rows successfully migrated (0 if dry run)
- `error` - Any error encountered during migration

**Features:**
- Atomic operation (single INSERT statement)
- Dry-run support for safety
- Pre and post verification
- Comprehensive logging
- Handles empty source tables gracefully

**Safety:**
- DOES NOT truncate target table (appends data)
- DOES NOT drop source table
- Fails fast if schemas don't match
- Warns on row count mismatches

**Use Cases:**
- Migrating detached partitions to archive tables
- Consolidating multiple tables into one
- Moving data between environments
- Table restructuring workflows

---

### MigrateTableDataRange

Migrates data within a specific date range from one table to another after verifying schemas match.

```go
startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

// Dry run first
rowsMigrated, err := dbc.MigrateTableDataRange("source_table", "target_table", "created_at", startDate, endDate, true)

// Actual migration
rowsMigrated, err := dbc.MigrateTableDataRange("source_table", "target_table", "created_at", startDate, endDate, false)
if err != nil {
    log.WithError(err).Error("migration failed")
}
```

**Process:**
1. Validates date range (endDate must be after startDate)
2. Verifies schemas match using `VerifyTablesHaveSameColumns`
3. Checks if target table is RANGE partitioned and verifies all necessary partitions exist for the date range
4. Counts rows in source table within date range
5. Performs `INSERT INTO target SELECT * FROM source WHERE date_column >= start AND date_column < end`
6. Verifies row counts after migration
7. Logs all steps with detailed metrics

**Parameters:**
- `sourceTable` - Table to copy data from
- `targetTable` - Table to copy data to
- `dateColumn` - Column name to filter by date (e.g., "created_at")
- `startDate` - Start of date range (inclusive, >=)
- `endDate` - End of date range (exclusive, <)
- `dryRun` - If true, only verifies without copying data

**Returns:**
- `rowsMigrated` - Number of rows successfully migrated (0 if dry run)
- `error` - Any error encountered during migration

**Features:**
- Atomic operation (single INSERT statement)
- Dry-run support for safety
- Pre and post verification
- Comprehensive logging
- Handles empty date ranges gracefully
- Date range validation
- Automatic partition coverage verification for RANGE partitioned tables
- Prevents migration failures due to missing partitions

**Safety:**
- DOES NOT truncate target table (appends data)
- DOES NOT drop source table
- Fails fast if schemas don't match
- Warns on row count mismatches
- Validates date range before execution

**Use Cases:**
- Migrating large tables incrementally (month by month, year by year)
- Testing migrations with a subset of data before full migration
- Moving specific time periods to archive tables
- Backfilling historical data into partitioned tables
- Reducing lock contention by migrating in smaller batches
- Being able to pause and resume large migrations

**Example - Incremental Monthly Migration:**
```go
// Migrate data month by month for 2024
for month := 1; month <= 12; month++ {
    startDate := time.Date(2024, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
    endDate := startDate.AddDate(0, 1, 0)

    rows, err := dbc.MigrateTableDataRange("orders", "orders_new", "order_date", startDate, endDate, false)
    if err != nil {
        log.WithError(err).WithField("month", month).Error("failed")
        continue
    }
    log.WithField("rows", rows).Info("month migrated")
}
```

---

### GetTableRowCount

Returns the number of rows in a table.

```go
count, err := dbc.GetTableRowCount("table_name")
if err != nil {
    log.WithError(err).Error("failed to get row count")
}
log.WithField("count", count).Info("table row count")
```

**Use Cases:**
- Pre-migration verification
- Monitoring table growth
- Validating migration success
- Capacity planning

---

### SyncIdentityColumn

Synchronizes the IDENTITY sequence for a column to match the current maximum value in the table.

```go
err := dbc.SyncIdentityColumn("table_name", "id")
if err != nil {
    log.WithError(err).Error("failed to sync identity column")
}
```

**How It Works**:
1. Queries the current maximum value of the column: `SELECT MAX(column) FROM table`
2. Calculates the next value (max + 1, or 1 if table is empty/all NULL)
3. Executes `ALTER TABLE table_name ALTER COLUMN column_name RESTART WITH next_value`
4. Logs the operation with the new sequence value

**Returns**: Error if the operation fails

**Use Cases**:
- After migrating data to a partitioned table with IDENTITY columns
- After bulk inserting data with explicit ID values
- When the IDENTITY sequence is out of sync with actual data
- After using `MigrateTableData` to copy data between tables

**Example Workflow**:
```go
// Migrate data from old table to new partitioned table
rows, err := dbc.MigrateTableData("old_table", "new_partitioned_table", false)
if err != nil {
    log.Fatal(err)
}

// Sync the IDENTITY sequence so new inserts start at the correct value
err = dbc.SyncIdentityColumn("new_partitioned_table", "id")
if err != nil {
    log.Fatal(err)
}

log.Info("Migration complete - sequence synchronized")
```

**Important Notes**:
- The column must be an IDENTITY column (created with `GENERATED BY DEFAULT AS IDENTITY`)
- This does NOT work with traditional PostgreSQL sequences created separately
- For traditional sequences, use: `SELECT setval('sequence_name', (SELECT MAX(id) FROM table))`
- Safe to run multiple times - idempotent operation

---

### GetPartitionStrategy

Checks if a table is partitioned and returns its partition strategy.

```go
strategy, err := dbc.GetPartitionStrategy("table_name")
if err != nil {
    log.WithError(err).Error("failed to check partition strategy")
}

if strategy == "" {
    log.Info("table is not partitioned")
} else if strategy == db.PartitionStrategyRange {
    log.Info("table uses RANGE partitioning")
}
```

**Returns**:
- Empty string `""` if table is not partitioned
- `PartitionStrategyRange`, `PartitionStrategyList`, `PartitionStrategyHash`, or `"UNKNOWN"` if partitioned

**Constants**:
```go
db.PartitionStrategyRange  // "RANGE"
db.PartitionStrategyList   // "LIST"
db.PartitionStrategyHash   // "HASH"
```

**Use Cases**:
- Before migrations, check if target table is partitioned
- Determine which partition management operations are applicable
- Validate table structure before data operations

**Example**:
```go
strategy, err := dbc.GetPartitionStrategy("orders")
if err != nil {
    log.Fatal(err)
}

switch strategy {
case db.PartitionStrategyRange:
    log.Info("table uses RANGE partitioning")
case db.PartitionStrategyList:
    log.Info("table uses LIST partitioning")
case db.PartitionStrategyHash:
    log.Info("table uses HASH partitioning")
case "":
    log.Info("table is not partitioned")
}
```

---

### VerifyPartitionCoverage

Verifies that all necessary partitions exist for a date range.

```go
startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

err := dbc.VerifyPartitionCoverage("orders", startDate, endDate)
if err != nil {
    // Prints: missing partitions for dates: [2024-01-15 2024-01-16]
    log.WithError(err).Error("partition coverage check failed")
}
```

**How It Works**:
1. Queries all existing partitions for the table
2. Checks that a partition exists for each day in the range [startDate, endDate)
3. Returns error listing all missing partition dates
4. Logs successful verification with partition count

**Assumptions**:
- Daily partitions with naming convention: `tablename_YYYY_MM_DD`
- Partitions are created for each calendar day
- Date range uses same convention as other functions (startDate inclusive, endDate exclusive)

**Returns**: Error if any partitions are missing, nil if all exist

**Use Cases**:
- Before migrating data to partitioned tables
- Verifying partition creation scripts completed successfully
- Pre-flight checks before bulk data operations
- Automated partition management validation

**Example - Create missing partitions**:
```go
import "github.com/openshift/sippy/pkg/db/partitions"

// Check if partitions exist
err := dbc.VerifyPartitionCoverage("orders", startDate, endDate)
if err != nil {
    log.WithError(err).Warn("missing partitions - creating them")

    // Create missing partitions using partitions package
    count, err := partitions.CreateMissingPartitions(dbc, "orders", startDate, endDate, false)
    if err != nil {
        log.Fatal(err)
    }
    log.WithField("created", count).Info("created missing partitions")
}

// Now verify again
if err := dbc.VerifyPartitionCoverage("orders", startDate, endDate); err != nil {
    log.Fatal("still missing partitions after creation")
}
```

---

## Helper Types

### ColumnInfo

Represents metadata about a database column.

```go
type ColumnInfo struct {
    ColumnName    string
    DataType      string
    IsNullable    string
    ColumnDefault sql.NullString
    OrdinalPos    int
}
```

---

### PartitionStrategy

Defines the partitioning strategy type for PostgreSQL partitioned tables.

```go
type PartitionStrategy string

const (
    PartitionStrategyRange PartitionStrategy = "RANGE"
    PartitionStrategyList  PartitionStrategy = "LIST"
    PartitionStrategyHash  PartitionStrategy = "HASH"
)
```

**Usage**:
- Returned by `GetPartitionStrategy()` to indicate table's partitioning type
- Used by the `partitions` package in `PartitionConfig.Strategy`
- Can be compared directly with constants or used in switch statements

**Example**:
```go
strategy, err := dbc.GetPartitionStrategy("orders")
if err != nil {
    return err
}

switch strategy {
case PartitionStrategyRange:
    // Handle RANGE partitioned table
case PartitionStrategyList:
    // Handle LIST partitioned table
case PartitionStrategyHash:
    // Handle HASH partitioned table
case "":
    // Table is not partitioned
}
```

---

### ColumnVerificationOptions

Controls which aspects of column definitions to verify when comparing tables.

```go
type ColumnVerificationOptions struct {
    CheckNullable bool  // Verify that columns have matching nullable constraints
    CheckDefaults bool  // Verify that columns have matching default values
    CheckOrder    bool  // Verify that columns are in the same ordinal position
}
```

**Predefined Options:**

```go
// DefaultColumnVerificationOptions - Full verification (all checks enabled)
opts := DefaultColumnVerificationOptions()
// Returns: ColumnVerificationOptions{CheckNullable: true, CheckDefaults: true, CheckOrder: true}

// DataMigrationColumnVerificationOptions - Minimal verification for migrations
opts := DataMigrationColumnVerificationOptions()
// Returns: ColumnVerificationOptions{CheckNullable: false, CheckDefaults: false, CheckOrder: true}
```

**Usage**:
- Used by `VerifyTablesHaveSameColumns()` to control verification behavior
- Column names and data types are **always** verified regardless of options
- Optional checks allow flexibility for different use cases

**Example - Custom Options**:
```go
// Custom verification: check types and nullability, skip defaults and order
opts := ColumnVerificationOptions{
    CheckNullable: true,
    CheckDefaults: false,
    CheckOrder:    false,
}
err := dbc.VerifyTablesHaveSameColumns("table1", "table2", opts)
```

**When to Use Each Option:**

| Scenario | Recommended Options |
|----------|-------------------|
| Verifying partition matches parent | `DefaultColumnVerificationOptions()` |
| Pre-migration compatibility check | `DataMigrationColumnVerificationOptions()` |
| Validating table replicas | `DefaultColumnVerificationOptions()` |
| Testing table clones | `DefaultColumnVerificationOptions()` |

---

## Data Type Normalization

The utilities normalize PostgreSQL data type names for accurate comparison:

| PostgreSQL Type | Normalized |
|----------------|------------|
| `character varying` | `varchar` |
| `integer`, `int4` | `int` |
| `int8`, `bigserial` | `bigint` |
| `serial` | `int` |
| `timestamp without time zone` | `timestamp` |
| `timestamp with time zone` | `timestamptz` |
| `double precision` | `float8` |
| `boolean` | `bool` |

This ensures that functionally equivalent types are treated as identical during comparison.

---

## Usage Examples

### Basic Migration

```go
// Step 1: Verify schemas match
err := dbc.VerifyTablesHaveSameColumns("source_table", "target_table")
if err != nil {
    log.Fatal(err)
}

// Step 2: Dry run
_, err = dbc.MigrateTableData("source_table", "target_table", true)
if err != nil {
    log.Fatal(err)
}

// Step 3: Actual migration
rows, err := dbc.MigrateTableData("source_table", "target_table", false)
log.WithField("rows", rows).Info("migration completed")
```

---

### Partition to Archive Migration

```go
// Migrate detached partition to archive table
partition := "test_analysis_by_job_by_dates_2024_01_15"
archive := "test_analysis_archive"

rows, err := dbc.MigrateTableData(partition, archive, false)
if err != nil {
    log.WithError(err).Error("migration failed")
    return
}

log.WithFields(log.Fields{
    "partition": partition,
    "rows":      rows,
}).Info("partition migrated to archive - safe to drop")
```

---

### Batch Migration

```go
partitions := []string{
    "table_2024_01_15",
    "table_2024_01_16",
    "table_2024_01_17",
}

var totalRows int64
for _, partition := range partitions {
    rows, err := dbc.MigrateTableData(partition, "archive_table", false)
    if err != nil {
        log.WithError(err).WithField("partition", partition).Error("failed")
        continue
    }
    totalRows += rows
}

log.WithField("total_rows", totalRows).Info("batch migration completed")
```

---

### Migration with Backup

```go
// Create backup before migration
_, err := dbc.MigrateTableData("target_table", "backup_table", false)
if err != nil {
    log.Fatal("backup failed")
}

// Perform migration
rows, err := dbc.MigrateTableData("source_table", "target_table", false)
if err != nil {
    log.Error("migration failed - restore from backup if needed")
    return
}

log.Info("migration successful - backup can be dropped")
```

---

### Incremental Migration by Date Range

```go
// Migrate large table incrementally by month to reduce lock contention
for month := 1; month <= 12; month++ {
    startDate := time.Date(2024, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
    endDate := startDate.AddDate(0, 1, 0)  // First day of next month

    log.WithFields(log.Fields{
        "month": time.Month(month).String(),
        "start": startDate.Format("2006-01-02"),
        "end":   endDate.Format("2006-01-02"),
    }).Info("migrating month")

    rows, err := dbc.MigrateTableDataRange("large_table", "large_table_new", "created_at", startDate, endDate, false)
    if err != nil {
        log.WithError(err).WithField("month", month).Error("migration failed")
        continue
    }

    log.WithFields(log.Fields{
        "month": month,
        "rows":  rows,
    }).Info("month migrated successfully")
}
```

---

### Migrate Specific Date Range to Archive

```go
// Move Q1 2024 data to archive table
startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)

// Dry run first
_, err := dbc.MigrateTableDataRange("orders", "orders_archive", "order_date", startDate, endDate, true)
if err != nil {
    log.Fatal(err)
}

// Actual migration
rows, err := dbc.MigrateTableDataRange("orders", "orders_archive", "order_date", startDate, endDate, false)
log.WithFields(log.Fields{
    "rows":       rows,
    "start_date": startDate.Format("2006-01-02"),
    "end_date":   endDate.Format("2006-01-02"),
}).Info("Q1 2024 data archived")
```

---

## Best Practices

### Always Use Dry Run First

```go
// GOOD: Verify before executing
_, err := dbc.MigrateTableData(source, target, true)
if err != nil {
    return err
}
rows, err := dbc.MigrateTableData(source, target, false)

// BAD: Direct migration without verification
rows, err := dbc.MigrateTableData(source, target, false)
```

### Verify Schemas Explicitly

```go
// GOOD: Explicit verification with clear error handling
if err := dbc.VerifyTablesHaveSameColumns(source, target); err != nil {
    log.WithError(err).Error("schema mismatch - cannot proceed")
    return err
}

// Migration happens in MigrateTableData, but explicit check is clearer
```

### Check Row Counts

```go
// GOOD: Verify counts before and after
sourceBefore, _ := dbc.GetTableRowCount(source)
targetBefore, _ := dbc.GetTableRowCount(target)

rows, err := dbc.MigrateTableData(source, target, false)

targetAfter, _ := dbc.GetTableRowCount(target)
expected := targetBefore + sourceBefore
if targetAfter != expected {
    log.Error("row count mismatch!")
}
```

### Use Transactions for Multiple Operations

When performing multiple related operations, use database transactions:

```go
tx := dbc.DB.Begin()

// Perform operations
// ...

if err != nil {
    tx.Rollback()
    return err
}

tx.Commit()
```

---

## Error Handling

All functions return detailed errors:

```go
err := dbc.VerifyTablesHaveSameColumns("table1", "table2")
if err != nil {
    // Error contains specific differences:
    // "column name mismatch: columns in table1 but not in table2: [col1, col2]"
    // "column definition mismatches: column foo: type mismatch (table1: int vs table2: bigint)"
}
```

Common errors:
- **Schema mismatch**: Tables have different columns or types
- **Table not found**: One or both tables don't exist
- **Permission denied**: Insufficient database privileges
- **Row count mismatch**: Data integrity issue after migration

---

## Testing

Unit tests cover:
- Data type normalization
- ColumnInfo struct
- Parameter validation

Run tests:
```bash
go test ./pkg/db -v
```

Integration tests require a live database and are in separate test suites.

---

## Logging

All functions use structured logging with relevant fields:

```go
log.WithFields(log.Fields{
    "source": sourceTable,
    "target": targetTable,
    "rows":   rowsMigrated,
}).Info("migration completed")
```

Log levels:
- **Debug**: Column-level comparisons
- **Info**: Operation start/completion, row counts
- **Warn**: Row count mismatches (non-fatal)
- **Error**: Schema mismatches, migration failures

---

## Integration with Partition Management

These utilities work seamlessly with the partition management APIs in `pkg/db/partitions`:

```go
import "github.com/openshift/sippy/pkg/db/partitions"

// Detach old partitions
detached, _ := partitions.DetachOldPartitions(dbc, "parent_table", 180, false)

// Migrate detached partitions to archive
for _, partition := range detachedPartitions {
    dbc.MigrateTableData(partition.TableName, "archive_table", false)
}

// Drop old partitions
partitions.DropOldDetachedPartitions(dbc, "parent_table", 180, false)
```

---

## Performance Considerations

- **Single INSERT statement**: Migration uses `INSERT INTO ... SELECT` for efficiency
- **No row-by-row operations**: Bulk operation handled by PostgreSQL
- **Network efficiency**: Single round-trip for data transfer
- **Index usage**: PostgreSQL optimizer handles query execution

For very large tables (millions of rows):
- Consider migrating in batches using WHERE clauses
- Monitor transaction log growth
- Use `ANALYZE` after migration
- Consider `VACUUM` on target table

---

## See Also

- [Partition Management APIs](./partitions/README.md) - For partition-specific operations
- [Database Schema](../../.claude/db-schema-analysis.md) - For schema documentation
- Examples in `utils_example.go` - For detailed usage patterns
