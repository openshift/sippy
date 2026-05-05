# TRT-2633: Support Linking Symptoms/Labels to Triage Records

**Date:** 2026-04-30
**JIRA:** [TRT-2633](https://redhat.atlassian.net/browse/TRT-2633)
**Author:** Stephen Goeddel

## Problem Statement

Triage can get messy in cases where a generic test is failing for lots of
different reasons. It's often hard to spot what's really going on across a set
of regressions tied to a single triage.

Symptom/label usage is increasing as a way to automatically detect problems
hidden deep in job logs and artifacts. Tying this data into triage records would
let engineers see at a glance which symptoms are present across all regressions
in a triage — and what percentage of those regressions exhibit each one.

## Current Architecture

### Symptom Pipeline

1. **Symptom definitions** live in PostgreSQL (`job_run_symptoms` table).
   Each has an `ID` (string), `Summary`, `MatcherType`, `FilePattern`,
   `MatchString`, and `LabelIDs` (labels to apply on match).

2. **Symptom detection** runs against job run artifacts. When a symptom
   matches, it creates entries in BigQuery's `job_labels` table with the
   `symptom_id` field populated, linking the label application back to the
   triggering symptom.

3. **Regression tracking** queries BigQuery for test details, joining
   `job_labels` to aggregate labels per job run:
   ```sql
   SELECT prowjob_build_id,
          STRING_AGG(DISTINCT label, ',' ORDER BY label) AS job_labels
   FROM <dataset>.job_labels ...
   ```
   These label IDs flow through the pipeline:
   `TestJobRunRows.JobLabels` → `JobRunStats.JobLabels` → `RegressionJobRun.JobLabels`

4. **Missing link**: Symptom IDs (`job_labels.symptom_id`) are not aggregated
   or carried through the pipeline. Only label IDs are preserved. There is no
   association between symptoms and triage records.

### Triage Architecture

- A `Triage` record has many `TestRegression` records (via `triage_regressions` join table).
- Each `TestRegression` has many `RegressionJobRun` records (via FK).
- Each `RegressionJobRun` has `JobLabels pq.StringArray` with label IDs from failed runs.
- The regression cache loader (`regressioncacheloader.go`) orchestrates regression
  tracking and job run syncing in a single pass per view.

## Proposed Solution

### Two Core Changes

1. **Carry symptom IDs through the pipeline**: Extend the BigQuery query to
   also aggregate `symptom_id` from `job_labels`, and carry `JobSymptoms`
   alongside `JobLabels` through `TestJobRunRows` → `JobRunStats` →
   `RegressionJobRun`.

2. **New `triage_symptoms` junction table**: A many-to-many association between
   triages and symptoms. During regression tracking, after job runs are synced,
   collect symptom IDs from job runs of regressions that belong to triages and
   upsert the associations.

### Why a Persistent Junction Table

- Provides fast triage detail queries without joining through
  regressions → job runs → symptom IDs → symptoms at request time.
- Creates a stable record of which symptoms were observed, even after
  regression job runs age out or the regression closes.
- Enables future features like notifications when new symptoms appear on a triage.
- Satisfies AC #1 (a many-to-many association must exist between triage records and symptoms in the database).

### Percentage Computation

For AC #5 (show the percentage of regressions exhibiting each symptom on the triage details page), the percentage is computed at
API time via a `COUNT(DISTINCT regression_id)` query against the
`triage_symptoms` junction table, grouped by `symptom_id`. This is fast (single
indexed query) and avoids walking the full regression → job run → symptom graph.

The junction table also stores a per-regression job run count (`job_run_count` column),
which is summed across regressions to produce `job_run_count` in the summary.

Junction rows are upserted and counts updated in place on each loader run.
Since job runs only accumulate, counts grow monotonically and can't
double-count.

## Implementation Plan

### Phase 1: Carry Symptom IDs Through the Data Pipeline

#### BigQuery query

**File:** `pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go`

Extend the `job_labels` aggregation subquery to also aggregate symptom IDs:

```go
jobLabelsJoin := fmt.Sprintf(`LEFT JOIN (
    SELECT prowjob_build_id,
           STRING_AGG(DISTINCT label, ',' ORDER BY label) AS job_labels,
           STRING_AGG(DISTINCT CASE WHEN symptom_id != '' THEN symptom_id END,
                      ',' ORDER BY symptom_id) AS job_symptoms
    FROM %s.job_labels
    WHERE prowjob_start >= DATETIME(@From)
    AND prowjob_start < DATETIME(@To)
    GROUP BY prowjob_build_id
) agg_labels ON junit.prowjob_build_id = agg_labels.prowjob_build_id
`, client.Dataset)
```

Add `ANY_VALUE(agg_labels.job_symptoms) AS job_symptoms` to the SELECT list
alongside the existing `job_labels` column.

#### Data structure changes

**File:** `pkg/apis/api/componentreport/crstatus/types.go`
```go
type TestJobRunRows struct {
    // ... existing fields ...
    JobLabels    []string `bigquery:"-" json:"job_labels,omitempty"`
    JobSymptoms  []string `bigquery:"-" json:"job_symptoms,omitempty"` // NEW
    TestFailures int      `bigquery:"-" json:"test_failures"`
}
```

**File:** `pkg/apis/api/componentreport/testdetails/types.go`
```go
type JobRunStats struct {
    // ... existing fields ...
    JobLabels    []string `json:"job_labels,omitempty"`
    JobSymptoms  []string `json:"job_symptoms,omitempty"` // NEW
    TestFailures int      `json:"test_failures"`
}
```

**File:** `pkg/db/models/triage.go`
```go
type RegressionJobRun struct {
    // ... existing fields ...
    JobLabels   pq.StringArray `json:"job_labels,omitempty" gorm:"column:job_labels;type:text[]"`
    JobSymptoms pq.StringArray `json:"job_symptoms,omitempty" gorm:"column:job_symptoms;type:text[]"` // NEW
}
```

#### Deserialization

**File:** `pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go`

In `deserializeRowToJobRunTestReportStatus()`, add handling alongside the
existing `job_labels` case:
```go
case col == "job_symptoms":
    if row[i] != nil {
        cts.JobSymptoms = strings.Split(row[i].(string), ",")
    }
```

#### Conversion through the pipeline

**File:** `pkg/api/componentreadiness/test_details.go` — `getJobRunStats()`:
```go
JobSymptoms: stats.JobSymptoms,
```

**File:** `pkg/api/componentreadiness/regressiontracker.go` — `FailedJobRunsFromTestDetails()`:
```go
jobRun := models.RegressionJobRun{
    // ... existing fields ...
    JobLabels:   pq.StringArray(run.JobLabels),
    JobSymptoms: pq.StringArray(run.JobSymptoms), // NEW
}
```

#### Update MergeJobRuns to refresh symptom data

**File:** `pkg/api/componentreadiness/regressiontracker.go`

`MergeJobRuns` currently uses `FirstOrCreate` which won't update existing
records. Extend it to update `JobSymptoms` (and `JobLabels`) on existing
records so that newly-detected symptoms are captured on subsequent loader runs:

```go
func (prs *PostgresRegressionStore) MergeJobRuns(regressionID uint, jobRuns []models.RegressionJobRun) error {
    for i := range jobRuns {
        jobRuns[i].RegressionID = regressionID
        res := prs.dbc.DB.
            Where("regression_id = ? AND prow_job_run_id = ?", regressionID, jobRuns[i].ProwJobRunID).
            Assign(models.RegressionJobRun{
                JobLabels:   jobRuns[i].JobLabels,
                JobSymptoms: jobRuns[i].JobSymptoms,
            }).
            FirstOrCreate(&jobRuns[i])
        if res.Error != nil {
            return fmt.Errorf("error merging job run %s for regression %d: %w",
                jobRuns[i].ProwJobRunID, regressionID, res.Error)
        }
    }
    return nil
}
```

`Assign` tells GORM to always update the specified fields — whether the record
is found or created. This ensures that if new symptoms appear on a job run that
was already processed, they are captured on subsequent loader runs. The BigQuery
query returns the full set of symptoms each time (via `STRING_AGG(DISTINCT ...)`),
so the stored value is replaced entirely, not appended to.

### Phase 2: Database Schema — Junction Table

#### New model

**File:** `pkg/db/models/triage.go`

```go
type TriageSymptom struct {
    TriageID     uint   `json:"triage_id" gorm:"primaryKey;column:triage_id"`
    SymptomID    string `json:"symptom_id" gorm:"primaryKey;column:symptom_id"`
    RegressionID uint   `json:"regression_id" gorm:"primaryKey;column:regression_id"`
    JobRunCount  int    `json:"job_run_count" gorm:"column:job_run_count;not null;default:0"`
}
```

The composite key `(triage_id, symptom_id, regression_id)` records exactly which
regression(s) surfaced each symptom on a given triage. `JobRunCount` stores how
many failed job runs on that regression exhibited the symptom.

Rows are upserted during the regression cache loader run, matching the
`FirstOrCreate`-then-update pattern used by `MergeJobRuns`. Since job runs only
accumulate (never deleted), symptom counts are monotonically increasing and
can be safely updated in place without a full replace.

#### Add association to Triage model

Since the junction table now includes `regression_id`, GORM's built-in
`many2many` tag no longer fits (it expects a two-column join). Instead, model
`TriageSymptom` as a standalone entity and query it directly.

```go
type Triage struct {
    // ... existing fields ...
    TriageSymptoms []TriageSymptom `json:"triage_symptoms,omitempty" gorm:"foreignKey:TriageID;constraint:OnDelete:CASCADE"`
}
```

The `OnDelete:CASCADE` GORM constraint ensures that deleting a triage
automatically removes its `TriageSymptom` junction rows. This follows the same
pattern used by `TestRegression.JobRuns` and `TestRegression.Views`.

#### Auto-migration

**File:** `pkg/db/db.go`

Add `TriageSymptom` to the auto-migrate list (this creates the
`triage_symptoms` table).

### Phase 3: Automatic Linking During Regression Tracking

#### New interface method

**File:** `pkg/api/componentreadiness/regressiontracker.go`

```go
type RegressionStore interface {
    // ... existing methods ...
    // SyncTriageSymptoms upserts symptom associations for all triages
    // that have regressions with the given symptom-bearing job runs.
    SyncTriageSymptoms(regressions []*models.TestRegression) error
}
```

#### Implementation

```go
func (prs *PostgresRegressionStore) SyncTriageSymptoms(regressions []*models.TestRegression) error {
    regIDs := make([]uint, len(regressions))
    for i, r := range regressions {
        regIDs[i] = r.ID
    }

    var regs []models.TestRegression
    res := prs.dbc.DB.
        Preload("Triages").
        Preload("JobRuns").
        Where("id IN ?", regIDs).
        Find(&regs)
    if res.Error != nil {
        return fmt.Errorf("error loading regressions for symptom sync: %w", res.Error)
    }

    for _, reg := range regs {
        if len(reg.Triages) == 0 {
            continue
        }
        // Count job runs per symptom for this regression
        symptomCounts := map[string]int{}
        for _, jr := range reg.JobRuns {
            seen := sets.New[string]()
            for _, s := range jr.JobSymptoms {
                if s != "" && !seen.Has(s) {
                    seen.Insert(s)
                    symptomCounts[s]++
                }
            }
        }
        // Upsert one junction row per (triage, symptom, regression)
        for symptomID, count := range symptomCounts {
            for _, triage := range reg.Triages {
                ts := models.TriageSymptom{
                    TriageID:     triage.ID,
                    SymptomID:    symptomID,
                    RegressionID: reg.ID,
                }
                result := prs.dbc.DB.Where(ts).FirstOrCreate(&ts)
                if result.Error != nil {
                    return fmt.Errorf("error syncing symptom %s to triage %d regression %d: %w",
                        symptomID, triage.ID, reg.ID, result.Error)
                }
                // Update count whether newly created or already existed
                if err := prs.dbc.DB.Model(&ts).Update("job_run_count", count).Error; err != nil {
                    return fmt.Errorf("error updating symptom count: %w", err)
                }
            }
        }
    }
    return nil
}
```

#### Integration in cache loader

**File:** `pkg/dataloader/regressioncacheloader/regressioncacheloader.go`

After the per-release regression closing loop (around line 180), add a global
symptom sync step:

```go
// SyncTriageSymptoms runs unconditionally — it is additive and idempotent,
// so partial errors in individual views should not block symptom linking.
var allActiveRegs []*models.TestRegression
for _, result := range releaseResults {
    for _, id := range result.activeIDs.UnsortedList() {
        allActiveRegs = append(allActiveRegs, &models.TestRegression{ID: id})
    }
}
if len(allActiveRegs) > 0 {
    if err := l.regressionStore.SyncTriageSymptoms(allActiveRegs); err != nil {
        l.logger.WithError(err).Error("error syncing triage symptoms")
        l.errs = append(l.errs, err)
    }
}
```

This runs once per loader execution, after all views are processed, ensuring
symptoms from all views are linked. It is not gated behind `!anyErrors` because
the operation is additive (only upserts) and idempotent — partial view errors
should not block symptom linking for regressions that were successfully processed.

### Phase 4: API Changes

#### New response type for symptom summaries

**File:** `pkg/api/componentreadiness/triage.go`

```go
type TriageSymptomSummary struct {
    Symptom         jobrunscan.Symptom `json:"symptom"`
    RegressionCount int                `json:"regression_count"`
    TotalCount      int                `json:"total_count"`
    Percentage      float64            `json:"percentage"`
    JobRunCount     int                `json:"job_run_count"`
    RegressionIDs   []uint             `json:"regression_ids"`
}
```

`RegressionIDs` lists which regressions on this triage exhibit each symptom.
The frontend uses this to build a per-regression symptom map and to filter
the regression table by symptom — without walking job runs client-side.

`GetTriageSymptomSummaries(dbc, triageID, totalRegressions)` queries the
`triage_symptoms` junction table with `COUNT(DISTINCT regression_id)` and
`SUM(job_run_count)` grouped by `symptom_id`, then loads symptom definitions
from `job_run_symptoms`, and finally queries the raw junction rows to build
the per-symptom `RegressionIDs` slice. Symptom IDs not found in
`job_run_symptoms` are silently skipped (handles stale/test data).

#### Extend ExpandedTriage response

**File:** `pkg/sippyserver/server.go`

```go
type ExpandedTriage struct {
    *models.Triage
    RegressedTests   map[string][]*componentreport.ReportTestSummary `json:"regressed_tests"`
    SymptomSummaries []componentreadiness.TriageSymptomSummary       `json:"symptom_summaries,omitempty"`
}
```

#### Comma-separated `expand` parameter

The `jsonGetTriageByID` handler parses `expand` as a comma-separated set of
field names (e.g., `?expand=regressions,symptoms`). Each field is handled
independently:

- `symptoms` — computes symptom summaries via `GetTriageSymptomSummaries`.
  This is cheap (single indexed query against the junction table).
- `regressions` — looks up regressed tests from the component report cache.
  This is more expensive, so it is only computed when explicitly requested.

Symptoms are also computed when `regressions` is requested (since the triage
detail page needs both), but `expand=symptoms` alone returns summaries
without the regressed test lookups.

### Phase 5: Frontend Changes

#### Triage Details Page Layout

**File:** `sippy-ng/src/component_readiness/Triage.js`

The current page layout is:
1. Header with action buttons (Ask Sippy, Update, Delete, etc.)
2. Metadata table (Resolved, Description, Type, Jira fields, etc.)
3. "Included Tests" section with the `TriagedRegressionTestList` DataGrid

The triage detail fetch uses `?expand=regressions,symptoms` to request both
regressed test lookups and symptom summaries in a single call.

Add a new **"Symptoms"** section between the metadata table and the
"Included Tests" section. This is the natural placement because symptoms
provide context for interpreting the regressions below.

**Symptoms section structure:**

- Section header: **"Symptoms"** with a count badge (e.g., "Symptoms (3)")
- If `symptom_summaries` is empty or absent, show a muted "No symptoms
  detected" message — don't hide the section entirely, so users know it exists.
- If populated, render a MUI `Table` (not DataGrid — the list will be small
  enough that pagination and filtering aren't needed) with these columns:

| Column | Source field | Display |
|--------|-------------|---------|
| Symptom | `symptom.summary` | Text, clickable link to the symptom detail page |
| Regressions | `regression_count` / `total_count` | e.g., "2 / 5", with a filter button (see below) |
| Percentage | `percentage` | MUI `LinearProgress` bar with percentage label |
| Failed Runs | `job_run_count` | Plain number — total failed job runs across regressions exhibiting this symptom |

- Table is sorted by percentage descending (already sorted by the API).
- Each symptom row's summary text should link to the Job Artifact Query page
  prefilled with that symptom, following the existing `JAQPrefilled` pattern
  from `JobArtifactQuery.js`.

#### Filtering Regressions by Symptom

The Regressions column shows how many of the triage's regressions exhibit each
symptom (e.g., "2 / 5"). Next to the count, add a small MUI `IconButton` with
a filter icon (`FilterList`). Clicking it filters the "Included Tests"
`TriagedRegressionTestList` DataGrid below to show only regressions that have
that symptom.

**Implementation:**

- `Triage.js` holds a `symptomFilter` state (symptom ID or `null`).
- Clicking the filter button sets `symptomFilter` to that symptom's ID.
- When a filter is active, show a `Chip` above the regressions table indicating
  the active filter (e.g., "Filtered by: <symptom summary>") with a clear (X)
  button that resets `symptomFilter` to `null`.
- `TriagedRegressionTestList` receives `symptomFilter` and `symptomSummaries`
  props. When `symptomFilter` is set, it filters rows to only regressions
  whose IDs appear in the matching symptom summary's `regression_ids` array.

#### Per-Regression Symptom Indicators (AC #6 — each regression row indicates which triage-associated symptoms it exhibits)

**File:** `sippy-ng/src/component_readiness/TriagedRegressionTestList.js`

A **"Symptoms"** column (flex: 10) appears after the Variants column, only
when `symptomSummaries` is provided. For each regression row, the component
builds a `regressionSymptomMap` from the `symptomSummaries` prop — iterating
each summary's `regression_ids` to map regression IDs to their symptom IDs.
Each matching symptom is rendered as a small MUI `Chip` with a truncated label
(first ~12 chars) and a `Tooltip` showing the full summary. Chips are colored
using a deterministic hash of the symptom ID for visual distinction.

## Test Plan

### Unit Tests: Pipeline Changes

**File:** `pkg/api/componentreadiness/regressiontracker_test.go`

Added to existing `TestFailedJobRunsFromTestDetails` table-driven tests:

| Test Case | Input | Expected |
|-----------|-------|----------|
| preserves JobSymptoms | Report with `JobSymptoms: ["SymA","SymB"]` | `runs[0].JobSymptoms` matches |
| empty JobSymptoms results in nil | Report with no symptoms | `runs[0].JobSymptoms` is nil |
| mixed runs: only symptomatic run carries symptoms | One run with `["SymA"]`, one without | Only first run has symptoms, second is nil |

### E2E Tests: MergeJobRuns Symptom Behavior

**File:** `test/e2e/componentreadiness/regressiontracker/regressiontracker_test.go`

Added to existing `Test_RegressionJobRuns`:

| Subtest | Setup | Assert |
|---------|-------|--------|
| new job run with symptoms | Merge run with `JobSymptoms: ["SymA"]` | Stored run has `JobSymptoms` = `["SymA"]` |
| existing job run gains symptoms on re-merge | First merge with no symptoms, second with `["SymA"]` | Stored run updated to `["SymA"]` |

### E2E Tests: SyncTriageSymptoms

**File:** `test/e2e/componentreadiness/regressiontracker/regressiontracker_test.go`

New top-level `Test_SyncTriageSymptoms` function:

| Subtest | Setup | Assert |
|---------|-------|--------|
| links symptoms to triage | Regression + triage + job run with `["SymA","SymB"]`, call `SyncTriageSymptoms` | 2 junction rows, correct `regression_id` and `job_run_count` |
| idempotent | Call `SyncTriageSymptoms` twice with same data | Same row count, same `job_run_count` |
| count accuracy | 3 job runs: 2 with SymA, 1 without | `job_run_count` = 2 for SymA |
| count grows with new job runs | After first sync, merge additional run with SymA, re-sync | `job_run_count` increments |
| regression without triage is skipped | Regression with symptoms but no triage | Zero `triage_symptoms` rows |
| multiple symptoms per run | Job run with `["SymA","SymB"]` | Both symptoms get junction rows |

### E2E Tests: Symptom Summaries in API Response

**File:** `test/e2e/componentreadiness/triage/triageapi_test.go`

Added to existing `Test_TriageAPI`:

| Subtest | Setup | Assert |
|---------|-------|--------|
| expanded triage includes symptom summaries | Regression + triage + job runs with symptoms, `SyncTriageSymptoms`, GET `?expand=regressions,symptoms` | `symptom_summaries` non-empty, correct `regression_ids`, `regression_count`, `job_run_count` |
| expand=symptoms only returns symptoms without regressed_tests | Same setup, GET `?expand=symptoms` | `symptom_summaries` present, `regressed_tests` nil |
| delete triage cascades to triage_symptoms | Create triage + symptoms, DELETE triage | Zero junction rows for that triage |

These tests seed `job_run_symptoms` (Symptom) records for the test symptom IDs
so `GetTriageSymptomSummaries` can resolve them via the join.

## Cascade and Deletion Behavior

| Action | Result |
|--------|--------|
| Delete triage | Junction rows in `triage_symptoms` removed. Symptoms unaffected. |
| Soft-delete symptom | Junction rows persist. Symptom filtered from preloaded queries (GORM soft delete). |
| Hard-delete symptom | Junction rows removed (DB-level FK cascade or explicit cleanup). |
| Delete regression from triage | Junction rows for that triage/regression pair should be removed (FK cascade or explicit cleanup). |
| Close regression | No effect on junction. Symptoms remain as historical record. |

## Migration and Backward Compatibility

1. **Zero impact on existing data.** The new `job_symptoms` column on
   `regression_job_runs` is added via GORM AutoMigrate with NULL default.
   Existing rows remain unchanged.

2. **No retroactive backfill.** Per AC #8 (no retroactive linking of symptoms to existing triages), existing triages will not have
   symptoms linked until the next regression cache loader run processes their
   regressions. Symptoms accumulate naturally going forward.

3. **BigQuery compatibility.** The `symptom_id` column already exists in the
   `job_labels` BigQuery table. The query change only adds an aggregation of
   this existing column.

4. **API backward compatibility.** The `symptom_summaries` field is additive
   on the `ExpandedTriage` response. Clients that don't use it are unaffected.
   The `symptoms` field on `Triage` is also additive.

## Out of Scope (Future Work)

- Manual association of symptoms to triages via the UI.
- Retroactive linking of symptoms to existing/historical triages.
- Display of symptom data on the Test Details page.
- Compare Sample Failures integration with symptom filtering.
