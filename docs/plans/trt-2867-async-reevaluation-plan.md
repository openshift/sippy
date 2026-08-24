# TRT-2867: Async Symptom Re-evaluation via River Job Queue

## Overview

Move the synchronous symptom re-evaluation API (`POST /api/jobs/runs/reevaluate`) to an
asynchronous batch model. The API handler creates a batch request and enqueues individual
re-evaluation work items via [River](https://github.com/riverqueue/river), a PostgreSQL-backed
job queue. The sippy-daemon processes work items using the existing `ReEvaluator` logic. The UI
polls a status endpoint for progress until all items complete.

**Jira:** [TRT-2867](https://redhat.atlassian.net/browse/TRT-2867)
**Predecessor:** [TRT-2695](https://redhat.atlassian.net/browse/TRT-2695) - synchronous
re-evaluation API (already implemented)

## Motivation

The current `POST /api/jobs/runs/reevaluate` endpoint processes all requested job runs
synchronously in the HTTP request. For large batches this causes:

- HTTP timeouts when evaluating many job runs against many symptoms
- No progress visibility; the UI blocks until the entire batch completes
- No deduplication; concurrent requests can re-evaluate the same job run, causing BigQuery
  streaming buffer conflicts (rows are not deletable within 90 minutes of insertion)
- No retry of individual failures; one GCS timeout fails the entire request

## Design Principles

1. **Reuse, don't rewrite** - the existing `ReEvaluator.reEvaluateOne()` becomes the work unit
   inside each River job worker, unchanged.
2. **Appropriate abstraction** - `pkg/sippyserver/workqueue` is a generic queue manager (River
   setup, batch lifecycle patterns). Symptom re-evaluation specifics live in a subpackage
   `pkg/sippyserver/workqueue/symptomre`, which defines domain-specific tables, job args,
   workers, and submission logic. This keeps the generic queue plumbing separate from the
   re-evaluation domain, and the `symptomre` package name gives clear context to all its types.
3. **River owns execution and dedup; application tables own batch semantics** - River handles job
   scheduling, concurrency, retries, and uniqueness. Custom tables handle batch-to-item
   associations and user-facing status.
4. **API specifies, daemon executes** - the API server creates a batch specification (the batch row,
   its items, and a "process batch" River job) but all execution logic runs in the sippy-daemon,
   which owns the shared symptom cache and controls the fan-out into individual River jobs. This
   separation ensures the daemon is the single owner of the symptom cache lifecycle.

## Prerequisites: Orientation

Before writing code, read and understand these files in addition to those listed in the
TRT-2695 plan:

| File | What to learn |
|------|---------------|
| `pkg/sippyserver/daemon_server.go` | `DaemonProcess` interface and `DaemonServer.Serve()` goroutine lifecycle. Each process runs independently with its own context. |
| `pkg/sippyserver/pr_commenting_processor.go` | `WorkProcessor` - existing daemon process example. Ticker-based polling, bounded worker goroutines, channel-based dispatch. |
| `cmd/sippy-daemon/main.go` | Daemon process registration and startup. New processes added via `processes = append(processes, ...)`. |
| `pkg/api/jobrunscan/reevaluate.go` | `ReEvaluator` struct, `ReEvaluateJobRuns()`, `reEvaluateOne()`. The synchronous implementation to be reused as the per-item worker. |
| `pkg/sippyserver/job_run_scan.go` | Current HTTP handler `jsonReEvaluateJobRunSymptoms` - the handler to be replaced with async dispatch. |

## Implementation methodology

For each step:
1. Have an agent with a fresh context implement the step according to this document and agent
   instructions from this repo.
   * Where it makes sense, inline comments to explain the code's context and purpose, based on the
     step's description and the Known Limitations given at the end of this plan.
   * Write unit tests for non-trivial code changes, unless they would be impractical.
2. Have an adversarial agent with a fresh context verify whether the changes completely and
   correctly implement the step, giving the implementing agent instructions if changes are needed.
3. Summarize the changes, pause, and require a human to verify and approve them to continue.
4. Commit the changes to git locally.

## Step 1: Add River and pgx/v5 dependencies

River requires `jackc/pgx/v5`. Sippy currently uses `jackc/pgx/v4` (v4.18.2).

**Approach:** Add `pgx/v5` alongside the existing `pgx/v4` dependency. Create a separate
`*pgxpool.Pool` (v5) for River's use. The rest of Sippy continues using v4 + gorm until a
broader migration is undertaken. Both driver versions can coexist in a Go module; they have
different import paths (`github.com/jackc/pgx/v4` vs `github.com/jackc/pgx/v5`).

Add both `river` and `pgx/v5` as direct dependencies and vendor them locally.

## Step 2: Add the `pkg/sippyserver/workqueue` and `workqueue/symptomre` Packages

### 2.1: Generic workqueue package (`pkg/sippyserver/workqueue/`)

Create a generic work queue package that manages River client setup and provides shared
batch lifecycle types. This package does not contain any symptom re-evaluation logic.

**Contents:**

- **`river.go`** - Factory for creating a River client (accepts a pgx/v5 pool, worker
  registry, and queue configuration). Provides helpers for insert-only mode (API server) vs.
  full worker mode (daemon).
- **`models.go`** - Shared types for batch status patterns:

```go
type BatchStatus string

const (
    BatchStatusPending    BatchStatus = "pending"
    BatchStatusProcessing BatchStatus = "processing"
    BatchStatusRunning    BatchStatus = "running"
    BatchStatusComplete   BatchStatus = "complete"
    BatchStatusFailed     BatchStatus = "failed"
)
```

  These are reusable constants, not tied to a specific table. Each workload subpackage
  defines its own table-backed models using these status values.

- **`status.go`** - Generic helpers for batch status aggregation (e.g., computing overall
  batch status from per-item River job states, lazy completion detection). These operate on
  interfaces or generic structs so subpackages can reuse them without code duplication.

### 2.2: Symptom re-evaluation subpackage (`pkg/sippyserver/workqueue/symptomre/`)

This subpackage contains all domain-specific logic for the symptom re-evaluation workflow.
The package name `symptomre` gives clear context to all its types and avoids generic
queue-processing names.

**Database models (`models.go`):**

```go
// Batch represents a user-initiated batch of symptom re-evaluation work.
type Batch struct {
    ID             uuid.UUID   `gorm:"type:uuid;primaryKey"          json:"id"`
    RequestedCount int         `gorm:"not null"                      json:"requested_count"`
    EnqueuedCount  int         `gorm:"not null"                      json:"enqueued_count"`
    DedupedCount   int         `gorm:"not null"                      json:"deduped_count"`
    Status         workqueue.BatchStatus `gorm:"not null;default:'pending'" json:"status"`
    CreatedAt      time.Time   `gorm:"autoCreateTime"                json:"created_at"`
    CompletedAt    *time.Time  `                                     json:"completed_at,omitempty"`
}

func (Batch) TableName() string { return "workqueue_symptom_re_batches" }

// BatchItem associates a batch with an individual job run to re-evaluate.
// Before the daemon processes the batch, RiverJobID is nil (the item is a specification).
// After processing, RiverJobID is populated with the River job that performs the work.
type BatchItem struct {
    ID         uint64    `gorm:"primaryKey;autoIncrement"                            json:"id"`
    BatchID    uuid.UUID `gorm:"type:uuid;not null;index:idx_symptom_re_batch_items" json:"batch_id"`
    RiverJobID *int64    `gorm:"index:idx_symptom_re_batch_items"                    json:"river_job_id"`
    ItemKey    string    `gorm:"not null"                                            json:"item_key"`
}

func (BatchItem) TableName() string { return "workqueue_symptom_re_batch_items" }
```

**Batch submission (`submitter.go`):**

Define a `Submitter` struct that accepts a gorm DB and a River client. Provide a `Submit`
method that:

1. Creates a `Batch` row with status `pending` and `requested_count` set to the number of
   job run IDs provided.
2. Creates `BatchItem` rows for each job run ID with `RiverJobID` set to nil. These rows
   record the specification of what needs to be re-evaluated.
3. Enqueues a single `ProcessBatchArgs` River job (see Step 3.1) containing the batch ID.
   This job is how the daemon discovers new batches to process.
4. Returns a `SubmitResult` with the batch ID and requested count.

The `Submitter` does **not** refresh the symptom cache, compute symptom hashes, or create
individual re-evaluation River jobs. Those are the daemon's responsibility (see Step 3).

**Transaction boundaries:** The batch/batch-item writes use gorm (pgx/v4) and the
`ProcessBatchArgs` River job insertion uses pgx/v5. These are separate transactions.
If the gorm transaction succeeds but the River insert fails, the batch row exists with
no corresponding River job to process it. The batch would remain in `pending` status
until either the user resubmits or a periodic cleanup removes it.

**Batch status query (`status.go`):**

Define a `StatusQuerier` struct with a `Query` method that:

1. Loads the `Batch` row by ID.
2. For items with a non-null `RiverJobID`, joins with `river_job` to get the current River
   state. Items with a null `RiverJobID` are reported as `pending` (not yet processed by
   the daemon).
3. Aggregates counts by state category: completed, failed (discarded/cancelled), running,
   and pending (null `river_job_id` or available/scheduled/retryable).
4. Returns a `BatchStatusResponse` containing the batch ID, overall status, aggregate counts,
   and per-item status list.

**Lazy completion detection:** When all items have a non-null `RiverJobID` and all have
reached a terminal state (`completed + failed >= total`), the query marks the batch as
complete (or failed if all items failed) and sets `completed_at`. This is idempotent.

### 2.3: Database migration

Add a migration using `golang-migrate/v4` (following existing patterns in `pkg/db/migrations/`):

```sql
-- Up
CREATE TABLE workqueue_symptom_re_batches (
    id              UUID PRIMARY KEY,
    requested_count INT NOT NULL,
    enqueued_count  INT NOT NULL DEFAULT 0,
    deduped_count   INT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_symptom_re_batches_status ON workqueue_symptom_re_batches (status);

CREATE TABLE workqueue_symptom_re_batch_items (
    id           BIGSERIAL PRIMARY KEY,
    batch_id     UUID NOT NULL REFERENCES workqueue_symptom_re_batches(id) ON DELETE CASCADE,
    river_job_id BIGINT,
    item_key     TEXT NOT NULL,
    UNIQUE(batch_id, item_key)
);

CREATE INDEX idx_symptom_re_batch_items_batch
    ON workqueue_symptom_re_batch_items (batch_id, river_job_id);

-- River's own migrations are handled by river.Migrator at startup (see Step 4).

-- Down
DROP TABLE IF EXISTS workqueue_symptom_re_batch_items;
DROP TABLE IF EXISTS workqueue_symptom_re_batches;
```

## Step 3: Define River Job Types (`pkg/sippyserver/workqueue/symptomre/`)

All River job args, workers, and constants for the symptom re-evaluation workflow live in the
`symptomre` subpackage. Create `workers.go` alongside the models.

### 3.1: Job args and constants

```go
const (
    BatchQueue            = "symptom_re_batch"
    ItemQueue             = "symptom_re_item"
    DedupPeriod           = 120 * time.Minute
    MaxAttemptsPerItem    = 3
    MaxJobRunsPerBatch    = 10000
)

// ProcessBatchArgs is a River job that the API enqueues when a new batch is created.
// The daemon picks this up, refreshes the symptom cache, and fans out into individual
// ReevaluateJobRunArgs jobs.
type ProcessBatchArgs struct {
    BatchID uuid.UUID `json:"batch_id"`
}

// ReevaluateJobRunArgs is a River job for re-evaluating a single job run's symptoms.
type ReevaluateJobRunArgs struct {
    ProwJobBuildID string `json:"prow_job_build_id" river:"unique"`
    SymptomHash    string `json:"symptom_hash"      river:"unique"`
}
```

The `InsertOpts` method on `ProcessBatchArgs` should return:

- `Queue`: `BatchQueue`
- `MaxAttempts`: 1 (batch processing is not retried; individual items have their own retries)

The `InsertOpts` method on `ReevaluateJobRunArgs` should return:

- `Queue`: `ItemQueue`
- `MaxAttempts`: `MaxAttemptsPerItem`
- `UniqueOpts`: `ByArgs: true`, `ByPeriod: DedupPeriod`

Key design decisions:

- Both `ProwJobBuildID` and `SymptomHash` are tagged `river:"unique"`, so deduplication is
  scoped to the combination of job run identity and symptom state. The same job run will be
  re-evaluated if symptoms have changed since the last evaluation.
- `ByPeriod: 120 * time.Minute` prevents re-evaluating the same job run within the same two-hour
  period, reducing the chances of the BigQuery 90m streaming buffer window returning a "rows not
  deletable within 90m" error. With River's block dedup scheme, it is always possible that a request
  at the end of a block and at the beginning of the next block could be dupes, so we still handle
  that error gracefully, but this at least limits agents from spamming the same requests; and if
  they do, the results are idempotent.
- No `BatchID` in the per-item args; batch association is tracked via
  `workqueue_symptom_re_batch_items`, not River job args. This keeps the unique hash clean
  and avoids the subtlety where different batch IDs would defeat deduplication.
- `ProcessBatchArgs` and `ReevaluateJobRunArgs` use separate queues (`symptom_re_batch` and
  `symptom_re_item`). This prevents a large batch of individual re-evaluation jobs from
  blocking the processing of a subsequent `ProcessBatchArgs` job. The batch queue processes
  fan-out promptly regardless of how many items are in flight.

### 3.2: Batch processor worker

Define a `ProcessBatchWorker` struct that embeds
`river.WorkerDefaults[ProcessBatchArgs]` and holds a reference to a `ReEvaluator` and a
gorm DB. The `Work` method:

1. Loads the `Batch` row and its `BatchItem` rows from the database.
2. Updates the batch status to `processing`.
3. Calls `RefreshSymptomCache()` on the `ReEvaluator` to load current symptoms and compute
   the symptom hash (see 3.4).
4. For each batch item, constructs a `ReevaluateJobRunArgs` with the item's `ItemKey` as the
   `ProwJobBuildID` and the current symptom hash.
5. Inserts all individual River jobs via `InsertManyTx`.
6. Updates each `BatchItem` row with the resulting `RiverJobID` (River returns the existing
   job row for duplicates, so the ID is always available).
7. Counts newly enqueued vs. deduplicated jobs (via `UniqueSkippedAsDuplicate`) and updates
   the batch's `EnqueuedCount`, `DedupedCount`, and status to `running`.

If step 5 or 6 fails, the batch remains in `processing` status. Since `ProcessBatchArgs`
has `MaxAttempts: 1`, it will not be retried automatically. The user can see the stuck
batch via the status endpoint and resubmit.

### 3.3: Re-evaluation item worker

Define a `ReevaluateWorker` struct that embeds `river.WorkerDefaults[ReevaluateJobRunArgs]`
and holds a reference to a `ReEvaluator`. The `Work` method calls the existing
`reEvaluateOne()` for the given `ProwJobBuildID` against the evaluator's cached symptoms,
returning an error if the evaluation fails (which triggers River's retry logic).

### 3.4: Symptom caching and hash-based uniqueness

The `ReEvaluator` should maintain a concurrency-safe cache of active symptoms to avoid
reloading them from the database for every individual work item.

Add a `RefreshSymptomCache()` method to `ReEvaluator` that loads all active symptoms via the
existing `loadActiveSymptoms()` and stores them in a field guarded by a `sync.RWMutex`. The
method also computes a hash (SHA-256 of the JSON serialization) of the sorted symptom IDs/versions and
stores it alongside the cached symptoms. The `reEvaluateOne()` method reads from the cache under an
`RLock`. `RefreshSymptomCache()` returns the computed hash string.

**Refresh trigger:** The symptom cache is refreshed by the `ProcessBatchWorker` in the
daemon when it processes a new batch (step 3.2, substep 3). The resulting symptom hash is
included in each `ReevaluateJobRunArgs` so that it participates in River's deduplication.
This means that if a user modifies symptoms and submits a new batch for the same job runs,
the changed hash defeats deduplication and the jobs run again with the updated symptoms.

We are not interested in tracking symptom changes that occur while a batch is processing.
The expectation is that the user submits a batch after making the symptom changes they care
about. If they make further changes, they submit another batch, and the new symptom hash
ensures those jobs are not deduplicated against the earlier run.

### 3.5: Retry policy

River's default retry policy uses exponential backoff with jitter. Configure
`MaxAttempts: 3` for individual re-evaluation jobs. A transient GCS timeout retries twice,
then marks the job `discarded` (permanent failure). This is surfaced in the batch status
response as a failed item.

## Step 4: Integrate River into sippy-daemon

### 4.1: River client setup

In `cmd/sippy-daemon/main.go`:

1. Create a `pgx/v5` connection pool using the existing database DSN. This pool is used
   exclusively by River and coexists with the existing pgx/v4 pool.
2. Run River's built-in migrations on startup via `rivermigrate.Migrator`.
3. Construct a `ReEvaluator` with the same dependencies used by the API server (BigQuery client, GCS
   client, GCS bucket, gorm DB, cache, job artifacts manager). Abstract out the existing client
   initialization in `cmd/sippy-daemon/main.go` to a shared helper, and add more as needed following
   existing patterns from there or `cmd/sippy/main.go`.
4. Register both workers from the `symptomre` package with River's worker registry:
   `ProcessBatchWorker` (handles batch fan-out) and `ReevaluateWorker` (handles individual
   re-evaluations).
5. Create and start the River client with two queues configured:
   - `symptom_re_batch` (concurrency: 1, processes batch fan-out sequentially)
   - `symptom_re_item` (concurrency: 12, processes individual re-evaluations; tunable based
     on load testing)

### 4.2: Daemon lifecycle integration

River manages its own goroutines internally. Wrap the River client in a thin adapter struct
implementing the `DaemonProcess` interface so that it participates in the existing
`DaemonServer.Serve()` lifecycle. The adapter's `Run` method initializes the symptom cache once,
starts the River client, blocks until the context is cancelled, then calls `Stop` with a graceful
shutdown timeout.

Register this adapter as a daemon process in `cmd/sippy-daemon/main.go` alongside the
existing `WorkProcessor`.

### 4.3: Job artifact query worker pool

The existing `ReEvaluator.reEvaluateOne()` uses `pkg/api/jobartifacts.Manager` for concurrent
GCS artifact scanning. Ensure the daemon's `ReEvaluator` receives a properly initialized
`jobartifacts.Manager` instance - this requires a GCS client, bucket name, and configuration
matching the API server's setup.

## Step 5: Modify the API

### 5.1: Change the existing endpoint to async

Modify the handler for `POST /api/jobs/runs/reevaluate` in `pkg/sippyserver/job_run_scan.go`:

- Validate the request body (same fields as today: `prow_job_build_ids` and `dry_run`).
- Validate that build ids are integers (error 400 if not), and deduplicate them.
- Enforce a maximum of 10,000 job runs per request.
- If `dry_run: true`, process synchronously using the existing `ReEvaluator` and return
  results immediately with `200 OK` (preserving current behavior).
- If `dry_run: false` (or omitted), use the `symptomre.Submitter` to create a batch
  specification (batch row, batch item rows, and a `ProcessBatchArgs` River job), then
  return `202 Accepted` with the batch ID and requested count.

The API does **not** refresh the symptom cache or create individual re-evaluation River jobs.
Those operations happen in the daemon when the `ProcessBatchWorker` picks up the
`ProcessBatchArgs` job (see Step 3.2). The `enqueued` and `deduped` counts in the submit
response are zero at this point; the user sees them populate when polling the status endpoint
after the daemon processes the batch.

### 5.2: Add the batch status endpoint

Register `GET /api/jobs/runs/reevaluate/{batch_id}` with the `LocalDBCapability` requirement.
The handler parses the batch UUID from the path, calls `StatusQuerier.Query()`, and returns
the result. Return `404` if the batch ID is not found.

### 5.3: API contract

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
    "requested": 50,
    "enqueued": 42,
    "deduped": 8,
    "completed": 35,
    "failed": 1,
    "running": 6,
    "pending": 8,
    "items": [
        {"item_key": "1234567890", "state": "completed"},
        {"item_key": "1234567891", "state": "running"},
        {"item_key": "1234567892", "state": "pending"},
        {"item_key": "1234567893", "state": "discarded"}
    ]
}
```

Batch status progresses through: `pending` (created by API, not yet picked up by daemon),
`processing` (daemon is refreshing symptom cache and creating individual jobs), `running`
(individual jobs are executing), `complete` or `failed` (all items terminal).

When `dry_run: true`, the existing synchronous behavior is preserved; results are returned
immediately with a `200 OK` response. This keeps the dry-run path simple and useful for testing.

## Step 6: Wire Up the API Server

The API server (sippy) needs a `symptomre.Submitter` and `symptomre.StatusQuerier` but does
**not** run River workers; those run in sippy-daemon only.

Create a River client in insert-only mode in `cmd/sippy/main.go`: configure the client without
registering any workers or queues and without calling `Start()`. The client can still insert
jobs via `InsertMany()` (used by the `Submitter` to enqueue the `ProcessBatchArgs` job).

Add `symptomReSubmitter` and `symptomReStatusQuerier` fields to the `Server` struct.
Initialize them during server construction, gated on `LocalDBCapability` (required for both
endpoints). The existing `POST /api/jobs/runs/reevaluate` already requires both
`LocalDBCapability` and `WriteEndpointsCapability`; the new
`GET /api/jobs/runs/reevaluate/{batch_id}` status endpoint requires only `LocalDBCapability`
(it is read-only).

## Step 7: Batch Lifecycle and Cleanup

### 7.1: Completed batch retention

Add a periodic cleanup job (either a River periodic job or a simple cron-like `DaemonProcess`)
that deletes `workqueue_symptom_re_batches` rows where `completed_at` is older than 7 days.
The `ON DELETE CASCADE` foreign key handles the `workqueue_symptom_re_batch_items` rows
automatically.

### 7.2: River job retention

Configure River's `CompletedJobRetention` and `DiscardedJobRetention` to 8 days, matching
the batch retention period. This ensures `river_job` rows don't grow unbounded while keeping
enough history for status queries on recent batches.

## Step 8: Functional Testing

Do not make mocks of vendored packages (River client, storage clients) for testing. Encapsulate
access to each client in an interface of conceptually high-level methods requiring only that client;
where tests exercise glue logic using multiple such interfaces, implement mock objects as needed.

Follow the project's functional test pattern: tests using external dependencies (PostgreSQL, GCS,
BigQuery) require a human to set environment variables for the necessary credentials, skipping when
they are not set.

- **Unit tests** should hopefully already exist for pure logic functions such as dedup counting,
  symptom hash computation, batch status aggregation from pre-populated row structs.
- **Functional tests** for `pkg/sippyserver/workqueue/symptomre/` - test
  `Submitter.Submit()`, `ProcessBatchWorker.Work()`, and `StatusQuerier.Query()` against a
  real PostgreSQL instance with River migrations applied. Verify batch creation, daemon-side
  fan-out (batch item population with River job IDs), dedup counting, completion detection,
  and status transitions. Skip unless `SIPPY_FUNCTIONAL_TEST_DSN` (or similar) is set.
- **Functional tests** for `ReevaluateWorker.Work()` using River's `rivertest` package
  against a real PostgreSQL instance. Verify success and error/retry paths.
- **End-to-end functional test** of the full flow: POST to the API, verify 202, poll status, verify
  items transition through `available` to `running` to `completed`. This should update existing
  functional testing in `pkg/api/jobrunscan/reevaluate_functional_test.go`, requiring the user to
  supply env vars for a PostgreSQL snapshot of real data (with River migrations) and edit-level
  GCS/BigQuery credentials for full coverage against a few real job runs (specified by the user,
  with results verified by the user). Include instructions for the user.

## Step 9. Update the React UI

Simplify the React UI (`sippy-ng/src/jobs/ReEvaluateSymptoms.jsx`) - replace the client-side worker
pool (`p-limit` concurrency, per-ID requests, retry logic) with a single batch POST that returns a
`batch_id`, then poll `GET /api/jobs/runs/reevaluate/{batch_id}` for progress. The component shows a
progress bar driven by the poll response counts (`completed`, `failed`, `running`, `pending`), and
displays a final snackbar when the batch reaches a terminal status (`complete` or `failed`). Remove
the `p-limit` dependency.

## Step 10. Clean up cruft

Search through the code modified by this process for anything that has become cruft: variables,
methods, and functions that are never used, code that can't be reached, comments that no longer
match the code, that sort of thing -- and fix them.

The adversarial review should verify that cleanup doesn't actually break anything.

## Step 11. Update project docs

Update `docs/features/job-analysis-symptoms.md` - document the async API behavior, new status
endpoint, deduplication semantics, and polling pattern.

## Known Limitations

1. **No batch cancellation** - There is no API to cancel an in-flight batch. Once River jobs
   are enqueued, they will run to completion (or exhaust retries). A future enhancement could
   add `DELETE /api/jobs/runs/reevaluate/{batch_id}` to cancel pending River jobs associated
   with a batch.

2. **Lazy batch completion** - The batch status is only updated to "complete" or "failed" when
   the status endpoint is polled and detects that all items have reached a terminal state. If
   no one polls after the last item completes, the batch row stays in "running" status
   indefinitely. This is cosmetic; the cleanup job (Step 7.1) will eventually delete it
   regardless of status.

3. **Stuck batch on processing failure** - If the `ProcessBatchWorker` fails (e.g., symptom
   cache refresh error or River insert error), the batch remains in `processing` status with
   `MaxAttempts: 1` meaning no automatic retry. The user must resubmit. The cleanup job
   eventually removes stale batches.

## Open Questions

1. **pgx/v5 coexistence** - Are there any known issues running pgx/v4 and v5 pools against
   the same PostgreSQL instance? Initial research suggests this is safe (separate import paths,
   separate connection pools), but worth a quick verification.
2. **MaxWorkers tuning** - 12 concurrent re-evaluation workers is a starting point. The right
   number depends on GCS rate limits and the job artifact query concurrency within each worker.
   May need adjustment based on load testing.
3. **UI polling interval** - Suggest 2-3 second polling interval from the UI. Consider
   implementing exponential backoff (e.g. 1s → 2s → 5s → 10s) or Server-Sent Events (SSE)
   in a future iteration to reduce polling overhead.
