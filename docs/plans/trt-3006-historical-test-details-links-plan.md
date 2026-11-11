# TRT-3006: Historical test-details links for closed regressions

## Scope and source issues

- **Original issue:** [TRT-3006](https://redhat.atlassian.net/browse/TRT-3006) (restore historical test-details links for closed regressions)
- **Planning subtask:** [TRT-3073](https://redhat.atlassian.net/browse/TRT-3073) (plan implementation of TRT-3006)
- **Repository:** openshift/sippy

### Desired outcome

When a regression closes and rolls off the current Component Readiness report, the triage details
page loses its test-details navigation links. Developers cannot inspect the test results that led
to a resolved regression without manually reconstructing a historical query. This work generates
historical test-details links for closed regressions by anchoring the sample window to the
regression's persisted `last_failure` date and resolving the basis from the associated view's
configuration. The links are surfaced in both the API (HATEOAS) and the frontend (distinct
historical icon in triage tables, closure context on the test-details page).

### Boundaries

- This card covers Component Readiness test-details navigation only.
- [TRT-3002](https://redhat.atlassian.net/browse/TRT-3002) remains responsible for Test Analysis
  navigation and its run-history actions. This work does not change that card.
- No schema migration or new storage of historical reports is expected.
- The feature reconstructs a historical query using current view configuration, not a stored
  snapshot of the original report.

## Current behavior and repository findings

### Backend link generation

`InjectRegressionHATEOASLinks` in `pkg/api/componentreadiness/triage.go:814-835` iterates
`regression.Views` and skips any view association where `rv.Active` is `false` (line 821). This
means closed regressions (whose view associations are all inactive) produce zero `test_details:`
links.

`generateTestDetailsURLFromRegression` in `triage.go:848-879` resolves dates by calling
`utils.GetViewReleaseOptions`, which delegates to `GetViewReleaseOptionsAt(time.Now(), ...)` in
`queryparamparser.go:317-323`. Relative expressions like `now-7d` resolve against the current
time, not the regression's historical context.

### Triage expansion (server)

`jsonGetTriageByID` in `pkg/sippyserver/server.go:1920-1984` builds the `ExpandedTriage` response.
It calls `GetViewsForTriage` (triage.go:882-896), which only collects active view names. The
expanded-regressions path then fetches a live component report for each active view. When all
view associations are inactive (as with triage 792), `viewNames` is empty and `RegressedTests`
remains `{}`.

### RegressionView model

`RegressionView` in `pkg/db/models/triage.go:264-270` stores:
- `Active bool` (whether regression currently appears in the view's report)
- `ClosedAt sql.NullTime` (when the view association became inactive)
- `OpenedAt time.Time`
- `ViewName string`

The `TestRegression` model (triage.go:212-260) stores:
- `LastFailure sql.NullTime` (the last failure in the sample while regression was open)
- `Closed sql.NullTime` (when the regression globally closed)
- `Views []RegressionView`
- `Variants pq.StringArray`
- Full test identity: `TestID`, `TestName`, `Component`, `Capability`, `Release`, `BaseRelease`

### View configuration and date resolution

Views are defined in `config/openshift.yaml` and loaded as `crview.View` structs
(`pkg/apis/api/componentreport/crview/types.go`). Each view has `BaseRelease` and `SampleRelease`
of type `reqopts.RelativeRelease`, which carry `RelativeStart` and `RelativeEnd` expressions
(e.g., `now-7d`, `ga-30d`, `ga`).

`GetViewReleaseOptionsAt` in `queryparamparser.go:328-346` resolves these expressions against a
reference time using `util.ParseCRReleaseTimeAt` in `pkg/util/utils.go:135`. The `now` anchor is
the first positional parameter. GA-based expressions resolve against the release's stored GA date
and are independent of the reference time.

The existing `GetViewReleaseOptionsAt` function already accepts an arbitrary `now` parameter. This
is the key enabler for historical date resolution: passing `lastFailure` as the `now` argument
will resolve `now-7d` and similar expressions relative to the regression's historical context.

### Frontend table rendering

`TriagedRegressionTestList.jsx` builds view columns from `props.allRegressedTests`, which is
keyed by view name. When `regressedTests` is `{}` (empty due to all-inactive views), no view
columns appear and no navigation links are rendered for any regression.

`generateTestDetailsReportLink` in `CompReadyUtils.jsx:856-862` extracts links from
`test.links` via `getTestDetailsLink`, which looks for `test_details:<viewName>` keys.

### TestDetailsReport.jsx

The test-details page (`sippy-ng/src/component_readiness/TestDetailsReport.jsx`) fetches the
statistical report from the API and displays regression context (ID, triages, assessment). It has
a `regressionId` state variable populated from the API response's `regression.id`. It does not
currently fetch the regression record independently or display closure metadata.

### CompReadyVars.jsx date precedence issue

`CompReadyVars.jsx:419-476` (`updateVarsFromView`) runs whenever `params.view` is set (line 420).
It unconditionally overwrites date state (lines 425-430) with the view's current resolved values.
This means explicit historical dates in the URL (e.g., `sampleStartTime=2026-09-22T00:00:00Z`)
will be replaced by the view's live `now-7d` range when the view parameter is also present.

## Proposed approach

### Design overview

Generate historical `test_details:<viewName>` links for regressions with inactive view
associations by resolving the view's date expressions against the regression's `last_failure`
timestamp. Serve these links alongside current links for active views. Enrich triage API
responses with both current regressed test data and historical links from the regression records
themselves. On the frontend, display historical links with a distinct clock/check icon and show
authoritative closure context on the test-details page.

### Key design decisions

1. **Reuse `GetViewReleaseOptionsAt` with `lastFailure` as reference time.** This avoids
   duplicating date-resolution logic. GA-based and absolute dates naturally retain their
   configured meanings because `ParseCRReleaseTimeAt` only uses the `now` parameter for
   `now-`prefixed expressions.

2. **Historical links are generated from metadata, not from running a report.** The link
   contains concrete serialized dates, test identity, and view configuration. No historical
   report execution is needed at link-generation time. This follows the HATEOAS principle:
   the link is a pointer the client can follow, not a precomputed result.

3. **Per-view-association behavior, not per-regression or per-triage.** A regression open in
   view A but closed in view B gets a current link for A and a historical link for B. The
   triage's resolved flag does not determine link type. This handles the mixed case (triage 811).

4. **Frontend table columns derived from union of current reports and regression view metadata.**
   This ensures columns appear even when no current report data exists.

5. **`updateVarsFromView` must respect explicit URL parameters.** When historical dates and a
   view name both appear in the URL, the explicit dates take precedence. This ensures the
   historical link's concrete dates survive navigation.

## Implementation steps

### Step 1: Generate historical HATEOAS links for inactive view associations

**Files:**
- `pkg/api/componentreadiness/triage.go` (modify `InjectRegressionHATEOASLinks`,
  add `generateHistoricalTestDetailsURLFromRegression`)

**Changes:**

1. In `InjectRegressionHATEOASLinks`, remove the `if !rv.Active { continue }` guard (line 821).
   Instead, branch on `rv.Active`:
   - Active: call existing `generateTestDetailsURLFromRegression` (current behavior).
   - Inactive: call new `generateHistoricalTestDetailsURLFromRegression`.

2. Create `generateHistoricalTestDetailsURLFromRegression`:
   - Validate that `regression.LastFailure` is valid. If not, log a warning and skip (no link).
   - Compute the historical sample window:
     - `sampleStart`: 00:00 UTC, 7 calendar days before the `last_failure` date
       (truncate `last_failure` to date, subtract 7 days).
     - `sampleEnd`: 00:00 UTC the day after the `last_failure` date (exclusive end,
       matching `>= start AND < end` semantics).
     - For regression 49971 (`last_failure` 2026-09-29T01:33:26Z):
       sampleStart = 2026-09-22T00:00:00Z, sampleEnd = 2026-09-30T00:00:00Z.
   - Resolve the basis using `GetViewReleaseOptionsAt` with the `last_failure` timestamp
     as the `now` parameter. GA-based expressions (`ga-30d`, `ga`) resolve against the GA
     date and are unaffected. Rolling expressions (`now-60d`, `now-30d`) resolve relative
     to `last_failure`.
   - Construct sample `reqopts.Release` with explicit `Start`/`End` times (no relative
     expressions in the URL).
   - Call `utils.GenerateTestDetailsURL` with the concrete dates, the regression's full
     identity (TestID, Component, Capability, Variants, BaseRelease), and the view's
     configuration (variant options, advanced options, test filters).
   - Add `regressionId=<id>` as an additional query parameter in the generated URL.

3. Use the link key `test_details:<viewName>` for both active and historical links (same key
   namespace). The frontend determines active vs. historical from the regression's view
   association metadata, not from the link key.

**Dependencies:** None.

### Step 2: Extend `GenerateTestDetailsURL` to support `regressionId`

**Files:**
- `pkg/api/componentreadiness/utils/utils.go` (modify `GenerateTestDetailsURL`)

**Changes:**

Add an optional `regressionID` parameter (type `uint`, 0 means omit) to `GenerateTestDetailsURL`.
When nonzero, add `regressionId=<id>` to the generated URL's query parameters. Update all existing
callers to pass `0` for the new parameter.

Alternatively, append the parameter after calling `GenerateTestDetailsURL` in the caller. The
simpler approach is to add it after the URL is built in
`generateHistoricalTestDetailsURLFromRegression`, since only historical links need it. This avoids
changing the signature. Prefer this approach.

**Dependencies:** Step 1 (or can be done in parallel if the URL modification happens in the caller).

### Step 3: Enrich triage API responses with historical links

**Files:**
- `pkg/api/componentreadiness/triage.go` (modify `GetTriage`, `injectHATEOASLinks`)
- `pkg/sippyserver/server.go` (modify `jsonGetTriageByID`)

**Changes:**

1. **`GetTriage` / `injectHATEOASLinks`:** The triage's embedded regressions already have their
   `Views` preloaded (line 32). After calling `injectHATEOASLinks` for the triage, also call
   `InjectRegressionHATEOASLinks` for each regression in the triage. This requires passing
   `views`, `releases`, and rounding parameters through. Modify `GetTriage` to accept these
   additional parameters (or use a struct/options pattern).

2. **`ListTriages`:** Similarly enrich regressions in listed triages. Ensure the preload includes
   `Regressions.Views`.

3. **`jsonGetTriageByID` expansion path:** The expanded-regressions path currently fetches live
   component reports only for active views. Extend it to also include historical links:
   - Collect all view names from regressions (both active and inactive).
   - For active views: continue fetching the live component report (current behavior).
   - For inactive views: do not fetch a report, but include the regression's historical
     `test_details` link (already generated in step 1) in the response.
   - This means the `RegressedTests` map may contain entries from historical links even when
     no current report data exists for that view.

   The simplest approach: after building `RegressedTests` from live reports, iterate each
   triage regression and copy its HATEOAS links into the response. The frontend already reads
   links from the regression objects, so as long as the regressions in the triage response
   carry their links, the frontend can render them.

   Confirm that `GetTriage` (which already preloads regressions and their views) now calls
   `InjectRegressionHATEOASLinks` (from step 1/3.1 above), so the regression objects in the
   triage response already contain both active and historical links.

**Dependencies:** Step 1.

### Step 4: Frontend: build table columns from regression views, not just current reports

**Files:**
- `sippy-ng/src/component_readiness/TriagedRegressionTestList.jsx`
- Potentially the parent component that provides `allRegressedTests` and regressions

**Changes:**

1. Derive the set of view columns from the union of:
   - View names present in `allRegressedTests` (current report data).
   - View names from `regression.views` on each regression in the triage (persisted
     associations, including inactive).

2. For each regression row and each view column:
   - If a current `regressedTest` exists for this view: render the current severity icon
     with a link (existing behavior).
   - If no current data exists but the regression has a `test_details:<viewName>` link:
     render a historical icon (see step 5).
   - Otherwise: render nothing.

3. Extract view names from regression objects: `regression.views.map(v => v.view_name)`.
   Merge with the keys of `allRegressedTests` to form the complete column set.

**Dependencies:** Step 3 (regressions in triage response carry links and view associations).

### Step 5: Frontend: historical icon with clock/check badge

**Files:**
- `sippy-ng/src/component_readiness/TriagedRegressionTestList.jsx`
- May create a small `HistoricalRegressionIcon` component or inline it

**Changes:**

1. When rendering a cell for a closed/inactive view association:
   - Use a Material UI `History` icon (clock icon) with a green `CheckCircle` badge
     (use MUI `Badge` or overlay).
   - Wrap in a `Tooltip` with text: "Closed regression. View historical test details for
     [date range]." Format the date range from the regression's `last_failure` (e.g.,
     "September 22-29, 2026").
   - If the regression is only closed in this view (but open in another), use tooltip text:
     "Closed in this view. View historical test details for [date range]."
   - Provide `aria-label` for accessibility and ensure keyboard navigation works (the icon
     should be wrapped in a focusable link element).

2. Determine whether a view association is active or inactive:
   - Check `regression.views` for the matching `view_name` and its `active` flag.
   - Or check whether the link came from the current report data (`allRegressedTests`) vs.
     from the regression's HATEOAS links.

3. Keep the current severity icon rendering for active/current report data unchanged.

**Dependencies:** Step 4.

### Step 6: Fix `updateVarsFromView` date precedence in CompReadyVars.jsx

**Files:**
- `sippy-ng/src/component_readiness/CompReadyVars.jsx`

**Changes:**

1. In `updateVarsFromView` (line 419), before overwriting date state variables, check whether
   the URL already contains explicit date parameters. If `params.sampleStartTime`,
   `params.sampleEndTime`, `params.baseStartTime`, or `params.baseEndTime` are set, preserve
   them instead of replacing with the view's resolved values.

2. Implementation approach: in the `useEffect` at line 251 that calls `updateVarsFromView`,
   the explicit params are already available. Modify `updateVarsFromView` to accept the
   current `params` object and only set dates from the view when the corresponding URL
   parameter is absent:
   ```
   setBaseStartTime(params.baseStartTime || formatLongDate(view.base_release.start, dateFormat))
   ```
   This pattern already exists for other parameters (e.g., line 279 for `baseRelease`).

3. Ensure consistency: the controls, displayed report, reload behavior, generated report, and
   copied link must all agree on the historical dates. After this fix, refreshing a historical
   URL should preserve the explicit dates rather than advancing them to `now-7d`.

4. Also ensure `regressionId` is read from URL params and preserved across view loading
   and navigation.

**Dependencies:** None (can be done in parallel with backend steps).

### Step 7: Display closure context on TestDetailsReport.jsx

**Files:**
- `sippy-ng/src/component_readiness/TestDetailsReport.jsx`

**Changes:**

1. Read `regressionId` from URL query parameters (add to the `useQueryParams` configuration
   or read from `urlParams`). Currently `regressionId` is only set from the API response's
   regression field, which requires the regression to appear in the current report.

2. When `regressionId` is present in the URL, fetch the regression record from
   `/api/component_readiness/regressions/<id>` alongside the statistical report. This
   endpoint already exists and returns the full regression with triages and views.

3. Validate the fetched regression matches the requested test (same `test_id`, `release`,
   and relevant variants) before displaying it as context.

4. Display a prominent notice when the regression is closed:
   - "Closed regression, historical report" banner (use MUI `Alert` with `severity="info"`).
   - Show: regression ID, closure date, last failure date, sample/basis windows.
   - Preserve the report's historical statistical assessment even when it shows red (the
     assessment reflects the historical period, not current state).

5. Keep historical lifecycle context separate from the open-regression tracking logic. Do not
   treat a closed record as a new active regression. Do not alter tracking state when viewing
   history.

6. Refresh closure metadata independently of the expensive statistical report cache.
   The regression fetch is a lightweight DB query; the statistical report may be cached.

**Dependencies:** Steps 2, 6.

### Step 8: Handle unavailable history explicitly

**Files:**
- `pkg/api/componentreadiness/triage.go`
- `sippy-ng/src/component_readiness/TriagedRegressionTestList.jsx`
- `sippy-ng/src/component_readiness/TestDetailsReport.jsx`

**Changes:**

1. **Backend:** When `LastFailure` is not valid (null), do not generate a historical link.
   Log a structured warning with the regression ID and view name. The `Links` map will not
   contain a `test_details:<viewName>` entry for this combination.

2. **Backend:** When the view is not found in the current configuration (removed view),
   skip link generation with a warning. This matches existing behavior (line 824-826).

3. **Frontend table:** When a regression has an inactive view association but no corresponding
   `test_details` link (missing `last_failure` or removed view), render a disabled/grayed icon
   with a tooltip explaining that historical data is unavailable.

4. **Frontend test-details page:** When the regression fetch returns a record but the
   statistical report returns no data or an error, show the persisted closure metadata
   (regression ID, dates, triages) with a notice that the historical data is no longer
   available.

5. Do not silently substitute today's window or display a passing result. A missing historical
   link is better than a misleading current-window link.

**Dependencies:** Steps 1, 5, 7.

### Step 9: Update documentation

**Files:**
- `pkg/api/README.md`

**Changes:**

1. Document the historical `test_details:<viewName>` links: when they appear, what their date
   semantics are, and the `regressionId` parameter.

2. Document the closure indicator icon and its meaning: green check = regression closed (not
   a passing assessment).

3. Document the date-availability limitations: historical links are only generated when
   `last_failure` is recorded and the view still exists in configuration.

4. Document the date precedence fix: explicit URL parameters take precedence over view defaults.

**Dependencies:** All previous steps.

## Validation

### Backend tests

**File:** `pkg/api/componentreadiness/triage_test.go`

1. **Extend `TestInjectRegressionHATEOASLinks`** with new table-driven cases:
   - Inactive view association with valid `last_failure`: verify `test_details:<viewName>` link
     is generated with correct historical dates.
   - Inactive view association with null `last_failure`: verify no link is generated.
   - Mixed active/inactive views: verify active views get current links and inactive views
     get historical links.
   - Regression closed in one view, open in another: verify per-view-association behavior.
   - Force-closed regression: verify historical link generation works.
   - Rolling basis expressions (`now-60d`): verify they resolve relative to `last_failure`.
   - GA-based basis expressions (`ga-30d`): verify they resolve against the GA date,
     not `last_failure`.

2. **New test for historical date computation:**
   Add a focused test for the sample window calculation:
   - `last_failure` at 2026-09-29T01:33:26Z: sample = Sep 22 00:00 to Sep 30 00:00.
   - `last_failure` at midnight exactly: sample includes that full day.
   - `last_failure` on today's UTC date: sample end is tomorrow midnight (future-safe).

3. **Test `GetViewsForTriage` update:** If `GetViewsForTriage` is modified to return both
   active and inactive views (for triage enrichment), add corresponding test cases.

**File:** `pkg/api/componentreadiness/utils/queryparamparser_test.go`

4. **Extend `TestGetViewReleaseOptionsAt`:** Add cases verifying that passing a historical
   `now` produces the expected basis window for both GA-based and rolling expressions.

### Frontend tests

**File:** `sippy-ng/bdd/features/` (new Gherkin feature)

5. **Playwright BDD scenarios** for triage details page:
   - Triage with all closed regressions (triage 792): verify the view column appears and
     shows a historical icon.
   - Triage with mixed open/closed regressions (triage 811): verify current icons for open
     regressions and historical icons for closed ones.
   - Click a historical link: verify navigation to test-details page with historical dates.
   - Verify the test-details page shows the "Closed regression, historical report" banner.
   - Verify the historical dates survive page refresh.

**File:** `sippy-ng/src/helpers.test.js` or `sippy-ng/src/component_readiness/CompReadyUtils.test.jsx`

6. **Vitest unit tests** for any new pure helper functions (date formatting for historical
   tooltips, URL parameter precedence logic).

### E2e tests

7. **API integration tests** (via `run_e2e` / `make e2e`):
   - Verify the triage API response for a triage with closed regressions includes
     `test_details` links in the regression objects.
   - Verify the expanded triage response includes historical context for closed regressions.
   - Verify the regression API response includes historical links for inactive views.

### Manual verification

8. Verify triages 792 and 811 against the production-like database:
   - Triage 792: historical 5.0-main link for regression 49971 despite empty `regressed_tests`.
   - Triage 811: current links for open regressions, historical links for closed ones.
   - Verify the shared CR modal (Component Readiness triage modal) renders correctly.
   - Verify the test-details page loads with historical dates and shows closure context.

### CI checks

9. Run `make lint`, `make test` (BDD + Go + Vitest), and `make e2e`. Read the e2e log file.

## Risks and compatibility

1. **URL parameter explosion.** Historical links serialize all view configuration into the
   URL. This is consistent with current link generation but produces long URLs. No change
   from current behavior for active links.

2. **`GetViewReleaseOptionsAt` edge cases.** Passing a historical `now` that predates the
   GA date for a release could produce unexpected basis windows if the view uses `now-`
   relative basis expressions. In practice, basis expressions for CR views use GA-relative
   dates (`ga-30d`), so this is unlikely. Add a guard: if the computed basis start is after
   the basis end, log a warning and skip the link.

3. **Performance of triage expansion.** The current expansion path fetches a live component
   report per active view. This work does not add more report fetches (historical links are
   generated from metadata). However, enriching all triage regressions with HATEOAS links
   requires passing view/release configuration through. This is a small overhead.

4. **Backward compatibility.** The `test_details:<viewName>` link key is unchanged. Clients
   that currently ignore links will continue to work. The `regressionId` query parameter on
   test-details URLs is new but ignored by the API if not handled; the frontend reads it
   from URL params. The `ExpandedTriage.RegressedTests` map gains entries for views that
   previously had none; clients iterating this map will see additional (not fewer) entries.

5. **View configuration drift.** Historical links use the view's current configuration at
   link generation time, not a snapshot from when the regression was active. If the view's
   variant filters, confidence settings, or grouping change between regression closure and
   link access, the historical report may differ from what was originally seen. The Jira
   issue explicitly accepts this: "Use the selected view's current configuration at link
   generation; this work reconstructs a historical query, not a stored snapshot."

6. **`updateVarsFromView` fix scope.** Changing the date precedence in `CompReadyVars.jsx`
   affects all CR pages, not just historical links. This is intentional (explicit URL dates
   should always take precedence) but should be tested for regressions in the normal view
   selection flow.

## Open questions

1. **Triage list API enrichment scope.** Should `ListTriages` also enrich each triage's
   regressions with HATEOAS links? This adds overhead for list endpoints. The Jira issue
   mentions "the shared triaged-regression table's data sources," which suggests yes.
   Recommend enriching list responses since the triaged-regression table uses list data.
   **Impact:** If deferred, the shared table won't show historical links until a triage is
   individually loaded.

2. **Historical data retention.** BigQuery and PostgreSQL have different data retention
   policies. The historical sample window (7 days before last failure) may reference data
   that has been pruned. The plan shows a clear error rather than empty results, but the
   exact error message depends on how the data providers handle date ranges with no data.
   **Resolution:** Test with known pruned date ranges and verify the test-details page
   handles empty results gracefully.

3. **`regressionId` parameter in `GenerateTestDetailsURL`.** The cleanest approach is to
   append it in the caller (`generateHistoricalTestDetailsURLFromRegression`) after the
   base URL is built, avoiding a signature change to the widely-used utility function.
   **Impact:** Minor. Either approach works; the appending approach is simpler.
