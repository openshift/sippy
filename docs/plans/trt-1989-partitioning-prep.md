# TRT-1989: Database Partitioning Preparation

**Date:** 2026-05-19
**JIRA:** [TRT-1989](https://redhat.atlassian.net/browse/TRT-1989)

## Problem Statement

The largest tables in sippy (`prow_job_runs`, `prow_job_run_tests`,
`prow_job_run_test_outputs`) grow continuously and are joined together in
nearly every significant query. Partitioning these tables by release and
timestamp would allow PostgreSQL to prune irrelevant partitions during
queries, dramatically reducing scan sizes and join costs.

PostgreSQL requires that a partitioned table's primary/unique key includes
the partition key columns. Foreign keys referencing a partitioned table must
match the full unique key. This means child tables that join to a partitioned
parent need the partition key columns — either to form composite FKs or to be
co-partitioned themselves.

## Approach

Denormalize `release` and `timestamp` from `prow_job_runs` (via `prow_jobs`)
onto every table that will be partitioned or that holds a FK into a
partitioned table. This is a prep step — no partitioning is applied yet, but
the columns are populated so the migration to partitioned tables can happen
in a subsequent PR.

## Tables Modified

### prow_job_runs

- Added `ProwJobRelease` (denormalized from `prow_jobs.release`)
- The `Timestamp` column already exists on this table

### prow_job_run_tests

- Added `ProwJobID` (avoids joining through `prow_job_runs` just to get the
  job ID, also needed for variant queries)
- Added `ProwJobRunRelease`
- Added `ProwJobRunTimestamp`

### prow_job_run_test_outputs

- Added `ProwJobRunTestRelease`
- Added `ProwJobRunTestTimestamp`
- Named with `ProwJobRunTest` prefix to avoid field name collisions when
  GORM preloads `ProwJobRunTestOutput` nested inside `ProwJobRunTest`

### prow_job_run_prow_pull_requests (many-to-many join table)

- Converted from implicit GORM-managed join table to explicit model
  (`ProwJobRunProwPullRequest`) using `SetupJoinTable`
- Added `ProwJobRunRelease` and `ProwJobRunTimestamp`
- Pull request association changed from GORM auto-association to manual
  insert so the denormalized fields are populated
- This table must migrate to partitioned at the same time as `prow_job_runs`
  because it holds a FK into that table

### prow_job_run_annotations

- Added `ProwJobRunRelease` and `ProwJobRunTimestamp`
- Same FK constraint as pull requests — must migrate with `prow_job_runs`

## Tables Not Modified (and why)

- **prow_jobs** — parent table, not partitioned, small (one row per job
  definition). Release already exists here as the source of truth.
- **tests, suites, test_ownerships, jira_components** — small dimension
  tables joined by their own primary key. No partition benefit.
- **bugs, bug_jobs, bug_tests** — small tables joined to `prow_jobs`, not
  to `prow_job_runs`. No partition benefit.
- **release_job_runs** — joins to `prow_job_run_tests` via
  `prow_job_run_id`, but is relatively small (one row per payload job run).
  Can be addressed later if needed.
- **RegressionJobRun** — stores a string `ProwJobRunID` from BigQuery, not
  a FK to `prow_job_runs`. Independent lifecycle.

## Migration Constraints

When `prow_job_runs` is partitioned:

1. Its primary key must include the partition key columns (e.g.
   `(id, prow_job_release, timestamp)`).
2. All tables with FKs into `prow_job_runs` must reference the full
   composite key — meaning they need the partition key columns too.
3. Tables with FKs **to** `prow_job_runs` (annotations, pull request
   join table) must either be co-partitioned or have their FKs dropped.

This means `prow_job_runs`, `prow_job_run_annotations`, and
`prow_job_run_prow_pull_requests` must all migrate to partitioned in a
single step. The cascade delete constraints on these relationships
(`constraint:OnDelete:CASCADE`) remain in place for now and will be
addressed during the partitioning migration.

## Backfill

Existing rows will have NULL/zero-value for the new columns. A backfill
migration is required before partitioning:

```sql
UPDATE prow_job_runs r
   SET prow_job_release = j.release
  FROM prow_jobs j
 WHERE r.prow_job_id = j.id
   AND (r.prow_job_release IS NULL OR r.prow_job_release = '');

UPDATE prow_job_run_tests t
   SET prow_job_id = r.prow_job_id,
       prow_job_run_release = j.release,
       prow_job_run_timestamp = r.timestamp
  FROM prow_job_runs r
  JOIN prow_jobs j ON r.prow_job_id = j.id
 WHERE t.prow_job_run_id = r.id
   AND (t.prow_job_run_release IS NULL OR t.prow_job_run_release = '');
```

Similar updates for `prow_job_run_test_outputs`, `prow_job_run_annotations`,
and `prow_job_run_prow_pull_requests`.

## Phased Migration Plan

The migration to partitioned tables can be done incrementally. Each phase
delivers value on its own and validates the approach before the next step.

### Phase 1: Column Prep (this PR)

Add denormalized release and timestamp columns to all tables. New data is
populated on insert. No query changes, no indexing changes.

### Phase 2: Backfill + Index

Backfill existing rows (see SQL above). Add composite indexes that mirror
the future partition key:

```sql
CREATE INDEX CONCURRENTLY idx_prow_job_runs_release_timestamp
    ON prow_job_runs (prow_job_release, timestamp);

CREATE INDEX CONCURRENTLY idx_prow_job_run_tests_release_timestamp
    ON prow_job_run_tests (prow_job_run_release, prow_job_run_timestamp);

CREATE INDEX CONCURRENTLY idx_prow_job_run_test_outputs_release_timestamp
    ON prow_job_run_test_outputs (prow_job_run_test_release, prow_job_run_test_timestamp);

CREATE INDEX CONCURRENTLY idx_prow_job_run_annotations_release_timestamp
    ON prow_job_run_annotations (prow_job_run_release, prow_job_run_timestamp);

CREATE INDEX CONCURRENTLY idx_prow_job_run_prow_pull_requests_release_timestamp
    ON prow_job_run_prow_pull_requests (prow_job_run_release, prow_job_run_timestamp);
```

### Phase 3: Migrate Queries

Update queries to filter on the denormalized columns instead of (or in
addition to) the joined parent columns. This can be done incrementally —
each query can be updated, validated with `EXPLAIN ANALYZE`, and merged
independently.

Queries currently filter on `prow_job_runs.timestamp` and
`prow_jobs.release` via joins. Once the child tables are partitioned,
those filters won't help the planner prune child table partitions. Each
query needs filters on the partitioned table's own columns.

#### Queries that need release + timestamp filters added

| Query | File | Currently filters on | Add filter on |
|-------|------|---------------------|---------------|
| `TestOutputs` | `pkg/db/query/test_queries.go:285` | `prow_job_runs.timestamp`, `prow_jobs.release` | `prow_job_run_test_outputs.prow_job_run_test_timestamp`, `.prow_job_run_test_release` |
| `TestDurations` | `pkg/db/query/test_queries.go:315` | `prow_job_runs.timestamp`, `prow_jobs.release` | `prow_job_run_tests.prow_job_run_timestamp`, `.prow_job_run_release` |
| `GetRecentTestFailures` | `pkg/api/recent_test_failures.go:32` | `prow_job_runs.timestamp`, `prow_jobs.release` | `prow_job_run_tests.prow_job_run_timestamp`, `.prow_job_run_release` |
| `testReportMatView` | `pkg/db/views.go:244-265` | `prow_job_runs."timestamp"` | `prow_job_run_tests.prow_job_run_timestamp`, `.prow_job_run_release` |
| `testAnalysisByJobMatView` | `pkg/db/views.go:298-308` | `prow_job_runs."timestamp"` | `prow_job_run_tests.prow_job_run_timestamp`, `.prow_job_run_release` |
| `prowJobFailedTestsMatView` | `pkg/db/views.go:314-322` | `prow_job_runs."timestamp"` (none explicit) | `prow_job_run_tests.prow_job_run_timestamp` |
| `payloadTestFailuresMatView` | `pkg/db/views.go:348` | `release_tags.release_time` | `prow_job_run_tests.prow_job_run_timestamp` |
| `testStatusQuery` (CR) | `pkg/api/componentreadiness/.../provider.go:312` | `pjr.timestamp`, `pj.release` | `pjrt.prow_job_run_timestamp`, `pjrt.prow_job_run_release` |
| `testDetailQuery` (CR) | `pkg/api/componentreadiness/.../provider.go:518` | `pjr.timestamp`, `pj.release` | `pjrt.prow_job_run_timestamp`, `pjrt.prow_job_run_release` |
| `test_results()` function | `pkg/db/functions.go:51-53` | `timestamp` (via join) | `prow_job_run_tests.prow_job_run_timestamp`, `.prow_job_run_release` |

#### Queries that need filters added (currently have none)

| Query | File | Tables scanned | Fix |
|-------|------|---------------|-----|
| `jobRunsReportMatView` CTEs | `pkg/db/views.go:157-192` | `prow_job_run_tests`, `prow_job_run_prow_pull_requests` | Add timestamp/release filters to each CTE |
| `job_results()` `repo_org_jobs` CTE | `pkg/db/functions.go:82-88` | `prow_job_runs`, `prow_job_run_prow_pull_requests` | Add release + timestamp filter |
| `job_results()` `merged_prs` CTE | `pkg/db/functions.go:91-99` | `prow_job_runs`, `prow_job_run_prow_pull_requests` | Add `prow_job_runs.timestamp` filter alongside `merged_at` |
| `HasBuildClusterData` | `pkg/db/query/build_clusters.go:14` | `prow_job_runs` | Add timestamp bound |
| `PrintOverallReleaseHealthFromDB` | `pkg/api/health.go:88` | `prow_job_runs` | Add release filter or timestamp bound |
| `PrintAutocompleteFromDB` (cluster) | `pkg/api/autocomplete.go:77` | `prow_job_runs` | Add timestamp bound |
| `ProwJobRunIDs` | `pkg/db/query/job_queries.go:59` | `prow_job_runs` | Add timestamp parameters |
| `BuildClusterHealth` | `pkg/db/query/build_clusters.go:21` | `prow_job_runs` | Move timestamp from CASE to WHERE |
| `BuildClusterAnalysis` | `pkg/db/query/build_clusters.go:60` | `prow_job_runs` | Add release filter |

#### Queries that can potentially drop joins

Once queries filter on the denormalized columns directly, some joins
become unnecessary. For example, queries that only joined
`prow_job_runs` to get `timestamp` or joined `prow_jobs` to get
`release` can use the local columns instead. This reduces join cost
independent of partitioning.

| Query | Join that can be dropped | Reason |
|-------|-------------------------|--------|
| `testAnalysisByJobMatView` | `JOIN prow_job_runs` | Only used for `timestamp` and to reach `prow_jobs` |
| `prowJobFailedTestsMatView` | `JOIN prow_job_runs` (partial) | Used for `timestamp` and `prow_job_id` — both now on `prow_job_run_tests` |
| `test_results()` function | `JOIN prow_job_runs` | Only used for `timestamp` and to reach `prow_jobs.release` |

### Phase 4: Partition Tables

With columns populated, indexes in place, and queries updated, apply
partitioning. This requires:

1. Migrate `prow_job_runs`, `prow_job_run_annotations`, and
   `prow_job_run_prow_pull_requests` together (FK constraints).
2. Migrate `prow_job_run_tests` (FK to `prow_job_runs` must use
   composite key).
3. Migrate `prow_job_run_test_outputs` (FK to `prow_job_run_tests`
   must use composite key).
4. Update cascade delete constraints or drop them in favor of
   partition-based data lifecycle management.
5. Validate with `EXPLAIN ANALYZE` that partition pruning is active.
