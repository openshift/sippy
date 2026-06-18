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

```jsx
import React, { useState } from 'react'
import {
  Alert,
  Button,
  LinearProgress,
  Snackbar,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material'
import RefreshIcon from '@mui/icons-material/Refresh'

const CONCURRENCY = 10
const MAX_RETRIES = 1

// Send a single-ID re-evaluate request. Returns the per-run result object.
async function reEvaluateOne(buildID) {
  const response = await fetch(
    process.env.REACT_APP_API_URL + '/api/jobs/runs/reevaluate',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prow_job_build_ids: [buildID] }),
    }
  )

  if (!response.ok) {
    // The backend returns JSON errors: {"code": N, "message": "..."}
    let errorMsg = `HTTP ${response.status}`
    try {
      const errBody = await response.json()
      if (errBody.message) errorMsg = errBody.message
    } catch {
      // fall back to status text if response isn't JSON
    }

    if (response.status === 503) {
      throw new Error(errorMsg)
    }
    return {
      prow_job_build_id: buildID,
      status: 'eval_error',
      error: errorMsg,
    }
  }

  const data = await response.json()
  return data.results?.[0] ?? { prow_job_build_id: buildID, status: 'eval_error' }
}

// Run a pool of workers over a list of IDs, calling onProgress after each.
async function runPool(ids, onProgress) {
  const results = []
  let index = 0

  async function worker() {
    while (index < ids.length) {
      const i = index++
      const result = await reEvaluateOne(ids[i])
      results.push(result)
      onProgress([...results])
    }
  }

  await Promise.all(
    Array.from({ length: Math.min(CONCURRENCY, ids.length) }, () => worker())
  )
  return results
}

export default function ReEvaluateButton({
  prowJobBuildIDs,
  onComplete,
  disabled = false,
}) {
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState(null) // { total, results: [...] }
  const [snackbar, setSnackbar] = useState(null) // { severity, message }

  const handleReEvaluate = async () => {
    if (!prowJobBuildIDs?.length) return
    setRunning(true)
    setSnackbar(null)

    const total = prowJobBuildIDs.length
    setProgress({ total, results: [] })

    try {
      // Initial pass
      let results = await runPool(prowJobBuildIDs, (partial) =>
        setProgress({ total, results: partial })
      )

      // Retry retryable errors (eval_error, rewrite_error) up to MAX_RETRIES times
      for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
        const retryable = results.filter(
          (r) => r.status === 'eval_error' || r.status === 'rewrite_error'
        )
        if (retryable.length === 0) break

        const kept = results.filter(
          (r) => r.status !== 'eval_error' && r.status !== 'rewrite_error'
        )
        const retryIDs = retryable.map((r) => r.prow_job_build_id)
        const retryResults = await runPool(retryIDs, (partial) =>
          setProgress({ total, results: [...kept, ...partial] })
        )
        results = [...kept, ...retryResults]
        setProgress({ total, results })
      }

      // Summarize
      const successCount = results.filter((r) => r.status === 'success').length
      const rewriteErrors = results.filter(
        (r) => r.status === 'rewrite_error'
      )
      const evalErrors = results.filter((r) => r.status === 'eval_error')
      const missingErrors = results.filter((r) => r.status === 'missing_error')

      if (rewriteErrors.length > 0) {
        setSnackbar({
          severity: 'error',
          message: `${rewriteErrors.length} job run(s) failed during rewrite and may be in an inconsistent state.`,
        })
      } else if (successCount === 0 && missingErrors.length === results.length) {
        setSnackbar({
          severity: 'error',
          message: 'None of the selected job run(s) were found in Sippy',
        })
      } else if (evalErrors.length > 0 && successCount === 0) {
        setSnackbar({
          severity: 'error',
          message: `Re-evaluation failed for all ${evalErrors.length} job run(s)`,
        })
      } else if (evalErrors.length > 0 || missingErrors.length > 0) {
        const parts = [`Re-evaluated ${successCount} job run(s)`]
        if (evalErrors.length > 0)
          parts.push(`${evalErrors.length} failed`)
        if (missingErrors.length > 0)
          parts.push(`${missingErrors.length} not found`)
        parts.push('Refresh the page to see updated labels.')
        setSnackbar({ severity: 'warning', message: parts.join(', ') })
      } else {
        setSnackbar({
          severity: 'success',
          message: `Successfully re-evaluated ${successCount} job run(s). Refresh the page to see updated labels.`,
        })
      }

      if (onComplete && successCount > 0) onComplete()
    } catch (err) {
      // 503 or other fatal error from reEvaluateOne
      setSnackbar({
        severity: 'error',
        message: `Re-evaluation failed: ${err.message}`,
      })
    } finally {
      setRunning(false)
    }
  }

  const isDisabled = disabled || running || !prowJobBuildIDs?.length

  // Progress summary while running
  const progressBar =
    running && progress ? (
      <Stack spacing={0.5} sx={{ minWidth: 200 }}>
        <LinearProgress
          variant="determinate"
          value={(progress.results.length / progress.total) * 100}
        />
        <Typography variant="caption" color="text.secondary">
          {progress.results.length}/{progress.total} completed
          {progress.results.filter((r) => r.status === 'success').length > 0 &&
            ` (${progress.results.filter((r) => r.status === 'success').length} succeeded)`}
          {progress.results.filter((r) => r.status !== 'success').length > 0 &&
            `, ${progress.results.filter((r) => r.status !== 'success').length} failed`}
        </Typography>
      </Stack>
    ) : null

  return (
    <>
      <Tooltip
        title={
          !prowJobBuildIDs?.length
            ? 'Select job runs to re-evaluate'
            : `Re-evaluate symptoms for ${prowJobBuildIDs.length} job run(s)`
        }
      >
        <span>
          <Button
            variant="contained"
            size="small"
            startIcon={<RefreshIcon />}
            onClick={handleReEvaluate}
            disabled={isDisabled}
          >
            Re-evaluate Symptoms
          </Button>
        </span>
      </Tooltip>

      {progressBar}

      <Snackbar
        open={!!snackbar}
        autoHideDuration={6000}
        onClose={() => setSnackbar(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          severity={snackbar?.severity}
          onClose={() => setSnackbar(null)}
          variant="filled"
        >
          {snackbar?.message}
        </Alert>
      </Snackbar>
    </>
  )
}
```

### Design decisions

- **One request per job run**: Each build ID is sent as a single-element request
  to the backend. This gives per-run progress feedback and isolates failures.
- **Worker pool (10 concurrent)**: Up to 10 requests run in parallel using a
  simple async queue pattern. Fast completions immediately start the next item,
  so throughput is limited by the slowest individual run, not block boundaries.
- **Automatic retries**: After the initial pass, `eval_error` and `rewrite_error`
  results are retried once. `eval_error` means nothing was written so retry is
  safe. `rewrite_error` means data may be inconsistent so retry is necessary.
  `missing_error` (build ID not found) is not retried.
- **Progress indicator**: A `LinearProgress` bar with text summary appears inline
  next to the button while running, showing completed/total and success/fail counts.
- **503 is fatal**: If the backend returns 503 (no BQ/GCS), `reEvaluateOne` throws
  and the entire operation stops immediately (no point retrying other IDs).
- **Error handling**: After all runs complete (including retries), a Snackbar shows
  the final summary with status-aware messaging.
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
existing `JAQOpenJobRunsButton` and `JAQCopyIdsButton`:

```jsx
import ReEvaluateButton from '../jobs/ReEvaluateSymptoms'

// In the bottom action bar:
<Stack direction="row" spacing={2}>
  <JAQOpenJobRunsButton />
  <JAQCopyIdsButton />
  <ReEvaluateButton
    prowJobBuildIDs={idsToReEvaluate}
  />
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

### 2.4: `onComplete` callback - refresh parent data

JAQ doesn't display symptom labels on job run rows (labels/symptoms are shown in
the parent CR test details view). The parent data comes from `CompReadyTestPanel`'s
`props.data`, which is fetched by a grandparent component. Threading a refetch
callback through multiple component layers would be complex.

Instead, on successful re-evaluation, include a note in the success Snackbar
telling the user to refresh the page to see updated labels:

```jsx
message: `Successfully re-evaluated ${successCount} job run(s). Refresh the page to see updated labels.`
```

For partial success (warning snackbar), append the same note. This is honest about
the limitation and avoids complex callback plumbing for the initial implementation.

If a proper parent refresh is added later, the `onComplete` prop on
`ReEvaluateButton` is already wired for it.

## Step 3: Testing

### 3.1: Component tests

Using React Testing Library (or whatever test framework the project uses):

```jsx
// ReEvaluateSymptoms.test.js

describe('ReEvaluateButton', () => {
  it('renders in default state', () => {
    // Button text visible, not loading, not disabled
  })

  it('is disabled when prowJobBuildIDs is empty', () => {
    // Button should be disabled with appropriate tooltip
  })

  it('is disabled when disabled prop is true', () => {
    // External disable override works
  })

  it('shows progress bar during execution', () => {
    // Mock fetch, click button, verify LinearProgress appears with counts
  })

  it('shows success snackbar when all runs succeed', () => {
    // Mock fetch with success response for each ID
  })

  it('shows error snackbar on rewrite_error (inconsistent state)', () => {
    // Mock fetch with rewrite_error result, verify urgent error message
  })

  it('shows error snackbar when all runs are missing_error', () => {
    // Mock fetch with all missing_error, verify "not found" message
  })

  it('retries eval_error and rewrite_error once', () => {
    // Mock fetch to return eval_error first, then success on retry
    // Verify fetch is called twice for the same ID
  })

  it('does not retry missing_error', () => {
    // Mock fetch with missing_error, verify no retry attempt
  })

  it('shows warning snackbar on partial success', () => {
    // Mock fetch with mixed success + eval_error/missing_error, verify warning
  })

  it('calls onComplete when at least one succeeds', () => {
    // Mock fetch, verify callback is invoked
  })

  it('does not call onComplete on total failure', () => {
    // Mock fetch with all errors, verify callback NOT invoked
  })

  it('sends one request per build ID', () => {
    // Pass 3 IDs, verify 3 separate fetch calls each with a single-element array
  })

  it('stops all workers on 503', () => {
    // Mock fetch to return 503, verify no further requests are made
  })
})
```

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
   - Ready - blue/primary, tooltip shows count of runs to re-evaluate
   - Running - disabled, with a `LinearProgress` bar and text summary
     ("5/20 completed, 4 succeeded, 1 failed") shown inline
   - Done - Snackbar shows final success/warning/error summary

4. **JAQ as single integration point**: Rather than adding the button to each
   parent page (test details, job runs table, payload table), it lives in JAQ's
   action bar. JAQ is already the symptom-focused UI, has its own job run selection
   (`selectedJobRunIds`), and is reachable from all relevant pages. This avoids
   duplicating selection wiring and keeps symptom actions in one place. The button
   follows the same visible-rows-intersection pattern as `JAQCopyIdsButton` and
   `JAQOpenJobRunsButton` for consistent behavior with table filters.

5. **Refresh via page reload**: JAQ doesn't display symptom labels on job run
   rows (labels are shown in the parent CR test details view). The parent data
   originates from a grandparent component, so threading a refetch callback
   would require changes across multiple layers. Instead, the success Snackbar
   tells the user to refresh the page. The `onComplete` prop on
   `ReEvaluateButton` is available for a proper refresh callback later.

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
