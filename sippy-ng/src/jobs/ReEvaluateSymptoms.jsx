import {
  Alert,
  Button,
  LinearProgress,
  Link,
  Snackbar,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material'
import pLimit from 'p-limit'
import PropTypes from 'prop-types'
import React, { useState } from 'react'
import RefreshIcon from '@mui/icons-material/Refresh'

const CONCURRENCY = 10
const MAX_RETRIES = 1
const REQUEST_TIMEOUT_MS = 120000

async function reEvaluateOne(buildID) {
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)
  try {
    const response = await fetch(
      import.meta.env.VITE_API_URL + '/api/jobs/runs/reevaluate',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prow_job_build_ids: [buildID] }),
        signal: controller.signal,
      }
    )

    if (!response.ok) {
      let errorMsg = `HTTP ${response.status}`
      try {
        const errBody = await response.json()
        if (errBody.message) errorMsg = errBody.message
      } catch {
        // fall back to status text if response isn't JSON
      }
      return {
        prow_job_build_id: buildID,
        status: 'eval_error',
        error: errorMsg,
      }
    }

    const data = await response.json()
    return (
      data.results?.[0] ?? { prow_job_build_id: buildID, status: 'eval_error' }
    )
  } catch (err) {
    return {
      prow_job_build_id: buildID,
      status: 'eval_error',
      error:
        err.name === 'AbortError'
          ? 'request timed out'
          : err.message || String(err),
    }
  } finally {
    clearTimeout(timeoutId)
  }
}

async function runPool(ids, onProgress) {
  const limit = pLimit(CONCURRENCY)
  const results = []
  const tasks = ids.map((id) =>
    limit(async () => {
      const result = await reEvaluateOne(id)
      results.push(result)
      onProgress([...results])
    })
  )
  await Promise.all(tasks)
  return results
}

function errorDetails(failures) {
  const byMessage = new Map()
  for (const f of failures) {
    const msg = f.error || 'unknown error'
    byMessage.set(msg, (byMessage.get(msg) || 0) + 1)
  }
  return (
    <ul style={{ margin: '4px 0 0', paddingLeft: 20 }}>
      {[...byMessage.entries()].map(([msg, count]) => (
        <li key={msg}>
          {count > 1 ? `(${count}x) ` : ''}
          {msg}
        </li>
      ))}
    </ul>
  )
}

function forceRefreshMessage(forceRefreshURL) {
  if (!forceRefreshURL) {
    return `Refresh the page to see updated labels.`
  }
  return (
    <>
      <Link href={forceRefreshURL} color="inherit" underline="always">
        Reload with fresh data
      </Link>{' '}
      to see updated labels.
    </>
  )
}

export default function ReEvaluateButton({
  prowJobBuildIDs,
  forceRefreshURL,
  disabled = false,
}) {
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState(null)
  const [snackbar, setSnackbar] = useState(null)

  const handleReEvaluate = async () => {
    if (!prowJobBuildIDs?.length) return
    setRunning(true)
    setSnackbar(null)

    const total = prowJobBuildIDs.length
    setProgress({ total, results: [] })

    try {
      let results = await runPool(prowJobBuildIDs, (partial) =>
        setProgress({ total, results: partial })
      )

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

      const successCount = results.filter((r) => r.status === 'success').length
      const rewriteErrors = results.filter((r) => r.status === 'rewrite_error')
      const evalErrors = results.filter((r) => r.status === 'eval_error')
      const missingErrors = results.filter((r) => r.status === 'missing_error')

      const allErrors = [...rewriteErrors, ...evalErrors, ...missingErrors]

      if (rewriteErrors.length > 0) {
        const parts = []
        if (successCount > 0) parts.push(`${successCount} succeeded`)
        parts.push(
          `${rewriteErrors.length} failed during rewrite and may be in an inconsistent state`
        )
        if (evalErrors.length > 0)
          parts.push(`${evalErrors.length} failed to evaluate`)
        if (missingErrors.length > 0)
          parts.push(`${missingErrors.length} not found`)
        setSnackbar({
          severity: 'error',
          message: (
            <>
              {parts.join(', ')}.
              {rewriteErrors.length > 0 && (
                <>
                  <strong>Rewrite errors:</strong>
                  {errorDetails(rewriteErrors)}
                </>
              )}
              {evalErrors.length > 0 && (
                <>
                  <strong>Evaluation errors:</strong>
                  {errorDetails(evalErrors)}
                </>
              )}
              {missingErrors.length > 0 && (
                <>
                  <strong>Not found:</strong>
                  {errorDetails(missingErrors)}
                </>
              )}
            </>
          ),
        })
      } else if (
        successCount === 0 &&
        missingErrors.length === results.length
      ) {
        setSnackbar({
          severity: 'error',
          message: (
            <>
              None of the selected job run(s) were found in Sippy
              {errorDetails(missingErrors)}
            </>
          ),
        })
      } else if (
        evalErrors.length > 0 &&
        successCount === 0 &&
        missingErrors.length === 0
      ) {
        setSnackbar({
          severity: 'error',
          message: (
            <>
              Re-evaluation failed for all {evalErrors.length} job run(s)
              {errorDetails(evalErrors)}
            </>
          ),
        })
      } else if (evalErrors.length > 0 || missingErrors.length > 0) {
        const parts = [`Re-evaluated ${successCount} job run(s)`]
        if (evalErrors.length > 0) parts.push(`${evalErrors.length} failed`)
        if (missingErrors.length > 0)
          parts.push(`${missingErrors.length} not found`)
        setSnackbar({
          severity: 'warning',
          message: (
            <>
              {parts.join(', ')}. {forceRefreshMessage(forceRefreshURL)}
              {errorDetails(allErrors)}
            </>
          ),
        })
      } else {
        setSnackbar({
          severity: 'success',
          message: (
            <>
              Successfully re-evaluated {successCount} job run(s).{' '}
              {forceRefreshMessage(forceRefreshURL)}
            </>
          ),
        })
      }
    } catch (err) {
      setSnackbar({
        severity: 'error',
        message: `Re-evaluation failed: ${err.message}`,
      })
    } finally {
      setRunning(false)
    }
  }

  const isDisabled = disabled || running || !prowJobBuildIDs?.length

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
            ` (${
              progress.results.filter((r) => r.status === 'success').length
            } succeeded)`}
          {progress.results.filter((r) => r.status !== 'success').length > 0 &&
            `, ${
              progress.results.filter((r) => r.status !== 'success').length
            } failed`}
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
            size="large"
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
        autoHideDuration={snackbar?.severity === 'success' ? 6000 : null}
        onClose={() => setSnackbar(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
        sx={{ maxWidth: '80vw' }}
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

ReEvaluateButton.propTypes = {
  prowJobBuildIDs: PropTypes.arrayOf(PropTypes.string),
  forceRefreshURL: PropTypes.string,
  disabled: PropTypes.bool,
}
