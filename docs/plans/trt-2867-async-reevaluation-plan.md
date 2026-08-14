# TRT-2867: Async Symptom Re-evaluation via River Job Queue

## Overview

Move the synchronous symptom re-evaluation API (`POST /api/jobs/runs/reevaluate`) to an
asynchronous batch model. The API handler creates a batch request and enqueues individual
re-evaluation work items via [River](https://github.com/riverqueue/river), a PostgreSQL-backed
job queue. The sippy-daemon processes work items using the existing `ReEvaluator` logic. The UI
polls a status endpoint for progress until all items complete.

**Jira:** [TRT-2867](https://redhat.atlassian.net/browse/TRT-2867)
**Predecessor:** [TRT-2695](https://redhat.atlassian.net/browse/TRT-2695) — synchronous
re-evaluation API (already implemented)

## Motivation

The current `POST /api/jobs/runs/reevaluate` endpoint processes all requested job runs
synchronously in the HTTP request. For large batches this causes:

- HTTP timeouts when evaluating many job runs against many symptoms
- No progress visibility — the UI blocks until the entire batch completes
- No deduplication — concurrent requests can re-evaluate the same job run, causing BigQuery
  streaming buffer conflicts (rows are not deletable within 90 minutes of insertion)
- No retry of individual failures — one GCS timeout fails the entire request

## Design Principles

1. **Reuse, don't rewrite** — the existing `ReEvaluator.reEvaluateOne()` becomes the work unit
   inside each River job worker, unchanged.
2. **Appropriate abstraction** — the new package (`pkg/sippyserver/workqueue`) is generic enough
   to support future async workloads (report generation, bulk annotation, etc.) without being
   tied to symptom re-evaluation specifics.
3. **River owns execution and dedup; application tables own batch semantics** — River handles job
   scheduling, concurrency, retries, and uniqueness. Custom tables handle batch-to-item
   associations and user-facing status.

## Prerequisites: Orientation

Before writing code, read and understand these files in addition to those listed in the
TRT-2695 plan:

| File | What to learn |
|------|---------------|
| `pkg/sippyserver/daemon_server.go` | `DaemonProcess` interface and `DaemonServer.Serve()` goroutine lifecycle. Each process runs independently with its own context. |
| `pkg/sippyserver/pr_commenting_processor.go` | `WorkProcessor` — existing daemon process example. Ticker-based polling, bounded worker goroutines, channel-based dispatch. |
| `cmd/sippy-daemon/main.go` | Daemon process registration and startup. New processes added via `processes = append(processes, ...)`. |
| `pkg/api/jobrunscan/reevaluate.go` | `ReEvaluator` struct, `ReEvaluateJobRuns()`, `reEvaluateOne()`. The synchronous implementation to be reused as the per-item worker. |
| `pkg/sippyserver/job_run_scan.go` | Current HTTP handler `jsonReEvaluateJobRunSymptoms` — the handler to be replaced with async dispatch. |

## Dependency: River and pgx/v5

River requires `jackc/pgx/v5`. Sippy currently uses `jackc/pgx/v4` (v4.18.2).

**Approach:** Add `pgx/v5` alongside the existing `pgx/v4` dependency. Create a separate
`*pgxpool.Pool` (v5) for River's use. The rest of Sippy continues using v4 + gorm until a
broader migration is undertaken. Both driver versions can coexist in a Go module; they have
different import paths (`github.com/jackc/pgx/v4` vs `github.com/jackc/pgx/v5`).

## Step 1: Add the `pkg/sippyserver/workqueue` Package

Create a generic work queue abstraction in `pkg/sippyserver/workqueue/`. This package wraps
River and provides the batch submission and status query patterns used by the re-evaluation
API, but is designed for reuse by future async workloads.

### 1.1: Database models (`pkg/sippyserver/workqueue/models.go`)

```go
type BatchStatus string

const (
    BatchStatusPending  BatchStatus = "pending"
    BatchStatusRunning  BatchStatus = "running"
    BatchStatusComplete BatchStatus = "complete"
    BatchStatusFailed   BatchStatus = "failed"
)

// Batch represents a user-initiated batch of work items.
// A batch groups related work items for status tracking and progress reporting.
type Batch struct {
    ID             uuid.UUID   `gorm:"type:uuid;primaryKey"          json:"id"`
    Kind           string      `gorm:"not null;index"                json:"kind"`
    RequestedCount int         `gorm:"not null"                      json:"requested_count"`
    EnqueuedCount  int         `gorm:"not null"                      json:"enqueued_count"`
    DedupedCount   int         `gorm:"not null"                      json:"deduped_count"`
    Status         BatchStatus `gorm:"not null;default:'pending'"    json:"status"`
    CreatedAt      time.Time   `gorm:"autoCreateTime"                json:"created_at"`
    CompletedAt    *time.Time  `                                     json:"completed_at,omitempty"`
}

// BatchItem associates a batch with a River job for many-to-many status tracking.
// Multiple batches can reference the same River job (when deduplication occurs).
type BatchItem struct {
    ID         uint64    `gorm:"primaryKey;autoIncrement"                    json:"id"`
    BatchID    uuid.UUID `gorm:"type:uuid;not null;index:idx_batch_items"    json:"batch_id"`
    RiverJobID int64     `gorm:"not null;index:idx_batch_items"              json:"river_job_id"`
    ItemKey    string    `gorm:"not null"                                    json:"item_key"`
}
```

- `Batch.Kind` identifies the type of work (e.g. `"reevaluate_symptoms"`) so the table can
  serve multiple async workflows.
- `BatchItem.ItemKey` stores the human-readable work item identifier (e.g. the
  `prow_job_build_id`) for display in status responses without needing to decode River job args.
- Add a unique constraint on `(batch_id, item_key)` to prevent a batch from listing the same
  item twice.

### 1.2: Batch submission (`pkg/sippyserver/workqueue/submitter.go`)

Define a `Submitter` struct that accepts a gorm DB and a River client. Provide a `Submit`
method that:

1. Creates a `Batch` row in the database.
2. Calls River's `InsertManyTx` to insert all work items as individual River jobs with
   deduplication (see Step 2 for `UniqueOpts`).
3. For each insert result, records a `BatchItem` row linking the batch to the River job ID.
   River returns the existing job row for duplicates, so `BatchItem` rows are created
   regardless of whether the job was new or deduped.
4. Counts how many jobs were newly enqueued vs. deduplicated (River's insert result indicates
   this via `UniqueSkippedAsDuplicate`) and updates the `Batch` row with those counts and
   sets status to `running`.
5. Returns a `SubmitResult` with the batch ID and new/dedup counts.

**Transaction boundaries:** The batch/batch-item writes use gorm (pgx/v4) while River's
`InsertManyTx` requires a pgx/v5 transaction. Because these are different driver versions,
they cannot share a single database transaction. Use two separate transactions: one gorm
transaction for the batch and batch-item rows, and one pgx/v5 transaction for River job
insertion. If the gorm transaction succeeds but the River insert fails, the batch row will
exist with no corresponding River jobs. If the River insert succeeds but the gorm transaction
fails, the River jobs will run without a tracking batch. In either failure case, the user can
submit a new batch with the same entries (deduplication prevents duplicate work) and poll that
batch for progress.

### 1.3: Batch status query (`pkg/sippyserver/workqueue/status.go`)

Define a `StatusQuerier` struct with a `Query` method that:

1. Loads the `Batch` row by ID.
2. Joins `workqueue_batch_items` with `river_job` to get the current River state for each item.
3. Aggregates counts by state category: completed, failed (discarded/cancelled), running,
   and pending (available/scheduled/retryable).
4. Returns a `BatchStatusResponse` containing the batch ID, overall status, aggregate counts,
   and per-item status list.

**Lazy completion detection:** When all items have reached a terminal state
(`completed + failed >= total`), the query marks the batch as complete (or failed if all items
failed) and sets `completed_at`. This is idempotent — no asynchronous bookkeeping is required.
The batch status is simply computed fresh on each poll and the batch row is updated as a
side effect when the terminal condition is met.

### 1.4: Database migration

Add a migration using `golang-migrate/v4` (following existing patterns in `pkg/db/migrations/`):

```sql
-- Up
CREATE TABLE workqueue_batches (
    id              UUID PRIMARY KEY,
    kind            TEXT NOT NULL,
    requested_count INT NOT NULL,
    enqueued_count  INT NOT NULL DEFAULT 0,
    deduped_count   INT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_workqueue_batches_kind_status ON workqueue_batches (kind, status);

CREATE TABLE workqueue_batch_items (
    id           BIGSERIAL PRIMARY KEY,
    batch_id     UUID NOT NULL REFERENCES workqueue_batches(id) ON DELETE CASCADE,
    river_job_id BIGINT NOT NULL,
    item_key     TEXT NOT NULL,
    UNIQUE(batch_id, item_key)
);

CREATE INDEX idx_workqueue_batch_items_batch ON workqueue_batch_items (batch_id, river_job_id);

-- River's own migrations are handled by river.Migrator at startup (see Step 3).

-- Down
DROP TABLE IF EXISTS workqueue_batch_items;
DROP TABLE IF EXISTS workqueue_batches;
```

## Step 2: Define the Re-evaluation River Job

Create `pkg/api/jobrunscan/reevaluate_worker.go` alongside the existing re-evaluation code.

### 2.1: Job args and constants

```go
const (
    ReevaluateJobKind     = "reevaluate_job_run"
    ReevaluateQueue       = "reevaluate"
    ReevaluateDedupPeriod = 90 * time.Minute
    ReevaluateMaxAttempts = 3
    MaxJobRunsPerBatch    = 10000
)

type ReevaluateJobRunArgs struct {
    ProwJobBuildID string `json:"prow_job_build_id" river:"unique"`
    SymptomHash    string `json:"symptom_hash"      river:"unique"`
}
```

The `InsertOpts` method on `ReevaluateJobRunArgs` should return:

- `Queue`: `ReevaluateQueue`
- `MaxAttempts`: `ReevaluateMaxAttempts`
- `UniqueOpts`: `ByArgs: true`, `ByPeriod: ReevaluateDedupPeriod`

Key design decisions:

- Both `ProwJobBuildID` and `SymptomHash` are tagged `river:"unique"`, so deduplication is
  scoped to the combination of job run identity and symptom state. The same job run will be
  re-evaluated if symptoms have changed since the last evaluation.
- `ByPeriod: 90 * time.Minute` prevents re-evaluating the same job run within the BigQuery
  streaming buffer window, avoiding the "rows not deletable within 90m" constraint.
- No `BatchID` in the args — batch association is tracked via `workqueue_batch_items`, not
  River job args. This keeps the unique hash clean and avoids the subtlety where different
  batch IDs in the args would defeat deduplication.

### 2.2: Job worker

Define a `ReevaluateWorker` struct that embeds `river.WorkerDefaults[ReevaluateJobRunArgs]`
and holds a reference to a `ReEvaluator`. The `Work` method calls the existing
`reEvaluateOne()` for the given `ProwJobBuildID` against the evaluator's cached symptoms,
returning an error if the evaluation fails (which triggers River's retry logic).

### 2.3: Symptom caching and hash-based uniqueness

The `ReEvaluator` should maintain a concurrency-safe cache of active symptoms to avoid
reloading them from the database for every individual work item.

Add a `RefreshSymptomCache()` method to `ReEvaluator` that loads all active symptoms via the
existing `loadActiveSymptoms()` and stores them in a field guarded by a `sync.RWMutex`. The
method also computes a hash (e.g. SHA-256) of the sorted symptom IDs/versions and stores it
alongside the cached symptoms. The `reEvaluateOne()` method reads from the cache under an
`RLock`.

**Refresh trigger:** The symptom cache is refreshed when a new batch is created. The API
handler (or `Submitter`) calls `RefreshSymptomCache()` during batch submission. The resulting
symptom hash is included in the `ReevaluateJobRunArgs` (see below) so that it participates
in River's deduplication. This means that if a user modifies symptoms and submits a new batch
for the same job runs, the changed hash defeats deduplication and the jobs run again with the
updated symptoms.

We are not interested in tracking symptom changes that occur while a batch is processing.
The expectation is that the user submits a batch after making the symptom changes they care
about. If they make further changes, they submit another batch, and the new symptom hash
ensures those jobs are not deduplicated against the earlier run.

### 2.4: Retry policy

River's default retry policy uses exponential backoff with jitter. Configure
`MaxAttempts: 3` — a transient GCS timeout retries twice, then marks the job `discarded`
(permanent failure). This is surfaced in the batch status response as a failed item.

## Step 3: Integrate River into sippy-daemon

### 3.1: River client setup

In `cmd/sippy-daemon/main.go`:

1. Create a `pgx/v5` connection pool using the existing database DSN. This pool is used
   exclusively by River and coexists with the existing pgx/v4 pool.
2. Run River's built-in migrations on startup via `rivermigrate.Migrator`.
3. Construct a `ReEvaluator` with the same dependencies used by the API server (BigQuery
   client, GCS client, GCS bucket, gorm DB, cache, job artifacts manager). If
   `cmd/sippy-daemon/main.go` does not currently initialize these clients, add the
   initialization following the patterns in `cmd/sippy/main.go`.
4. Register the `ReevaluateWorker` with River's worker registry.
5. Create and start the River client with the `reevaluate` queue configured (suggested
   starting concurrency: 8 workers, tunable based on load testing).

### 3.2: Daemon lifecycle integration

River manages its own goroutines internally. Wrap the River client in a thin adapter struct
implementing the `DaemonProcess` interface so that it participates in the existing
`DaemonServer.Serve()` lifecycle. The adapter's `Run` method starts the River client, blocks
until the context is cancelled, then calls `Stop` with a graceful shutdown timeout.

Register this adapter as a daemon process in `cmd/sippy-daemon/main.go` alongside the
existing `WorkProcessor`.

### 3.3: Job artifact query worker pool

The existing `ReEvaluator.reEvaluateOne()` uses `pkg/api/jobartifacts.Manager` for concurrent
GCS artifact scanning. Ensure the daemon's `ReEvaluator` receives a properly initialized
`jobartifacts.Manager` instance — this requires a GCS client, bucket name, and configuration
matching the API server's setup.

## Step 4: Modify the API

### 4.1: Change the existing endpoint to async

Modify the handler for `POST /api/jobs/runs/reevaluate` in `pkg/sippyserver/job_run_scan.go`:

- Validate the request body (same fields as today: `prow_job_build_ids` and `dry_run`).
- Enforce a maximum of 10,000 job runs per request.
- If `dry_run: true`, process synchronously using the existing `ReEvaluator` and return
  results immediately with `200 OK` (preserving current behavior).
- If `dry_run: false` (or omitted), use the `Submitter` to create a batch and enqueue River
  jobs, then return `202 Accepted` with the batch ID and dedup counts.
- Trigger a symptom cache refresh on the `ReEvaluator` during batch submission (see Step 2.3).

### 4.2: Add the batch status endpoint

Register `GET /api/jobs/runs/reevaluate/{batch_id}` with the `LocalDBCapability` requirement.
The handler parses the batch UUID from the path, calls `StatusQuerier.Query()`, and returns
the result. Return `404` if the batch ID is not found.

### 4.3: API contract

**Submit batch (POST):**

```
POST /api/jobs/runs/reevaluate
Content-Type: application/json

{
    "prow_job_build_ids": ["1234567890", "1234567891", ...],
    "dry_run": false
}
```

Response (`202 Accepted`):

```json
{
    "batch_id": "a1b2c3d4-...",
    "requested": 50,
    "enqueued": 42,
    "deduped": 8,
    "links": {
        "status": "/api/jobs/runs/reevaluate/a1b2c3d4-..."
    }
}
```

**Poll status (GET):**

```
GET /api/jobs/runs/reevaluate/{batch_id}
```

Response (`200 OK`):

```json
{
    "batch_id": "a1b2c3d4-...",
    "status": "running",
    "total": 50,
    "completed": 35,
    "failed": 1,
    "running": 6,
    "pending": 8,
    "items": [
        {"item_key": "1234567890", "state": "completed"},
        {"item_key": "1234567891", "state": "running"},
        {"item_key": "1234567892", "state": "available"},
        {"item_key": "1234567893", "state": "discarded"}
    ]
}
```

When `dry_run: true`, the existing synchronous behavior is preserved — results are returned
immediately with a `200 OK` response. This keeps the dry-run path simple and useful for testing.

## Step 5: Wire Up the API Server

The API server (sippy) needs a `Submitter` and `StatusQuerier` from
`pkg/sippyserver/workqueue` but does **not** run River workers — those run in sippy-daemon only.

Create a River client in insert-only mode in `cmd/sippy/main.go`: configure the client without
registering any workers or queues and without calling `Start()`. The client can still insert
jobs via `InsertMany()`.

Add `workqueueSubmitter` and `workqueueStatusQuerier` fields to the `Server` struct. Initialize
them during server construction, gated on `LocalDBCapability` (required for both endpoints).
The existing `POST /api/jobs/runs/reevaluate` already requires both `LocalDBCapability` and
`WriteEndpointsCapability`; the new `GET /api/jobs/runs/reevaluate/{batch_id}` status endpoint
requires only `LocalDBCapability` (it is read-only).

## Step 6: Batch Lifecycle and Cleanup

### 6.1: Completed batch retention

Add a periodic cleanup job (either a River periodic job or a simple cron-like `DaemonProcess`)
that deletes `workqueue_batches` rows where `completed_at` is older than 7 days. The
`ON DELETE CASCADE` foreign key handles the `workqueue_batch_items` rows automatically.

### 6.2: River job retention

Configure River's `CompletedJobRetention` and `DiscardedJobRetention` to 8 days, matching
the batch retention period. This ensures `river_job` rows don't grow unbounded while keeping
enough history for status queries on recent batches.

## Step 7: Testing

Avoid mocking any substantial part of these packages (River client, ReEvaluator, storage
clients). Instead, follow the project's functional test pattern (see
`pkg/api/jobrunscan/reevaluate_functional_test.go`): tests that require external dependencies
(PostgreSQL, GCS, BigQuery) are gated behind environment variables and skipped when those
variables are not set. A human runs them by providing the necessary credentials.

- **Unit tests** for pure logic functions only: dedup counting, symptom hash computation,
  batch status aggregation from pre-populated row structs. No mocking of River or database
  clients.
- **Functional tests** for `pkg/sippyserver/workqueue/` — test `Submitter.Submit()` and
  `StatusQuerier.Query()` against a real PostgreSQL instance with River migrations applied.
  Verify dedup counting, batch item creation, completion detection, and status transitions.
  Skip unless `SIPPY_FUNCTIONAL_TEST_DSN` (or similar) is set.
- **Functional tests** for `ReevaluateWorker.Work()` using River's `rivertest` package
  against a real PostgreSQL instance. Verify success and error/retry paths.
- **End-to-end functional test** of the full flow: POST to the API, verify 202, poll status,
  verify items transition through `available` to `running` to `completed`. Requires
  PostgreSQL with River migrations, and optionally GCS/BigQuery credentials for full coverage.

## Step 8: Implementation Order

1. Add `pgx/v5` and River dependencies to `go.mod`.
2. Create `pkg/sippyserver/workqueue/` — models, submitter, status querier.
3. Create `ReevaluateJobRunArgs` and `ReevaluateWorker` in `pkg/api/jobrunscan/`.
4. Add symptom cache with `sync.RWMutex` and symptom hash computation to `ReEvaluator`.
5. Write database migration for `workqueue_batches` and `workqueue_batch_items`.
6. Integrate River into `cmd/sippy-daemon/main.go` — pgx/v5 pool, migrations, worker
   registration, daemon process adapter, GCS/BQ client initialization.
7. Modify API handler — async dispatch for non-dry-run, new status endpoint.
8. Wire up API server — insert-only River client, submitter, status querier on Server struct.
9. Tests (unit, integration, end-to-end).
10. Update `docs/features/job-analysis-symptoms.md` — document the async API behavior,
    new status endpoint, deduplication semantics, and polling pattern.

## Known Limitations

1. **No batch cancellation** — There is no API to cancel an in-flight batch. Once River jobs
   are enqueued, they will run to completion (or exhaust retries). A future enhancement could
   add `DELETE /api/jobs/runs/reevaluate/{batch_id}` to cancel pending River jobs associated
   with a batch.

2. **Lazy batch completion** — The batch status is only updated to "complete" or "failed" when
   the status endpoint is polled and detects that all items have reached a terminal state. If
   no one polls after the last item completes, the batch row stays in "running" status
   indefinitely. This is cosmetic; the cleanup job (Step 6.1) will eventually delete it
   regardless of status.

## Open Questions

1. **pgx/v5 coexistence** — Are there any known issues running pgx/v4 and v5 pools against
   the same PostgreSQL instance? Initial research suggests this is safe (separate import paths,
   separate connection pools), but worth a quick verification.
2. **MaxWorkers tuning** — 8 concurrent re-evaluation workers is a starting point. The right
   number depends on GCS rate limits and the job artifact query concurrency within each worker.
   May need adjustment based on load testing.
3. **UI polling interval** — Suggest 2-3 second polling interval from the UI. Consider
   implementing exponential backoff (e.g. 1s → 2s → 5s → 10s) or Server-Sent Events (SSE)
   in a future iteration to reduce polling overhead.
