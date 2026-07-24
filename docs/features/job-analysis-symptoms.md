# Job Analysis: Symptoms and Labels

**Epic:** [TRT-2370](https://redhat.atlassian.net/browse/TRT-2370)

## Purpose

CI job runs produce large volumes of artifacts. Diagnosing job failures often requires manually
searching through those artifacts for known patterns. The symptoms feature automates this: it lets
engineers define artifact patterns that indicate a known condition (a "symptom"), and automatically
tags matching job runs with human-readable labels, which are visible across the Sippy UI, Spyglass,
and triage workflows.

## Core Concepts

### Labels

A **label** is a named tag applied to a job run. Labels have:

- An immutable `ID` (e.g. `InfraFailure`, `ClusterDNSFlake`) - used as the stable reference in BQ
  and expressions.
- A human-readable `label_title` that can be updated freely.
- An optional markdown `explanation`.
- `hide_display_contexts` - a denylist of display contexts where the
  label should not be shown (e.g. `"spyglass"`, `"metrics"`).

Labels are the output of the symptom system.

### Symptoms

A **symptom** defines a rule for detecting a condition in job artifacts. Symptoms have:

- An immutable `ID` and human-readable `summary`.
- A **matcher** that determines how artifacts are searched:
  - `string` - glob file pattern + substring match in file content.
  - `regex` - glob file pattern + regex match in file content.
  - `file` - glob file pattern only (file existence check).
- `label_ids` - labels to apply when this symptom matches.
- **Applicability filters** - optional scoping by release, release status, product, and time window
  (`valid_from`/`valid_until`). Note: the field exists for this, but nothing uses it yet.

### Relationship

```text
Symptom  ──matches──▶  job artifact
   │                      │
   │ applies              │
   ▼                      ▼
 Label  ◀──recorded on── job run (BQ + GCS + Postgres)
```

Multiple symptoms can apply the same label; a single symptom can apply multiple labels.

## Data Flow

### 1. Definition

Symptoms and labels are created and managed through:

- **Sippy REST API** - full CRUD at `/api/jobs/symptoms` and `/api/jobs/labels`.
- **JAQ UI** - the Job Artifact Query dialog (available from test details and job runs pages) can
  define a query, match it against job runs, and save it as a symptom definition.
- **Seed data** - `cmd/sippy/seed_data.go` bootstraps built-in label/symptom definitions.
- **Client library** - `pkg/sippyclient/jobrunscan/` provides a Go client used by the cloud function
  and CLI tools.

Definitions are stored in Sippy's PostgreSQL database (tables `job_run_symptoms` and
`job_run_labels`).

### 2. Detection (Cloud Function)

A GCS-triggered Google Cloud Function (ci-data-loader in `openshift/ci-cloud-functions`) fires when
files are created in the Prow job artifact bucket:

1. Fetches and briefly caches symptom definitions from the Sippy API.
2. For each new artifact file, checks if it matches any symptom's `file_pattern` glob.
3. If a file matches, applies the content matcher (substring, regex, or file-exists) against the
   file content.
4. On match, writes two things:
   - A row in BigQuery's `ci_analysis_us.job_labels` table (with `symptom_id`, `label`, timestamps,
     and provenance).
   - A JSON file in the job's GCS bucket at `artifacts/job_labels/{label_id}-{filename}-{hash}.json`
     containing the symptom, label, matched file, and matched text.
5. When `finished.json` is created (signals Prow job completion), generates an HTML summary
   (`artifacts/job_labels/label-summary.html`) from all label JSON files. Spyglass displays this via
   the html lens.

> The matching and bucket-writing logic lives in the **sippy** codebase (not ci-data-loader) as a
> central reusable library.

### 3. Ingestion into Sippy

The `fetchdata` CronJob (sippy prow loader) reads labels from BigQuery and populates the `Labels`
array on `ProwJobRun` records in PostgreSQL (which are written to the `prow_job_runs` table).

The sippy release loader does the same for payload job runs (`release_job_runs` table).

### 4. Triage Integration

The regression cache loader syncs symptom associations to triage records via `TriageSymptom`
junction records. This allows the triage UI to show which symptoms are present across a triage's
regressions and what percentage exhibit each one.

### 5. UI Display

Labels are relevant in several Sippy pages:

- Classic job runs table - labels column.
- Payload job runs table - labels column.
- JAQ dialog - symptom management and label display.
- Triage details - symptom summaries per regression displayed on test details pages.
- Re-evaluation controls - button in the JAQ dialog action bar. Triggers retroactive symptom
  matching for selected (or all visible) job runs in the dialog (requires SSO authentication
  via the write-enabled deployment).

They are also displayed in Spyglass (the Deck display of a Prow job) - an HTML summary added to the
job's bucket entry is included by the html lens.

### 6. Retroactive Re-evaluation

The `POST /api/jobs/runs/reevaluate` endpoint re-runs symptom detection for specified job runs.
Unlike the cloud function (which processes files as they arrive), the re-evaluator scans all
artifacts at once for completed job runs.

Flow:

1. Load all active symptom definitions from PostgreSQL (excluding unimplemented matcher types).
2. For each job run, run one `JobArtifactQuery` per symptom against GCS artifacts.
3. Delete existing symptom-originated labels (BQ rows with non-empty `symptom_id`, GCS label files).
4. Write new results to BQ (`job_labels` table), GCS (label JSON files + HTML summary), and
   PostgreSQL (`prow_job_runs.labels` and `release_job_runs.labels`).

The delete-then-insert strategy makes re-evaluation idempotent: if a symptom is modified, added, or
removed, re-evaluating produces the correct result. Manually-applied labels (those with empty
`symptom_id`) are preserved through re-evaluation.

## Key Code Locations

### Sippy (`openshift/sippy`)

| Path | Contents |
|------|----------|
| `pkg/db/models/jobrunscan/` | Data models: `Symptom`, `Label`, `Metadata` structs and their Postgres table mappings. |
| `pkg/db/models/job_labels.go` | `JobRunLabel` - BigQuery row schema for the `job_labels` table. |
| `pkg/db/models/prow.go` | `ProwJobRun.Labels` - the label array stored in Postgres. |
| `pkg/db/models/triage.go` | `TriageSymptom` - junction table linking symptoms to triage records. |
| `pkg/api/jobrunscan/` | API handlers for symptom/label CRUD and re-evaluation, with validation logic. |
| `pkg/api/jobrunscan/reevaluate.go` | Re-evaluation service: symptom scanning, BQ/GCS/PostgreSQL write logic. |
| `pkg/sippyserver/job_run_scan.go` | HTTP route handlers delegating to the jobrunscan API package. |
| `pkg/sippyclient/jobrunscan/` | Go client library for symptom/label APIs (used by cloud function). |
| `pkg/componentreadiness/jobrunannotator/jobrunannotator.go` | `JobRunAnnotator` - the `annotate-job-runs` tool which can add labels but doesn't (yet) know about symptoms. |
| `pkg/componentreadiness/jobrunannotator/prow_bucket.go` | `JobRunBucketLabel`, `WriteHTMLSummaryToBucket` - writes label files and HTML summaries to GCS. Shared with cloud function. |
| `pkg/api/jobartifacts/` | `JobArtifactQuery`, `ContentMatcher` - the artifact querying and matching engine used by JAQ and symptom evaluation. Results (matched lines per file) are cached by `(jobRunID, pathGlob, matcherKey)` to avoid re-scanning the same job run for the same query; this does **not** cache raw GCS file contents. |
| `pkg/dataloader/prowloader/prow.go` | `GatherLabelsFromBQ` - reads labels from BQ during fetchdata. |
| `pkg/api/componentreadiness/regressiontracker.go` | `SyncTriageSymptoms` - links symptoms to triage records. |
| `cmd/sippy/seed_data.go` | Bootstrap definitions of symptoms and labels for use in manual testing. |
| `sippy-ng/src/component_readiness/JobArtifactQuery.js` | JAQ dialog including symptom creation UI. |
| `sippy-ng/src/component_readiness/TriageSymptoms.js` | Triage symptom display component. |

### Cloud Function (`openshift/ci-cloud-functions`)

The `ci-data-loader` cloud function imports sippy packages (`sippyclient`, `jobrunannotator`) and
orchestrates per-file symptom evaluation and label writing. See the ci-cloud-functions repo for
deployment and configuration.

## API Summary

All endpoints are under `/api/jobs/` and support standard CRUD:

- `GET/POST /api/jobs/labels` - list / create labels
- `GET/PUT/DELETE /api/jobs/labels/{id}` - read / update / delete
- `GET/POST /api/jobs/symptoms` - list / create symptoms
- `GET/PUT/DELETE /api/jobs/symptoms/{id}` - read / update / delete
- `POST /api/jobs/runs/reevaluate` - re-evaluate symptoms for specified job runs

See `pkg/api/jobrunscan/` for validation rules and `pkg/api/README.md` for broader API
documentation.

## Storage Locations

| Store | What | Purpose |
|-------|------|---------|
| PostgreSQL `job_run_symptoms` | Symptom definitions | Authoritative source for symptom rules. |
| PostgreSQL `job_run_labels` | Label definitions | Authoritative source for label metadata. |
| PostgreSQL `prow_job_runs.labels` | Applied label IDs per job run | Sippy queries and UI display. |
| PostgreSQL `release_job_runs.labels` | Applied label IDs per payload job run | Sippy queries and UI display. |
| PostgreSQL `triage_symptoms` | Symptom↔triage associations | Triage UI symptom summaries. |
| BigQuery `ci_analysis_us.job_labels` | Applied labels with provenance | Warehouse for analytics; source of truth during fetchdata. |
| GCS `artifacts/job_labels/*.json` | Per-match label files | Provenance and Spyglass display. |
| GCS `artifacts/job_labels/label-summary.html` | HTML summary | Rendered by Spyglass html lens. |

## Status

The symptoms pipeline (definition → detection → labeling → display) is fully operational.
Active/planned work includes:

- **Compound symptoms** ([TRT-2466](https://redhat.atlassian.net/browse/TRT-2466))
  - richer CEL-based label composition.
- **Full management UI** ([TRT-2479](https://redhat.atlassian.net/browse/TRT-2479))
  - dedicated UI for label/symptom lifecycle.
