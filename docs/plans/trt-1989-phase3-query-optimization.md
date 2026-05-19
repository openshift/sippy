# TRT-1989 Phase 3: Query Optimization Using Denormalized Columns

**Date:** 2026-05-19
**JIRA:** [TRT-1989](https://redhat.atlassian.net/browse/TRT-1989)
**Depends on:** Phase 1 (column prep), Phase 2 (indexes)

## Purpose

Phase 1 added denormalized `release` and `timestamp` columns to child
tables (`prow_job_run_tests`, `prow_job_run_test_outputs`,
`prow_job_run_prow_pull_requests`, `prow_job_run_annotations`). Phase 2
added composite indexes on those columns.

Nearly every significant query in sippy filters on
`prow_job_runs.timestamp` and/or `prow_jobs.release` via joins. Once these
tables are partitioned, those join-based filters **won't help the planner
prune child table partitions** — the planner needs WHERE clauses on each
partitioned table's own partition key columns.

This phase adds filters on the denormalized columns and drops joins where
all referenced columns have local replacements. This is safe to ship
before partitioning — the extra WHERE clauses let the planner use the
composite indexes from Phase 2. After partitioning, they become the
primary mechanism for partition pruning.

## Guiding Principles

1. **Add filters first, then replace when validated** — keep existing
   join-based filters alongside new local filters during rollout.
   After local denormalized columns are validated, replace old filters
   and drop no-longer-needed joins where safe.

2. **Drop joins only when safe** — a join can be dropped only if *every*
   column it provides (in SELECT, WHERE, GROUP BY, ORDER BY, FILTER) has
   a local replacement.

3. **Materialized views use `|||TIMENOW|||` templates** — filters added
   to mat views must use the same template tokens, not `$1`-style params.

4. **SQL functions use `$N` params** — new WHERE clauses reuse existing
   params from the function signature.

5. **GORM queries use `?` placeholders** — pass the same Go variables
   already available in the function scope.

## Changes by Query

### Group A: Queries starting from `prow_job_run_tests`

#### A1. `prowJobFailedTestsMatView` — `pkg/db/views.go`

Rewritten to start from `prow_job_run_tests`. Replaced
`prow_job_runs."timestamp"` with `pjrt.prow_job_run_timestamp` and
`prow_job_runs.prow_job_id` with `pjrt.prow_job_id`. **Dropped JOIN
`prow_job_runs`**.

#### A2. `testAnalysisByJobMatView` — `pkg/db/views.go`

Replaced all `prow_job_runs."timestamp"` references with
`prow_job_run_tests.prow_job_run_timestamp`. Replaced `prow_jobs.release`
with `prow_job_run_tests.prow_job_run_release`. **Dropped JOIN
`prow_job_runs`**. Rewired JOIN `prow_jobs` via
`prow_job_run_tests.prow_job_id`.

#### A3. `testReportMatView` — `pkg/db/views.go`

Replaced all `prow_job_runs."timestamp"` in WHERE and FILTER clauses with
`prow_job_run_tests.prow_job_run_timestamp`. Replaced `prow_jobs.release`
with `prow_job_run_tests.prow_job_run_release`. **Dropped JOIN
`prow_job_runs`**. Rewired JOIN `prow_jobs` via
`prow_job_run_tests.prow_job_id`. JOIN `prow_jobs` kept for
`prow_jobs.variants`.

#### A4. `test_results()` function — `pkg/db/functions.go`

Added `WHERE prow_job_run_tests.prow_job_run_timestamp BETWEEN $1 AND $3`
to limit the scan. Replaced `timestamp` in all CASE expressions with
`prow_job_run_tests.prow_job_run_timestamp`. Replaced `prow_jobs.release`
with `prow_job_run_tests.prow_job_run_release`. **Dropped JOINs
`prow_job_runs` and `prow_jobs`**.

#### A5. `ProwJobHistoricalTestCounts` — `pkg/db/query/job_queries.go`

Replaced `prow_job_runs.prow_job_id` with
`prow_job_run_tests.prow_job_id` and `prow_job_runs.timestamp` with
`prow_job_run_tests.prow_job_run_timestamp`. **Dropped JOIN
`prow_job_runs`**.

#### A6. `GetRecentTestFailures` — `pkg/api/recent_test_failures.go`

Added redundant local filters (`prow_job_run_tests.prow_job_run_timestamp`
and `prow_job_run_tests.prow_job_run_release`) to all four queries:
main query, NOT EXISTS subquery, last-pass lookback, and failure outputs.
Joins kept — `prow_job_runs` still needed for `timestamp` in SELECT and
`url`; `prow_jobs` still needed for `name`.

#### A7. `testStatusQuery` (CR) — `pkg/api/componentreadiness/.../provider.go`

Added `pjrt.prow_job_run_release = ?` and `pjrt.prow_job_run_timestamp`
range filters to the CTE WHERE clause. Joins kept — `prow_job_runs`
needed for `labels`, `prow_jobs` needed for variant lookup.

#### A8. `testDetailQuery` (CR) — `pkg/api/componentreadiness/.../provider.go`

Same pattern as A7 — added local release and timestamp filters. Joins
kept — `pjr.url`, `pjr.timestamp`, `pjr.labels`, `pj.name`, `pj.id`
still needed in SELECT.

#### A9. `payloadTestFailuresMatView` — `pkg/db/views.go`

Added `pjrt.prow_job_run_timestamp > (|||TIMENOW||| - '14 days'::interval)`
to WHERE. Joins kept — `release_tags`, `release_job_runs`, `prow_jobs`,
`prow_job_runs` still needed for other columns.

### Group B: Queries starting from `prow_job_run_test_outputs`

#### B1. `TestOutputs` — `pkg/db/query/test_queries.go`

Added `prow_job_run_test_outputs.prow_job_run_test_timestamp`,
`prow_job_run_test_outputs.prow_job_run_test_release`,
`prow_job_run_tests.prow_job_run_timestamp`, and
`prow_job_run_tests.prow_job_run_release` filters. Joins kept —
`prow_job_runs` for URL, `prow_jobs` for variants.

#### B2. `TestDurations` — `pkg/db/query/test_queries.go`

Replaced `prow_job_runs.timestamp` filter with
`prow_job_run_tests.prow_job_run_timestamp`. Replaced ambiguous
`"timestamp"` in SELECT/GROUP BY/ORDER BY with explicit
`prow_job_run_tests.prow_job_run_timestamp`. Replaced `prow_jobs.release`
with `prow_job_run_tests.prow_job_run_release`. **Dropped JOIN
`prow_job_runs`**. JOIN `prow_jobs` rewired via
`prow_job_run_tests.prow_job_id` (needed for variants).

### Group C: Queries on `prow_job_runs` directly

#### C1. `BuildClusterHealth` — `pkg/db/query/build_clusters.go`

Added `WHERE prow_job_runs.timestamp BETWEEN @start AND @end` so the
planner can use the timestamp index to limit the scan. The `@start` and
`@end` params already exist in the function signature.

#### C2-C4. No changes

- `BuildClusterAnalysis` — already has timestamp in WHERE, cross-release
- `HasBuildClusterData` — existence check, timestamp bound would be wrong
- `ProwJobRunIDs` — simple lookup, already indexed

### Group D: SQL functions with PR join tables

#### D1. `job_results()` — `pkg/db/functions.go`

- **`repo_org_jobs` CTE**: Added `WHERE prow_job_runs.prow_job_release = $1`
- **`merged_prs` CTE**: Added `AND prow_job_runs.timestamp BETWEEN $2 AND $4`
- **`results` CTE**: Added `WHERE prow_job_runs.prow_job_release = $1`
- **`last_pass` CTE**: No change — intentionally cross-release

#### D2. `jobRunsReportMatView` — No change

The CTEs materialize all data. Adding filters would require
parameterizing the mat view. Deferred to a future change.

## Joins Dropped Summary

| Query | Join dropped | Columns replaced |
|-------|-------------|-----------------|
| `prowJobFailedTestsMatView` | `prow_job_runs` | `timestamp` → `pjrt.prow_job_run_timestamp`, `prow_job_id` → `pjrt.prow_job_id` |
| `testAnalysisByJobMatView` | `prow_job_runs` | `timestamp` → local, `prow_job_id` → local; `prow_jobs` rewired via `prow_job_run_tests.prow_job_id` |
| `testReportMatView` | `prow_job_runs` | `timestamp` → local; `prow_jobs` rewired via `prow_job_run_tests.prow_job_id` |
| `test_results()` | `prow_job_runs` + `prow_jobs` | `timestamp` → local, `release` → local |
| `ProwJobHistoricalTestCounts` | `prow_job_runs` | `prow_job_id` → local, `timestamp` → local |
| `TestDurations` | `prow_job_runs` | `timestamp` → local; `prow_jobs` rewired via `prow_job_run_tests.prow_job_id` |
