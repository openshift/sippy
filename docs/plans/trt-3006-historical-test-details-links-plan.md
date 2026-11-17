# Implementation Plan: Historical Test-Details Links for Closed Regressions

- **Original issue:** [TRT-3006](https://redhat.atlassian.net/browse/TRT-3006) (Sippy: restore historical test-details links for closed regressions)
- **Planning subtask:** [TRT-3073](https://redhat.atlassian.net/browse/TRT-3073)
- **Repository:** openshift/sippy

## Scope and Desired Outcome

When a regression closes and rolls off the current Component Readiness report, its `test_details:<view>` links disappear because `InjectRegressionHATEOASLinks` skips inactive view associations. Developers inspecting a resolved regression from a triage detail page cannot navigate to the test results that led to the regression without manually constructing a historical query.

This plan covers generating historical test-details links for closed regressions, anchoring the sample window to the regression's last-failure date, resolving the basis from the view's configuration, displaying a distinct icon in the triage table, showing authoritative closure context on the test-details page, and fixing `CompReadyVars.jsx` so explicit URL parameters take precedence over view defaults.

TRT-3002 (Test Analysis navigation and run-history actions) is not in scope.

## Current Behavior and Repository Findings

### Backend: Link generation

**`pkg/api/componentreadiness/triage.go:812-835`** (`InjectRegressionHATEOASLinks`):
Iterates `regression.Views`, skips any view where `rv.Active` is false (line 821), and calls `generateTestDetailsURLFromRegression` only for active views. Closed regressions with all views inactive get no `test_details:*` links.

**`pkg/api/componentreadiness/triage.go:848-879`** (`generateTestDetailsURLFromRegression`):
Calls `utils.GetViewReleaseOptions` which delegates to `GetViewReleaseOptionsAt(time.Now(), ...)`. The `now` anchor means sample dates like `now-7d` through `now` always resolve to the current time, not the time the regression was active.

**`pkg/api/componentreadiness/utils/queryparamparser.go:317-346`** (`GetViewReleaseOptions`, `GetViewReleaseOptionsAt`):
Already accepts a `now` parameter for clock pinning. The `At` variant resolves `now`-relative and `ga`-relative expressions against the given reference time. This is the reuse point for historical date resolution.

**`pkg/util/utils.go:135`** (`ParseCRReleaseTimeAt`):
The relative time parser. Supports `now`, `now-7d`, `ga`, `ga-30d`, `end-90d`. The `now` anchor is received as a parameter. GA-based expressions resolve from the release's GA date and are not affected by the reference time.

### Backend: Triage expansion

**`pkg/sippyserver/server.go:1920-1985`** (`jsonGetTriageByID`):
When `expand=regressions` is requested, calls `GetViewsForTriage` (which returns only active view names), then fetches a live component report for each view. This means closed regressions have no views returned and no regressed_tests data.

**`pkg/api/componentreadiness/triage.go:882-896`** (`GetViewsForTriage`):
Filters to `rv.Active == true` only. This function drives which views appear in the expanded triage response.

### Models

**`pkg/db/models/triage.go:212-260`** (`TestRegression`):
Has `LastFailure sql.NullTime`, `Closed sql.NullTime`, `Variants pq.StringArray`, `Views []RegressionView`, `Links map[string]string` (gorm:"-"). The `RegressionView` struct has `Active bool`, `ClosedAt sql.NullTime`.

### Frontend: Triage table

**`sippy-ng/src/component_readiness/TriagedRegressionTestList.jsx`**:
View columns are built from `props.allRegressedTests` (line 102), which is populated from the expanded triage response's `regressed_tests` map. When a view has no regressed tests (because all its regressions rolled off), the column does not appear. The `CompSeverityIcon` and link are rendered per cell based on the regressed test's status and links.

### Frontend: CompReadyVars

**`sippy-ng/src/component_readiness/CompReadyVars.jsx:419-476`** (`updateVarsFromView`):
Unconditionally overwrites date controls (lines 425-430) with the view's resolved dates. When a historical URL contains explicit `sampleStartTime`/`sampleEndTime` parameters, they are first read from the URL (lines 284-304) but then immediately overwritten when `updateVarsFromView` runs (line 351). This means explicit historical dates in a shared URL are lost.

### Frontend: TestDetailsReport

**`sippy-ng/src/component_readiness/TestDetailsReport.jsx:119,197-201`**:
Reads `regressionId` from the fetched test-details response's `analyses[0].regression.id`. Does not currently accept `regressionId` as a URL parameter or fetch the regression endpoint independently.

## Proposed Approach

### Design Overview

1. **Backend link generation**: Extend `InjectRegressionHATEOASLinks` to process inactive views for closed regressions, generating historical links with concrete dates anchored to the regression's `LastFailure`. Use the existing `GetViewReleaseOptionsAt` function with a pinned reference time derived from `LastFailure`, not `time.Now()`.

2. **Backend triage enrichment**: Extend triage expansion to include links from regressions (via the already-generated HATEOAS links on each regression), independent of whether a live component report is available. This avoids running expensive historical reports.

3. **Frontend table**: Build view columns from both `regressed_tests` (current, active) and regression links (historical, inactive). Show a distinct icon for historical links.

4. **Frontend URL precedence**: Fix `updateVarsFromView` to not overwrite explicitly provided URL parameters, so historical links with concrete dates survive page load.

5. **Frontend test-details closure context**: Accept `regressionId` as a URL parameter. When present, fetch the regression endpoint to obtain authoritative closure metadata and display a "historical report" banner.

### Key Design Decisions

- **No historical report execution**: Historical links are generated from persisted regression metadata and view configuration. The backend does not run a historical component report to populate regressed_tests for closed regressions. This keeps the triage expansion fast.
- **Per-view-association closure**: Whether a link is historical depends on `RegressionView.Active`, not on the global `TestRegression.Closed` field. A regression open in view A but closed in view B gets a current link for A and a historical link for B.
- **Sample window**: 7 calendar days ending on the last-failure date, using whole UTC days. `sampleStartTime = lastFailure.Date - 6 days at 00:00 UTC`, `sampleEndTime = lastFailure.Date + 1 day at 00:00 UTC` (exclusive). This ensures the last-failure day is fully included.
- **Basis window**: Resolved from the view's `BaseRelease` configuration using `GetViewReleaseOptionsAt` with the last-failure date as the reference time. For GA-based expressions (e.g., `ga-30d` through `ga`), the dates are fixed by the GA date and do not change. For `now`-relative expressions, they resolve against the last-failure date.
- **Link key**: Historical links use the same `test_details:<view_name>` key as current links. The URL itself contains concrete dates, so there is no ambiguity. A new `historical=true` query parameter distinguishes the link type for the frontend.

## Implementation Steps

### Step 1: Generate historical HATEOAS links (Backend)

**Files:** `pkg/api/componentreadiness/triage.go`, new helper in same file or `utils/`

1. In `InjectRegressionHATEOASLinks`, remove the `if !rv.Active { continue }` gate. Instead, branch on `rv.Active`:
   - **Active**: Call existing `generateTestDetailsURLFromRegression` (current behavior).
   - **Inactive**: Call a new `generateHistoricalTestDetailsURLFromRegression` that uses `GetViewReleaseOptionsAt(lastFailureDate, ...)` for the sample window and basis resolution.

2. Implement `generateHistoricalTestDetailsURLFromRegression`:
   - Validate that `regression.LastFailure` is set and valid. If not, skip the link (handled in step 6).
   - Compute the historical sample window:
     - `lastFailureDate`: truncate `LastFailure.Time` to UTC midnight.
     - `sampleStart`: `lastFailureDate.AddDate(0, 0, -6)` at 00:00 UTC (7 calendar days inclusive).
     - `sampleEnd`: `lastFailureDate.AddDate(0, 0, 1)` at 00:00 UTC (exclusive end).
   - Resolve the basis using `GetViewReleaseOptionsAt(regression.LastFailure.Time, releases, "basis", view.BaseRelease, 0, 0)`.
   - Construct the sample `reqopts.Release` with concrete dates (not calling `GetViewReleaseOptions` which uses `time.Now()`).
   - Call `utils.GenerateTestDetailsURL(...)` with all the same parameters as the current-link path.
   - Append `&regressionId=<id>&historical=true` to the generated URL.

3. Use the same link key `test_details:<view_name>`. The URL's concrete dates and `historical=true` parameter distinguish it.

**Tests:** Add table-driven test cases in `triage_test.go` to `TestInjectRegressionHATEOASLinks`:
- Inactive view with valid `LastFailure` produces a historical link with correct sample/basis dates.
- Inactive view without `LastFailure` produces no link.
- Mixed active and inactive views produce correct links for each.
- `now`-relative basis expressions resolve against `LastFailure`, not current time.
- GA-based basis expressions are unchanged.
- Regression with `BaseRelease` override preserves the override in the historical URL.

### Step 2: Enrich triage responses with regression links (Backend)

**Files:** `pkg/api/componentreadiness/triage.go` (`GetTriage`, `injectHATEOASLinks`), `pkg/sippyserver/server.go` (`jsonGetTriageByID`)

1. In `GetTriage`, after loading the triage with `Preload("Regressions.Views")`, call `InjectRegressionHATEOASLinks` for each regression. This requires passing `views`, `releases`, rounding factors, and the base URLs. These are available from the server config.

   Option A: Pass views/releases into `GetTriage` (changes its signature).
   Option B: Extract a `EnrichTriageRegressionLinks(triage, views, releases, ...)` function called from the server handler after `GetTriage`.

   **Recommendation**: Option B. Keep `GetTriage` focused on DB access and have the handler (`jsonGetTriageByID`) call both `GetTriage` and an enrichment function. This matches the existing pattern where `InjectRegressionHATEOASLinks` is called by the handler layer (`ListRegressions`, `GetRegression`).

2. In `jsonGetTriageByID`, after fetching the triage, iterate `triage.Regressions` and call `InjectRegressionHATEOASLinks` on each. This provides both active and historical links on every regression embedded in the triage response.

3. In the expanded-regressions path of `jsonGetTriageByID`, the existing code already adds `regressed_tests` for active views. Historical regressions will not appear in `regressed_tests` (no live report is generated), but their links will be present on the regression objects themselves. The frontend reads links from both sources.

4. Apply the same enrichment in `ListTriages` so the list endpoint also includes regression links. The `injectHATEOASLinks` function (for the triage itself) already runs; now also enrich each regression.

**Tests:** E2e test with triage 792 (all regressions closed) should return regressions with `test_details:5.0-main` historical links even though `regressed_tests` is empty.

### Step 3: Frontend, build view columns from regression links (Frontend)

**Files:** `sippy-ng/src/component_readiness/TriagedRegressionTestList.jsx`

1. Currently, view columns are derived from `props.allRegressedTests` keys (line 102-110). Extend this to also scan `props.regressions` for `links` entries matching `test_details:<viewName>`.

2. Build a combined set of view names from:
   - `Object.keys(regressedTestsByView)` (current, from expanded regressed_tests).
   - For each regression in `triagedRegressions`, extract view names from links keys matching `test_details:*`.

3. In the cell renderer (`valueGetter`), when no regressed test is found in `regressedTestsByView` for this regression, check the regression's own `links` for a `test_details:<viewName>` entry:
   - If found and the link contains `historical=true`, render a historical icon.
   - If found without `historical=true`, render the normal severity icon.
   - If not found, render null (no data).

4. For the historical icon, use a Material UI `History` icon (from `@mui/icons-material`) with a green `CheckCircle` badge overlaid using MUI's `Badge` component:
   - Tooltip: "Closed regression. View historical test details for {start} to {end}." Parse dates from the link URL parameters.
   - Provide `aria-label` for accessibility: "Historical test details for closed regression".

5. When only the view association is closed but the regression is still open in other views, adjust the tooltip: "Closed in this view. View historical test details for {start} to {end}."

**Tests:** Playwright BDD scenario in `sippy-ng/bdd/features/triage-detail.feature`:
- Scenario: Triage with closed regression shows historical icon in view column.
- Scenario: Triage with mixed active/closed regressions shows both icon types.

### Step 4: Fix URL parameter precedence in CompReadyVars (Frontend)

**Files:** `sippy-ng/src/component_readiness/CompReadyVars.jsx`

1. In `updateVarsFromView` (line 419), add a check for each date parameter: only set the view's value if the corresponding URL parameter is not explicitly present.

2. The URL parameters are already parsed into `params` state. Check if the specific parameter was explicitly provided in the URL (not just defaulted). The `useQueryParams` hook returns `undefined` for parameters not present in the URL.

3. Guard each `set*` call in `updateVarsFromView`:
   ```javascript
   if (params.baseStartTime === undefined)
     setBaseStartTime(formatLongDate(view.base_release.start, dateFormat))
   if (params.baseEndTime === undefined)
     setBaseEndTime(formatLongDate(view.base_release.end, dateFormat))
   // ... same for sample dates
   ```

4. This ensures that when a historical link with explicit `sampleStartTime`, `sampleEndTime`, `baseStartTime`, `baseEndTime` is opened, those values survive the view loading process.

5. Verify that:
   - The sidebar controls display the historical dates, not the view's current defaults.
   - The displayed report uses the historical dates.
   - Refreshing the page preserves the historical dates.
   - Copying the page URL preserves the historical dates.
   - The generated API call uses the historical dates.

**Tests:** Vitest test for a pure helper (if logic is extracted) or Playwright BDD scenario:
- Scenario: Opening a URL with explicit date params and a view does not overwrite the dates.

### Step 5: Display closure context on test-details page (Frontend)

**Files:** `sippy-ng/src/component_readiness/TestDetailsReport.jsx`, `sippy-ng/src/component_readiness/CompReadyUtils.jsx`

1. Add `regressionId` as a URL query parameter (via `useQueryParams` in CompReadyVars or locally in TestDetailsReport).

2. When `regressionId` is present in the URL (from a historical link), fetch `/api/component_readiness/regressions/<id>` alongside the statistical report.

3. Before displaying closure context, validate that the fetched regression matches:
   - Same `test_id` as the requested `testId`.
   - Same `release` as the requested `sampleRelease`.
   - Variants overlap with the requested variants.

4. If the regression is closed and matches, display a prominent `Alert` banner:
   - Severity: `info`
   - Content: "Closed regression (ID: {id}). Historical report for {sampleStart} to {sampleEnd}."
   - Include: closure date, last failure date, sample/basis windows.
   - Use the `History` icon in the alert.

5. Preserve the report's statistical assessment even if it shows red. The banner clarifies it is historical, not an active regression.

6. Keep the closure metadata fetch independent of the expensive statistical report cache. Use a separate `useEffect` for the regression fetch so the banner can appear even if the report is still loading.

7. Add the `historical` URL parameter to the generated link's `regressionId` parameter in `GenerateTestDetailsURL` (backend, step 1).

**Tests:** Playwright BDD scenario:
- Scenario: Historical test-details page shows closure banner with regression metadata.

### Step 6: Handle unavailable history (Backend + Frontend)

**Backend** (`pkg/api/componentreadiness/triage.go`):

1. When `LastFailure` is not set (`!regression.LastFailure.Valid`), skip historical link generation for that regression. Log a warning.

2. When the view referenced by `rv.ViewName` is not found in the current view configuration (view removed), skip the link. Log a warning. This is already handled by the `FindViewByName` check on line 823.

3. When `GetViewReleaseOptionsAt` fails (e.g., release not found, no GA date for GA-based basis), skip the link and log the error (same as current behavior for active links, line 829-832).

**Frontend** (`sippy-ng/src/component_readiness/TestDetailsReport.jsx`):

1. If the regression fetch (from step 5) returns a regression that does not match the requested test/release/variants, do not display closure context. Show an informational note: "Regression data unavailable or does not match this report."

2. If the regression fetch fails (404, network error), do not block the report. Show the statistical report without closure context, with a note: "Unable to load regression metadata."

3. If a historical link's dates fall outside available data (BigQuery/PostgreSQL retention), the report itself will show no data. The existing "No data found" handling applies. The closure banner from step 5 still provides useful context (the regression's metadata).

**Tests:**
- Backend unit test: regression without `LastFailure` gets no historical link.
- Backend unit test: regression with a removed view gets no link for that view.

### Step 7: Documentation updates

**Files:** `pkg/api/README.md`, `docs/features/` (new file or update if applicable)

1. Document the `historical=true` and `regressionId` query parameters added to test-details URLs.

2. Document the historical link semantics: sample window anchored to last-failure, basis resolved from view config at the reference time.

3. Document the closure indicator icon and tooltip in the triage detail view.

4. Document data-availability limitations (retention, removed views).

## Validation

### Automated Tests

**Backend (Go, table-driven):**
- `TestInjectRegressionHATEOASLinks`: Extended with cases for inactive views, missing `LastFailure`, mixed active/inactive, `now`-relative vs GA-based basis, basis-release overrides.
- `TestGenerateHistoricalTestDetailsURL`: New test for the historical URL generation function, covering date computation, basis resolution, edge cases (last failure at midnight, last failure today, force-closed regressions).
- `TestGetViewsForTriage` does not need changes (it correctly returns active-only for the purpose of live report expansion).

**Frontend (Playwright BDD):**
- Triage 792: historical icon visible in 5.0-main column, clicking navigates to historical test-details.
- Triage 811: mixed active (open regression 50084) and historical (closed regression 50913) icons.
- Historical test-details page: closure banner displayed, correct dates in sidebar, refresh preserves dates, copied URL preserves dates.
- URL parameter precedence: opening a link with explicit dates + view does not overwrite dates.

**Frontend (Vitest):**
- If a pure date-computation helper is extracted, unit test the sample window calculation.

### Manual Verification

- Load triage 792 in the UI: 5.0-main column should appear with a historical icon (clock+check).
- Click the historical icon: should navigate to test-details with September 22-29, 2026 sample dates.
- Verify the test-details page shows a "Closed regression, historical report" banner.
- Refresh the page: dates should not change to current defaults.
- Copy the URL and open in a new tab: same behavior.
- Load triage 811: open regression 50084 should show normal severity icon; closed regression 50913 should show historical icon.
- Test with a regression that has no `last_failure`: no historical link should appear, but the regression row is still visible.

### Existing CI Checks

- `make test` (Go tests, Vitest, BDD)
- `make lint` (Go lint, eslint, prettier)
- `make e2e` (end-to-end API tests)

## Risks and Compatibility

1. **Link enrichment performance**: Adding `InjectRegressionHATEOASLinks` calls to triage responses means loading views and releases for each regression. For triages with many regressions, this could add latency. Mitigation: views and releases are already loaded at server start and cached; the enrichment is pure URL construction, not DB queries.

2. **URL parameter collision**: Adding `historical` and `regressionId` params to test-details URLs. These parameters do not conflict with existing ones. The backend test-details endpoint ignores unknown parameters today, so the `historical` parameter is purely a frontend signal.

3. **View configuration drift**: Historical links use the view's current configuration (variant options, advanced options) at link-generation time, not the configuration that was active when the regression was originally detected. If view configuration changes significantly between regression closure and link generation, the historical report may differ from the original. This is documented as a limitation (the plan reconstructs a historical query, not a stored snapshot).

4. **Data retention**: BigQuery and PostgreSQL may not retain data for the historical window, especially for old regressions. The report will show "no data" in this case. The closure banner still provides useful metadata.

5. **Backward compatibility**: The API response for regressions gains additional links for inactive views. Existing clients that only check `test_details:<viewName>` will see more keys but should not break. The `historical=true` parameter is new and informational.

## Open Questions

1. **Cache interaction**: The test-details API uses a caching layer. Historical queries with concrete dates should cache well (the dates don't move), but the cache key must include the full date parameters. This likely works already since dates are part of the query parameters, but should be verified during implementation.

2. **Cross-compare regressions**: Regressions with `CrossCompare: true` may need special handling in historical link generation if the view's `VariantCrossCompare` configuration is involved. The current link generation (`generateTestDetailsURLFromRegression`) already handles this via `view.VariantOptions`. Verify during implementation that cross-compare historical links work correctly.

3. **Force-closed regressions**: Force-closed regressions (`ForceClosed: true`) should behave the same as naturally closed regressions for historical link purposes. The `LastFailure` timestamp is the anchor regardless of how the regression was closed. Confirm during implementation.
