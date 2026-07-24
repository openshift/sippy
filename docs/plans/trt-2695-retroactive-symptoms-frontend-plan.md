# TRT-2695: Frontend Implementation Plan - Re-evaluate Symptoms UI Controls

## Overview

Add UI controls to Sippy that allow users to trigger retroactive symptom
re-evaluation for selected job runs. The controls call the backend API endpoint
(`POST /api/jobs/runs/reevaluate`) added in the backend implementation.

**Jira:** [TRT-2695](https://redhat.atlassian.net/browse/TRT-2695)
**Dependency:** The backend API endpoint must exist (or be mocked) before this work
can be functionally tested.
**Context doc:** `docs/features/job-analysis-symptoms.md` - update this doc when
implementation is complete (see Step 4).

## Prerequisites: Orientation

Read and understand these files before starting:

| File | What to learn |
|------|---------------|
| `sippy-ng/src/component_readiness/JobArtifactQuery.js` | JAQ dialog - the sole integration point for the re-evaluate button. Uses standard `fetch()` for symptom CRUD and artifact queries. Has its own job run selection via `selectedJobRunIds` (a `Set` initialized at line 164) with `SelectingCheckbox` per row. `searchJobRunIds` is also a `Set` (typed as `PropTypes.object`, uses `.has()`, `.union()`, `.difference()`, `.keys()`). Both contain prow build ID strings. The bottom action bar (near `JAQOpenJobRunsButton`, `JAQCopyIdsButton`, and the Close button) is where the re-evaluate button goes. All IDs in JAQ are strings, so no JS number precision concern. The existing action buttons (`JAQCopyIdsButton`, `JAQOpenJobRunsButton`) use a visible-rows-intersection pattern via `filteredRows` to scope their actions to visible rows only. |

## Authentication Pattern

Sippy uses SSO via an oauth proxy at the deployment level. The React frontend makes standard
`fetch()` calls **without** injecting Authorization headers. Authentication is handled automatically
by browser cookies and same-origin policy. Follow the same pattern - no auth logic needed in the
component.

## Step 1: Create the ReEvaluateButton Component

Create `sippy-ng/src/jobs/ReEvaluateSymptoms.js`:

See the actual implementation in `sippy-ng/src/jobs/ReEvaluateSymptoms.js`.

### Design decisions

- **One request per job run**: Each build ID is sent as a single-element request
  to the backend. This gives per-run progress feedback and isolates failures.
- **Worker pool (10 concurrent)**: Up to 10 requests run in parallel using
  `p-limit`. Fast completions immediately start the next item, so throughput
  is limited by the slowest individual run, not block boundaries.
- **Automatic retries**: After the initial pass, `eval_error` and `rewrite_error`
  results are retried once. `eval_error` means nothing was written so retry is
  safe. `rewrite_error` means data may be inconsistent so retry is necessary.
  `missing_error` (build ID not found) is not retried.
- **Progress indicator**: A `LinearProgress` bar with text summary appears inline
  next to the button while running, showing completed/total and success/fail counts.
- **All request failures are per-build results**: `reEvaluateOne` wraps everything
  in a try/catch so that 503s, network failures, and JSON parse errors all return
  `eval_error` results rather than throwing. This keeps every ID flowing through
  the pool, retry, and progress reporting path. No single failure can abort the
  entire batch.
- **Error details in snackbar**: When there are failures, the snackbar includes a
  deduplicated bulleted list of error messages from the API responses, with counts
  when the same message repeats (e.g. "(3x) HTTP 503").
- **Snackbar auto-hide**: Only success snackbars auto-hide after 6 seconds. Error
  and warning snackbars stay open until the user dismisses them, giving time to
  read error details.
- **No batch size cap**: Since each request sends a single ID, the backend's 50-item
  limit is irrelevant. The concurrency limit (10) provides natural throttling.

## Step 2: Integrate into the JAQ Dialog

The re-evaluate button lives inside `JobArtifactQuery.js`, not in each parent page.
JAQ is opened from multiple places (CR test details, job runs table) and already has
its own job run selection (`selectedJobRunIds`, a `Set<string>` of build IDs). By
placing the button in JAQ's action bar, all callers get re-evaluation for free.

### 2.1: Determine which IDs to use

Follow the same visible-rows-intersection pattern used by `JAQCopyIdsButton` (line
851) and `JAQOpenJobRunsButton` (line 892). These buttons scope their actions to
visible (filtered) rows and intersect with the user's checkbox selection:

```jsx
let visible = new Set(filteredRows.map((row) => row.job_run_id))
let selected = selectedJobRunIds.intersection(visible)
const idsToReEvaluate = (selected.size > 0 ? selected : visible).keys().toArray()
```

This ensures the button respects table filters: if the user has filtered the table,
only visible rows are targeted. If the user has also checked specific rows, only
those checked+visible rows are targeted.

### 2.2: Add the button to the action bar

In the bottom `<Stack>` (around line 1778), add `ReEvaluateButton` alongside the
existing `JAQOpenJobRunsButton` and `JAQCopyIdsButton`. The button is conditionally
rendered based on the `write_endpoints` server capability (same pattern as
`JAQSaveAsSymptomSection`):

```jsx
import ReEvaluateButton from '../jobs/ReEvaluateSymptoms'

// In the bottom action bar:
<Stack direction="row" spacing={2}>
  <JAQOpenJobRunsButton />
  <JAQCopyIdsButton />
  {capabilitiesContext.includes('write_endpoints') && (
    <ReEvaluateButton
      prowJobBuildIDs={idsToReEvaluate}
      forceRefreshURL={forceRefreshURL}
    />
  )}
  <Tooltip title="Return to details report">
    <Button size="large" variant="contained" onClick={handleToggleJAQOpen}>
      <Close />
      Close
    </Button>
  </Tooltip>
</Stack>
```

### 2.3: Tooltip behavior

The tooltip should reflect the current targeting:
- No checkbox selection: "Re-evaluate symptoms for N visible job run(s)"
- With checkbox selection: "Re-evaluate symptoms for N selected job run(s)"

### 2.4: Cache invalidation via `forceRefreshURL`

The test_details report is cached (up to 8 hours or until the next rounding
boundary). Re-evaluating symptoms writes new labels to BigQuery but does not
invalidate the cache.

To solve this without complex callback plumbing, the JAQ dialog accepts an
optional `forceRefreshURL` prop. When provided, the success/warning snackbar
shows a clickable "Reload with fresh data" link that navigates to the current
page URL with `forceRefresh=true` appended, which tells the backend to bypass
the cache.

`CompReadyTestPanel` (the test_details parent) computes and passes this URL:

```jsx
forceRefreshURL={(() => {
  const url = new URL(window.location.href)
  url.searchParams.set('forceRefresh', 'true')
  return url.toString()
})()}
```

Other JAQ callers do not pass `forceRefreshURL`, so they get the default
"Refresh the page to see updated labels" text instead.

## Step 3: Testing

### 3.1: Component tests

See `sippy-ng/src/jobs/ReEvaluateSymptoms.test.js` for the full implementation.
Key test cases:

- Renders in default state (button text visible, not disabled)
- Disabled when `prowJobBuildIDs` is empty or `disabled` prop is true
- Shows progress bar during execution (uses a deferred fetch response to assert
  the `progressbar` role and "1/2 completed" text while a request is in flight)
- Shows success snackbar when all runs succeed
- Shows error snackbar on `rewrite_error` (inconsistent state)
- Shows error snackbar when all runs are `missing_error`
- Retries `eval_error` and `rewrite_error` once (verifies fetch called twice)
- Does not retry `missing_error` (verifies fetch called once)
- Shows warning snackbar on partial success (mixed success + errors)
- Sends one request per build ID (verifies single-element body per call)
- Treats 503 as `eval_error` and retries (verifies 6 fetch calls for 3 IDs,
  error details shown in snackbar)

### 3.2: Integration / smoke tests

- Button appears in JAQ action bar (bottom of dialog)
- Opening JAQ from CR test details shows the button with all job runs
- Opening JAQ from job runs table shows the button with selected job runs
- With no JAQ selection, button targets all visible/filtered job runs
- With JAQ selection, button targets only selected+visible runs
- Filtering the table reduces the set of targeted job runs
- Clicking sends one request per build ID (not a batch)
- Progress bar updates as each request completes
- Retryable errors are retried automatically
- Snackbar shows final summary after all runs complete (including retries)

## Step 4: Update Documentation

### 4.1: Update `docs/features/job-analysis-symptoms.md`

In the "UI Display" section, add a bullet for re-evaluation:

```markdown
- Re-evaluation controls - button in the JAQ dialog action bar. Triggers
  retroactive symptom matching for selected (or all) job runs in the dialog
  (requires SSO authentication via the write-enabled deployment).
```

## Design Notes

1. **No explicit auth headers**: SSO is handled by the oauth proxy. Standard
   `fetch()` works because the browser includes cookies automatically. Do NOT add
   Authorization header logic - it would break the existing auth flow.

2. **One request per job run**: Each build ID is sent as a separate single-element
   API request. This avoids the backend's 50-item batch limit entirely, gives
   per-run progress feedback, and isolates failures so one bad run doesn't block
   others. Up to 10 requests run concurrently for throughput.

3. **Button state clarity**: The button has three visible states:
   - Ready - blue/primary (`size="large"` to match other action bar buttons),
     tooltip shows count of runs to re-evaluate
   - Running - disabled, with a `LinearProgress` bar and text summary
     ("5/20 completed, 4 succeeded, 1 failed") shown inline
   - Done - Snackbar shows final success/warning/error summary. Error and
     warning snackbars include a deduplicated bulleted list of API error
     messages and stay open until dismissed. Success snackbars auto-hide
     after 6 seconds.

4. **JAQ as single integration point**: Rather than adding the button to each
   parent page (test details, job runs table, payload table), it lives in JAQ's
   action bar. JAQ is already the symptom-focused UI, has its own job run selection
   (`selectedJobRunIds`), and is reachable from all relevant pages. This avoids
   duplicating selection wiring and keeps symptom actions in one place. The button
   follows the same visible-rows-intersection pattern as `JAQCopyIdsButton` and
   `JAQOpenJobRunsButton` for consistent behavior with table filters. The button
   is only rendered when the server reports the `write_endpoints` capability
   (same gate used by `JAQSaveAsSymptomSection`).

5. **Cache invalidation via forceRefresh link**: The test_details report is cached
   in Redis (up to 8 hours). Re-evaluating symptoms writes new labels to BigQuery
   but does not invalidate the cache. Rather than adding complex callback plumbing,
   `CompReadyTestPanel` passes a `forceRefreshURL` prop (current page URL with
   `forceRefresh=true` appended). On success, the snackbar shows a clickable
   "Reload with fresh data" link. Other JAQ callers that don't pass the prop get
   a plain "Refresh the page" message.

6. **No confirmation dialog**: For the initial implementation, clicking the button
   immediately starts re-evaluation. A confirmation dialog could be added later if
   users find accidental clicks problematic, but symptom re-evaluation is non-
   destructive (it's idempotent - running it twice produces the same result).

7. **`dry_run` not exposed**: The backend accepts an optional `dry_run` boolean in
   the request body (logs what would happen without writing). This is useful for
   debugging via curl but not needed in the UI. The frontend omits it (the backend
   defaults to `false`).

8. **Backend strict parsing**: The backend handler uses `DisallowUnknownFields()`
   on the request body decoder. The frontend must only send known fields
   (`prow_job_build_ids` and optionally `dry_run`). Adding extra fields to the
   request body would cause a 400 error. Error responses are JSON
   (`{"code": N, "message": "..."}`), so the frontend parses `.message` from the
   response body for user-facing error display.

9. **Future enhancement**: Consider adding a "Re-evaluate" option to a row-level
   context menu (right-click or kebab menu) for single-run re-evaluation without
   needing to use checkbox selection. This can be a follow-up.
