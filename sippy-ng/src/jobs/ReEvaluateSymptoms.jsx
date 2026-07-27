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
import PropTypes from 'prop-types'
import React, { useCallback, useEffect, useRef, useState } from 'react'
import RefreshIcon from '@mui/icons-material/Refresh'

const POLL_INTERVAL_MS = 2000
const MAX_RETRIES = 1

async function submitReEvaluation(buildIDs) {
  const response = await fetch(
    import.meta.env.VITE_API_URL + '/api/jobs/runs/reevaluate',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prow_job_build_ids: buildIDs }),
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
    throw new Error(errorMsg)
  }

  return response.json()
}

async function pollTaskStatus(taskID) {
  const response = await fetch(
    import.meta.env.VITE_API_URL +
      '/api/jobs/runs/reevaluate/' +
      encodeURIComponent(taskID)
  )

  if (!response.ok) {
    let errorMsg = `HTTP ${response.status}`
    try {
      const errBody = await response.json()
      if (errBody.message) errorMsg = errBody.message
    } catch {
      // fall back to status text if response isn't JSON
    }
    throw new Error(errorMsg)
  }

  return response.json()
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
  const pollTimerRef = useRef(null)
  const taskIDRef = useRef(null)

  // Clean up polling on unmount
  useEffect(() => {
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current)
      }
    }
  }, [])

  const handleTaskComplete = useCallback(
    (results) => {
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
    },
    [forceRefreshURL]
  )

  const startPolling = useCallback(
    (taskID, total) => {
      taskIDRef.current = taskID

      pollTimerRef.current = setInterval(async () => {
        try {
          const task = await pollTaskStatus(taskID)

          setProgress({
            total: task.total,
            processed: task.processed,
            results: task.results || [],
          })

          if (task.status === 'completed' || task.status === 'failed') {
            clearInterval(pollTimerRef.current)
            pollTimerRef.current = null
            setRunning(false)

            if (task.status === 'failed') {
              setSnackbar({
                severity: 'error',
                message: `Re-evaluation failed: ${task.error || 'unknown error'}`,
              })
            } else {
              handleTaskComplete(task.results || [])
            }
          }
        } catch (err) {
          clearInterval(pollTimerRef.current)
          pollTimerRef.current = null
          setRunning(false)
          setSnackbar({
            severity: 'error',
            message: `Error polling task status: ${err.message}`,
          })
        }
      }, POLL_INTERVAL_MS)
    },
    [handleTaskComplete]
  )

  const handleReEvaluate = async () => {
    if (!prowJobBuildIDs?.length) return
    setRunning(true)
    setSnackbar(null)

    const total = prowJobBuildIDs.length
    setProgress({ total, processed: 0, results: [] })

    let lastError = null
    for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
      try {
        const data = await submitReEvaluation(prowJobBuildIDs)
        startPolling(data.id, total)
        return
      } catch (err) {
        lastError = err
        if (attempt < MAX_RETRIES) {
          continue
        }
      }
    }

    setRunning(false)
    setSnackbar({
      severity: 'error',
      message: `Re-evaluation failed: ${lastError?.message || 'unknown error'}`,
    })
  }

  const isDisabled = disabled || running || !prowJobBuildIDs?.length

  const progressBar =
    running && progress ? (
      <Stack spacing={0.5} sx={{ minWidth: 200 }}>
        <LinearProgress
          variant="determinate"
          value={(progress.processed / progress.total) * 100}
        />
        <Typography variant="caption" color="text.secondary">
          {progress.processed}/{progress.total} completed
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
