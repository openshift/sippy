# Implementation Plan: Regression-View Junction Table

This plan covers the implementation of a `regression_views` junction table that formally associates regressions with the views that detected them.

**Updated 2026-04-24** to reflect completed implementation and remaining work for cross-compare view regression tracking.

## Status Summary

| Section | Status |
|---------|--------|
| 1. New Model and Schema | Done |
| 2. RegressionStore Interface | Done |
| 3. Unified Loader Changes | Done |
| 4. HATEOAS Link Generation | Done |
| 5. Preload Views on Queries | Done |
| 6. Triage Expansion Updates | Done |
| 7. Cleanup of Deprecated Functions | Done |
| 8. Migration and Backfill | Done |
| 9. Frontend Updates | Done |
| 10. Potential Matches API | Done |
| 11. Cross-Compare View Regression Tracking | Done |

## 1–8: Backend Implementation (Complete)

All backend changes from the original plan have been implemented:

- **`regression_views` table** with composite PK `(test_regression_id, view_name)` and `active` boolean, with `ON DELETE CASCADE` from `test_regressions`
- **`RegressionStore` interface** extended with `UpsertRegressionView` and `DeactivateRolledOffViews` (originally named `DeactivateStaleViews`, renamed for clarity)
- **`RegressionCacheLoader`** tracks `activeViewMap` per release, calls `UpsertRegressionView` after sync, calls `DeactivateRolledOffViews` after closing
- **HATEOAS links** on regression objects use composite keys `test_details:<viewName>` generated per active view from `InjectRegressionHATEOASLinks`
- **Preloads** added for `Views` / `Regressions.Views` on all relevant queries
- **`GetViewsForTriage`** reads from preloaded `regression.Views` instead of runtime release matching
- **`getRegressedTestsForRegressions`** collects view names from preloaded `reg.Views`
- **`FindViewByName`** exported for use by server.go
- **Schema migration** handled by GORM `AutoMigrate`; backfill happens naturally on first loader run

### Files changed (backend)

| File | Changes |
|------|---------|
| `pkg/db/models/triage.go` | Added `RegressionView` model, added `Views` field with `ON DELETE CASCADE` to `TestRegression` |
| `pkg/db/db.go` | Added `RegressionView` to auto-migrate list |
| `pkg/api/componentreadiness/regressiontracker.go` | Extended `RegressionStore` interface, implemented `UpsertRegressionView` and `DeactivateRolledOffViews` |
| `pkg/dataloader/regressioncacheloader/regressioncacheloader.go` | Track `activeViewMap`, call `UpsertRegressionView` after sync, call `DeactivateRolledOffViews` after closing |
| `pkg/api/componentreadiness/triage.go` | Rewrote `InjectRegressionHATEOASLinks` with per-view composite keys, updated `GetViewsForTriage`, exported `FindViewByName` |
| `pkg/db/query/triage_queries.go` | Added `.Preload("Views")` / `.Preload("Regressions.Views")` |
| `pkg/sippyserver/server.go` | Updated expanded triage handler and `getRegressedTestsForRegressions` to use preloaded views |
| `pkg/api/componentreadiness/middleware/linkinjector/linkinjector.go` | Kept plain `test_details` key (regressed tests in component reports are already scoped to a view) |

## 9. Frontend Updates (Complete)

### 9.1 `getTestDetailsLink` utility

**File:** `sippy-ng/src/component_readiness/CompReadyUtils.js`

Added a utility function that handles both link formats:

```js
export function getTestDetailsLink(links, viewName) {
  if (!links) return null
  if (links['test_details']) return links['test_details']
  if (viewName && links[`test_details:${viewName}`]) {
    return links[`test_details:${viewName}`]
  }
  const key = Object.keys(links).find((k) => k.startsWith('test_details:'))
  return key ? links[key] : null
}
```

Lookup order:
1. Plain `test_details` — preferred (used by regressed tests in component reports)
2. `test_details:<viewName>` — view-specific composite key (used by regression objects)
3. First `test_details:*` — fallback to any composite key

`generateTestDetailsReportLink` updated to accept an optional `viewName` parameter.

### 9.2 Per-file changes

| File | Object type | Strategy |
|------|------------|----------|
| `RegressionRedirect.js` | Regression | Prefers `-main` view key, falls back to `getTestDetailsLink` |
| `ComponentReadinessIndicator.js` | Regression | Uses <code>getTestDetailsLink(links, \`${release}-main\`)</code> |
| `TriagedRegressionTestList.js` | Regressed test (per-view columns) | Passes `viewName` from column loop to `generateTestDetailsReportLink` |

### 9.3 Two link formats

The backend produces two distinct link formats:

- **Regression objects** (`TestRegression`): Composite keys `test_details:<viewName>` — set by `InjectRegressionHATEOASLinks`, because a single regression can appear in multiple views.
- **Regressed tests** (`ReportTestSummary` from component reports): Plain `test_details` key — set by `LinkInjector` middleware, because these are already in the context of a single view.

The frontend `getTestDetailsLink` handles both formats transparently.

## 10. Potential Matches API (Complete)

### 10.1 Backend

**File:** `pkg/sippyserver/server.go`

`jsonTriagePotentialMatchingRegressions` now accepts `?view=<name>` instead of `?baseRelease=...&sampleRelease=...`. The view is resolved via `FindViewByName` to obtain the sample release for the regression query.

### 10.2 Frontend

**File:** `sippy-ng/src/component_readiness/TriagePotentialMatches.js`

- Replaced base/sample release dropdowns with a single **View** dropdown
- Available views extracted from `triage.regressions[].views[]` (active only), sorted with `-main` views first
- Fetch URL sends `?view=...`

## 11. Cross-Compare View Regression Tracking (Done)

Cross-compare views (e.g., `5.0-ha-vs-two-node-fencing`, `5.0-techpreview-rhcos9-vs-rhcos10`) are currently configured with `RegressionTracking.Enabled = false`. Enabling regression tracking on these views would encounter a regression identity collision problem.

### 11.1 The problem

`FindOpenRegression` matches regressions by `(release, testID, variants)`. Cross-compare views share the same sample release as the main view (e.g., both use `5.0`), so the same test with the same variants could be independently regressed in both:

- `5.0-main` (base=4.22): test X regressed because it was passing in 4.22 and failing in 5.0
- `5.0-ha-vs-two-node-fencing` (base=5.0/HA): test X regressed because it passes on HA but fails on two-node-fencing

These are different regressions with different root causes, but `FindOpenRegression` would match them as the same regression because `(release=5.0, testID, variants)` are identical. This causes:

1. **Regression collision**: Two conceptually distinct regressions get merged into one record
2. **`BaseRelease` overwrite**: Whichever view processes last overwrites the `BaseRelease` field, corrupting it for the other view

### 11.2 Rejected approach: match on base release

Adding `baseRelease` to the `FindOpenRegression` matching identity was considered but rejected because `BaseRelease` serves two conflicting purposes:

1. **Matching identity** — needs to be stable across runs
2. **Link generation** — `generateTestDetailsURLFromRegression` reads `regression.BaseRelease` to produce `testBasisRelease` URL params for release fallback

Release fallback makes `BaseRelease` unstable: when a test lacks sufficient data in the configured base release (e.g., 4.22), it falls back to a prior release (e.g., 4.21). This fallback can change between runs. If `BaseRelease` is part of the matching identity, a fallback shift creates duplicate regressions. Additionally, the `RegressionTracker` middleware only has access to the view's configured base release, not the actual fallback release, creating an inconsistency with `SyncRegressionsForReport`.

### 11.3 Solution: `CrossCompare` flag on `TestRegression`

Cross-compare regressions are fundamentally different from standard regressions — a cross-compare regression will never be shared with a standard view, and vice versa. A `CrossCompare bool` field on `TestRegression` partitions regressions into two non-overlapping pools. Cross-compare views can share regressions with each other. A view is cross-compare if `len(view.VariantOptions.VariantCrossCompare) > 0`.

GORM `AutoMigrate` adds the column with `DEFAULT false`, so existing regressions are automatically standard.

### 11.4 Changes

| File | Changes |
|------|---------|
| `pkg/db/models/triage.go` | Added `CrossCompare bool` field with `gorm:"not null;default:false"` to `TestRegression` |
| `pkg/api/componentreadiness/middleware/regressiontracker/regressiontracker.go` | Added `crossCompare bool` param to `FindOpenRegression` with `tr.CrossCompare != crossCompare` check. `PreAnalysis`/`PostAnalysis` pass `len(r.reqOptions.VariantOption.VariantCrossCompare) > 0`. |
| `pkg/api/componentreadiness/regressiontracker.go` | `SyncRegressionsForReport` passes `crossCompare` from view config. `OpenRegression` sets `CrossCompare` on new regressions. |
| `pkg/api/componentreadiness/middleware/regressiontracker/regressiontracker_test.go` | Updated all `FindOpenRegression` calls with `crossCompare` param. Added `TestFindOpenRegression_CrossCompareIsolation` (2 subtests) and "no match when cross-compare flag differs" case. |

### 11.5 Enable regression tracking on cross-compare views

All 6 cross-compare views already have `regression_tracking.enabled: true` in `config/views.yaml`. No config changes needed.

## 12. Files Changed Summary (All Sections)

| File | Changes |
|------|---------|
| `pkg/db/models/triage.go` | `RegressionView` model, `Views` field with CASCADE on `TestRegression`, `CrossCompare` field |
| `pkg/db/db.go` | `RegressionView` in auto-migrate list |
| `pkg/api/componentreadiness/regressiontracker.go` | `RegressionStore` interface: `UpsertRegressionView`, `DeactivateRolledOffViews`. `OpenRegression` sets `CrossCompare`. `SyncRegressionsForReport` passes `crossCompare` to `FindOpenRegression`. |
| `pkg/dataloader/regressioncacheloader/regressioncacheloader.go` | `activeViewMap` tracking, view upsert after sync, deactivation after closing |
| `pkg/api/componentreadiness/triage.go` | Per-view HATEOAS links, `GetViewsForTriage` from preloaded views, `FindViewByName` exported |
| `pkg/api/componentreadiness/middleware/regressiontracker/regressiontracker.go` | `crossCompare` param on `FindOpenRegression`, `PreAnalysis`/`PostAnalysis` pass cross-compare flag from request options |
| `pkg/api/componentreadiness/middleware/linkinjector/linkinjector.go` | Plain `test_details` key preserved for component report context |
| `pkg/db/query/triage_queries.go` | `.Preload("Views")` / `.Preload("Regressions.Views")` |
| `pkg/sippyserver/server.go` | Expanded triage uses preloaded views, potential matches uses `?view=` param |
| `sippy-ng/src/component_readiness/CompReadyUtils.js` | `getTestDetailsLink` utility, `viewName` param on `generateTestDetailsReportLink` |
| `sippy-ng/src/component_readiness/RegressionRedirect.js` | Uses `getTestDetailsLink`, prefers `-main` view |
| `sippy-ng/src/component_readiness/ComponentReadinessIndicator.js` | Uses `getTestDetailsLink` with `${release}-main` |
| `sippy-ng/src/component_readiness/TriagedRegressionTestList.js` | Passes `viewName` to `generateTestDetailsReportLink` |
| `sippy-ng/src/component_readiness/TriagePotentialMatches.js` | View dropdown replaces base/sample release dropdowns |

### Tests

| File | Changes |
|------|---------|
| `pkg/api/componentreadiness/triage_test.go` | `TestInjectRegressionHATEOASLinks` (7 cases), `TestFindViewByName` (3 cases), `TestGetViewsForTriage` (6 cases) |
| `.../regressiontracker/regressiontracker_test.go` | `TestFindOpenRegression_CrossCompareIsolation` (2 subtests), cross-compare flag mismatch case, all calls updated with `crossCompare` param |
| `test/e2e/.../regressiontracker_test.go` | `Test_RegressionViews` (10 subtests), CASCADE cleanup, `Test_CrossCompareIsolation` (5 subtests: OpenRegression sets CrossCompare, SyncRegressionsForReport creates isolated regressions, standard/cross-compare sync don't cross-match, re-sync reuses within same pool) |
| `test/e2e/.../triageapi_test.go` | Composite HATEOAS link assertions, `UpsertRegressionView` calls, `?view=` param for potential matches, updated "error when view does not exist" test to use `?view=` instead of old `baseRelease`/`sampleRelease` params |
