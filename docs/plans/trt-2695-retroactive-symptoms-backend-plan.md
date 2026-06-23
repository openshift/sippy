# TRT-2695: Backend Implementation Plan - Retroactive Symptom Re-evaluation API

## Overview

Add an authenticated API endpoint to Sippy that re-evaluates all symptom matches for specified job
runs. The endpoint re-runs symptom detection against job artifacts and updates all three storage
backends (BigQuery, GCS, PostgreSQL).

**Jira:** [TRT-2695](https://redhat.atlassian.net/browse/TRT-2695)
**Context doc:** `docs/features/job-analysis-symptoms.md` - update this doc when
implementation is complete (see Step 8).

## Prerequisites: Orientation

Before writing code, read and understand these files:

| File | What to learn |
|------|---------------|
| `pkg/db/models/jobrunscan/symptom.go` | `Symptom` model composed of `SymptomContent` (ID, Summary, MatcherType, FilePattern, MatchString, LabelIDs), `ApplicabilityFilters` (`valid_from`/`valid_until` - **fields exist but not yet used**), and `Metadata`. Implemented MatcherType values: `string`, `regex`, `none`. **`cel` is defined but NOT implemented** (TRT-2466). |
| `pkg/db/models/jobrunscan/label.go` | `Label` model: `ID`, `label_title`, `explanation`, `hide_display_contexts`. |
| `pkg/db/models/job_labels.go` | `JobRunLabel` - BigQuery row schema for `ci_analysis_us.job_labels`. Fields: `ID` (prowjob_build_id), `StartTime`, `Label`, `SymptomID`, `SourceTool`, timestamps. Note: no `ProwJobName`, `MatchedFile`, or `MatchedText` fields - see Design Note 3. |
| `pkg/componentreadiness/jobrunannotator/jobrunannotator.go` | `JobRunAnnotator` - orchestrates BQ reads via `getJobRunsFromBigQuery`, artifact content matching via `filterJobRunByArtifact`, and BQ writes via `bulkInsertJobRunAnnotations` (batches of 500, dry-run support). **Important:** this tool adds labels with *empty* `SymptomID`; it does NOT evaluate symptoms. Reuse its BQ batch-write logic (extracted to standalone function), not its matching logic. |
| `pkg/componentreadiness/jobrunannotator/prow_bucket.go` | `JobRunBucketLabel` struct + `WriteJSONToBucket()` - writes individual label JSON files to GCS. `WriteHTMLSummaryToBucket()` - reads all label JSONs from `artifacts/job_labels/` and generates `label-summary.html` for Spyglass. `JobRunBucketLabelContainer` provides versioned schema wrapping (`symptom_label_v1`). These are the same functions the cloud function uses. |
| `pkg/api/jobartifacts/` | The artifact query and content matching engine: `ContentMatcher` interface, `lineMatcher` struct, `Manager` for concurrent artifact scanning, `getJobRunFiles` with `PathGlob` for GCS file discovery. `query.go` implements the GCS interaction with `maxJobFilesToScan` limits. |
| `pkg/api/jobrunscan/` | Symptom/label CRUD API handlers with validation logic. Follow these patterns for the new handler. |
| `pkg/sippyserver/job_run_scan.go` | HTTP route registration using gorilla/mux. Handlers: `jsonListSymptoms`, `jsonCreateSymptom`, `jsonGetSymptom`, `jsonUpdateSymptom`, `jsonDeleteSymptom` (and same pattern for labels). |
| `pkg/sippyclient/jobrunscan/symptoms.go` | `SymptomsClient` - Go client for symptom CRUD. Used by the cloud function. |
| `pkg/dataloader/prowloader/prow.go` | `GatherLabelsFromBQ()` - queries BQ for labels by build IDs and start time, returns `map[string]pq.StringArray`. Called during fetchdata to populate `prow_job_runs.labels`. |
| `pkg/dataloader/releaseloader/releasesync.go` | Release sync does the same for `release_job_runs.labels`. |
| `pkg/api/componentreadiness/regressiontracker.go` | `SyncTriageSymptoms()` and `MergeJobRuns()` - these are called by the regression cache loader, which re-queries BQ on each run and will automatically pick up re-evaluation changes. No direct calls needed from the re-evaluator. |

## Step 1: Create the Re-evaluation Service

Create `pkg/api/jobrunscan/reevaluate.go` (alongside the existing CRUD handlers).

### 1.1: ReEvaluator struct

```go
type ReEvaluator struct {
    bqClient    *bigquery.Client       // for BQ reads/writes
    gcsClient   *storage.Client        // for artifact reads and label file writes
    gcsBucket   string                 // GCS bucket name for label writes
    db          *gorm.DB               // Sippy PostgreSQL
    dryRun      bool                   // if true, log what would happen without writing
}

type ReEvalStatus string  // one of "missing_error", "eval_error", "rewrite_error", "success"
type ReEvaluationResult struct{
    ProwJobBuildID     string
    Status             ReEvalStatus
    SymptomsEvaluated  int
    SymptomsMatched    []string
    LabelsApplied      []string
    BQEntriesWritten   int
    GCSArtifactsWritten int
    PostgresUpdated    bool
    Error              string            // human-readable error message, empty on success
    Links              map[string]string  // HATEOAS links (job_run URL, matched symptom endpoints)
}
type ReEvaluationResponse struct {
    Results []ReEvaluationResult
    Links   map[string]string    // top-level HATEOAS links (e.g. "self")
}
```

Per-result HATEOAS links include the job run's Prow URL and an endpoint for each matched symptom
(`symptom:{id}` -> `/api/jobs/symptoms/{id}`). Top-level links include a `self` link to the
reevaluate endpoint. See `InjectReEvalHATEOASLinks` for the implementation.

### 1.2: Core method: `ReEvaluateJobRuns`

```go
func (r *ReEvaluator) ReEvaluateJobRuns(ctx context.Context, prowJobBuildIDs []string) ([]ReEvaluationResult, error) {
    // Each of the following is a discrete enough concept that it probably requires its own method,
    // and perhaps even its own struct with methods.

    // 1. Load all active symptom definitions from PostgreSQL (job_run_symptoms table)
    //    - Only include implemented matcher types: string, regex, none (file exists)
    //    - Future: apply applicability filters (valid_from/valid_until) when implemented

    // 2. For each job run, for each symptom, create and run a JobArtifactQuery
    //    - The query only needs a single file match, a single match in the file, and no context lines
    //    - Collect all (symptom, job run, file_match, text_match) tuples
    //    - No retries on timeout - return per-run status to the client for retry decisions
    //    - Actual query failures are final and populate the per-run error status

    // 3. For each job run that succeeded evaluation:
    //    a. Clear existing symptom-originated data (Step 2 below)
    //    b. Write results to BQ, GCS, and PostgreSQL (Steps 3-5 below)
    //    c. Populate the per-run result

    // 4. Return results for all runs (mix of successes and errors)
}
```

### 1.3: Reuse patterns, don't duplicate

The cloud function's flow is: fetch symptoms → for each arriving artifact file, check if it matches
any symptom → write labels. The re-evaluator does the same thing but for a completed job run: fetch
symptoms → list all artifacts → evaluate each symptom against matching artifacts → write labels. The
matching logic in `pkg/api/jobartifacts/` is the same engine. The key difference is the re-evaluator
processes all artifacts at once (not file-by-file as they arrive).

**Artifact scanning approach:** Run one `JobArtifactQuery` per symptom per job run. This reuses the
existing `Manager.Query()` pipeline and its `ContentMatcher` implementations with no modifications.
This means reading the same GCS file multiple times for different symptoms (the JAQ cache stores
match results keyed by `(jobRunID, pathGlob, matcherKey)`, not raw file contents, so different
symptoms with different matchers each read from GCS independently). This is acceptable because
re-evaluation is a rare operation, GCS reads are fairly fast, and trying to optimize with
multi-symptom-per-file matching would complicate the code significantly.

### 1.4: Extract BQ batch insert helper

Extract the core batch-insert logic from `JobRunAnnotator.bulkInsertJobRunAnnotations` into a
standalone function in `jobrunannotator.go`:

```go
func BulkInsertJobRunLabels(ctx context.Context, bqClient *bigquery.Client, dataset, table string,
    labels []models.JobRunLabel, batchSize int, dryRun bool) error
```

Both `JobRunAnnotator` and `ReEvaluator` call this function. The existing method becomes a thin
wrapper.

## Step 2: Clear Existing Symptom-Originated Labels

Before re-inserting, remove existing symptom-originated data. This makes re-evaluation idempotent
across symptom definition changes (added, modified, or removed symptoms).

### 2.1: BigQuery cleanup

Delete existing rows from `ci_analysis_us.job_labels` where:
- `prowjob_build_id` matches the target
- `symptom_id IS NOT NULL AND symptom_id != ''`
- timestamp matches the job runs evaluated (respecting partitioning)

This preserves manually-applied labels from the `annotate-job-runs` CLI tool (which
writes labels with empty `symptom_id`).

Use a DML DELETE statement. Note BQ streaming buffer constraints - recently inserted rows may not be
immediately deletable, so limit by `created_at` at least 90m old. (This could leave some entries
that should have been deleted; this will be rare and is an acceptable risk.)

### 2.2: GCS cleanup

Delete existing files from `{jobRunPath}/artifacts/job_labels/` in the job's GCS bucket. The JSON
files all contain symptom information (`symptom_label_v1` schema), so they can be safely removed and
regenerated. Nothing else writes to this path, so delete all `*.json` files found. Also
delete `label-summary.html` - it will be regenerated.

### 2.3: PostgreSQL cleanup

The `prow_job_runs.labels` and `release_job_runs.labels` fields are updated via diff
in Step 5, so they don't require separate cleanup.

## Step 3: Write to BigQuery `job_labels`

For each `(symptom, job run, file_match, text_match)` result:

1. Create a `models.JobRunLabel` struct with:
   - `ID` = the prow job build ID (this is the `prowjob_build_id` column)
   - `StartTime` from the job run metadata
   - `Label` = the label's ID
   - `SymptomID` = the symptom's ID (non-empty - distinguishes from manual labels)
   - `SourceTool` = `"sippy-api-reevaluate"` (distinct from `"ci-data-loader"` and
     `"annotate-job-runs"` for provenance tracking)
   - Timestamps

2. Use the extracted `BulkInsertJobRunLabels` helper for efficient batched insertion
   (batches of 500, supports dry-run mode).

**Note:** The `JobRunLabel` model does not have `MatchedFile`/`MatchedText` fields. Provenance of
what matched is captured in the GCS JSON artifacts (Step 4) but not in the BQ table. If BQ-level
provenance is needed later, the model would need extending.

## Step 4: Write to GCS

For each result:

1. Create a `JobRunBucketLabel` struct (from `prow_bucket.go`):
   - `Symptom`: `jobrunscan.SymptomContent` from the matched symptom
   - `Label`: `jobrunscan.LabelContent` from the applied label
   - `FileMatch`: relative path of the matched file
   - `TextMatch`: first matched text (empty for `none` matcher type)
   - `Bucket`, `JobRunPath`: from the job run metadata

2. Call `WriteJSONToBucket()` - writes to
   `{jobRunPath}/artifacts/job_labels/{label_id}-{basename}-{sha256}.json`

3. After all labels are written, call `WriteHTMLSummaryToBucket()`:
   - Reads all JSON files in `artifacts/job_labels/`
   - Generates and writes `label-summary.html` with `text/html` content type
   - Spyglass displays this via the html lens
   - Returns the count of labels included

## Step 5: Update PostgreSQL

### 5.1: Update `prow_job_runs.labels` (diff-based)

The `prow_job_runs.labels` field may contain both symptom-originated labels and manually-applied
labels. Since newly-written BQ entries are stuck in the streaming buffer and can't be queried,
we compute the diff locally:

1. Read the current `Labels` array from the `ProwJobRun` record
2. Query BQ for existing non-symptom-originated labels for evaluated build IDs (not the ones we
   deleted in Step 2.1 - use the opposite filter: `symptom_id IS NULL OR symptom_id == ''`)
4. Add the new symptom labels from re-evaluation results
5. Deduplicate the labels and update the Postgres job run record

This avoids depending on BQ query-ability of recently-streamed rows while preserving
manually-applied labels.

### 5.2: Update `release_job_runs.labels`

Check if the job run is also present in `release_job_runs` (payload job runs) and update that
record's `Labels` field with the same results from `prow_job_runs`. See
`pkg/dataloader/releaseloader/releasesync.go` for the pattern.

### 5.3: Regression data: no action needed

`RegressionJobRun` records store `JobSymptoms` populated from BigQuery during regression cache
loading. The regression cache loader runs multiple times per day and re-queries BQ fresh each time,
calling `MergeJobRuns()` (which updates `job_symptoms` via upsert) and `SyncTriageSymptoms()` (which
recounts triage associations). It processes open regressions and those closed within the last 5 days.

Since re-evaluation updates BQ's `job_labels` table, the next regression cache loader run will
automatically pick up the changes. No direct update from the re-evaluator is needed.

## Step 6: Register the API Endpoint

### 6.1: Add the route in `server.go`

Follow the existing endpoint registration pattern in `Serve()` which uses a slice of `apiEndpoints`
structs. Add the endpoint alongside the existing symptom/label routes:

```go
{
    HandlerFunc:  s.jsonReEvaluateJobRunSymptoms,
    EndpointPath: "/api/jobs/runs/reevaluate",
    Methods:      []string{"POST"},
    Capabilities: []string{LocalDBCapability, WriteEndpointsCapability},
}
```

### 6.2: Implement the handler

Add `jsonReEvaluateJobRunSymptoms` on the Server struct:

```go
func (s *Server) jsonReEvaluateJobRunSymptoms(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    var req struct {
        ProwJobBuildIDs []string `json:"prow_job_build_ids"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    // Validate: non-empty, cap at 50 IDs
    if len(req.ProwJobBuildIDs) == 0 {
        http.Error(w, "prow_job_build_ids is required", http.StatusBadRequest)
        return
    }
    if len(req.ProwJobBuildIDs) > 50 {
        http.Error(w, "maximum 50 job runs per request", http.StatusBadRequest)
        return
    }

    // Create ReEvaluator with server's BQ/GCS/DB clients
    re := &ReEvaluator{
        bqClient:  s.bigQueryClient,
        gcsClient: s.gcsClient,
        gcsBucket: s.gcsBucket,
        db:        s.db,
    }

    results, err := re.ReEvaluateJobRuns(ctx, req.ProwJobBuildIDs)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    resp := &ReEvaluationResponse{Results: results}
    InjectReEvalHATEOASLinks(resp, baseURL(r))
    respondJSON(w, http.StatusOK, resp)
}
```

### 6.3: Capability gating

This endpoint mutates BQ, GCS, and PostgreSQL. It requires both `LocalDBCapability` and
`WriteEndpointsCapability` (enabled via `--enable-write-endpoints` flag). SSO authentication
is handled at the deployment level by the oauth proxy.

### 6.4: Request/Response schema

**Request (POST):**
```json
{
  "prow_job_build_ids": ["1234567890", "0987654321"]
}
```

**Response (200 OK):**

The response includes HATEOAS links at both the top level (self link to the endpoint) and per-result
(links to the job run URL and each matched symptom's API endpoint).

```json
{
  "results": [
    {
      "prow_job_build_id": "1234567890",
      "status": "success",
      "symptoms_evaluated": 42,
      "symptoms_matched": ["DNSTimeout", "PodSandboxFailure", "NetworkError"],
      "labels_applied": ["InfraFailure", "NodeProblem"],
      "bq_entries_written": 3,
      "gcs_artifacts_written": 4,
      "postgres_updated": true,
      "links": {
        "job_run": "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/.../1234567890",
        "symptom:DNSTimeout": "http://localhost:8080/api/jobs/symptoms/DNSTimeout",
        "symptom:PodSandboxFailure": "http://localhost:8080/api/jobs/symptoms/PodSandboxFailure",
        "symptom:NetworkError": "http://localhost:8080/api/jobs/symptoms/NetworkError"
      }
    },
    {
      "prow_job_build_id": "0987654321",
      "status": "missing_error",
      "error": "job run 0987654321 not found in database"
    }
  ],
  "links": {
    "self": "http://localhost:8080/api/jobs/runs/reevaluate"
  }
}
```

## Step 7: Testing

### 7.1: Unit tests (pure logic, no client mocks)

Structure the code if possible with logic separate from storage client calls. Unit test the logic
functions directly, without mocking BQ, GCS, or DB. Go interfaces are not designed for easy mock
substitution of concrete SDK clients, and mock-heavy tests tend to test the mock implementation
rather than real behavior.

Create `pkg/api/jobrunscan/reevaluate_test.go` for pure-logic tests:

- Test request validation: empty IDs, exceeding batch limit, malformed input
- Test symptom filtering: only implemented matcher types (string, regex, none) are included;
  `cel` matchers are excluded
- Test result aggregation: given a set of match results, verify the correct
  `ReEvaluationResult` structs are produced
- Test diff-based label merge logic: given existing labels (some manual, some symptom-originated)
  and new symptom matches, verify the merged label set preserves manual labels, removes old
  symptom labels, and adds new ones
- Test `JobRunLabel` construction: given a symptom match and job run metadata, verify the
  correct `SourceTool`, `SymptomID`, label fields, etc.

### 7.2: Functional / integration tests (real clients, user-supplied credentials)

Follow the pattern in `pkg/dataloader/releaseloader/releasesync_functional_test.go`: tests skip
unless the user supplies environment variables with real connection credentials.

Create `pkg/api/jobrunscan/reevaluate_functional_test.go`:

Required environment variables:
- `GOOGLE_APPLICATION_CREDENTIALS` - path to GCP service account JSON key file
- `BIGQUERY_PROJECT` - GCP project ID
- `BIGQUERY_DATASET` - BigQuery dataset
- `GCS_BUCKET` - GCS bucket for artifact reads/writes
- `SIPPY_DATABASE_DSN` - PostgreSQL connection string
- `PROW_JOB_BUILD_ID` - a real build ID to re-evaluate

Test cases (each skipped when env vars are missing):
- End-to-end: API POST with real credentials, verify BQ entries created, GCS artifacts written,
  PostgreSQL updated
- Idempotency: calling twice with the same input produces the same result
- Symptom change: modify a symptom definition, re-evaluate, verify labels change
- Symptom removal: delete a symptom, re-evaluate, verify its labels are removed
- Manual label preservation: verify annotator-created labels survive re-evaluation

## Step 8: Update Documentation

### 8.1: Update `docs/features/job-analysis-symptoms.md`

1. **Data Flow section**: Add a new subsection "6. Retroactive Re-evaluation" describing:
   - The API endpoint and what it does
   - The three-backend update flow (BQ → GCS → PostgreSQL)
   - How it differs from the cloud function (all-at-once vs. file-by-file)
   - The delete-then-insert strategy for idempotency

2. **API Summary section**: Add:
   - `POST /api/jobs/runs/reevaluate` - re-evaluate symptoms for specified job runs

3. **Key Code Locations table**: Add:
   - `pkg/api/jobrunscan/reevaluate.go` - Re-evaluation service and API handler

4. **Status section**: Update TRT-2695 entry to reflect completion

### 8.2: Update `pkg/api/README.md`

Add the new `/api/jobs/runs/reevaluate` endpoint with request/response schema.

## Design Notes

1. **Delete-then-insert for idempotency**: Clear existing symptom-originated labels
   before re-inserting. This handles symptom definition changes cleanly: if a
   symptom is modified, added, or removed, re-evaluation produces the correct result.

2. **Preserve manual labels**: Only clear entries with non-empty `symptom_id`. Labels
   applied via the `annotate-job-runs` CLI tool have empty `symptom_id` and must be
   preserved through re-evaluation.

3. **Source tracking**: Set `SourceTool = "sippy-api-reevaluate"` so re-evaluated
   labels are distinguishable from cloud function (`"ci-data-loader"`) and CLI
   (`"annotate-job-runs"`) origins in BQ queries. The `JobRunLabel` model does not have
   `MatchedFile`/`MatchedText` fields - file/text match provenance lives in the GCS
   JSON artifacts only.

4. **CEL matchers**: Skip any symptoms with `matcher_type = "cel"`. Log a warning.
   This matcher type is defined in the model but not yet implemented (TRT-2466).

5. **Applicability filters**: `valid_from`/`valid_until` fields exist on the Symptom
   model but nothing uses them yet. When implementing the re-evaluator, add a TODO
   for future integration. When applicability filters are implemented, the
   re-evaluator should respect them (e.g. only evaluate symptoms whose validity
   window covers the job run's start time).

6. **Batch limits**: Cap at 50 job runs per API request. This prevents long-running
   requests, BQ quota issues, and GCS rate limiting.

7. **Synchronous execution**: For the initial implementation, synchronous processing
   is sufficient. Individual job run re-evaluation should complete in seconds. If
   larger batch sizes are needed later, consider async with a task ID + polling
   pattern.

8. **BQ streaming buffer**: Recently streamed rows (< ~90 minutes) may not be
   deletable via DML. For the delete-then-insert pattern, this is usually not a
   problem because re-evaluation targets older job runs. Document this limitation.

9. **One query per symptom**: Rather than building a compound multi-symptom matcher,
   run one `JobArtifactQuery` per symptom per job run. Each query with a different
   matcher reads from GCS independently (the JAQ cache keys on matcher, so different
   symptoms don't share cached reads). This is simpler, reuses existing code without
   modification, and reliability matters more than speed for this rare operation.
   Optimization to multi-symptom matching can be revisited later.

10. **No retries on artifact scan timeout**: If a scan times out for a job run, return
    `eval_error` status for that run in the response. The client can retry individual
    failures. This avoids unbounded retry loops in the API.

11. **Diff-based PostgreSQL label updates**: Because newly-written BQ rows are in the streaming
    buffer and not queryable, we can't use `GatherLabelsFromBQ()` to refresh `prow_job_runs.labels`.
    Instead, query BQ for non-symptom labels, augment them with the re-eval results, and update the
    PostgreSQL label array. This preserves manually-applied labels while avoiding streaming buffer
    issues.

12. **Regression data left to loader**: `regression_job_runs.job_symptoms` and
    `triage_symptoms` are populated by the regression cache loader, which re-queries BQ
    fresh on each run (multiple times/day). Re-evaluation updates BQ, and the loader
    picks up the changes automatically. No direct regression updates from the API.

13. **Job run lookup**: The prow build ID (string) *is* the GORM primary
    key on `ProwJobRun` - just parse to `int64` with `strconv.ParseInt`. The
    `JobArtifactQuery` takes `JobRunIDs []int64` and looks up `ProwJobRun` by primary key,
    extracting the GCS bucket path from the `URL` field. No additional lookup needed.

14. **Limited file matches**: The cloud function can create labels for each file that matches a
    symptom; by limiting the matches during re-evaluation, we may actually delete duplicate labels
    that were generated initially. This is acceptable as duplicates of the same label for a job run
    have no real purpose at this time (but all are visible in Spyglass).                 
