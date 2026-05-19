# TRT-1989 Phase 2: Composite Indexes on Denormalized Columns

**Date:** 2026-05-19
**JIRA:** [TRT-1989](https://redhat.atlassian.net/browse/TRT-1989)
**Depends on:** Phase 1 — column prep (`trt-1989-partitioning-prep.md`)

## Purpose

Phase 1 added denormalized `release` and `timestamp` columns to every table
that will be partitioned or holds a FK into a partitioned table. Phase 2
adds composite indexes on those columns so the query planner can use them
immediately — before partitioning is applied.

These indexes mirror the future partition key `(release, timestamp)`. Once
the tables are partitioned, each partition inherits a local copy of the
index, and the planner uses partition pruning instead. The indexes added
here serve two purposes:

1. **Immediate benefit** — queries migrated in Phase 3 to filter on the
   denormalized columns will use these indexes on the current
   non-partitioned tables.
2. **Validation** — exercising the indexes under production workload
   confirms the column data is correct before committing to partitioning.

## Changes

All changes are GORM index tags on model structs in
`pkg/db/models/prow.go`. GORM `AutoMigrate` creates the indexes
automatically on the next migration run.

### prow_job_runs

Added composite index `idx_prow_job_runs_release_timestamp` across
`ProwJobRelease` and `Timestamp`.

Also added a standalone index on `ProwJobRunTest.ProwJobID` to support
variant queries that previously required joining through `prow_job_runs`.

### prow_job_run_tests

Added composite index `idx_prow_job_run_tests_release_timestamp` across
`ProwJobRunTimestamp` and `ProwJobRunRelease`.

### prow_job_run_test_outputs

Added composite index `idx_prow_job_run_test_outputs_release_timestamp`
across `ProwJobRunTestTimestamp` and `ProwJobRunTestRelease`.

### prow_job_run_prow_pull_requests

Added composite index
`idx_prow_job_run_prow_pull_requests_release_timestamp` across
`ProwJobRunRelease` and `ProwJobRunTimestamp`.

### prow_job_run_annotations

Added composite index `idx_prow_job_run_annotations_release_timestamp`
across `ProwJobRunRelease` and `ProwJobRunTimestamp`.

## Explicit SQL

GORM `AutoMigrate` will create these indexes on the next migration run.
If you prefer to create them manually — for example, using `CONCURRENTLY`
to avoid locking production tables — run these statements directly.

`CREATE INDEX CONCURRENTLY` cannot run inside a transaction, so each
statement must be executed individually (not wrapped in `BEGIN`/`COMMIT`).

### prow_job_runs

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prow_job_runs_release_timestamp
    ON prow_job_runs (prow_job_release, "timestamp");
```

### prow_job_run_tests

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prow_job_run_tests_prow_job_id
    ON prow_job_run_tests (prow_job_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prow_job_run_tests_release_timestamp
    ON prow_job_run_tests (prow_job_run_timestamp, prow_job_run_release);
```

### prow_job_run_test_outputs

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prow_job_run_test_outputs_release_timestamp
    ON prow_job_run_test_outputs (prow_job_run_test_timestamp, prow_job_run_test_release);
```

### prow_job_run_prow_pull_requests

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prow_job_run_prow_pull_requests_release_timestamp
    ON prow_job_run_prow_pull_requests (prow_job_run_release, prow_job_run_timestamp);
```

### prow_job_run_annotations

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prow_job_run_annotations_release_timestamp
    ON prow_job_run_annotations (prow_job_run_release, prow_job_run_timestamp);
```

## Notes

- **Safe to create before deploying model updates.** GORM `AutoMigrate`
  only adds — it never drops indexes, columns, or tables it doesn't
  recognize. Indexes created manually will persist through any number of
  `AutoMigrate` runs on the old model. Once the updated model with index
  tags is deployed, `AutoMigrate` sees the indexes already exist and
  skips them. There is no rollback risk.
- `CONCURRENTLY` avoids taking an exclusive lock on the table, allowing
  reads and writes to continue during index creation. It is slower but
  safe for production use.
- If the index already exists (e.g., GORM created it during a prior
  migration), `IF NOT EXISTS` makes the statement a no-op.
- GORM `AutoMigrate` does **not** use `CONCURRENTLY` — it takes a brief
  lock. On large tables this can block writes for the duration of the
  index build. For production deployments, prefer creating the indexes
  manually with the SQL above ahead of the code deploy, so that
  `AutoMigrate` finds them already in place.
- Column order in the index matches the expected query pattern: most
  queries filter on release first (equality), then timestamp (range).
  The `prow_job_run_tests` index leads with timestamp because the
  materialized view queries filter primarily on timestamp ranges.
