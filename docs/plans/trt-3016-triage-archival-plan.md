# TRT-3016: Triage Archival

**Date:** 2026-10-27
**JIRA:** [TRT-3016](https://redhat.atlassian.net/browse/TRT-3016)
**Planning ticket:** [TRT-3053](https://redhat.atlassian.net/browse/TRT-3053)

## Incomplete Input Disclosure

This plan was produced without access to TRT-3016's current description and
comments. The Atlassian integration required authentication that was unavailable
during planning. All requirements below are inferred from the codebase and the
planning ticket's description. The plan should be reviewed for alignment with
TRT-3016's actual requirements and acceptance criteria before implementation
begins.

Sections that depend on the parent's requirements are marked with
**[needs TRT-3016 verification]** to distinguish evidence-grounded proposals
from assumptions that require confirmation.

## 1. Objective

Introduce an archival mechanism for resolved triage records so that triages
associated with releases that have reached General Availability (GA) can be
moved out of the default listing. This keeps the active triage listing focused
on in-progress work while preserving historical records for audit and reference.

### Non-Goals

- Deleting archived triages or their associated data (regressions, job runs,
  audit logs). Archival is a visibility filter, not a data-retention change.
- Changing the resolution logic itself (`ResolveTriages`, Jira progression,
  user resolution). The existing resolution mechanisms remain unchanged.
- Modifying the regression cache loader or regression tracking behavior.
  Archived triages are inert from the loader's perspective because their
  regressions are already closed.
- Retroactive backfill of archival state for triages that do not meet the
  archival criteria at the time the feature ships. **[needs TRT-3016
  verification]**: the parent may require a one-time backfill for existing
  resolved triages on GA releases.

### Requirements and Acceptance Criteria Mapping

**[needs TRT-3016 verification]**: The following are inferred acceptance
criteria. Each should be verified against TRT-3016's actual requirements.

| # | Inferred Requirement | Source |
|---|---------------------|--------|
| R1 | Resolved triages whose regressions belong exclusively to GA releases can be archived | Codebase: `Triage.Resolved`, `TestRegression.Release`, `ReleaseDefinition.GADate` |
| R2 | The default triage listing endpoint excludes archived triages | Codebase: `ListTriages` in `pkg/db/query/triage_queries.go` |
| R3 | An explicit query parameter allows clients to include archived triages | Codebase: existing `view` parameter pattern on `ListTriages` |
| R4 | Archival can be triggered automatically when a release goes GA | Codebase: `ReleaseDefinition.GADate` populated by `releasedefloader` |
| R5 | Archival can be reversed (unarchived) if needed | Codebase: existing pattern where Jira progression resolution can be cleared |
| R6 | The frontend filters archived triages by default with a toggle to show them | Codebase: `TriagedRegressions.jsx` bookmark pattern |
| R7 | Archived triages remain accessible by direct ID lookup | Codebase: `GetTriage` by ID has no filtering |

## 2. Current Behavior and Relevant Repository Paths

### Triage Lifecycle

A triage record progresses through these states:

1. **Created** (unresolved): `Triage.Resolved` is `NULL`.
2. **Resolved**: `Triage.Resolved` is set via one of three mechanisms:
   - `User`: a human sets the resolution time via the API
     (`pkg/api/componentreadiness/triage.go:UpdateTriage`)
   - `RegressionsRolledOff`: all associated regressions have been closed for
     at least 5 days (`pkg/api/componentreadiness/regressiontracker.go:ResolveTriages`)
   - `JiraProgression`: the linked Jira bug moved to a resolved status
     (`pkg/dataloader/bugloader/bugloader.go:reconcileTriages`)
3. **No archival state exists today.** Resolved triages remain in the listing
   indefinitely.

### Key Paths and Symbols

| Path | Symbol | Role |
|------|--------|------|
| `pkg/db/models/triage.go` | `Triage` | Core model. Has `Resolved sql.NullTime`, `ResolutionReason`, `Regressions []TestRegression` |
| `pkg/db/models/triage.go` | `resolutionReason` | Enum: `User`, `RegressionsRolledOff`, `JiraProgression` |
| `pkg/db/models/triage.go` | `TestRegression` | Has `Release string`, `Closed sql.NullTime`, `Views []RegressionView` |
| `pkg/db/models/releases.go` | `ReleaseDefinition` | Has `GADate *civil.Date`, `Release string` |
| `pkg/db/query/triage_queries.go` | `ListTriages(dbc, view)` | Lists triages, optionally filtered by view via active regressions. No resolved/archived filter. |
| `pkg/db/query/triage_queries.go` | `ListRegressions(dbc, release)` | Lists regressions optionally filtered by release. |
| `pkg/api/componentreadiness/triage.go` | `ListTriages(dbc, req)` | API-layer listing with HATEOAS link injection. Delegates to `query.ListTriages`. |
| `pkg/api/componentreadiness/triage.go` | `GetTriage(dbc, id, req)` | Single triage lookup by ID. No archival filter. |
| `pkg/sippyserver/server.go:1870` | `jsonGetTriages` | HTTP handler for `GET /api/component_readiness/triages`. Supports `view` and `fields` parameters. |
| `pkg/sippyserver/server.go:1899` | `jsonGetTriageByID` | HTTP handler for `GET /api/component_readiness/triages/{id}`. Supports `expand` parameter. |
| `pkg/api/componentreadiness/regressiontracker.go:167` | `ResolveTriages()` | Auto-resolves triages with all regressions closed past hysteresis. |
| `pkg/dataloader/bugloader/bugloader.go` | `reconcileTriages` | Auto-resolves/unresolves triages based on Jira bug status progression. |
| `pkg/dataloader/regressioncacheloader/regressioncacheloader.go` | `Load()` | Orchestrates regression tracking, calls `ResolveTriages()` after all releases processed. |
| `pkg/dataloader/releasedefloader/releasedefloader.go` | `Load()` | Syncs release definitions (including `GADate`) from BigQuery. |
| `sippy-ng/src/component_readiness/TriageList.jsx` | `TriageList` | Frontend page listing all triages. No archival filtering. |
| `sippy-ng/src/component_readiness/TriagedRegressions.jsx` | `TriagedRegressions` | DataGrid component rendering triage entries with "Resolved"/"Not Resolved" bookmarks. |

### Release-to-Triage Association

A triage does not directly store a release. The association is:

```
Triage --many2many--> TestRegression.Release --lookup--> ReleaseDefinition.GADate
```

- `triage_regressions` join table: `(triage_id, test_regression_id)`
- `test_regressions.release`: the OCP release string (e.g., "4.18")
- `release_definitions.ga_date`: `DATE` column, `NULL` for in-development releases

A single triage can span multiple releases if its regressions cover different
releases. This is relevant for archival eligibility: a triage should only be
archived when *all* of its associated releases have gone GA.

### Resolution Timestamp Storage

- `Triage.Resolved` is `sql.NullTime` (Go) / `TIMESTAMPTZ` (PostgreSQL).
- Written by three paths: `UpdateTriage` (user), `ResolveTriages` (regression
  tracker), `reconcileTriages` (bug loader).
- The `ResolutionReason` field tracks which mechanism set it.

### Reopening and Re-resolution Semantics

- Jira-progression resolutions can be cleared when the linked bug returns to
  an open status (`bugloader.go:reconcileTriages`, unresolve branch).
- User and regressions-rolled-off resolutions are never automatically cleared.
- There is no concept of re-archival after unarchival today.

## 3. Recommended Design

### Approach: Column-Based Archival with Daemon-Driven Auto-Archive

Add an `Archived` boolean column to the `triages` table. The regression cache
loader (or a new step in the existing daemon cycle) evaluates resolved triages
for archival eligibility after each run. The listing endpoint filters out
archived triages by default, with a query parameter to include them.

**Why this approach over alternatives:**

- **Column on the model vs. separate table**: A boolean column is simpler,
  requires no joins, and is consistent with how `Resolved` is stored. A
  separate archival table would add complexity without benefit since archival
  is a property of the triage itself, not a relationship.

- **Daemon-driven vs. API-triggered**: Auto-archival in the daemon ensures
  triages are archived promptly after a release goes GA without requiring user
  action. This is consistent with how auto-resolution works today
  (`ResolveTriages` runs in the regression cache loader).

- **Boolean vs. timestamp**: A boolean is sufficient because the audit log
  already captures when changes happen. Adding an `ArchivedAt` timestamp would
  be redundant. **[needs TRT-3016 verification]**: if the parent requires
  tracking *when* archival occurred independently of the audit log, use
  `sql.NullTime` instead.

### Archival Eligibility Criteria

A triage is eligible for auto-archival when:

1. It is resolved (`Resolved IS NOT NULL`).
2. All of its associated regressions belong to releases that have a GA date
   set (`ReleaseDefinition.GADate IS NOT NULL` for every distinct
   `TestRegression.Release` in its regressions).
3. It is not already archived.

**Edge case: triage with no regressions.** A resolved triage with zero
regressions has no release association. Conservative default: do not
auto-archive these. They can be manually archived via the API if needed.
**[needs TRT-3016 verification]**

**Edge case: multi-release triage.** A triage spanning releases 4.17 (GA) and
4.18 (in-development) is NOT eligible for archival until 4.18 also reaches GA.
This prevents premature archival of triages that are still relevant to active
development.

### Verification Needs

- **Archival eligibility query performance**: The query joins `triages` ->
  `triage_regressions` -> `test_regressions` -> `release_definitions`. With
  indexes on `test_regressions.release` (existing
  `idx_test_regression_release`) and `release_definitions.release` (existing
  unique index), this should be efficient. Validate on prod-like data before
  merging.
- **Race condition with re-resolution**: If a triage is auto-archived and then
  unresolved (Jira bug reopened), the archival should be cleared. The daemon
  should check `Resolved IS NOT NULL` as part of eligibility, so an unresolved
  triage will not be re-archived. Additionally, the unresolve path in
  `bugloader.go` should clear `Archived` when clearing resolution.

## 4. Implementation Phases

### Phase 1: Data Model and Migration

**Affected files:**

- `pkg/db/models/triage.go`
- `pkg/db/db.go` (AutoMigrate list, already includes `&models.Triage{}`)

**Changes:**

Add an `Archived` field to the `Triage` struct:

```go
type Triage struct {
    // ... existing fields ...

    // Archived indicates whether this triage has been archived. Archived triages
    // are excluded from the default listing but remain accessible by ID.
    Archived bool `json:"archived" gorm:"not null;default:false;index:idx_triages_archived"`
}
```

GORM AutoMigrate will add the column with `DEFAULT false` and create the index.
No data migration is needed because the default value is `false`, matching the
current implicit state of all existing triages.

The index `idx_triages_archived` supports the listing query's `WHERE archived =
false` filter. A partial index (`WHERE archived = true`) could be more
efficient if the vast majority of triages are not archived, but the standard
B-tree index is simpler and sufficient until proven otherwise.

**Dependencies:** None. This phase is independent.

### Phase 2: Auto-Archive in the Daemon

**Affected files:**

- `pkg/api/componentreadiness/regressiontracker.go` (new method on `RegressionStore`)
- `pkg/dataloader/regressioncacheloader/regressioncacheloader.go` (call site)

**Changes:**

Add a new method to the `RegressionStore` interface:

```go
type RegressionStore interface {
    // ... existing methods ...

    // ArchiveResolvedTriages marks resolved triages as archived when all of their
    // associated regressions belong to GA releases.
    ArchiveResolvedTriages() error
}
```

Implementation in `PostgresRegressionStore`:

```go
func (prs *PostgresRegressionStore) ArchiveResolvedTriages() error {
    // Find resolved, non-archived triages where every regression's release has a GA date.
    // Uses NOT EXISTS with a subquery that checks for any regression whose release
    // lacks a GA date. If no such regression exists, all releases are GA.
    //
    // Also excludes triages with zero regressions (no release association).
    res := prs.dbc.DB.Exec(`
        UPDATE triages
        SET archived = true, updated_at = NOW()
        WHERE resolved IS NOT NULL
          AND archived = false
          AND EXISTS (
              SELECT 1 FROM triage_regressions tr
              WHERE tr.triage_id = triages.id
          )
          AND NOT EXISTS (
              SELECT 1
              FROM triage_regressions tr
              JOIN test_regressions r ON tr.test_regression_id = r.id
              LEFT JOIN release_definitions rd ON rd.release = r.release
              WHERE tr.triage_id = triages.id
                AND (rd.ga_date IS NULL OR rd.release IS NULL)
          )
    `)
    if res.Error != nil {
        return fmt.Errorf("error archiving resolved triages: %w", res.Error)
    }
    if res.RowsAffected > 0 {
        log.WithField("archived", res.RowsAffected).Info("archived resolved triages for GA releases")
    }
    return nil
}
```

**Integration in the regression cache loader** (`Load()` method):

Call `ArchiveResolvedTriages()` after `ResolveTriages()`, gated behind the same
`!anyErrors` check:

```go
if !anyErrors {
    l.logger.Info("resolving triages with all regressions closed")
    if err := l.regressionStore.ResolveTriages(); err != nil {
        l.errs = append(l.errs, fmt.Errorf("error resolving triages: %w", err))
    }
    l.logger.Info("archiving resolved triages for GA releases")
    if err := l.regressionStore.ArchiveResolvedTriages(); err != nil {
        l.errs = append(l.errs, fmt.Errorf("error archiving triages: %w", err))
    }
}
```

**Why after `ResolveTriages`:** A triage must be resolved before it can be
archived. Running archival after resolution ensures newly-resolved triages are
evaluated in the same cycle.

**Assumptions:**
- The `release_definitions` table is populated before the regression cache
  loader runs. This is true: the `releasedefloader` runs as a separate loader
  in the daemon cycle.
- The raw SQL UPDATE bypasses GORM hooks (including the audit log hook on
  `Triage.AfterUpdate`). This is intentional for bulk operations, consistent
  with how `ResolveTriages` uses `Save` with a system user context. If audit
  logging is required for archival events, use the `Save` path with
  `models.CurrentUserKey` set to a system identifier like
  `"regression-tracker"`. **[needs TRT-3016 verification]**: confirm whether
  archival events need audit logging.

**Dependencies:** Phase 1 (column exists).

### Phase 3: Unarchive on Re-Open

**Affected files:**

- `pkg/dataloader/bugloader/bugloader.go` (unresolve branch in `reconcileTriages`)

**Changes:**

In the unresolve path of `reconcileTriages`, when a Jira-progression-resolved
triage is unresolved because the bug reopened, also clear the `Archived` flag:

```go
// Existing unresolve logic:
triage.Resolved = sql.NullTime{}
triage.ResolutionReason = ""
// Add:
triage.Archived = false
```

This ensures that a reopened triage returns to the active listing.

**Note:** User-resolved and regressions-rolled-off resolutions are never
automatically cleared, so their archival state persists. A user can manually
unarchive via the API (Phase 4) if needed.

**Dependencies:** Phase 1.

### Phase 4: API Changes

**Affected files:**

- `pkg/db/query/triage_queries.go` (`ListTriages`)
- `pkg/api/componentreadiness/triage.go` (`ListTriages`, `UpdateTriage`)
- `pkg/sippyserver/server.go` (`jsonGetTriages`, `jsonUpdateTriage`)

#### 4a. Listing Endpoint Filter

Modify `query.ListTriages` to accept an `includeArchived` parameter:

```go
func ListTriages(dbc *db.DB, view string, includeArchived bool) ([]models.Triage, error) {
    var triages []models.Triage
    q := dbc.DB.Preload("Bug").Preload("Regressions.JobRuns").
        Preload("Regressions.Views").Preload("Regressions")

    if !includeArchived {
        q = q.Where("triages.archived = false")
    }

    if view != "" {
        q = q.Distinct().
            Joins("JOIN triage_regressions ON triage_regressions.triage_id = triages.id").
            Joins("JOIN regression_views ON regression_views.test_regression_id = triage_regressions.test_regression_id").
            Where("regression_views.view_name = ? AND regression_views.active = true", view)
    }

    res := q.Find(&triages)
    if res.Error != nil {
        log.WithField("view", view).WithError(res.Error).Error("error listing triages")
    }
    return triages, res.Error
}
```

The API-layer `ListTriages` in `triage.go` parses `include_archived` from the
query string and passes it through.

The HTTP handler `jsonGetTriages` reads the query parameter:

```go
includeArchived := req.URL.Query().Get("include_archived") == "true"
```

**Default behavior:** `include_archived` defaults to `false`, so existing
clients see no archived triages without code changes.

#### 4b. Single Triage Lookup

`GetTriage` by ID does NOT filter by archival state. Archived triages remain
accessible by direct URL/ID for audit and reference purposes. The `archived`
field is included in the JSON response so clients can display the state.

#### 4c. Manual Archive/Unarchive via Update

The existing `UpdateTriage` endpoint already allows updating any Triage field.
Since `Archived` is a regular field on the model, it can be set via the
existing PUT endpoint without additional API changes. The `Archived` field
will be included in the JSON serialization and deserialization.

Validate that the audit log correctly captures archival changes (the existing
`compareTriageObjects` function in `triage.go` should be extended to compare
the `Archived` field).

#### 4d. HATEOAS Links

No new endpoints are introduced, so no new HATEOAS links are needed. The
existing `self`, `potential_matches`, and `audit_logs` links remain valid for
archived triages.

**Dependencies:** Phase 1.

### Phase 5: Frontend Changes

**Affected files:**

- `sippy-ng/src/component_readiness/TriageList.jsx`
- `sippy-ng/src/component_readiness/TriagedRegressions.jsx`
- `sippy-ng/src/component_readiness/CompReadyUtils.jsx` (if `getTriagesAPIUrl` needs updating)

#### 5a. Listing Page

Add an `include_archived` query parameter to the triage listing fetch. Default
to `false`. Add a toggle (MUI `Switch` or `FormControlLabel` with `Checkbox`)
to the page header that sets `include_archived=true` when checked.

#### 5b. DataGrid Bookmarks

Add new bookmarks to the `TriagedRegressions` component:

```javascript
{
  name: 'Archived',
  model: [
    {
      id: 1,
      field: 'archived',
      operator: 'equals',
      value: 'true',
    },
  ],
},
{
  name: 'Not Archived',
  model: [
    {
      id: 1,
      field: 'archived',
      operator: 'equals',
      value: 'false',
    },
  ],
},
```

These bookmarks only produce results when `include_archived=true` is set on
the API request (otherwise there are no archived triages in the response to
filter).

#### 5c. Visual Indicator

Add a visual indicator (e.g., `Chip` with "Archived" label, or an `Archive`
icon from MUI) on triage rows where `archived === true`. This provides
immediate context when viewing a mixed list with `include_archived=true`.

#### 5d. Triage Detail Page

On the triage detail page (`Triage.jsx`), display the archival status in the
metadata table. If archived, show a banner or chip indicating the triage is
archived, with an option to unarchive (via the existing update mechanism).

**Dependencies:** Phase 4 (API changes).

## 5. Testing

### Unit Tests

**File:** `pkg/api/componentreadiness/regressiontracker_test.go` (or new file)

No new unit tests for the SQL-based `ArchiveResolvedTriages` (it requires a
real database). Pure logic tests are not applicable here since the archival
logic is a single SQL statement.

**File:** `pkg/api/componentreadiness/triage_test.go`

Add a test case for `compareTriageObjects` that verifies the `Archived` field
change is detected:

| Test Case | Old Triage | New Triage | Expected Change |
|-----------|-----------|-----------|-----------------|
| archived field changed | `Archived: false` | `Archived: true` | `FieldChange{FieldName: "archived", Original: "false", Modified: "true"}` |

### Integration Tests

**File:** `test/integration/triage_archival_test.go` (new)

These tests run against a real PostgreSQL instance using testcontainers-go,
following the pattern in `test/integration/regression_forceclose_test.go`.

| Test Case | Setup | Assert |
|-----------|-------|--------|
| auto-archives resolved triage on GA release | Create triage with regressions on release "4.18", set release GA date, resolve triage, call `ArchiveResolvedTriages()` | Triage `Archived = true` |
| does not archive unresolved triage | Create triage with regressions on GA release, leave unresolved, call `ArchiveResolvedTriages()` | Triage `Archived = false` |
| does not archive triage on non-GA release | Create resolved triage with regressions on release without GA date | Triage `Archived = false` |
| does not archive multi-release triage with one non-GA release | Resolved triage with regressions on "4.18" (GA) and "4.19" (no GA), call `ArchiveResolvedTriages()` | Triage `Archived = false` |
| archives multi-release triage when all releases GA | Same as above, then set "4.19" GA date, call again | Triage `Archived = true` |
| does not archive triage with no regressions | Resolved triage with no regressions, call `ArchiveResolvedTriages()` | Triage `Archived = false` |
| idempotent | Call `ArchiveResolvedTriages()` twice | Same result, no errors |
| unarchive on Jira reopen | Archived triage, simulate Jira reopen (clear resolution), verify | Triage `Archived = false`, `Resolved` is NULL |

### E2E Tests

**File:** `test/e2e/componentreadiness/triage/triageapi_test.go` (extend existing)

| Test Case | Setup | Assert |
|-----------|-------|--------|
| default listing excludes archived triages | Create two triages, archive one, GET `/api/component_readiness/triages` | Response contains only the non-archived triage |
| include_archived=true shows all triages | Same setup, GET `?include_archived=true` | Response contains both triages |
| archived triage accessible by ID | GET `/api/component_readiness/triages/{id}` for archived triage | Returns 200 with `archived: true` |
| update can unarchive | PUT to archived triage with `archived: false` | Triage `Archived = false`, appears in default listing |

### Query Validation

**[Unexecuted]**: The archival eligibility query should be tested against
prod-like data to verify:

1. **Correctness**: Only triages meeting all criteria are archived. Pay
   attention to edge cases: triages with regressions on releases not present
   in `release_definitions`, triages with regressions on the special
   `Presubmits` release (which has no GA date).
2. **Performance**: Run `EXPLAIN ANALYZE` on the UPDATE query with prod-like
   data volume. Expected to use the index on `triages.archived` for the
   `WHERE archived = false` filter and the existing indexes on
   `test_regressions.release` and `release_definitions.release`.

Validation procedure:
```sql
-- Check how many triages would be archived (dry run)
SELECT COUNT(*)
FROM triages
WHERE resolved IS NOT NULL
  AND archived = false
  AND EXISTS (
      SELECT 1 FROM triage_regressions tr
      WHERE tr.triage_id = triages.id
  )
  AND NOT EXISTS (
      SELECT 1
      FROM triage_regressions tr
      JOIN test_regressions r ON tr.test_regression_id = r.id
      LEFT JOIN release_definitions rd ON rd.release = r.release
      WHERE tr.triage_id = triages.id
        AND (rd.ga_date IS NULL OR rd.release IS NULL)
  );

-- Examine the query plan
EXPLAIN ANALYZE <same query as UPDATE but with SELECT>;
```

## 6. Remaining Uncertainties

### Must verify against TRT-3016

| # | Uncertainty | Recommendation | Verification Step |
|---|-------------|----------------|-------------------|
| U1 | Exact acceptance criteria for archival | Implement the inferred criteria (resolved + all releases GA) | Read TRT-3016 and compare with R1-R7 above |
| U2 | Whether auto-archival should run in the daemon or be user-triggered only | Recommend daemon-driven auto-archive (consistent with auto-resolution) | Confirm with TRT-3016 |
| U3 | Whether audit logging is required for archival events | Recommend using GORM `Save` with system user context for audit trail | Confirm with TRT-3016 |
| U4 | Whether a one-time backfill of archival state is needed for existing triages | Recommend no backfill: let the daemon archive eligible triages on its next run | Confirm with TRT-3016 |
| U5 | Whether the `Presubmits` release (no GA date) should be excluded from the eligibility check | Recommend treating `Presubmits` regressions as blocking archival (conservative) | Confirm with TRT-3016 |

### Implementation-time verification

| # | Uncertainty | Recommendation | Verification Step |
|---|-------------|----------------|-------------------|
| U6 | Archival query performance on prod-like data | Expect efficient execution with existing indexes | Run `EXPLAIN ANALYZE` on prod-like database (see Query Validation above) |
| U7 | Whether `compareTriageObjects` needs explicit `Archived` handling | Yes, add the comparison for audit log accuracy | Verify during Phase 4 implementation |
| U8 | Impact on existing frontend bookmark filters | Minimal: "Resolved"/"Not Resolved" bookmarks remain functional, "Archived" bookmarks add new capability | Verify with manual testing during Phase 5 |
| U9 | Interaction with `GetRegressionPotentialMatchingTriages` | This function already skips triages resolved more than 6 weeks ago. Archived triages would also be resolved, so the existing filter handles them. No change needed. | Verify the 6-week cutoff is sufficient |
| U10 | Cross-release compatibility of the `release_definitions` join | The `test_regressions.release` field stores release strings like "4.18". The `release_definitions.release` column has a unique index on the same format. The LEFT JOIN handles regressions whose release string is not in `release_definitions` by treating them as non-GA (blocking archival), which is the conservative behavior. | Verify with a query against prod-like data showing all distinct `test_regressions.release` values and their `release_definitions` matches |

## Rejected Alternatives

### Time-based archival (archive N days after resolution)

A fixed time window (e.g., 90 days after resolution) is simpler but does not
align with the release lifecycle. A triage resolved on day 1 of a release
cycle would be archived long before the release goes GA, while the triage is
still relevant to developers working on that release.

### Soft-delete (deleted_at timestamp)

GORM's soft-delete mechanism (`gorm.DeletedAt`) would hide archived triages
from all queries automatically. However, this conflicts with the existing
hard-delete semantics (`DeleteTriage`) and would make archived triages
invisible to direct ID lookups without special handling. The explicit
`Archived` boolean is more transparent.

### View-scoped archival

Archiving per-view rather than per-triage would allow a triage to be archived
in one view but visible in another. This adds complexity without clear benefit
since archival is tied to the release GA date, not the view.
