# Database Utilities

This package provides utility functions for database operations including schema verification and data migration.

## Overview

The utilities in `utils.go` provide safe, validated operations for working with database tables, particularly useful for:
- Schema migration and validation
- Data migration between tables
- Atomic table renames and swaps
- Sequence management and auditing
- Partition management workflows
- Table consolidation and archival

## Quick Function Reference

**Schema Verification:**
- `VerifyTablesHaveSameColumns` - Compare table schemas
- `GetTableColumns` - Get column metadata for a table

**Data Migration:**
- `MigrateTableData` - Copy all data between tables
- `MigrateTableDataRange` - Copy data for specific date range
- `GetTableRowCount` - Count rows in a table

**Table Renaming:**
- `RenameTables` - Atomically rename tables, sequences, and partitions

**Sequence Management:**
- `GetSequenceMetadata` - Get detailed linkage info (SERIAL vs IDENTITY)
- `GetTableSequences` - List sequences for a specific table
- `ListAllTableSequences` - List sequences for all tables
- `SyncIdentityColumn` - Sync IDENTITY sequence after data migration

**Partition Information:**
- `GetTablePartitions` - List partitions for a specific table
- `GetPartitionStrategy` - Check if table is partitioned (RANGE/LIST/HASH)
- `VerifyPartitionCoverage` - Verify all partitions exist for date range

**Constraint Information:**
- `GetTableConstraints` - List constraints for a specific table

**Index Information:**
- `GetTableIndexes` - List indexes for a specific table

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
rowsMigrated, err := dbc.MigrateTableData("source_table", "target_table", nil, true)

// Actual migration
rowsMigrated, err := dbc.MigrateTableData("source_table", "target_table", nil, false)
if err != nil {
    log.WithError(err).Error("migration failed")
}

// Migrate with omitting columns (e.g., to use target's auto-increment for id)
rowsMigrated, err := dbc.MigrateTableData("source_table", "target_table", []string{"id"}, false)
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
- `omitColumns` - List of column names to omit from migration (e.g., `[]string{"id"}` to use target's auto-increment). Pass `nil` to copy all columns.
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
rowsMigrated, err := dbc.MigrateTableDataRange("source_table", "target_table", "created_at", startDate, endDate, nil, true)

// Actual migration
rowsMigrated, err := dbc.MigrateTableDataRange("source_table", "target_table", "created_at", startDate, endDate, nil, false)
if err != nil {
    log.WithError(err).Error("migration failed")
}

// Migrate with omitting columns (e.g., to use target's auto-increment for id)
rowsMigrated, err := dbc.MigrateTableDataRange("source_table", "target_table", "created_at", startDate, endDate, []string{"id"}, false)
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
- `omitColumns` - List of column names to omit from migration (e.g., `[]string{"id"}` to use target's auto-increment). Pass `nil` to copy all columns.
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

### RenameTables

Renames multiple tables atomically in a single transaction.

```go
// Order matters - renames are executed in the order provided
renames := []db.TableRename{
    {From: "orders_old", To: "orders_backup"},
    {From: "orders_new", To: "orders"},
}

// Dry run first (renameSequences=true, renamePartitions=true, renameConstraints=true, renameIndexes=true)
_, err := dbc.RenameTables(renames, true, true, true, true, true)
if err != nil {
    log.WithError(err).Error("validation failed")
}

// Execute renames (renameSequences=true, renamePartitions=true, renameConstraints=true, renameIndexes=true)
count, err := dbc.RenameTables(renames, true, true, true, true, false)
if err != nil {
    log.WithError(err).Error("rename failed")
}
log.WithField("renamed", count).Info("tables, partitions, sequences, constraints, and indexes renamed")
```

**How It Works**:
1. Validates that all source tables exist
2. Checks for conflicts (target table already exists, unless it's also being renamed)
3. Executes all `ALTER TABLE ... RENAME TO ...` statements in the order provided
4. Either all renames succeed or all are rolled back in a single transaction

**Parameters**:
- `tableRenames`: Ordered slice of TableRename structs specifying renames to execute
- `renameSequences`: If true, also renames sequences owned by table columns (SERIAL, BIGSERIAL, IDENTITY)
- `renamePartitions`: If true, also renames child partitions of partitioned tables
- `renameConstraints`: If true, also renames table constraints (primary keys, foreign keys, unique, check)
- `renameIndexes`: If true, also renames table indexes (including those backing constraints)
- `dryRun`: If true, only validates without executing

**Returns**:
- `renamedCount`: Number of tables successfully renamed (0 if dry run)
- `error`: Any error encountered

**Note**: Caller is responsible for ordering renames correctly to avoid naming conflicts. For table swaps (A→B, B→C), ensure B→C comes before A→B in the array.

**Features**:
- **Atomic operation**: All renames happen in one transaction
- **Validation**: Checks source tables exist and no conflicts
- **Dry-run support**: Test before executing
- **Fast**: PostgreSQL only updates metadata, not data
- **Safe**: Views, indexes, and foreign keys are automatically updated

**Use Cases**:
- Swapping partitioned tables with non-partitioned tables
- Renaming related tables together for consistency
- Atomic schema migrations
- Creating backups before migrations
- Rolling back failed migrations

**Important Notes**:
- All renames must succeed or all will fail (atomic)
- Table swap scenarios are detected and allowed (when target is also a source)
- Extremely fast - only metadata is updated
- PostgreSQL automatically updates dependent object **references** (views, FKs) but NOT their names
- **Sequences are NOT automatically renamed by PostgreSQL** - use `renameSequences=true` to rename them
- **Partitions are NOT automatically renamed by PostgreSQL** - use `renamePartitions=true` to rename them
- **Constraints are NOT automatically renamed by PostgreSQL** - use `renameConstraints=true` to rename them
- **Indexes are NOT automatically renamed by PostgreSQL** - use `renameIndexes=true` to rename them
- **Rename order matters** - sequences/constraints/indexes are processed in sorted order to avoid naming conflicts during table swaps

**Understanding SERIAL vs IDENTITY:**

Both create auto-increment columns, but with different syntax and internal linkage:

```sql
-- Old way: SERIAL (still widely used)
CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    name TEXT
);
-- Creates sequence: orders_id_seq
-- Linkage: pg_depend (deptype='a') + column DEFAULT nextval('orders_id_seq')

-- Modern way: IDENTITY (SQL standard, recommended)
CREATE TABLE orders (
    id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    name TEXT
);
-- Creates sequence: orders_id_seq
-- Linkage: pg_depend (deptype='i') + pg_attribute.attidentity
```

**Key Differences:**

| Aspect | SERIAL | IDENTITY |
|--------|--------|----------|
| SQL Standard | No (PostgreSQL-specific) | Yes (SQL:2003 standard) |
| Dependency Type | `'a'` (auto) | `'i'` (internal) |
| Column Default | `nextval('seq_name')` (name-based) | None (OID-based internally) |
| Rename Safety | Default uses sequence NAME | Fully OID-based, safer |
| PostgreSQL Tables | `pg_depend` + `pg_attrdef` | `pg_depend` + `pg_attribute` |

**How Sequences Are Linked to Columns:**

PostgreSQL uses multiple mechanisms to link sequences to columns:

1. **`pg_depend`** - Dependency tracking (OID-based, survives renames)
   - SERIAL: `deptype = 'a'` (auto dependency)
   - IDENTITY: `deptype = 'i'` (internal dependency)

2. **Column Metadata:**
   - SERIAL: Column default = `nextval('sequence_name')` (stored as text!)
   - IDENTITY: `pg_attribute.attidentity` = `'d'` or `'a'` (uses OID reference)

3. **Sequence Ownership:**
   - Both: `pg_sequence` records which table.column owns the sequence

**Why Our RenameTables Function Works Safely:**

When we execute `ALTER SEQUENCE old_seq RENAME TO new_seq`:

✅ **IDENTITY columns (safe):**
- `pg_depend` uses OID, not name → automatically updated
- `pg_attribute.attidentity` uses OID → no change needed
- Column has NO default expression → nothing to update
- **Result: Fully automatic, zero risk**

⚠️ **SERIAL columns (mostly safe):**
- `pg_depend` uses OID, not name → automatically updated
- BUT: Column default `nextval('old_seq')` is stored as TEXT
- PostgreSQL does NOT automatically update the default expression
- **However**: `nextval()` resolves the sequence name at runtime, and PostgreSQL's search path finds the renamed sequence
- **Result: Works in practice, but default text is stale**

**Both are captured by:**
- `GetTableSequences` / `ListAllTableSequences`
- `RenameTables(renameSequences=true)`
- `SyncIdentityColumn`

**About Sequence Renaming:**

When you rename a table in PostgreSQL, **sequences are NOT automatically renamed**. This can lead to naming inconsistencies:

```sql
-- Before rename:
-- Table: orders
-- Sequence: orders_id_seq

ALTER TABLE orders RENAME TO orders_old;

-- After rename:
-- Table: orders_old
-- Sequence: orders_id_seq (still has old name!)
```

To keep sequence names consistent with table names, use `renameSequences=true`:
- Finds all sequences owned by table columns (SERIAL, BIGSERIAL, IDENTITY)
- Renames them to match new table name: `newtable_columnname_seq`
- All renames (tables + sequences) happen in one atomic transaction
- If any rename fails, all are rolled back

**When to use `renameSequences=true`:**
- ✅ When swapping production tables (keeps naming consistent)
- ✅ When table names are part of your naming conventions
- ✅ When you want clean, matching names for monitoring/debugging
- ❌ When sequences are shared or manually managed
- ❌ When you don't care about sequence naming consistency

**About Partition Renaming:**

When you rename a partitioned table in PostgreSQL, **child partitions are NOT automatically renamed**:

```sql
-- Before rename:
-- Parent table: orders
-- Partitions: orders_2024_01_01, orders_2024_01_02, etc.

ALTER TABLE orders RENAME TO orders_old;

-- After rename:
-- Parent table: orders_old
-- Partitions: orders_2024_01_01, orders_2024_01_02, etc. (still have old prefix!)
```

To keep partition names consistent with the parent table, use `renamePartitions=true`:
- Finds all child partitions using PostgreSQL's inheritance system
- Extracts the suffix from each partition name (e.g., `_2024_01_01`)
- Renames to match new parent: `newtable_2024_01_01`
- All renames (tables + partitions + sequences) happen in one atomic transaction
- If any rename fails, all are rolled back

**How Partition Renaming Works:**
```text
Old table: orders
Old partitions: orders_2024_01_01, orders_2024_01_02

New table: orders_old
New partitions: orders_old_2024_01_01, orders_old_2024_01_02

Suffix extraction: _2024_01_01, _2024_01_02
New naming: newtable + suffix
```

**When to use `renamePartitions=true`:**
- ✅ When swapping partitioned tables in production
- ✅ When partition naming follows table name prefix convention
- ✅ When you want consistent naming for all related objects
- ✅ When monitoring/debugging relies on naming patterns
- ❌ When partitions use custom naming unrelated to table name
- ❌ When partitions are manually managed with specific names

**Renaming Partition Sequences, Constraints, and Indexes:**

When `renamePartitions=true`, the function will **also** rename sequences, constraints, and indexes on those partition tables if the respective flags are enabled:

- `renamePartitions=true` + `renameSequences=true` → Renames sequences on both parent table AND partition tables
- `renamePartitions=true` + `renameConstraints=true` → Renames constraints on both parent table AND partition tables
- `renamePartitions=true` + `renameIndexes=true` → Renames indexes on both parent table AND partition tables

Example:
```go
renames := []db.TableRename{
    {From: "orders", To: "orders_v2"},
}

// Rename table, partitions, and all their sequences/constraints/indexes
count, err := dbc.RenameTables(renames, true, true, true, true, false)
//                                        ↑     ↑     ↑     ↑
//                            sequences ──┘     │     │     │
//                           partitions ────────┘     │     │
//                          constraints ──────────────┘     │
//                              indexes ────────────────────┘

// Result:
// Parent table:
//   - orders_v2
//   - orders_v2_id_seq
//   - orders_v2_pkey
//   - orders_v2_pkey (index)
//
// Partitions:
//   - orders_v2_2024_01
//   - orders_v2_2024_01_pkey
//   - orders_v2_2024_01_pkey (index)
//   - orders_v2_2024_02
//   - orders_v2_2024_02_pkey
//   - orders_v2_2024_02_pkey (index)
```

This ensures complete naming consistency across the entire partitioned table hierarchy.

**About Constraint Renaming:**

When you rename a table in PostgreSQL, **constraints are NOT automatically renamed**:

```sql
-- Before rename:
-- Table: orders
-- Constraints: orders_pkey, orders_email_key, orders_customer_id_fkey

ALTER TABLE orders RENAME TO orders_old;

-- After rename:
-- Table: orders_old
-- Constraints: orders_pkey, orders_email_key, orders_customer_id_fkey (still have old names!)
```

To keep constraint names consistent with table names, use `renameConstraints=true`:
- Finds all constraints for the table (primary keys, foreign keys, unique, check, exclusion)
- Extracts the suffix from each constraint name (e.g., `_pkey`, `_email_key`)
- Renames to match new table: `newtable_pkey`, `newtable_email_key`
- All renames (tables + partitions + sequences + constraints) happen in one atomic transaction
- If any rename fails, all are rolled back

**How Constraint Renaming Works:**
```text
Old table: orders
Old constraints: orders_pkey, orders_email_key, orders_customer_id_fkey

New table: orders_old
New constraints: orders_old_pkey, orders_old_email_key, orders_old_customer_id_fkey

Suffix extraction: _pkey, _email_key, _customer_id_fkey
New naming: newtable + suffix
```

**Constraint Types Renamed:**
- Primary keys (`p`) - e.g., `tablename_pkey`
- Foreign keys (`f`) - e.g., `tablename_column_fkey`
- Unique constraints (`u`) - e.g., `tablename_column_key`
- Check constraints (`c`) - e.g., `tablename_column_check`
- Exclusion constraints (`x`) - e.g., `tablename_excl`

**Important Note about Indexes:**
Renaming a constraint does NOT rename the backing index. Indexes are separate objects in PostgreSQL and must be renamed separately. Use `renameIndexes=true` in `RenameTables` to rename indexes alongside constraints.

**When to use `renameConstraints=true`:**
- ✅ When swapping tables in production (keeps naming consistent)
- ✅ When constraint names follow table name prefix convention
- ✅ When you want clean, matching names for schema documentation
- ✅ When monitoring/debugging relies on naming patterns
- ❌ When constraints use custom naming unrelated to table name
- ❌ When constraints are manually managed with specific names

**About Index Renaming:**

When you rename a table in PostgreSQL, **indexes are NOT automatically renamed**:

```sql
-- Before rename:
-- Table: orders
-- Indexes: orders_pkey, orders_email_key, orders_customer_id_idx

ALTER TABLE orders RENAME TO orders_old;

-- After rename:
-- Table: orders_old
-- Indexes: orders_pkey, orders_email_key, orders_customer_id_idx (still have old names!)
```

To keep index names consistent with table names, use `renameIndexes=true`:
- Finds all indexes for the table (including those backing constraints)
- Extracts the suffix from each index name (e.g., `_pkey`, `_email_key`, `_customer_id_idx`)
- Renames to match new table: `newtable_pkey`, `newtable_email_key`, `newtable_customer_id_idx`
- All renames (tables + partitions + sequences + constraints + indexes) happen in one atomic transaction
- If any rename fails, all are rolled back

**How Index Renaming Works:**
```text
Old table: orders
Old indexes: orders_pkey, orders_email_key, orders_customer_id_idx

New table: orders_old
New indexes: orders_old_pkey, orders_old_email_key, orders_old_customer_id_idx

Suffix extraction: _pkey, _email_key, _customer_id_idx
New naming: newtable + suffix
```

**Index Types Renamed:**
- Primary key indexes - e.g., `tablename_pkey`
- Unique indexes - e.g., `tablename_column_key`
- Regular indexes (B-tree, GIN, GiST, etc.) - e.g., `tablename_column_idx`
- Partial indexes - Any index following the naming pattern

**Important: Indexes vs Constraints**

Indexes and constraints are separate objects in PostgreSQL:
- Renaming a constraint does NOT rename the backing index
- Renaming an index does NOT rename the constraint
- When you create a primary key, PostgreSQL creates both a constraint AND an index with the same name
- **Recommendation:** Use both `renameConstraints=true` and `renameIndexes=true` together to keep names consistent

**Performance Note:**
Index renaming is extremely fast - it only updates metadata in PostgreSQL system catalogs, without touching the actual index data structure. However, it does require a brief `ACCESS EXCLUSIVE` lock on the index.

**When to use `renameIndexes=true`:**
- ✅ When swapping tables in production (keeps naming consistent)
- ✅ When index names follow table name prefix convention
- ✅ When you want clean, matching names for performance analysis
- ✅ When monitoring/debugging relies on naming patterns
- ✅ When renaming constraints (to keep constraint and index names aligned)
- ❌ When indexes use custom naming unrelated to table name
- ❌ When indexes are manually managed with specific names

**Rename Order Handling:**

When swapping tables (e.g., `A -> B, C -> A`), the order of operations matters to avoid naming conflicts:

```go
// Order matters - rename table_base first to free up the name
renames := []db.TableRename{
    {From: "table_base", To: "table_old"},  // Free up "table_base" namespace
    {From: "table_new", To: "table_base"},  // Now safe to use "table_base"
}
```

Without proper ordering, renames could fail:
```sql
-- Wrong order (if table_new renamed first):
ALTER TABLE table_new RENAME TO table_base;  -- ERROR! table_base already exists

-- Correct order (as specified in array):
ALTER TABLE table_base RENAME TO table_old;  -- Frees up "table_base"
ALTER TABLE table_new RENAME TO table_base;  -- Now safe
```

**How it works:**
- Tables are renamed in the order specified in the array
- Each rename happens within a single transaction
- Caller is responsible for specifying correct order to avoid conflicts
- All operations are deterministic - renames execute in array order

**Example - Table Swap**:
```go
// Swap old table with new partitioned table atomically
// Order matters: rename orders first to free up the name
renames := []db.TableRename{
    {From: "orders", To: "orders_old"},              // Save current table
    {From: "orders_partitioned", To: "orders"},      // New table becomes production
}

// Rename sequences, partitions, constraints, and indexes too
count, err := dbc.RenameTables(renames, true, true, true, true, false)
if err != nil {
    // If any rename fails, all are rolled back
    log.Fatal(err)
}
```

**Example - Three-Way Swap**:
```go
// Rotate three tables: production -> backup, new -> production, backup -> archive
// Order matters - must free up names in the right order:
renames := []db.TableRename{
    {From: "orders_backup", To: "orders_archive"},  // Free up "orders_backup"
    {From: "orders", To: "orders_backup"},          // Free up "orders"
    {From: "orders_new", To: "orders"},             // New becomes production
}

// Rename sequences, partitions, constraints, and indexes too
count, err := dbc.RenameTables(renames, true, true, true, true, false)
// All renames happen atomically (tables + partitions + sequences + constraints + indexes)
```

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

### GetSequenceMetadata

Returns detailed metadata about how sequences are linked to columns in a table.

```go
metadata, err := dbc.GetSequenceMetadata("orders")
if err != nil {
    log.WithError(err).Error("failed to get metadata")
}

for _, m := range metadata {
    linkageType := "SERIAL"
    if m.IsIdentityColumn {
        linkageType = "IDENTITY"
    }

    log.WithFields(log.Fields{
        "column":       m.ColumnName,
        "sequence":     m.SequenceName,
        "linkage_type": linkageType,
        "dep_type":     m.DependencyType,
        "owner":        m.SequenceOwner,
    }).Info("sequence linkage")
}
```

**Returns**: List of `SequenceMetadata` structs containing:
- `SequenceName`: Name of the sequence
- `TableName`: Name of the table
- `ColumnName`: Name of the column
- `DependencyType`: `'a'` (SERIAL) or `'i'` (IDENTITY)
- `IsIdentityColumn`: `true` if column uses GENERATED AS IDENTITY
- `SequenceOwner`: Owner in format `table.column`

**Use Cases**:
- Understanding the internal linkage mechanism (OID vs name-based)
- Debugging why a sequence rename might cause issues
- Determining if columns use SERIAL or IDENTITY
- Validating sequence ownership before renames
- Educational/documentation purposes

**Example - Compare SERIAL vs IDENTITY Linkage**:
```go
metadata, _ := dbc.GetSequenceMetadata("orders")
for _, m := range metadata {
    if m.IsIdentityColumn {
        fmt.Printf("%s: IDENTITY (OID-based, safe to rename)\n", m.ColumnName)
    } else {
        fmt.Printf("%s: SERIAL (default uses name, usually safe)\n", m.ColumnName)
    }
}
```

---

### GetTableSequences

Returns all sequences owned by columns in a specific table (SERIAL, BIGSERIAL, IDENTITY).

```go
sequences, err := dbc.GetTableSequences("orders")
if err != nil {
    log.WithError(err).Error("failed to get sequences")
}

for _, seq := range sequences {
    log.WithFields(log.Fields{
        "sequence": seq.SequenceName,
        "table":    seq.TableName,
        "column":   seq.ColumnName,
    }).Info("found sequence")
}
```

**Returns**: List of `SequenceInfo` structs containing:
- `SequenceName`: Name of the sequence
- `TableName`: Name of the table owning the sequence
- `ColumnName`: Name of the column using the sequence

**Sequence Types Captured:**
- **SERIAL/BIGSERIAL**: Creates a sequence like `tablename_columnname_seq`
- **IDENTITY**: Creates an internal sequence like `tablename_columnname_seq` (GENERATED BY DEFAULT AS IDENTITY)

**Use Cases**:
- Checking which sequences will be renamed
- Auditing sequence ownership for a specific table
- Debugging sequence-related issues
- Understanding table dependencies before renames

**Example - Check Before Rename**:
```go
// Check what sequences exist before renaming
sequences, _ := dbc.GetTableSequences("orders_old")
if len(sequences) > 0 {
    log.WithField("count", len(sequences)).Info("found sequences - will rename with table")

    // Use renameSequences=true to keep them consistent
    renames := []db.TableRename{{From: "orders_old", To: "orders"}}
    dbc.RenameTables(renames, true, false, false, false, false)
} else {
    // No sequences to worry about
    renames := []db.TableRename{{From: "orders_old", To: "orders"}}
    dbc.RenameTables(renames, false, false, false, false, false)
}
```

---

### ListAllTableSequences

Returns all sequences owned by table columns across the entire database (public schema).

```go
allSequences, err := dbc.ListAllTableSequences()
if err != nil {
    log.WithError(err).Error("failed to list sequences")
}

for tableName, sequences := range allSequences {
    log.WithFields(log.Fields{
        "table": tableName,
        "count": len(sequences),
    }).Info("table sequences")

    for _, seq := range sequences {
        log.WithFields(log.Fields{
            "sequence": seq.SequenceName,
            "column":   seq.ColumnName,
        }).Debug("sequence detail")
    }
}
```

**Returns**: Map where:
- **Key**: Table name
- **Value**: List of `SequenceInfo` structs for that table

**Use Cases**:
- Database-wide sequence auditing
- Understanding auto-increment usage patterns
- Finding all sequences that need syncing after bulk operations
- Generating database documentation
- Preparing for bulk table renames
- Identifying orphaned sequences

**Example - Audit All Sequences**:
```go
allSequences, err := dbc.ListAllTableSequences()
if err != nil {
    log.Fatal(err)
}

log.WithField("tables", len(allSequences)).Info("tables with sequences")

// Show summary
totalSequences := 0
for tableName, sequences := range allSequences {
    totalSequences += len(sequences)
    fmt.Printf("Table: %s has %d sequence(s)\n", tableName, len(sequences))
    for _, seq := range sequences {
        fmt.Printf("  - %s.%s → %s\n", seq.TableName, seq.ColumnName, seq.SequenceName)
    }
}

log.WithField("total_sequences", totalSequences).Info("audit complete")
```

**Example - Find Tables Without Sequences**:
```go
// Get all tables
allTables := []string{"orders", "items", "users", "logs"}

// Get tables with sequences
tablesWithSequences, _ := dbc.ListAllTableSequences()

// Find tables without sequences
for _, table := range allTables {
    if _, hasSequence := tablesWithSequences[table]; !hasSequence {
        log.WithField("table", table).Info("table has no sequences - using explicit IDs")
    }
}
```

**Example - Sync All Identity Sequences**:
```go
// Get all tables with sequences
allSequences, err := dbc.ListAllTableSequences()
if err != nil {
    log.Fatal(err)
}

// Sync identity column for each table with sequences
for tableName, sequences := range allSequences {
    for _, seq := range sequences {
        // Only sync if column looks like an ID column
        if seq.ColumnName == "id" {
            err := dbc.SyncIdentityColumn(tableName, seq.ColumnName)
            if err != nil {
                log.WithError(err).WithField("table", tableName).Error("sync failed")
            } else {
                log.WithField("table", tableName).Info("synced identity")
            }
        }
    }
}
```

---

### GetTablePartitions

Returns all child partitions of a partitioned table.

```go
partitions, err := dbc.GetTablePartitions("orders")
if err != nil {
    log.WithError(err).Error("failed to get partitions")
}

for _, part := range partitions {
    log.WithFields(log.Fields{
        "partition": part.PartitionName,
        "parent":    part.ParentTable,
    }).Info("found partition")
}
```

**Returns**: List of `PartitionTableInfo` structs containing:
- `PartitionName`: Name of the partition
- `ParentTable`: Name of the parent partitioned table

**Use Cases**:
- Checking which partitions will be renamed
- Auditing partition structure
- Understanding table dependencies before renames
- Verifying partition naming conventions

**Example - Check Partitions Before Rename**:
```go
// Check what partitions exist before renaming
partitions, _ := dbc.GetTablePartitions("orders_old")
log.WithField("count", len(partitions)).Info("found partitions")

for _, part := range partitions {
    // Extract suffix to see naming pattern
    suffix := strings.TrimPrefix(part.PartitionName, "orders_old")
    log.WithFields(log.Fields{
        "partition": part.PartitionName,
        "suffix":    suffix,
    }).Info("partition details")
}

// If partitions follow naming convention, rename them too
if len(partitions) > 0 {
    renames := []db.TableRename{{From: "orders_old", To: "orders"}}
    dbc.RenameTables(renames, true, true, true, true, false) // renamePartitions=true, renameConstraints=true, renameIndexes=true
}
```

---

### GetTableConstraints

Returns all constraints for a table (primary keys, foreign keys, unique, check, exclusion).

```go
constraints, err := dbc.GetTableConstraints("orders")
if err != nil {
    log.WithError(err).Error("failed to get constraints")
}

for _, cons := range constraints {
    log.WithFields(log.Fields{
        "constraint": cons.ConstraintName,
        "type":       cons.ConstraintType,
        "definition": cons.Definition,
    }).Info("found constraint")
}
```

**Returns**: List of `ConstraintInfo` structs containing:
- `ConstraintName`: Name of the constraint (e.g., "orders_pkey")
- `TableName`: Name of the table
- `ConstraintType`: Single character type code:
  - `'p'` - Primary key
  - `'f'` - Foreign key
  - `'u'` - Unique
  - `'c'` - Check
  - `'x'` - Exclusion
- `Definition`: SQL definition of the constraint (e.g., "PRIMARY KEY (id)")

**Use Cases**:
- Checking which constraints will be renamed
- Auditing constraint naming conventions
- Understanding table dependencies before renames
- Verifying constraint structure

**Example - Check Constraints Before Rename**:
```go
// Check what constraints exist before renaming
constraints, _ := dbc.GetTableConstraints("orders_old")
log.WithField("count", len(constraints)).Info("found constraints")

for _, cons := range constraints {
    // Extract suffix to see naming pattern
    suffix := strings.TrimPrefix(cons.ConstraintName, "orders_old")
    typeNames := map[string]string{
        "p": "PRIMARY KEY",
        "f": "FOREIGN KEY",
        "u": "UNIQUE",
        "c": "CHECK",
        "x": "EXCLUSION",
    }

    log.WithFields(log.Fields{
        "constraint": cons.ConstraintName,
        "suffix":     suffix,
        "type":       typeNames[cons.ConstraintType],
    }).Info("constraint details")
}

// If constraints follow naming convention, rename them too
if len(constraints) > 0 {
    renames := []db.TableRename{{From: "orders_old", To: "orders"}}
    dbc.RenameTables(renames, true, true, true, true, false) // renameConstraints=true, renameIndexes=true
}
```

---

### GetTableIndexes

Returns all indexes for a table (including those backing constraints).

```go
indexes, err := dbc.GetTableIndexes("orders")
if err != nil {
    log.WithError(err).Error("failed to get indexes")
}

for _, idx := range indexes {
    log.WithFields(log.Fields{
        "index":      idx.IndexName,
        "is_primary": idx.IsPrimary,
        "is_unique":  idx.IsUnique,
    }).Info("found index")
}
```

**Returns**: List of `IndexInfo` structs containing:
- `IndexName`: Name of the index (e.g., "orders_pkey")
- `TableName`: Name of the table
- `Definition`: Full CREATE INDEX statement
- `IsPrimary`: true if this is a primary key index
- `IsUnique`: true if this is a unique index

**Use Cases**:
- Checking which indexes will be renamed
- Auditing index naming conventions
- Understanding table performance characteristics
- Verifying index structure before operations

**Important Note**:
Indexes and constraints are separate objects. An index backing a primary key or unique constraint has the same name as the constraint, but they are different objects. Renaming one does NOT rename the other.

**Example - Check Indexes Before Rename**:
```go
// Check what indexes exist before renaming
indexes, _ := dbc.GetTableIndexes("orders_old")
log.WithField("count", len(indexes)).Info("found indexes")

for _, idx := range indexes {
    // Extract suffix to see naming pattern
    suffix := strings.TrimPrefix(idx.IndexName, "orders_old")

    indexType := "REGULAR"
    if idx.IsPrimary {
        indexType = "PRIMARY KEY"
    } else if idx.IsUnique {
        indexType = "UNIQUE"
    }

    log.WithFields(log.Fields{
        "index":  idx.IndexName,
        "suffix": suffix,
        "type":   indexType,
    }).Info("index details")
}

// If indexes follow naming convention, rename them too
if len(indexes) > 0 {
    renames := []db.TableRename{{From: "orders_old", To: "orders"}}
    dbc.RenameTables(renames, true, true, true, true, false) // renameIndexes=true
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

### SequenceInfo

Represents basic information about a sequence associated with a table column.

```go
type SequenceInfo struct {
    SequenceName string
    TableName    string
    ColumnName   string
}
```

**Usage**:
- Returned by `GetTableSequences()` to show sequences owned by table columns
- Used internally by `RenameTables()` when `renameSequences=true`
- Includes sequences from SERIAL, BIGSERIAL, and IDENTITY columns

---

### SequenceMetadata

Represents detailed metadata about how a sequence is linked to a column.

```go
type SequenceMetadata struct {
    SequenceName     string
    TableName        string
    ColumnName       string
    DependencyType   string // 'a' = auto (SERIAL), 'i' = internal (IDENTITY)
    IsIdentityColumn bool   // true if column uses GENERATED AS IDENTITY
    SequenceOwner    string // Table.Column that owns this sequence
}
```

**Usage**:
- Returned by `GetSequenceMetadata()` to show detailed linkage information
- Helps understand the difference between SERIAL and IDENTITY columns
- Shows PostgreSQL's internal dependency mechanism (OID-based vs name-based)
- Useful for debugging and educational purposes

---

### PartitionTableInfo

Represents information about a table partition.

```go
type PartitionTableInfo struct {
    PartitionName string
    ParentTable   string
}
```

**Usage**:
- Returned by `GetTablePartitions()` to show child partitions of a table
- Used internally by `RenameTables()` when `renamePartitions=true`
- Works with any partition type (RANGE, LIST, HASH)

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

## Omitting Columns During Migration

Both `MigrateTableData` and `MigrateTableDataRange` support omitting specific columns during migration. This is useful when:

- **Auto-increment columns**: The target table has an `id` column with `GENERATED BY DEFAULT AS IDENTITY` and you want new IDs to be generated instead of copying from source
- **Computed columns**: The target table has columns that should be calculated rather than copied
- **Different schemas**: Some columns exist in the source but shouldn't be migrated to the target

### Example: Omitting ID Column

```go
// Migrate data but let target table generate new IDs
rows, err := dbc.MigrateTableData(
    "old_table",
    "new_table",
    []string{"id"},  // Omit the id column
    false,
)
if err != nil {
    log.WithError(err).Error("migration failed")
    return
}

log.WithField("rows", rows).Info("migrated with new IDs generated")
```

### Example: Omitting Multiple Columns

```go
// Omit multiple columns during range migration
rows, err := dbc.MigrateTableDataRange(
    "source",
    "target",
    "created_at",
    startDate,
    endDate,
    []string{"id", "updated_at", "version"},  // Omit these columns
    false,
)
```

### How It Works

When you specify `omitColumns`:
1. The function retrieves all columns from the source table
2. Filters out any columns in the `omitColumns` list
3. Generates `INSERT INTO target (col1, col2, ...) SELECT col1, col2, ... FROM source`
4. Only the non-omitted columns are included in both the INSERT and SELECT clauses

**Important Notes:**
- If you omit a `NOT NULL` column without a default, the migration will fail
- Omitted columns in the target table must either be nullable or have default values
- Pass `nil` or `[]string{}` to copy all columns (default behavior)

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
_, err = dbc.MigrateTableData("source_table", "target_table", nil, true)
if err != nil {
    log.Fatal(err)
}

// Step 3: Actual migration
rows, err := dbc.MigrateTableData("source_table", "target_table", nil, false)
log.WithField("rows", rows).Info("migration completed")
```

---

### Partition to Archive Migration

```go
// Migrate detached partition to archive table
partition := "test_analysis_by_job_by_dates_2024_01_15"
archive := "test_analysis_archive"

rows, err := dbc.MigrateTableData(partition, archive, nil, false)
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
    rows, err := dbc.MigrateTableData(partition, "archive_table", nil, false)
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
_, err := dbc.MigrateTableData("target_table", "backup_table", nil, false)
if err != nil {
    log.Fatal("backup failed")
}

// Perform migration
rows, err := dbc.MigrateTableData("source_table", "target_table", nil, false)
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

    rows, err := dbc.MigrateTableDataRange("large_table", "large_table_new", "created_at", startDate, endDate, nil, false)
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

### Swap Partitioned Table with Non-Partitioned Table

```go
// Complete workflow: Migrate to partitioned table and swap atomically

oldTable := "orders"
newPartitionedTable := "orders_partitioned"

// Step 1: Verify data was migrated successfully
oldCount, _ := dbc.GetTableRowCount(oldTable)
newCount, _ := dbc.GetTableRowCount(newPartitionedTable)

if oldCount != newCount {
    log.Fatal("row count mismatch - cannot swap")
}

// Step 2: Perform atomic table swap
// Order matters: rename orders first to free up the name
renames := []db.TableRename{
    {From: "orders", To: "orders_old"},              // Save current table
    {From: "orders_partitioned", To: "orders"},      // New table becomes production
}

// Dry run first
_, err := dbc.RenameTables(renames, true, true, true, true, true)
if err != nil {
    log.Fatal(err)
}

// Execute swap (rename sequences, partitions, constraints, and indexes too)
count, err := dbc.RenameTables(renames, true, true, true, true, false)
if err != nil {
    log.Fatal(err)
}

log.WithFields(log.Fields{
    "renamed":     count,
    "old_table":   "orders_old",
    "new_table":   "orders",
    "partitioned": true,
}).Info("tables swapped - partitioned table is now active")

// If something goes wrong, you can easily rollback:
// rollback := []db.TableRename{
//     {From: "orders", To: "orders_partitioned"},
//     {From: "orders_old", To: "orders"},
// }
// dbc.RenameTables(rollback, true, true, true, true, false)
```

---

### Three-Way Table Rotation

```go
// Rotate tables: archive old backup, current becomes backup, new becomes current
// Order matters - must free up names in the right order:
renames := []db.TableRename{
    {From: "orders_backup", To: "orders_archive"},  // Free up "orders_backup"
    {From: "orders", To: "orders_backup"},          // Free up "orders"
    {From: "orders_new", To: "orders"},             // New becomes production
}

// All three renames happen atomically in one transaction (rename sequences and partitions too)
count, err := dbc.RenameTables(renames, true, true, false, false, false)
if err != nil {
    log.WithError(err).Error("rotation failed - no changes made")
    return
}

log.WithField("renamed", count).Info("three-way rotation completed")

// Result:
// - orders (was orders_new) - now in production
// - orders_backup (was orders) - current backup
// - orders_archive (was orders_backup) - archived
```

---

### Migrate with Auto-Generated IDs

```go
// When migrating to a table with auto-increment ID, omit the id column
// so the target table generates new sequential IDs

sourceTable := "prow_job_run_tests"
targetTable := "prow_job_run_tests_partitioned"
startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

// Dry run first to verify
_, err := dbc.MigrateTableDataRange(
    sourceTable,
    targetTable,
    "created_at",
    startDate,
    endDate,
    []string{"id"},  // Omit id column - target will auto-generate
    true,
)
if err != nil {
    log.Fatal(err)
}

// Actual migration
rows, err := dbc.MigrateTableDataRange(
    sourceTable,
    targetTable,
    "created_at",
    startDate,
    endDate,
    []string{"id"},  // Omit id column
    false,
)
if err != nil {
    log.Fatal(err)
}

log.WithFields(log.Fields{
    "rows":       rows,
    "start_date": startDate.Format("2006-01-02"),
    "end_date":   endDate.Format("2006-01-02"),
}).Info("data migrated with new IDs generated")

// Note: No need to sync identity column since we're omitting id
// The target table's auto-increment will continue from its current value
```

---

### Migrate Specific Date Range to Archive

```go
// Move Q1 2024 data to archive table
startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)

// Dry run first
_, err := dbc.MigrateTableDataRange("orders", "orders_archive", "order_date", startDate, endDate, nil, true)
if err != nil {
    log.Fatal(err)
}

// Actual migration
rows, err := dbc.MigrateTableDataRange("orders", "orders_archive", "order_date", startDate, endDate, nil, false)
log.WithFields(log.Fields{
    "rows":       rows,
    "start_date": startDate.Format("2006-01-02"),
    "end_date":   endDate.Format("2006-01-02"),
}).Info("Q1 2024 data archived")
```

---

### Complete Partitioned Table Migration Workflow

```go
// End-to-end example: Migrate from non-partitioned to partitioned table

// Step 1: Create partitioned table (using partitions package)
// import "github.com/openshift/sippy/pkg/db/partitions"
// partitionConfig := partitions.NewRangePartitionConfig("created_at")
// _, err := partitions.CreatePartitionedTableFromExisting(dbc, "orders", "orders_partitioned", partitionConfig, false)

// Step 2: Create necessary partitions
startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Now()
// _, err := partitions.CreateMissingPartitions(dbc, "orders_partitioned", startDate, endDate, false)

// Step 3: Migrate data (omit id to use auto-increment)
rows, err := dbc.MigrateTableDataRange(
    "orders",
    "orders_partitioned",
    "created_at",
    startDate,
    endDate,
    []string{"id"},  // Omit id - let target generate new IDs
    false,
)
if err != nil {
    log.Fatal(err)
}
log.WithField("rows", rows).Info("data migrated")

// Step 4: Verify row counts match
oldCount, _ := dbc.GetTableRowCount("orders")
newCount, _ := dbc.GetTableRowCount("orders_partitioned")
if oldCount != newCount {
    log.Fatal("row count mismatch!")
}

// Step 5: Atomically swap tables (order matters)
renames := []db.TableRename{
    {From: "orders", To: "orders_old"},
    {From: "orders_partitioned", To: "orders"},
}

count, err := dbc.RenameTables(renames, true, true, false, false, false)
if err != nil {
    log.Fatal(err)
}

log.WithFields(log.Fields{
    "renamed":     count,
    "rows":        rows,
    "partitioned": true,
}).Info("migration completed - partitioned table is now active")

// Step 6: After verification period, drop old table
// DROP TABLE orders_old;
```

---

## Best Practices

### Always Use Dry Run First

```go
// GOOD: Verify before executing
_, err := dbc.MigrateTableData(source, target, nil, true)
if err != nil {
    return err
}
rows, err := dbc.MigrateTableData(source, target, nil, false)

// BAD: Direct migration without verification
rows, err := dbc.MigrateTableData(source, target, nil, false)
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

rows, err := dbc.MigrateTableData(source, target, nil, false)

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

### Test Table Renames with Dry Run

```go
// GOOD: Always dry run before renaming
renames := []db.TableRename{
    {From: "orders_old", To: "orders_backup"},
    {From: "orders_new", To: "orders"},
}

_, err := dbc.RenameTables(renames, true, true, true, true, true)
if err != nil {
    log.WithError(err).Error("validation failed")
    return
}

count, err := dbc.RenameTables(renames, true, true, true, true, false)

// BAD: Direct rename without validation
count, err := dbc.RenameTables(renames, true, true, true, true, false)
```

### Verify Before Swapping Tables

```go
// GOOD: Verify data integrity before swapping
oldCount, _ := dbc.GetTableRowCount("orders")
newCount, _ := dbc.GetTableRowCount("orders_partitioned")

if oldCount != newCount {
    log.Error("cannot swap - row counts don't match")
    return
}

// Now safe to swap
dbc.RenameTables([]db.TableRename{
    {From: "orders", To: "orders_old"},
    {From: "orders_partitioned", To: "orders"},
}, true, true, false, false, false)

// BAD: Swap without verifying data
dbc.RenameTables(renames, true, true, false, false, false)
```

### Keep Rollback Plans Ready

```go
// GOOD: Define rollback before making changes
renames := []db.TableRename{
    {From: "orders", To: "orders_old"},
    {From: "orders_new", To: "orders"},
}

// Define rollback upfront (reverse order)
rollback := []db.TableRename{
    {From: "orders", To: "orders_new"},
    {From: "orders_old", To: "orders"},
}

// Execute rename
_, err := dbc.RenameTables(renames, true, true, false, false, false)
if err != nil {
    log.Error("rename failed - no rollback needed")
    return
}

// If issues found after rename, easy to rollback
// dbc.RenameTables(rollback, true, true, false, false, false)
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
    dbc.MigrateTableData(partition.TableName, "archive_table", nil, false)
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
