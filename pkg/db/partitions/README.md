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
partitions, err := partitions.GetPartitionsForRemoval(dbc, 180)
if err != nil {
    log.WithError(err).Error("failed to get partitions for removal")
}

fmt.Printf("Found %d partitions older than 180 days\n", len(partitions))
```

**Parameters**:
- `retentionDays` - Retention period in days

**Returns**: `[]PartitionInfo` for partitions older than retention period (can be deleted or detached)

---

#### GetRetentionSummary
Provides a summary of what would be affected by a retention policy.

```go
summary, err := partitions.GetRetentionSummary(dbc, 180)
if err != nil {
    log.WithError(err).Error("failed to get summary")
}

fmt.Printf("Would delete %d partitions, reclaiming %s\n",
    summary.PartitionsToRemove, summary.StoragePretty)
```

**Parameters**:
- `retentionDays` - Retention period in days

**Returns**: `*RetentionSummary` containing:
- `RetentionDays` - Policy retention period
- `CutoffDate` - Date cutoff for removal
- `PartitionsToRemove` - Count of partitions to remove
- `StorageToReclaim` / `StoragePretty` - Storage to be freed
- `OldestPartition` / `NewestPartition` - Range of affected partitions

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

### Example 4: Complete Workflow

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
