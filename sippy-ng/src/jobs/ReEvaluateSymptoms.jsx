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

const POLL_INTERVAL_MS = 2500

// submitBatch sends all job run IDs in a single POST and returns the batch
// metadata (batch_id, requested, links).
async function submitBatch(prowJobBuildIDs) {
  const response = await fetch(
    import.meta.env.VITE_API_URL + '/api/jobs/runs/reevaluate',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prow_job_build_ids: prowJobBuildIDs }),
    }
  )

  if (!response.ok) {
    let errorMsg = `HTTP ${response.status}`
    try {
      const errBody = await response.json()
      if (errBody.message) errorMsg = errBody.message
    } catch {
      // fall back to status text if response body is not JSON
    }
    throw new Error(errorMsg)
  }

  return response.json()
}

// fetchBatchStatus polls the batch status endpoint.
async function fetchBatchStatus(batchID) {
  const response = await fetch(
    import.meta.env.VITE_API_URL +
      '/api/jobs/runs/reevaluate/' +
      encodeURIComponent(batchID)
  )

  if (!response.ok) {
    let errorMsg = `HTTP ${response.status}`
    try {
      const errBody = await response.json()
      if (errBody.message) errorMsg = errBody.message
    } catch {
      // fall back to status text
    }
    throw new Error(errorMsg)
  }

  return response.json()
}

function isTerminalStatus(status) {
  return status === 'complete' || status === 'failed'
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

  const stopPolling = useCallback(() => {
    if (pollTimerRef.current != null) {
      clearInterval(pollTimerRef.current)
      pollTimerRef.current = null
    }
  }, [])

  // Clean up polling on unmount.
  useEffect(() => {
    return stopPolling
  }, [stopPolling])

  const handleBatchComplete = useCallback(
    (statusData) => {
      stopPolling()
      setRunning(false)

      const { completed, failed, requested } = statusData

      if (failed > 0 && completed === 0) {
        setSnackbar({
          severity: 'error',
          message: `Re-evaluation failed for all ${failed} job run(s).`,
        })
      } else if (failed > 0) {
        setSnackbar({
          severity: 'warning',
          message: (
            <>
              Re-evaluated {completed} job run(s), {failed} failed.{' '}
              {forceRefreshMessage(forceRefreshURL)}
            </>
          ),
        })
      } else if (requested === 0) {
        setSnackbar({
          severity: 'warning',
          message: 'No job runs were submitted for re-evaluation.',
        })
      } else {
        setSnackbar({
          severity: 'success',
          message: (
            <>
              Successfully re-evaluated {completed} job run(s).{' '}
              {forceRefreshMessage(forceRefreshURL)}
            </>
          ),
        })
      }
    },
    [forceRefreshURL, stopPolling]
  )

  const startPolling = useCallback(
    (batchID) => {
      pollTimerRef.current = setInterval(async () => {
        try {
          const statusData = await fetchBatchStatus(batchID)
          setProgress({
            requested: statusData.requested,
            completed: statusData.completed,
            failed: statusData.failed,
            running: statusData.running,
            pending: statusData.pending,
            status: statusData.status,
          })

          if (isTerminalStatus(statusData.status)) {
            handleBatchComplete(statusData)
          }
        } catch (err) {
          // A single poll failure is not fatal; keep polling.
          // If the batch has already finished, the next poll will pick it up.
          console.error('Error polling batch status:', err)
        }
      }, POLL_INTERVAL_MS)
    },
    [handleBatchComplete]
  )

  const handleReEvaluate = async () => {
    if (!prowJobBuildIDs?.length) return
    setRunning(true)
    setSnackbar(null)
    setProgress(null)

    try {
      const submitData = await submitBatch(prowJobBuildIDs)
      setProgress({
        requested: submitData.requested,
        completed: 0,
        failed: 0,
        running: 0,
        pending: submitData.requested,
        status: 'pending',
      })

      startPolling(submitData.batch_id)
    } catch (err) {
      setRunning(false)
      setSnackbar({
        severity: 'error',
        message: `Re-evaluation failed: ${err.message}`,
      })
    }
  }

  const isDisabled = disabled || running || !prowJobBuildIDs?.length

  let progressBar = null
  if (running && progress) {
    const terminal = progress.completed + progress.failed
    const pct =
      progress.requested > 0 ? (terminal / progress.requested) * 100 : 0

    progressBar = (
      <Stack spacing={0.5} sx={{ minWidth: 200 }}>
        <LinearProgress variant="determinate" value={pct} />
        <Typography variant="caption" color="text.secondary">
          {terminal}/{progress.requested} completed
          {progress.completed > 0 && ` (${progress.completed} succeeded)`}
          {progress.failed > 0 && `, ${progress.failed} failed`}
        </Typography>
      </Stack>
    )
  }

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
