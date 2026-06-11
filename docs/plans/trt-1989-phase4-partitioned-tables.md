# TRT-1989 Phase 4: Partitioned Table DDL Migration Plan

## Context

Sippy's largest tables are growing continuously and being joined in nearly every significant query. Converting these tables to PostgreSQL partitioned tables will enable:

- **Partition pruning** during queries (dramatically reduced scan sizes)
- **Independent partition management** (drop old partitions instead of DELETE)
- **Improved query performance** via reduced join costs

**Prerequisites completed:**
- ✅ Phase 1: Denormalized `release` and `timestamp` columns added to all child tables
- ✅ Phase 2: Composite indexes on `(release, timestamp)` created
- ✅ Phase 3: Queries updated to filter on denormalized columns

**This phase** creates partitioned versions of the 3 largest/most query-intensive tables. The remaining `prow_job_runs`-family tables (`prow_job_runs`, `prow_job_run_annotations`, `prow_job_run_prow_pull_requests`) are deferred to a future phase.

## Phased Table Partitioning Strategy

### Phase 4a (This Phase): Largest Tables + Test Analysis

These tables are the highest-impact targets — the largest by row count and most frequently scanned in queries:

| Table | Partitioning | Rationale |
|-------|-------------|-----------|
| `prow_job_run_tests` | LIST by release → RANGE by timestamp (daily) | Largest table (~816M rows), hottest indexes (164M scans on `prow_job_run_id`) |
| `prow_job_run_test_outputs` | LIST by release → RANGE by timestamp (daily) | Second largest table, stores test output text blobs |
| `test_analysis_by_job_by_dates` | LIST by release | Pre-aggregated analysis table, natural partition boundary on release |

### Phase 4b (Future Phase): Remaining ProwJobRun-Based Tables

The following tables depend on `prow_job_runs` and will be partitioned together in a future phase, along with query optimization work for their specific access patterns:

| Table | Planned Partitioning | Notes |
|-------|---------------------|-------|
| `prow_job_runs` | LIST by release → RANGE by timestamp (daily) | Parent table for annotations and pull requests; must migrate WITH dependents |
| `prow_job_run_annotations` | LIST by release → RANGE by timestamp (daily) | FK dependency on `prow_job_runs` |
| `prow_job_run_prow_pull_requests` | LIST by release → RANGE by timestamp (daily) | FK dependency on `prow_job_runs` |

Phase 4b will also include query performance optimization for queries that join through `prow_job_runs` to its child tables.

## Migration Strategy Overview

The migration creates partitioned tables directly with their production names using `CREATE TABLE IF NOT EXISTS`. This approach works because:

1. On **fresh installs**: Tables are created as partitioned from the start
2. On **existing databases**: `IF NOT EXISTS` is a no-op; a separate data migration + table swap process handles the transition

Migration files are under `pkg/db/migrations/` using golang-migrate's pattern.

## Migration File Structure

### 000001_create_partitioned_tables.up.sql

Creates 3 partitioned tables with all required indexes:

1. `prow_job_run_tests` — LIST by `prow_job_run_release` → RANGE by `prow_job_run_timestamp`
2. `prow_job_run_test_outputs` — LIST by `prow_job_run_test_release` → RANGE by `prow_job_run_test_timestamp`
3. `test_analysis_by_job_by_dates` — LIST by `release`

Partition creation (release-level and daily sub-partitions) is handled separately via the partition management system.

### 000001_create_partitioned_tables.down.sql

Drops all 3 tables:
- `DROP TABLE IF EXISTS prow_job_run_tests CASCADE;`
- `DROP TABLE IF EXISTS prow_job_run_test_outputs CASCADE;`
- `DROP TABLE IF EXISTS test_analysis_by_job_by_dates CASCADE;`

## DDL Details per Table

All tables below are defined in `000001_create_partitioned_tables.up.sql`.

### 1. prow_job_run_tests

**Primary Key:**
`PRIMARY KEY (id, prow_job_run_release, prow_job_run_timestamp)`

**Partition Strategy:**
```sql
PARTITION BY LIST (prow_job_run_release)
-- Each release partition is further partitioned by RANGE (prow_job_run_timestamp)
```

**Structure:**
```
prow_job_run_tests (LIST by prow_job_run_release)
  ├─ prow_job_run_tests_p4_17 PARTITION BY RANGE (prow_job_run_timestamp)
  │   └─ Daily sub-partitions (managed separately)
  ├─ prow_job_run_tests_p4_18 PARTITION BY RANGE (prow_job_run_timestamp)
  │   └─ Daily sub-partitions (managed separately)
  └─ prow_job_run_tests_default (catches unmapped releases)
```

**Indexes:**
1. ✅ `idx_prow_job_run_tests_release_timestamp` on `(prow_job_run_timestamp, prow_job_run_release)` — partition pruning (timestamp first for range filters)
2. ✅ `idx_prow_job_run_tests_prow_job_run_id` on `(prow_job_run_id)` — FK join to prow_job_runs (HOTTEST: 164M scans)
3. ✅ `idx_prow_job_run_tests_test_id` on `(test_id)` — FK join to tests (16M scans)
4. ✅ `idx_prow_job_run_tests_status` on `(status)` — result filtering (2.7M scans)
5. ✅ `idx_prow_job_run_tests_prow_job_id` on `(prow_job_id)` — variant queries (Phase 2 addition)
6. ✅ `idx_prow_job_run_tests_test_id_status` on `(test_id, status)` — composite for Component Readiness queries
7. ⚠️ **SKIP**: `idx_prow_job_run_tests_deleted_at` (unused, 9.6 GB waste)
8. ⚠️ **SKIP**: `idx_prow_job_run_tests_created_at` (11 scans only, 7.4 GB waste)
9. ⚠️ **SKIP**: `idx_prow_job_run_tests_suite_id` (inefficient: 20K scans, 10 GB)

**Space Savings:** ~27 GB by dropping unused indexes

### 2. prow_job_run_test_outputs

**Primary Key:**
`PRIMARY KEY (id, prow_job_run_test_release, prow_job_run_test_timestamp)`

**Partition Strategy:**
```sql
PARTITION BY LIST (prow_job_run_test_release)
-- Each release partition is further partitioned by RANGE (prow_job_run_test_timestamp)
```

**Indexes:**
1. ✅ `idx_prow_job_run_test_outputs_release_timestamp` on `(prow_job_run_test_timestamp, prow_job_run_test_release)` — partition pruning
2. ✅ `idx_prow_job_run_test_outputs_prow_job_run_test_id` on `(prow_job_run_test_id)` — FK join to prow_job_run_tests
3. ⚠️ **SKIP**: `idx_prow_job_run_test_outputs_created_at` (not used for business queries)

### 3. test_analysis_by_job_by_dates

**Partition Strategy:**
```sql
PARTITION BY LIST (release)
-- Single-level LIST partitioning (no nested RANGE — date is part of the unique index instead)
```

This table uses single-level LIST partitioning by release because it's a pre-aggregated analysis table with one row per (date, test_id, release, job_name) combination. Daily RANGE sub-partitioning is unnecessary since the `date` column is already part of the unique constraint.

**Indexes:**
1. ✅ `idx_test_analysis_test_release_date` (UNIQUE) on `(date, test_id, release, job_name)` — matches original `test_release_date` constraint
2. ✅ `idx_test_analysis_name_and_job` on `(test_name, job_name)` — matches original lookup index

**Note:** This table has no explicit primary key — uniqueness is enforced by the unique index.

## Referential Integrity Strategy

**Foreign key constraints will NOT be created** on the partitioned tables for the following reasons:

### Why No FKs

1. **Performance at Scale**: With nested LIST→RANGE daily partitioning across multiple releases, tables will have 1,000+ partitions. Each partition creates its own FK constraint trigger, causing significant INSERT/UPDATE overhead.

2. **Partition-Based Lifecycle**: When old partitions are detached/dropped, associated child data in dependent tables is automatically removed when their partitions are dropped.

3. **Application-Level Integrity**: Sippy's data loader (`pkg/dataloader/prowloader/`) controls all writes and maintains referential integrity through atomic transaction boundaries and lookup of parent IDs before child inserts.

4. **Write Throughput**: Sippy ingests high volumes of CI test results. FK validation across thousands of partitions would create a bottleneck.

### Integrity Guarantees

**Still enforced at database level:**
- All UNIQUE constraints
- All NOT NULL constraints
- Primary key constraints (composite, include partition keys)

**Application-level:**
- Prowloader ensures parent exists before inserting children
- Partition lifecycle management handles cleanup
- Indexes on foreign key columns enable efficient joins

### Monitoring

Add application-level checks to detect orphaned records:

```sql
-- Periodic validation: Find orphaned prow_job_run_tests
SELECT COUNT(*)
FROM prow_job_run_tests pjrt
LEFT JOIN prow_job_runs pjr
    ON pjrt.prow_job_run_id = pjr.id
    AND pjrt.prow_job_run_release = pjr.prow_job_release
    AND pjrt.prow_job_run_timestamp = pjr.timestamp
WHERE pjr.id IS NULL;
```

## Data Migration SQL (For Existing Databases)

For databases with existing non-partitioned tables, standalone SQL scripts handle data migration. These scripts are **idempotent** and can be run multiple times — they only copy data not already migrated.

The new partitioned tables must first be created with temporary `_new` suffix (or the migration applied to an empty database), then data migrated, then tables swapped.

### migrate_prow_job_run_tests_data.sql
```sql
DO $$
DECLARE
    max_created_at TIMESTAMP;
BEGIN
    SELECT MAX(created_at) INTO max_created_at FROM prow_job_run_tests_new;

    INSERT INTO prow_job_run_tests_new (
        id, created_at, updated_at, deleted_at,
        prow_job_run_id, prow_job_id, prow_job_run_timestamp,
        prow_job_run_release, test_id, suite_id, status, duration
    )
    SELECT
        id, created_at, updated_at, deleted_at,
        prow_job_run_id, prow_job_id, prow_job_run_timestamp,
        prow_job_run_release, test_id, suite_id, status, duration
    FROM prow_job_run_tests
    WHERE created_at > COALESCE(max_created_at, '-infinity'::timestamp)
    ORDER BY created_at, id;
END $$;
```

### migrate_prow_job_run_test_outputs_data.sql
```sql
DO $$
DECLARE
    max_created_at TIMESTAMP;
BEGIN
    SELECT MAX(created_at) INTO max_created_at FROM prow_job_run_test_outputs_new;

    INSERT INTO prow_job_run_test_outputs_new (
        id, created_at, updated_at, deleted_at,
        prow_job_run_test_id, output,
        prow_job_run_test_timestamp, prow_job_run_test_release
    )
    SELECT
        id, created_at, updated_at, deleted_at,
        prow_job_run_test_id, output,
        prow_job_run_test_timestamp, prow_job_run_test_release
    FROM prow_job_run_test_outputs
    WHERE created_at > COALESCE(max_created_at, '-infinity'::timestamp)
    ORDER BY created_at, id;
END $$;
```

### migrate_test_analysis_data.sql
```sql
INSERT INTO test_analysis_by_job_by_dates_new (
    date, test_id, release, job_name, test_name,
    runs, passes, flakes, failures
)
SELECT
    date, test_id, release, job_name, test_name,
    runs, passes, flakes, failures
FROM test_analysis_by_job_by_dates
ON CONFLICT (date, test_id, release, job_name) DO NOTHING;
```

## Identity Sequence Sync SQL

After data migration, sync sequences to ensure new inserts don't conflict:

```sql
SELECT setval('prow_job_run_tests_new_id_seq',
    (SELECT COALESCE(MAX(id), 1) FROM prow_job_run_tests_new), true);

SELECT setval('prow_job_run_test_outputs_new_id_seq',
    (SELECT COALESCE(MAX(id), 1) FROM prow_job_run_test_outputs_new), true);
```

(`test_analysis_by_job_by_dates` has no identity sequence.)

## Table Swap SQL (Atomic Rename)

After data migration and validation, swap tables atomically:

```sql
BEGIN;

-- Drop foreign keys pointing at old tables
ALTER TABLE prow_job_run_test_outputs DROP CONSTRAINT IF EXISTS fk_prow_job_run_test_outputs_prow_job_run_test_id;

-- Rename old sequences
ALTER SEQUENCE prow_job_run_tests_id_seq RENAME TO prow_job_run_tests_old_id_seq;
ALTER SEQUENCE prow_job_run_test_outputs_id_seq RENAME TO prow_job_run_test_outputs_old_id_seq;

-- Rename old tables
ALTER TABLE prow_job_run_tests RENAME TO prow_job_run_tests_old;
ALTER TABLE prow_job_run_test_outputs RENAME TO prow_job_run_test_outputs_old;
ALTER TABLE test_analysis_by_job_by_dates RENAME TO test_analysis_by_job_by_dates_old;

-- Rename new sequences to production names
ALTER SEQUENCE prow_job_run_tests_new_id_seq RENAME TO prow_job_run_tests_id_seq;
ALTER SEQUENCE prow_job_run_test_outputs_new_id_seq RENAME TO prow_job_run_test_outputs_id_seq;

-- Rename new tables to production names
ALTER TABLE prow_job_run_tests_new RENAME TO prow_job_run_tests;
ALTER TABLE prow_job_run_test_outputs_new RENAME TO prow_job_run_test_outputs;
ALTER TABLE test_analysis_by_job_by_dates_new RENAME TO test_analysis_by_job_by_dates;

COMMIT;
```

## GORM Model Updates

**CRITICAL**: After table swap, GORM models MUST be updated to include partition keys in `primaryKey` tags.

**Models requiring updates:**

1. **ProwJobRunTest** — Add `primaryKey` to partition columns
   ```go
   type ProwJobRunTest struct {
       gorm.Model
       ProwJobRunTimestamp time.Time `gorm:"primaryKey;index:..."`
       ProwJobRunRelease   string    `gorm:"primaryKey;index:..."`
       // ... other fields
   }
   ```
   Database PK: `(id, prow_job_run_release, prow_job_run_timestamp)`

2. **ProwJobRunTestOutput** — Add `primaryKey` to partition columns
   ```go
   type ProwJobRunTestOutput struct {
       gorm.Model
       ProwJobRunTestTimestamp time.Time `gorm:"primaryKey;index:..."`
       ProwJobRunTestRelease   string    `gorm:"primaryKey;index:..."`
       // ... other fields
   }
   ```
   Database PK: `(id, prow_job_run_test_release, prow_job_run_test_timestamp)`

3. **TestAnalysisByJobByDate** — No `gorm.Model`; update partition key handling as needed

**Why Required:**
Without `primaryKey` tags on partition columns, GORM generates `ON CONFLICT` clauses targeting only the `id` column, which doesn't match any unique constraint on partitioned tables:
```
ERROR: there is no unique or exclusion constraint matching the ON CONFLICT specification (SQLSTATE 42P10)
```

## Partition Management

Partition creation is NOT included in the migration files. The migration DDL only creates parent table definitions. Actual partitions are created and managed by the partition management system (`pkg/db/PARTITIONS_README.md`).

**Example partition structure:**
```sql
-- Level 1: Release partition (LIST)
CREATE TABLE prow_job_run_tests_p4_18 PARTITION OF prow_job_run_tests
    FOR VALUES IN ('4.18')
    PARTITION BY RANGE (prow_job_run_timestamp);

-- Level 2: Daily sub-partitions (RANGE) within 4.18
CREATE TABLE prow_job_run_tests_p4_18_2026_05_24 PARTITION OF prow_job_run_tests_p4_18
    FOR VALUES FROM ('2026-05-24') TO ('2026-05-25');
```

For `test_analysis_by_job_by_dates`, only LIST partitions are created (no daily sub-partitions):
```sql
CREATE TABLE test_analysis_by_job_by_dates_p4_18 PARTITION OF test_analysis_by_job_by_dates
    FOR VALUES IN ('4.18');
```

## Validation Steps

### After DDL Migration
```sql
-- Verify partitioned tables exist
SELECT tablename FROM pg_tables
WHERE tablename IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates');

-- Verify indexes
SELECT indexname, tablename FROM pg_indexes
WHERE tablename IN ('prow_job_run_tests', 'prow_job_run_test_outputs', 'test_analysis_by_job_by_dates')
ORDER BY tablename, indexname;
```

### After Data Migration
```sql
SELECT 'prow_job_run_tests' AS table_name,
    (SELECT COUNT(*) FROM prow_job_run_tests_old) AS old_count,
    (SELECT COUNT(*) FROM prow_job_run_tests) AS new_count;

SELECT 'prow_job_run_test_outputs' AS table_name,
    (SELECT COUNT(*) FROM prow_job_run_test_outputs_old) AS old_count,
    (SELECT COUNT(*) FROM prow_job_run_test_outputs) AS new_count;

SELECT 'test_analysis_by_job_by_dates' AS table_name,
    (SELECT COUNT(*) FROM test_analysis_by_job_by_dates_old) AS old_count,
    (SELECT COUNT(*) FROM test_analysis_by_job_by_dates) AS new_count;
```

### After Table Swap
```sql
-- Verify partition pruning is working
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM prow_job_run_tests
WHERE prow_job_run_timestamp > NOW() - INTERVAL '7 days'
  AND prow_job_run_release = '4.18';
-- Should show "Partitions pruned: N" in output
```

## Migration Execution Plan

### Step 1: Apply DDL Migrations (Non-disruptive)
```bash
go run ./cmd/sippy migrate --database-dsn "$SIPPY_PRODLIKE_DATABASE_DSN"
```
**Impact:** None — creates new empty partitioned tables

### Step 2: Data Migration (Can Run Multiple Times)
```bash
psql "$SIPPY_PRODLIKE_DATABASE_DSN" -f scripts/migrate_prow_job_run_tests_data.sql
psql "$SIPPY_PRODLIKE_DATABASE_DSN" -f scripts/migrate_prow_job_run_test_outputs_data.sql
psql "$SIPPY_PRODLIKE_DATABASE_DSN" -f scripts/migrate_test_analysis_data.sql
```
**Duration:** Initial run depends on data size (estimate 1-2 hours for ~816M rows in prow_job_run_tests). Subsequent runs only migrate new data.

### Step 3: Sequence Sync
```bash
psql "$SIPPY_PRODLIKE_DATABASE_DSN" -f scripts/sync_sequences.sql
```

### Step 4: Validation
Run validation queries from the section above.

### Step 5: Atomic Table Swap (Brief Outage)
```bash
psql "$SIPPY_PRODLIKE_DATABASE_DSN" -f scripts/swap_tables.sql
```
**Duration:** Seconds (single transaction with table renames)

### Step 6: Update GORM Models (Code Change)
Update `primaryKey` tags in `pkg/db/models/prow.go` for `ProwJobRunTest` and `ProwJobRunTestOutput`.

### Step 7: Restart and Verify
```bash
go run ./cmd/sippy serve --database-dsn "$SIPPY_PRODLIKE_DATABASE_DSN"
```

## Rollback Plan

If issues are discovered after table swap:

```sql
BEGIN;

-- Rename partitioned sequences back to _new
ALTER SEQUENCE prow_job_run_tests_id_seq RENAME TO prow_job_run_tests_new_id_seq;
ALTER SEQUENCE prow_job_run_test_outputs_id_seq RENAME TO prow_job_run_test_outputs_new_id_seq;

-- Rename partitioned tables back to _new
ALTER TABLE prow_job_run_tests RENAME TO prow_job_run_tests_new;
ALTER TABLE prow_job_run_test_outputs RENAME TO prow_job_run_test_outputs_new;
ALTER TABLE test_analysis_by_job_by_dates RENAME TO test_analysis_by_job_by_dates_new;

-- Restore old sequences
ALTER SEQUENCE prow_job_run_tests_old_id_seq RENAME TO prow_job_run_tests_id_seq;
ALTER SEQUENCE prow_job_run_test_outputs_old_id_seq RENAME TO prow_job_run_test_outputs_id_seq;

-- Restore old tables
ALTER TABLE prow_job_run_tests_old RENAME TO prow_job_run_tests;
ALTER TABLE prow_job_run_test_outputs_old RENAME TO prow_job_run_test_outputs;
ALTER TABLE test_analysis_by_job_by_dates_old RENAME TO test_analysis_by_job_by_dates;

-- Recreate original FK
ALTER TABLE prow_job_run_test_outputs
    ADD CONSTRAINT fk_prow_job_run_test_outputs_prow_job_run_test_id
    FOREIGN KEY (prow_job_run_test_id)
    REFERENCES prow_job_run_tests (id)
    ON DELETE CASCADE;

COMMIT;
```

After database rollback, revert GORM model changes (remove `primaryKey` from partition columns).

## Space Savings

By dropping unused indexes on `prow_job_run_tests`: ~27 GB (dropped `deleted_at`, `created_at`, `suite_id` indexes)

## Future Work

### Phase 4b: Remaining ProwJobRun-Based Tables
Partition `prow_job_runs`, `prow_job_run_annotations`, and `prow_job_run_prow_pull_requests` using the same nested LIST→RANGE strategy. This phase will:
- Partition all 3 tables together (they share FK dependencies on `prow_job_runs`)
- Optimize queries that join through `prow_job_runs` to its child tables
- Update GORM models for `ProwJobRun`, `ProwJobRunAnnotation`, and `ProwJobRunProwPullRequest`

### Post-Migration
1. **Partition lifecycle management** — Implement partition creation for new releases and daily ranges
2. **Partition retention** — Set up detach/drop workflow for partitions older than retention period
3. **Monitoring** — Add metrics for partition count/size, orphaned record detection, write throughput
4. **Cleanup** — Drop `_old` tables after 30-day safety period

## References

- `pkg/db/models/prow.go` — Model definitions
- `pkg/db/migrations/000001_create_partitioned_tables.up.sql` — Migration DDL
- `pkg/db/PARTITIONS_README.md` — Partition management API spec
- `pkg/db/MIGRATION_README.md` — Migration workflow spec
- `docs/plans/trt-1989-partitioning-prep.md` — Phase 1-3 background
- `docs/plans/trt-1989-phase3-query-optimization.md` — Query updates
- `.claude/db-index-usage-analysis.md` — Index usage analysis
