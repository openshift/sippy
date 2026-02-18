# Partition Management APIs

This package provides GORM-based APIs for managing PostgreSQL table partitions, specifically for `test_analysis_by_job_by_dates`.

## Overview

The partition management APIs provide read-only analysis and write operations (with dry-run support) for managing the lifecycle of table partitions based on retention policies.

**Based on**: [partition-retention-management-guide.md](../../../.claude/partition-retention-management-guide.md)

## Features

- ✅ List all partitions with metadata
- ✅ Get partition statistics and summaries
- ✅ Identify partitions for removal based on retention policy
- ✅ Analyze partitions by age groups and time periods
- ✅ Validate retention policies (safety checks)
- ✅ Dry-run support for all destructive operations
- ✅ Comprehensive logging
- ✅ SQL injection protection

## API Reference

### Read-Only Operations

#### ListTablePartitions
Returns all partitions for a given table with metadata.

```go
partitions, err := partitions.ListTablePartitions(dbc, "test_analysis_by_job_by_dates")
if err != nil {
    log.WithError(err).Error("failed to list partitions")
}

for _, p := range partitions {
    fmt.Printf("%s: %s, Age: %d days, Size: %s\n",
        p.TableName, p.PartitionDate, p.Age, p.SizePretty)
}
```

**Parameters**:
- `tableName` - Name of the partitioned parent table

**Returns**: `[]PartitionInfo` containing:
- `TableName` - Partition table name
- `SchemaName` - Schema (always "public")
- `PartitionDate` - Date the partition represents
- `Age` - Days since partition date
- `SizeBytes` - Storage in bytes
- `SizePretty` - Human-readable size
- `RowEstimate` - Estimated row count

---

#### GetPartitionStats
Returns aggregate statistics about all partitions.

```go
stats, err := partitions.GetPartitionStats(dbc)
if err != nil {
    log.WithError(err).Error("failed to get stats")
}

fmt.Printf("Total: %d partitions, %s\n",
    stats.TotalPartitions, stats.TotalSizePretty)
fmt.Printf("Range: %s to %s\n",
    stats.OldestDate.Format("2006-01-02"),
    stats.NewestDate.Format("2006-01-02"))
```

**Returns**: `*PartitionStats` containing:
- `TotalPartitions` - Total partition count
- `TotalSizeBytes` / `TotalSizePretty` - Total storage
- `OldestDate` / `NewestDate` - Date range
- `AvgSizeBytes` / `AvgSizePretty` - Average partition size

---

#### GetPartitionsForRemoval
Identifies partitions older than the retention period.

```go
// Get all partitions (attached + detached) older than 180 days
partitions, err := partitions.GetPartitionsForRemoval(dbc, "test_analysis_by_job_by_dates", 180, false)
if err != nil {
    log.WithError(err).Error("failed to get partitions for removal")
}

fmt.Printf("Found %d partitions older than 180 days\n", len(partitions))

// Get only attached partitions older than 180 days
attachedPartitions, err := partitions.GetPartitionsForRemoval(dbc, "test_analysis_by_job_by_dates", 180, true)
```

**Parameters**:
- `tableName` - Name of the partitioned parent table
- `retentionDays` - Retention period in days
- `attachedOnly` - If true, only returns attached partitions; if false, returns all partitions

**Returns**: `[]PartitionInfo` for partitions older than retention period

**Use When**:
- `attachedOnly = true`: Before detaching partitions (can only detach what's attached)
- `attachedOnly = false`: Before dropping partitions (can drop both attached and detached)

---

#### GetRetentionSummary
Provides a summary of what would be affected by a retention policy.

```go
// Get summary for all partitions (attached + detached)
summary, err := partitions.GetRetentionSummary(dbc, "test_analysis_by_job_by_dates", 180, false)
if err != nil {
    log.WithError(err).Error("failed to get summary")
}

fmt.Printf("Would delete %d partitions, reclaiming %s\n",
    summary.PartitionsToRemove, summary.StoragePretty)

// Get summary for attached partitions only
attachedSummary, err := partitions.GetRetentionSummary(dbc, "test_analysis_by_job_by_dates", 180, true)
```

**Parameters**:
- `tableName` - Name of the partitioned parent table
- `retentionDays` - Retention period in days
- `attachedOnly` - If true, only considers attached partitions; if false, considers all partitions

**Returns**: `*RetentionSummary` containing:
- `RetentionDays` - Policy retention period
- `CutoffDate` - Date cutoff for removal
- `PartitionsToRemove` - Count of partitions to remove
- `StorageToReclaim` / `StoragePretty` - Storage to be freed
- `OldestPartition` / `NewestPartition` - Range of affected partitions

**Use When**:
- `attachedOnly = true`: Before detaching partitions or when validating against active data only
- `attachedOnly = false`: Before dropping partitions or when showing complete impact

---

#### GetPartitionsByAgeGroup
Returns partition counts and sizes grouped by age buckets.

```go
groups, err := partitions.GetPartitionsByAgeGroup(dbc)
if err != nil {
    log.WithError(err).Error("failed to get age groups")
}

for _, group := range groups {
    fmt.Printf("%s: %d partitions, %s (%.2f%%)\n",
        group["age_bucket"],
        group["partition_count"],
        group["total_size"],
        group["percentage"])
}
```

**Age Buckets**:
- Future (dates in the future)
- 0-30 days
- 30-90 days
- 90-180 days
- 180-365 days
- 365+ days

---

#### GetPartitionsByMonth
Returns partition counts and sizes grouped by month.

```go
months, err := partitions.GetPartitionsByMonth(dbc)
if err != nil {
    log.WithError(err).Error("failed to get monthly data")
}
```

**Returns**: Monthly aggregates with partition counts and sizes

---

#### ValidateRetentionPolicy
Validates that a retention policy is safe to apply.

```go
err := partitions.ValidateRetentionPolicy(dbc, "test_analysis_by_job_by_dates", 180)
if err != nil {
    log.WithError(err).Error("retention policy is not safe")
}
```

**Parameters**:
- `tableName` - Name of the partitioned parent table
- `retentionDays` - Retention period in days

**Safety Checks**:
- Minimum 90 days retention
- Maximum 75% of attached partitions deleted
- Maximum 80% of attached storage deleted

**Important**: Only considers **attached partitions** when validating thresholds. Detached partitions are excluded from calculations to ensure the policy is safe for active data.

**Returns**: Error if policy would be unsafe

---

### Write Operations (Require Write Access)

⚠️ **Warning**: All write operations require database write access. Read-only users will get permission errors.

#### CreatePartitionedTable
Creates a new partitioned table from a GORM model struct with a specified partitioning strategy.

```go
// Define your model (or use an existing one)
type MyModel struct {
    ID        uint      `gorm:"primaryKey"`
    CreatedAt time.Time `gorm:"index"`
    Name      string
    Data      string
}

// RANGE partitioning (most common - for dates, timestamps)
config := partitions.NewRangePartitionConfig("created_at")

// Dry run - see the SQL that would be executed
sql, err := partitions.CreatePartitionedTable(dbc, &MyModel{}, "my_partitioned_table", config, true)
if err != nil {
    log.WithError(err).Error("dry run failed")
}
// Prints the CREATE TABLE statement with PARTITION BY RANGE clause

// Actual creation
sql, err = partitions.CreatePartitionedTable(dbc, &MyModel{}, "my_partitioned_table", config, false)
```

**Parameters**:
- `model` - GORM model struct (must be a pointer, e.g., `&models.MyModel{}`)
- `tableName` - Name for the partitioned table
- `config` - Partition configuration (strategy, columns, etc.)
- `dryRun` - If true, prints SQL without executing

**Partition Strategies**:

1. **RANGE Partitioning** (for dates, timestamps, sequential values):
```go
config := partitions.NewRangePartitionConfig("created_at")
// Generates: PARTITION BY RANGE (created_at)
```

2. **LIST Partitioning** (for discrete categories):
```go
config := partitions.NewListPartitionConfig("region")
// Generates: PARTITION BY LIST (region)
```

3. **HASH Partitioning** (for load distribution):
```go
config := partitions.NewHashPartitionConfig(4, "user_id")
// Generates: PARTITION BY HASH (user_id)
// Modulus = 4 means 4 hash partitions will be needed
```

**How It Works**:
1. Validates partition configuration
2. Checks if table already exists (returns without error if it does)
3. Parses the GORM model to extract schema information
4. **Converts GORM/Go types to PostgreSQL types** (see Data Type Mapping below)
5. Generates `CREATE TABLE` statement with columns and data types
6. **Adds PRIMARY KEY constraint** (automatically includes partition columns if not already in primary key)
7. Adds `PARTITION BY [RANGE|LIST|HASH] (columns)` clause
8. Creates indexes (skips unique indexes without all partition keys)
9. In dry-run mode, prints SQL; otherwise executes it

**Data Type Mapping**:
The function automatically converts Go/GORM types to PostgreSQL types:
- `uint`, `uint32`, `uint64`, `int` → `bigint`
- `uint8`, `int8`, `int16` → `smallint`
- `uint16`, `int32` → `integer`
- `int64` → `bigint`
- `float`, `float64` → `double precision`
- `float32` → `real`
- `string` → `text`
- `bool` → `boolean`
- `time.Time` → `timestamp with time zone`
- `[]byte` → `bytea`

This ensures your GORM models with Go types like `uint` work correctly with PostgreSQL.

**Important Notes**:
- **Primary keys**: Automatically generated with `PRIMARY KEY (columns)` constraint
  - If your model's primary key doesn't include partition columns, they are automatically added
  - For example, if you have `ID` as primary key and partition by `created_at`, the constraint will be `PRIMARY KEY (id, created_at)`
  - This is a PostgreSQL requirement for partitioned tables
- **Primary key NOT NULL**: Automatically adds NOT NULL to primary key columns
- **Auto-increment fields**: Fields marked with `gorm:"autoIncrement"` are implemented using `GENERATED BY DEFAULT AS IDENTITY`
  - IDENTITY columns are automatically NOT NULL (PostgreSQL requirement)
  - Supports `autoIncrementIncrement` for custom increment values (e.g., `gorm:"autoIncrement;autoIncrementIncrement:10"` generates `IDENTITY (INCREMENT BY 10)`)
  - Example: `ID uint \`gorm:"primaryKey;autoIncrement"\`` generates `id bigint GENERATED BY DEFAULT AS IDENTITY`
- **Column deduplication**: Automatically deduplicates columns to prevent the same column from appearing multiple times
  - GORM can include duplicate fields in `stmt.Schema.Fields` (e.g., from embedded structs like `gorm.Model`)
  - First occurrence of each column is used, subsequent duplicates are skipped with debug logging
- **Unique indexes**: Must include ALL partition columns (PostgreSQL requirement)
- **After creation**: Create actual partitions based on strategy
- Table creation is a one-time operation (cannot easily modify schema after)
- **Data types**: Automatically converted from Go types to PostgreSQL types

**Example Models**:

```go
// Basic model with auto-increment primary key
type MyModel struct {
    ID        uint      `gorm:"primaryKey;autoIncrement"`
    Name      string    `gorm:"not null"`
    CreatedAt time.Time `gorm:"index"`
}
// Generated SQL:
// id bigint GENERATED BY DEFAULT AS IDENTITY
// PRIMARY KEY (id, created_at)  -- includes partition column

// Model with custom increment value
type CustomIncrement struct {
    ID        uint      `gorm:"primaryKey;autoIncrement;autoIncrementIncrement:10"`
    Data      string
    CreatedAt time.Time
}
// Generated SQL:
// id bigint GENERATED BY DEFAULT AS IDENTITY (INCREMENT BY 10)
```

**Complete Workflows**:

**RANGE Partitioning (Date-based)**:
```go
// 1. Create the partitioned table structure
config := partitions.NewRangePartitionConfig("created_at")
_, err := partitions.CreatePartitionedTable(dbc, &models.MyModel{}, "my_table", config, false)

// 2. Create partitions for date range
startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Now()
created, err := partitions.CreateMissingPartitions(dbc, "my_table", startDate, endDate, false)
```

**HASH Partitioning (Load Distribution)**:
```go
// 1. Create the partitioned table structure
config := partitions.NewHashPartitionConfig(4, "user_id")
_, err := partitions.CreatePartitionedTable(dbc, &models.MyModel{}, "my_table", config, false)

// 2. Create hash partitions manually
for i := 0; i < 4; i++ {
    partName := fmt.Sprintf("my_table_%d", i)
    sql := fmt.Sprintf("CREATE TABLE %s PARTITION OF my_table FOR VALUES WITH (MODULUS 4, REMAINDER %d)", partName, i)
    dbc.DB.Exec(sql)
}
```

**LIST Partitioning (Category-based)**:
```go
// 1. Create the partitioned table structure
config := partitions.NewListPartitionConfig("region")
_, err := partitions.CreatePartitionedTable(dbc, &models.MyModel{}, "my_table", config, false)

// 2. Create list partitions manually
regions := []string{"us-east", "us-west", "eu-central"}
for _, region := range regions {
    partName := fmt.Sprintf("my_table_%s", region)
    sql := fmt.Sprintf("CREATE TABLE %s PARTITION OF my_table FOR VALUES IN ('%s')", partName, region)
    dbc.DB.Exec(sql)
}
```

---


#### UpdatePartitionedTable
Updates an existing partitioned table schema to match a GORM model.

```go
// Define your updated model
type MyModel struct {
    ID        uint      `gorm:"primaryKey"`
    CreatedAt time.Time `gorm:"index"`
    Name      string
    Data      string
    NewField  string    `gorm:"index"` // New field added
    // OldField removed
}

// Dry run - see what changes would be made
sql, err := partitions.UpdatePartitionedTable(dbc, &MyModel{}, "my_partitioned_table", true)
if err != nil {
    log.WithError(err).Error("dry run failed")
}
// Prints all ALTER TABLE statements that would be executed

// Actual update
sql, err = partitions.UpdatePartitionedTable(dbc, &MyModel{}, "my_partitioned_table", false)
```

**Parameters**:
- `model` - GORM model struct with desired schema (must be a pointer, e.g., `&models.MyModel{}`)
- `tableName` - Name of the existing partitioned table
- `dryRun` - If true, prints SQL without executing

**How It Works**:
1. Checks if the table exists
2. Parses the GORM model to get desired schema
3. Queries database for current schema (columns, indexes, partition keys)
4. Compares schemas and generates ALTER statements for:
   - **New columns**: `ALTER TABLE ADD COLUMN`
   - **Modified columns**: `ALTER COLUMN TYPE`, `SET/DROP NOT NULL`, `SET/DROP DEFAULT`
   - **Removed columns**: `ALTER TABLE DROP COLUMN`
   - **New indexes**: `CREATE INDEX`
   - **Modified indexes**: `DROP INDEX` + `CREATE INDEX`
   - **Removed indexes**: `DROP INDEX`
5. In dry-run mode, prints SQL; otherwise executes it

**Important Notes**:
- **Cannot change partition keys**: Partition columns cannot be modified after creation
- **Unique indexes**: Must include ALL partition columns (PostgreSQL requirement)
- **Primary key indexes**: Skipped (named `_pkey` by convention)
- **Primary key NOT NULL**: Automatically adds NOT NULL to primary key columns (PostgreSQL requirement)
- **Data types**: Automatically converted from Go types to PostgreSQL types (same as CreatePartitionedTable)
- **Type changes**: Use caution with data type changes that could cause data loss
- **Column removal**: Destructive operation - ensure data is not needed
- Always run dry-run first to preview changes

**Schema Changes Detected**:

1. **Column Changes**:
   - New columns added with appropriate data type, NOT NULL, and DEFAULT
   - Primary key columns automatically get NOT NULL constraint
   - Type changes detected through normalized comparison (uses converted PostgreSQL types)
   - NULL constraint changes
   - DEFAULT value changes
   - Removed columns

2. **Index Changes**:
   - New indexes created
   - Modified indexes (column list changes) dropped and recreated
   - Removed indexes dropped
   - Validates unique indexes include partition keys

**Use When**:
- Your GORM model schema has evolved
- Adding new fields to track additional data
- Modifying column types or constraints
- Adding or removing indexes
- Schema migrations in production

**Safety Features**:
- Dry-run mode to preview all changes
- Validates unique indexes include partition keys
- Skips primary key indexes (prevents accidental modification)
- Comprehensive logging for each change
- Returns all SQL executed for audit trail

**Example Workflow**:
```go
// 1. Update your GORM model
type TestResults struct {
    ID          uint      `gorm:"primaryKey"`
    CreatedAt   time.Time `gorm:"index"`
    TestName    string    `gorm:"index"`
    NewMetric   float64   // Added field
    // RemovedField deleted
}

// 2. Dry run to see changes
sql, err := partitions.UpdatePartitionedTable(dbc, &TestResults{}, "test_results", true)
fmt.Println("Would execute:", sql)

// 3. Review changes, then apply
sql, err = partitions.UpdatePartitionedTable(dbc, &TestResults{}, "test_results", false)
if err != nil {
    log.Fatal(err)
}
```

**Limitations**:
- Cannot modify partition strategy (RANGE to LIST, etc.)
- Cannot change partition columns
- Cannot split or merge partitions
- Type conversions must be PostgreSQL-compatible
- For major schema changes, consider creating a new table and migrating data

---

#### DropPartition
Drops a single partition.

```go
// Dry run (safe)
err := partitions.DropPartition(dbc, "test_analysis_by_job_by_dates_2024_10_29", true)

// Actual drop (DESTRUCTIVE)
err := partitions.DropPartition(dbc, "test_analysis_by_job_by_dates_2024_10_29", false)
```

**Parameters**:
- `partitionName` - Full partition table name
- `dryRun` - If true, only logs what would happen

**Safety Features**:
- Validates partition name format
- Prevents SQL injection
- Logs all operations

---

#### DetachPartition
Detaches a partition from the parent table (safer alternative to DROP).

```go
// Dry run
err := partitions.DetachPartition(dbc, "test_analysis_by_job_by_dates_2024_10_29", true)

// Actual detach
err := partitions.DetachPartition(dbc, "test_analysis_by_job_by_dates_2024_10_29", false)
```

**Use When**:
- You want to archive data before deletion
- You want a reversible operation (can reattach if needed)

---

#### ListAttachedPartitions
Lists all partitions currently attached to the parent table.

```go
attached, err := partitions.ListAttachedPartitions(dbc, "test_analysis_by_job_by_dates")
if err != nil {
    log.WithError(err).Error("failed to list attached partitions")
}

for _, p := range attached {
    fmt.Printf("%s: %s, Size: %s\n", p.TableName, p.PartitionDate, p.SizePretty)
}
```

**Parameters**:
- `tableName` - Name of the partitioned parent table

**Returns**: `[]PartitionInfo` for attached partitions only

**How It Works**:
- Queries `pg_inherits` to find partitions in the inheritance hierarchy
- Returns only partitions that are currently attached to the parent table

**Use When**:
- You need to analyze only active partitions
- You want to distinguish between attached and detached partitions
- You need to check the current state of the partitioned table

---

#### ListDetachedPartitions
Lists all partitions that have been detached from the parent table.

```go
detached, err := partitions.ListDetachedPartitions(dbc, "test_analysis_by_job_by_dates")
if err != nil {
    log.WithError(err).Error("failed to list detached partitions")
}

for _, p := range detached {
    fmt.Printf("%s: %s, Size: %s\n", p.TableName, p.PartitionDate, p.SizePretty)
}
```

**Parameters**:
- `tableName` - Name of the partitioned parent table

**Returns**: `[]PartitionInfo` for detached partitions

**How It Works**:
- Queries `pg_inherits` to find attached partitions
- Returns tables matching the naming pattern but NOT in the inheritance hierarchy

---

#### GetAttachedPartitionStats
Returns statistics about attached partitions only.

```go
stats, err := partitions.GetAttachedPartitionStats(dbc, "test_analysis_by_job_by_dates")
if err != nil {
    log.WithError(err).Error("failed to get attached stats")
}

fmt.Printf("Attached: %d partitions (%s)\n",
    stats.TotalPartitions, stats.TotalSizePretty)
```

**Parameters**:
- `tableName` - Name of the partitioned parent table

**Returns**: `*PartitionStats` with aggregate statistics for attached partitions only

**Use When**:
- Validating retention policies (should only consider active partitions)
- Analyzing current active storage usage
- Monitoring production partition health

---

#### GetDetachedPartitionStats
Returns statistics about detached partitions.

```go
stats, err := partitions.GetDetachedPartitionStats(dbc, "test_analysis_by_job_by_dates")
if err != nil {
    log.WithError(err).Error("failed to get detached stats")
}

fmt.Printf("Detached: %d partitions (%s)\n",
    stats.TotalPartitions, stats.TotalSizePretty)
```

**Returns**: `*PartitionStats` for detached partitions only

---

#### IsPartitionAttached
Checks if a specific partition is currently attached to the parent table.

```go
isAttached, err := partitions.IsPartitionAttached(dbc, "test_analysis_by_job_by_dates_2024_10_29")
if err != nil {
    log.WithError(err).Error("check failed")
}

if isAttached {
    fmt.Println("Partition is part of the parent table")
} else {
    fmt.Println("Partition is detached (standalone table)")
}
```

**Returns**: `bool` indicating attachment status

---

#### ReattachPartition
Reattaches a previously detached partition back to the parent table.

```go
// Dry run
err := partitions.ReattachPartition(dbc, "test_analysis_by_job_by_dates_2024_10_29", true)

// Actual reattach
err := partitions.ReattachPartition(dbc, "test_analysis_by_job_by_dates_2024_10_29", false)
```

**Use When**:
- You need to restore archived data
- You detached a partition by mistake
- Historical analysis requires old data

**Note**: Automatically calculates the date range from the partition name

---

#### CreateMissingPartitions
Creates missing partitions for a date range.

```go
startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)

// Dry run - see what would be created
created, err := partitions.CreateMissingPartitions(dbc, "test_analysis_by_job_by_dates", startDate, endDate, true)
fmt.Printf("Would create %d partitions\n", created)

// Actual creation
created, err = partitions.CreateMissingPartitions(dbc, "test_analysis_by_job_by_dates", startDate, endDate, false)
fmt.Printf("Created %d partitions\n", created)
```

**Parameters**:
- `tableName` - Name of the partitioned parent table
- `startDate` - Start of date range (inclusive)
- `endDate` - End of date range (inclusive)
- `dryRun` - If true, only simulates the operation

**How It Works**:
1. Lists all existing partitions (attached + detached)
2. Generates list of dates in range that don't have partitions
3. For each missing partition:
   - Creates table with same structure as parent (CREATE TABLE ... LIKE)
   - Attaches partition with appropriate date range (FOR VALUES FROM ... TO ...)
4. Skips partitions that already exist
5. Returns count of partitions created

**Use When**:
- Setting up a new partitioned table with historical dates
- Backfilling missing partitions after data gaps
- Preparing partitions in advance for future dates
- Recovering from partition management issues

**Safety Features**:
- Checks for existing partitions before creating
- Dry-run mode to preview what will be created
- Automatically cleans up if attachment fails
- Comprehensive logging for each partition

---

#### DetachOldPartitions
Bulk operation to detach all partitions older than retention period.

```go
// Dry run
detached, err := partitions.DetachOldPartitions(dbc, 180, true)
fmt.Printf("Would detach %d partitions\n", detached)

// Actual detach
detached, err := partitions.DetachOldPartitions(dbc, 180, false)
fmt.Printf("Detached %d partitions\n", detached)
```

**Parameters**:
- `retentionDays` - Retention period in days
- `dryRun` - If true, only simulates the operation

**Features**:
- Validates retention policy before execution
- Processes partitions in order (oldest first)
- Logs each partition detachment
- Returns count of partitions detached

---

#### DropOldPartitions
Bulk operation to drop all partitions older than retention period.

```go
// Dry run - see what would happen
dropped, err := partitions.DropOldPartitions(dbc, 180, true)
fmt.Printf("Would drop %d partitions\n", dropped)

// Actual cleanup (DESTRUCTIVE)
dropped, err := partitions.DropOldPartitions(dbc, 180, false)
fmt.Printf("Dropped %d partitions\n", dropped)
```

**Parameters**:
- `retentionDays` - Retention period in days
- `dryRun` - If true, only simulates the operation

**Features**:
- Validates retention policy before execution
- Processes partitions in order (oldest first)
- Logs each partition drop
- Returns count of partitions dropped

---

#### DropOldDetachedPartitions
Bulk operation to drop detached partitions older than retention period.

```go
// Dry run - see what would happen
dropped, err := partitions.DropOldDetachedPartitions(dbc, "test_analysis_by_job_by_dates", 180, true)
fmt.Printf("Would drop %d detached partitions\n", dropped)

// Actual cleanup (DESTRUCTIVE)
dropped, err := partitions.DropOldDetachedPartitions(dbc, "test_analysis_by_job_by_dates", 180, false)
fmt.Printf("Dropped %d detached partitions\n", dropped)
```

**Parameters**:
- `tableName` - Name of the parent table
- `retentionDays` - Retention period in days
- `dryRun` - If true, only simulates the operation

**Use When**:
- You have detached partitions that have been archived
- You want to clean up old detached partitions no longer needed
- You need to reclaim storage from detached partitions

**Features**:
- Lists all detached partitions first
- Filters by retention period
- Processes partitions in order (oldest first)
- Logs each partition drop
- Returns count of partitions dropped

**Note**: Unlike `DropOldPartitions`, this only affects detached partitions. Attached partitions remain untouched.

---

## Usage Examples

### Example 1: Analyze Current State

```go
import "github.com/openshift/sippy/pkg/db/partitions"

func analyzePartitions(dbc *db.DB) {
    // Get overall statistics
    stats, err := partitions.GetPartitionStats(dbc)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Total: %d partitions (%s)\n",
        stats.TotalPartitions, stats.TotalSizePretty)

    // Analyze by age groups
    groups, err := partitions.GetPartitionsByAgeGroup(dbc)
    if err != nil {
        log.Fatal(err)
    }

    for _, group := range groups {
        fmt.Printf("%s: %s\n", group["age_bucket"], group["total_size"])
    }
}
```

### Example 2: Dry Run Cleanup

```go
func dryRunCleanup(dbc *db.DB, retentionDays int) {
    // Validate policy
    if err := partitions.ValidateRetentionPolicy(dbc, retentionDays); err != nil {
        log.Fatalf("Policy validation failed: %v", err)
    }

    // Get summary
    summary, err := partitions.GetRetentionSummary(dbc, retentionDays)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Would delete %d partitions, reclaiming %s\n",
        summary.PartitionsToRemove, summary.StoragePretty)

    // Perform dry run
    dropped, err := partitions.DropOldPartitions(dbc, retentionDays, true)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Dry run complete: %d partitions would be dropped\n", dropped)
}
```

### Example 3: Execute Cleanup (Production)

```go
func executeCleanup(dbc *db.DB, retentionDays int) {
    // Always validate first
    if err := partitions.ValidateRetentionPolicy(dbc, retentionDays); err != nil {
        return fmt.Errorf("retention policy failed validation: %w", err)
    }

    // Get summary for logging
    summary, err := partitions.GetRetentionSummary(dbc, retentionDays)
    if err != nil {
        return err
    }

    log.WithFields(log.Fields{
        "retention_days":       retentionDays,
        "partitions_to_delete": summary.PartitionsToRemove,
        "storage_to_reclaim":   summary.StoragePretty,
    }).Info("starting partition cleanup")

    // Execute cleanup (NOT a dry run)
    dropped, err := partitions.DropOldPartitions(dbc, retentionDays, false)
    if err != nil {
        return fmt.Errorf("cleanup failed: %w", err)
    }

    log.WithField("dropped", dropped).Info("partition cleanup completed")
    return nil
}
```

### Example 4: Detach Instead of Drop (Safer)

```go
func detachForArchival(dbc *db.DB, retentionDays int) error {
    // Validate policy
    if err := partitions.ValidateRetentionPolicy(dbc, retentionDays); err != nil {
        return err
    }

    // Detach old partitions instead of dropping
    detached, err := partitions.DetachOldPartitions(dbc, retentionDays, false)
    if err != nil {
        return fmt.Errorf("detach failed: %w", err)
    }

    log.WithField("detached", detached).Info("partitions detached for archival")

    // Now archive the detached partitions (external process)
    // archiveDetachedPartitions(dbc)

    return nil
}
```

### Example 5: Compare Attached vs Detached Partitions

```go
func comparePartitionState(dbc *db.DB, tableName string) error {
    // Get all partitions (attached + detached)
    allPartitions, err := partitions.ListTablePartitions(dbc, tableName)
    if err != nil {
        return err
    }

    // Get only attached partitions
    attached, err := partitions.ListAttachedPartitions(dbc, tableName)
    if err != nil {
        return err
    }

    // Get only detached partitions
    detached, err := partitions.ListDetachedPartitions(dbc, tableName)
    if err != nil {
        return err
    }

    // Display summary
    fmt.Printf("Partition State for %s:\n", tableName)
    fmt.Printf("  Total:    %d partitions\n", len(allPartitions))
    fmt.Printf("  Attached: %d partitions\n", len(attached))
    fmt.Printf("  Detached: %d partitions\n", len(detached))

    // Calculate storage breakdown
    var attachedSize, detachedSize int64
    for _, p := range attached {
        attachedSize += p.SizeBytes
    }
    for _, p := range detached {
        detachedSize += p.SizeBytes
    }

    fmt.Printf("\nStorage Breakdown:\n")
    fmt.Printf("  Attached: %d bytes\n", attachedSize)
    fmt.Printf("  Detached: %d bytes\n", detachedSize)
    fmt.Printf("  Total:    %d bytes\n", attachedSize+detachedSize)

    return nil
}
```

---

### Example 6: Working with Detached Partitions

```go
func manageDetachedPartitions(dbc *db.DB) error {
    // List all detached partitions
    detached, err := partitions.ListDetachedPartitions(dbc, "test_analysis_by_job_by_dates")
    if err != nil {
        return err
    }

    fmt.Printf("Found %d detached partitions\n", len(detached))

    // Get statistics
    stats, err := partitions.GetDetachedPartitionStats(dbc, "test_analysis_by_job_by_dates")
    if err != nil {
        return err
    }

    fmt.Printf("Detached partitions total: %s\n", stats.TotalSizePretty)

    // Check if specific partition is detached
    for _, p := range detached {
        isAttached, err := partitions.IsPartitionAttached(dbc, p.TableName)
        if err != nil {
            continue
        }

        if !isAttached {
            fmt.Printf("%s is detached and ready for archival\n", p.TableName)
            // Archive this partition to S3, compress, etc.
        }
    }

    return nil
}
```

---

### Example 7: Reattach Archived Data

```go
func restoreArchivedPartition(dbc *db.DB, partitionName string) error {
    // Check current status
    isAttached, err := partitions.IsPartitionAttached(dbc, partitionName)
    if err != nil {
        return err
    }

    if isAttached {
        return fmt.Errorf("partition %s is already attached", partitionName)
    }

    log.WithField("partition", partitionName).Info("reattaching partition")

    // Reattach the partition
    err = partitions.ReattachPartition(dbc, partitionName, false)
    if err != nil {
        return fmt.Errorf("reattach failed: %w", err)
    }

    log.Info("partition reattached successfully")
    return nil
}
```

---

### Example 8: Create Missing Partitions for Date Range

```go
func ensurePartitionsExist(dbc *db.DB, tableName string, startDate, endDate time.Time) error {
    // Check what partitions would be created
    created, err := partitions.CreateMissingPartitions(dbc, tableName, startDate, endDate, true)
    if err != nil {
        return fmt.Errorf("dry run failed: %w", err)
    }

    if created == 0 {
        log.Info("all partitions already exist")
        return nil
    }

    log.WithFields(log.Fields{
        "table":       tableName,
        "start_date":  startDate.Format("2006-01-02"),
        "end_date":    endDate.Format("2006-01-02"),
        "to_create":   created,
    }).Info("creating missing partitions")

    // Create the missing partitions
    created, err = partitions.CreateMissingPartitions(dbc, tableName, startDate, endDate, false)
    if err != nil {
        return fmt.Errorf("partition creation failed: %w", err)
    }

    log.WithField("created", created).Info("partitions created successfully")
    return nil
}

// Example: Prepare partitions for next month
func prepareNextMonthPartitions(dbc *db.DB) error {
    now := time.Now()
    startOfNextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
    endOfNextMonth := startOfNextMonth.AddDate(0, 1, -1)

    return ensurePartitionsExist(dbc, "test_analysis_by_job_by_dates", startOfNextMonth, endOfNextMonth)
}

// Example: Backfill missing partitions for last 90 days
func backfillRecentPartitions(dbc *db.DB) error {
    endDate := time.Now()
    startDate := endDate.AddDate(0, 0, -90)

    return ensurePartitionsExist(dbc, "test_analysis_by_job_by_dates", startDate, endDate)
}
```

---

### Example 9: Create a New Partitioned Table from GORM Model

```go
package main

import (
    "time"
    "github.com/openshift/sippy/pkg/db"
    "github.com/openshift/sippy/pkg/db/partitions"
)

// Define your model
type TestResults struct {
    ID          uint      `gorm:"primaryKey"`
    TestName    string    `gorm:"index"`
    JobName     string    `gorm:"index"`
    Result      string
    CreatedAt   time.Time `gorm:"index"` // This will be the partition column
    TestOutput  string
    Duration    int
}

func setupPartitionedTestResults(dbc *db.DB) error {
    tableName := "test_results_partitioned"

    // Configure RANGE partitioning by created_at
    config := partitions.NewRangePartitionConfig("created_at")

    // Step 1: Create the partitioned table (dry-run first)
    sql, err := partitions.CreatePartitionedTable(
        dbc,
        &TestResults{},
        tableName,
        config,
        true, // dry-run
    )
    if err != nil {
        return fmt.Errorf("dry run failed: %w", err)
    }

    log.Info("Would execute SQL:")
    log.Info(sql)

    // The generated SQL will look like:
    // CREATE TABLE IF NOT EXISTS test_results_partitioned (
    //     id bigint NOT NULL,
    //     test_name text,
    //     job_name text,
    //     result text,
    //     created_at timestamp with time zone NOT NULL,
    //     test_output text,
    //     duration bigint,
    //     PRIMARY KEY (id, created_at)
    // ) PARTITION BY RANGE (created_at)
    //
    // Note: created_at is automatically added to the primary key
    // because it's the partition column (PostgreSQL requirement)

    // Step 2: Create the table for real
    _, err = partitions.CreatePartitionedTable(
        dbc,
        &TestResults{},
        tableName,
        config,
        false, // execute
    )
    if err != nil {
        return fmt.Errorf("table creation failed: %w", err)
    }

    log.WithField("table", tableName).Info("partitioned table created")

    // Step 3: Create partitions for the last 90 days
    endDate := time.Now()
    startDate := endDate.AddDate(0, 0, -90)

    created, err := partitions.CreateMissingPartitions(
        dbc,
        tableName,
        startDate,
        endDate,
        false,
    )
    if err != nil {
        return fmt.Errorf("partition creation failed: %w", err)
    }

    log.WithFields(log.Fields{
        "table":      tableName,
        "partitions": created,
    }).Info("created partitions")

    return nil
}

// You can now use the table normally with GORM
func insertTestResult(dbc *db.DB) error {
    result := TestResults{
        TestName:   "test-api-health",
        JobName:    "periodic-ci-test",
        Result:     "passed",
        CreatedAt:  time.Now(),
        TestOutput: "All checks passed",
        Duration:   125,
    }

    // GORM will automatically route to the correct partition based on created_at
    return dbc.DB.Create(&result).Error
}
```

**Key Points**:
- Model must have the partition column (e.g., `created_at`)
- PRIMARY KEY constraint is automatically generated
- Partition columns are automatically added to the primary key (PostgreSQL requirement)
- In the example above, `PRIMARY KEY (id, created_at)` is generated even though only `id` is marked as primaryKey
- Unique indexes must include the partition column
- Data is automatically routed to correct partition by PostgreSQL

---

### Example 10: Update Partitioned Table Schema

```go
package main

import (
    "time"
    "github.com/openshift/sippy/pkg/db"
    "github.com/openshift/sippy/pkg/db/partitions"
)

// Original model (what was created initially)
type TestResultsV1 struct {
    ID          uint      `gorm:"primaryKey"`
    TestName    string    `gorm:"index"`
    JobName     string    `gorm:"index"`
    Result      string
    CreatedAt   time.Time `gorm:"index"`
    TestOutput  string
    Duration    int
}

// Updated model with schema changes
type TestResultsV2 struct {
    ID          uint      `gorm:"primaryKey"`
    TestName    string    `gorm:"index"`
    JobName     string    `gorm:"index"`
    Result      string
    CreatedAt   time.Time `gorm:"index"`
    TestOutput  string
    Duration    int
    // New fields
    TestSuite   string    `gorm:"index"` // Added: track test suite
    ErrorCount  int                      // Added: count of errors
    // Removed: RemovedField no longer needed
}

func updateTestResultsSchema(dbc *db.DB) error {
    tableName := "test_results_partitioned"

    log.Info("Updating table schema to match new model...")

    // Step 1: Dry run to see what would change
    sql, err := partitions.UpdatePartitionedTable(
        dbc,
        &TestResultsV2{},
        tableName,
        true, // dry-run
    )
    if err != nil {
        return fmt.Errorf("dry run failed: %w", err)
    }

    log.Info("Schema changes that would be applied:")
    log.Info(sql)

    // Step 2: Review the changes and confirm
    fmt.Println("\nReview the changes above.")
    fmt.Print("Apply these changes? (yes/no): ")
    var response string
    fmt.Scanln(&response)

    if response != "yes" {
        log.Info("Schema update cancelled")
        return nil
    }

    // Step 3: Apply the changes
    sql, err = partitions.UpdatePartitionedTable(
        dbc,
        &TestResultsV2{},
        tableName,
        false, // execute
    )
    if err != nil {
        return fmt.Errorf("schema update failed: %w", err)
    }

    log.WithFields(log.Fields{
        "table":   tableName,
        "changes": sql,
    }).Info("schema updated successfully")

    return nil
}

// Automated schema migration (for CI/CD)
func automatedSchemaMigration(dbc *db.DB) error {
    tableName := "test_results_partitioned"

    // Check what changes would be made
    sql, err := partitions.UpdatePartitionedTable(
        dbc,
        &TestResultsV2{},
        tableName,
        true,
    )
    if err != nil {
        return fmt.Errorf("schema check failed: %w", err)
    }

    if sql == "" {
        log.Info("Schema is up to date, no changes needed")
        return nil
    }

    // Log the planned changes
    log.WithField("sql", sql).Info("applying schema changes")

    // Apply changes
    sql, err = partitions.UpdatePartitionedTable(
        dbc,
        &TestResultsV2{},
        tableName,
        false,
    )
    if err != nil {
        return fmt.Errorf("schema migration failed: %w", err)
    }

    log.Info("schema migration completed successfully")
    return nil
}

// Example: Gradual schema evolution
func evolveSchema(dbc *db.DB) error {
    tableName := "test_results_partitioned"

    // Phase 1: Add nullable columns first (safe)
    type PhaseOne struct {
        ID          uint      `gorm:"primaryKey"`
        CreatedAt   time.Time `gorm:"index"`
        TestName    string
        TestSuite   string    // New, nullable
    }

    log.Info("Phase 1: Adding nullable columns")
    _, err := partitions.UpdatePartitionedTable(dbc, &PhaseOne{}, tableName, false)
    if err != nil {
        return err
    }

    // Phase 2: Populate new columns with data
    log.Info("Phase 2: Populating new columns")
    // (Application code populates test_suite from test_name)

    // Phase 3: Add indexes after data is populated
    type PhaseTwo struct {
        ID          uint      `gorm:"primaryKey"`
        CreatedAt   time.Time `gorm:"index"`
        TestName    string
        TestSuite   string    `gorm:"index"` // Now indexed
    }

    log.Info("Phase 3: Adding indexes")
    _, err = partitions.UpdatePartitionedTable(dbc, &PhaseTwo{}, tableName, false)
    if err != nil {
        return err
    }

    log.Info("Schema evolution completed")
    return nil
}
```

**Key Scenarios**:

1. **Adding Columns**: New fields in the model are added to the table
2. **Removing Columns**: Fields removed from model are dropped (use caution)
3. **Changing Types**: Data type changes are detected and applied
4. **Adding Indexes**: New `gorm:"index"` tags create indexes
5. **Modifying Constraints**: NOT NULL and DEFAULT changes

**Best Practices**:
- Always run dry-run first to preview changes
- Review generated SQL before applying
- Test schema changes in a development environment first
- For production, consider gradual evolution (add nullable, populate, add constraints)
- Back up data before major type conversions
- Monitor query performance after index changes

---

### Example 11: Complete Workflow

See [examples.go](./examples.go) for a complete workflow demonstration including:
- Current state analysis
- Age distribution
- Retention policy comparison
- Dry run execution

---

## Integration with Automation

### Option 1: Kubernetes CronJob

```go
// In your scheduled job
func scheduledCleanup() {
    dbc := db.New(...)

    // 180-day retention policy
    dropped, err := partitions.DropOldPartitions(dbc, 180, false)
    if err != nil {
        log.WithError(err).Error("scheduled cleanup failed")
        return
    }

    log.WithField("dropped", dropped).Info("scheduled cleanup completed")
}
```

### Option 2: CLI Command

```go
func main() {
    retentionDays := flag.Int("retention-days", 180, "Retention period in days")
    dryRun := flag.Bool("dry-run", true, "Perform dry run only")
    flag.Parse()

    dbc := db.New(...)

    dropped, err := partitions.DropOldPartitions(dbc, *retentionDays, *dryRun)
    if err != nil {
        log.Fatal(err)
    }

    if *dryRun {
        fmt.Printf("DRY RUN: Would drop %d partitions\n", dropped)
    } else {
        fmt.Printf("Dropped %d partitions\n", dropped)
    }
}
```

---

## Safety Features

### Input Validation
- Partition names are validated against expected format
- SQL injection protection through parameterized queries
- Minimum retention period enforcement (30 days)

### Threshold Checks
- Maximum 75% of partitions can be deleted
- Maximum 80% of storage can be deleted
- Policy must be validated before execution

### Dry Run Support
- All destructive operations support dry-run mode
- Dry runs log what would happen without making changes
- Always test with dry-run first

### Comprehensive Logging
- All operations are logged with structured fields
- Errors include context for debugging
- Timing information for performance monitoring

---

## Error Handling

All functions return errors that should be checked:

```go
partitions, err := partitions.ListTablePartitions(dbc, "test_analysis_by_job_by_dates")
if err != nil {
    log.WithError(err).Error("failed to list partitions")
    return err
}
```

Common error scenarios:
- Database connection issues
- Permission denied (read-only user attempting writes)
- Invalid retention policy
- Partition name validation failures

---

## Testing

Run the test suite:

```bash
go test ./pkg/db/partitions/...
```

Test coverage includes:
- Partition name validation
- Struct initialization
- Edge cases and invalid inputs

---

## Detach/Archive Workflow

### Understanding Detached Partitions

When a partition is **detached**, it:
1. Becomes a standalone table (no longer part of the partitioned table)
2. Keeps all its data intact
3. Can still be queried directly by table name
4. Can be archived, compressed, or exported
5. Can be reattached if needed
6. Doesn't show up in queries against the parent table

### How to Find Detached Partitions

PostgreSQL tracks partition relationships in `pg_inherits`. Detached partitions:
- Still exist as tables in `pg_tables`
- Are NOT in the `pg_inherits` hierarchy
- Match the partition naming pattern

**Query to find them:**
```go
detached, err := partitions.ListDetachedPartitions(dbc, "test_analysis_by_job_by_dates")
// Returns all tables matching naming pattern but not attached
```

### Typical Detach/Archive Workflow

#### Step 1: Detach Old Partitions
```go
// Detach partitions older than 180 days
detached, err := partitions.DetachOldPartitions(dbc, 180, false)
log.Printf("Detached %d partitions\n", detached)
```

**Result**: Partitions are now standalone tables

#### Step 2: List Detached Partitions
```go
// Find all detached partitions
detached, err := partitions.ListDetachedPartitions(dbc, "test_analysis_by_job_by_dates")

for _, p := range detached {
    fmt.Printf("Detached: %s (%s)\n", p.TableName, p.SizePretty)
}
```

#### Step 3: Archive Detached Partitions
External archival process (examples):

**Option A: Export to CSV/Parquet**
```bash
# Export to compressed CSV
psql $SIPPY_DSN -c "
COPY test_analysis_by_job_by_dates_2024_10_29
TO STDOUT CSV HEADER
" | gzip > partition_2024_10_29.csv.gz

# Upload to S3
aws s3 cp partition_2024_10_29.csv.gz s3://sippy-archive/
```

**Option B: Use pg_dump**
```bash
pg_dump $SIPPY_DSN \
  -t test_analysis_by_job_by_dates_2024_10_29 \
  --format=custom \
  | gzip > partition_2024_10_29.pgdump.gz
```

**Option C: Direct S3 export (requires aws_s3 extension)**
```sql
SELECT aws_s3.query_export_to_s3(
    'SELECT * FROM test_analysis_by_job_by_dates_2024_10_29',
    aws_commons.create_s3_uri('sippy-archive', 'partitions/2024_10_29.parquet', 'us-east-1'),
    options := 'FORMAT PARQUET'
);
```

#### Step 4: Verify Archive
```bash
# Verify archive exists and is readable
aws s3 ls s3://sippy-archive/partition_2024_10_29.csv.gz
# Check file size matches expected
```

#### Step 5: Drop Detached Partitions

**Option A: Bulk drop old detached partitions (recommended)**
```go
// Drop all detached partitions older than 180 days
// (Assumes they have already been archived)

// Dry run first
dropped, err := partitions.DropOldDetachedPartitions(dbc, "test_analysis_by_job_by_dates", 180, true)
fmt.Printf("Would drop %d detached partitions\n", dropped)

// Actual drop
dropped, err = partitions.DropOldDetachedPartitions(dbc, "test_analysis_by_job_by_dates", 180, false)
fmt.Printf("Dropped %d detached partitions\n", dropped)
```

**Option B: Selective drop with archive verification**
```go
// After successful archive, drop detached partitions
detached, err := partitions.ListDetachedPartitions(dbc, "test_analysis_by_job_by_dates")

for _, p := range detached {
    // Verify this partition has been archived
    if isArchived(p.TableName) {
        err := partitions.DropPartition(dbc, p.TableName, false)
        if err != nil {
            log.WithError(err).Error("failed to drop detached partition")
        }
    }
}
```

#### Step 6: Restore if Needed
If you need to restore archived data:

1. **Restore from archive**:
```bash
# Restore table from pg_dump
gunzip -c partition_2024_10_29.pgdump.gz | pg_restore -d $SIPPY_DSN
```

2. **Reattach partition**:
```go
err := partitions.ReattachPartition(dbc, "test_analysis_by_job_by_dates_2024_10_29", false)
```

### Advantages of Detach vs. DROP

| Aspect | DETACH | DROP |
|--------|--------|------|
| **Reversible** | ✅ Yes (can reattach) | ❌ No (permanent) |
| **Data preserved** | ✅ Yes (in detached table) | ❌ No (deleted) |
| **Immediate space** | ❌ No (table still exists) | ✅ Yes (storage freed) |
| **Archive time** | ✅ After detach | ⚠️ Before drop |
| **Risk** | 🟢 Low | 🔴 High |
| **Speed** | ⚡ Fast | ⚡ Fast |
| **Query detached data** | ✅ Yes (by table name) | ❌ No (gone) |

### Complete Automation Example

```go
func automatedArchiveCleanup(dbc *db.DB, archiver Archiver) error {
    retentionDays := 180

    // 1. Detach old partitions
    detached, err := partitions.DetachOldPartitions(dbc, retentionDays, false)
    if err != nil {
        return err
    }

    log.Printf("Detached %d partitions\n", detached)

    // 2. Get list of detached partitions
    detachedList, err := partitions.ListDetachedPartitions(dbc)
    if err != nil {
        return err
    }

    // 3. Archive each detached partition
    for _, p := range detachedList {
        // Archive to S3
        err := archiver.Archive(p.TableName)
        if err != nil {
            log.WithError(err).WithField("partition", p.TableName).Error("archive failed")
            continue
        }

        // Verify archive
        if !archiver.Verify(p.TableName) {
            log.WithField("partition", p.TableName).Error("archive verification failed")
            continue
        }

        // Drop detached partition
        err = partitions.DropPartition(dbc, p.TableName, false)
        if err != nil {
            log.WithError(err).WithField("partition", p.TableName).Error("drop failed")
            continue
        }

        log.WithField("partition", p.TableName).Info("archived and dropped successfully")
    }

    return nil
}
```

---

## Related Documentation

- [Partition Retention Management Guide](../../../.claude/partition-retention-management-guide.md) - Complete guide with SQL examples
- [Database Schema Analysis](../../../.claude/db-schema-analysis.md) - Overall database structure
- [Database Analysis Index](../../../.claude/db-analysis-index.md) - Navigation to all analysis docs

---

## Recommended Retention Policies

Based on analysis in the retention management guide:

| Policy | Retention | Storage | Use Case |
|--------|-----------|---------|----------|
| Conservative | 365 days | ~900 GB | Full year of data, Y-o-Y comparisons |
| **Recommended** | **180 days** | **~450 GB** | **6 months, covers release cycles** |
| Aggressive | 90 days | ~225 GB | Recent CI health only, max savings |

**Current recommendation**: **180-day retention**
- Balances historical data access with storage efficiency
- Covers typical OpenShift release cycles
- Would reclaim ~160 GB immediately
- Stabilizes storage at ~450 GB

---

## Notes

- All operations require `*db.DB` instance (GORM wrapper)
- Read-only operations are safe with read-only database credentials
- Write operations require admin credentials
- Partition format: `test_analysis_by_job_by_dates_YYYY_MM_DD`
- Only `test_analysis_by_job_by_dates` partitions are supported currently
