import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Snackbar,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import { Block, CheckCircle, Warning } from '@mui/icons-material'
import { formatDateToSeconds } from '../helpers'
import {
  getForceClosePreviewUrl,
  getForceCloseRegressionsUrl,
} from './CompReadyUtils'
import { makeStyles } from '@mui/styles'
import { SippyCapabilitiesContext } from '../App'
import PropTypes from 'prop-types'
import React, { Fragment, useContext, useState } from 'react'

const useStyles = makeStyles((theme) => ({
  triggerButton: {
    marginLeft: '10px',
  },
  loadingContainer: {
    padding: theme.spacing(3),
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
  },
  loadingText: {
    marginLeft: theme.spacing(2),
  },
  alert: {
    marginBottom: theme.spacing(2),
    marginTop: theme.spacing(2),
  },
  description: {
    marginBottom: theme.spacing(2),
  },
  spacingTop: {
    marginTop: theme.spacing(2),
  },
  tableContainer: {
    maxHeight: 300,
    overflow: 'auto',
    marginBottom: theme.spacing(2),
  },
  sectionHeading: {
    marginTop: theme.spacing(2),
  },
  dateCell: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
  },
}))

// ForceCloseRegressionsModal renders a button that, for a resolved triage, lets a
// user preview and then force close the regressions associated with the triage.
// It is self contained: it checks the write_endpoints capability itself and
// renders nothing when the current user is not permitted to perform writes.
export default function ForceCloseRegressionsModal({
  triageId,
  resolved,
  setIsUpdated,
}) {
  const classes = useStyles()
  const capabilitiesContext = useContext(SippyCapabilitiesContext)
  const writeEndpointsEnabled = capabilitiesContext.includes('write_endpoints')

  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [preview, setPreview] = useState(null)
  const [error, setError] = useState('')
  const [reason, setReason] = useState('')
  const [reasonError, setReasonError] = useState('')
  const [alertText, setAlertText] = useState('')
  const [alertSeverity, setAlertSeverity] = useState('success')

  const handleAlertClose = (event, closeReason) => {
    if (closeReason === 'clickaway') {
      return
    }
    setAlertText('')
    setAlertSeverity('success')
  }

  // Parse an error response body, falling back to the HTTP status when the body
  // is not the expected JSON error shape.
  const parseErrorResponse = (response) => {
    return response.json().then(
      (data) => {
        throw new Error(
          data?.message
            ? `${data.code || response.status}: ${data.message}`
            : `API server returned ${response.status}`
        )
      },
      () => {
        throw new Error(`API server returned ${response.status}`)
      }
    )
  }

  // Fetch the preview when the modal opens. Uses an AbortController so an
  // in-flight request is cancelled if the modal is closed before it resolves.
  React.useEffect(() => {
    if (!open) {
      return
    }
    const abortController = new AbortController()
    setLoading(true)
    setError('')
    setPreview(null)

    fetch(getForceClosePreviewUrl(triageId), {
      signal: abortController.signal,
    })
      .then((response) => {
        if (!response.ok) {
          return parseErrorResponse(response)
        }
        return response.json()
      })
      .then((data) => {
        setPreview(data)
        setLoading(false)
      })
      .catch((err) => {
        if (err.name === 'AbortError') {
          return
        }
        setError(err.toString())
        setLoading(false)
      })

    return () => {
      abortController.abort()
    }
  }, [open, triageId])

  const handleOpen = () => {
    setReason('')
    setReasonError('')
    setError('')
    setOpen(true)
  }

  const handleClose = () => {
    // Prevent closing while a submission is in flight so we don't leave the
    // operation in an ambiguous state for the user.
    if (submitting) {
      return
    }
    setOpen(false)
  }

  const handleConfirm = () => {
    if (reason.trim().length === 0) {
      setReasonError('A reason is required to force close regressions')
      return
    }
    setReasonError('')
    setSubmitting(true)

    fetch(getForceCloseRegressionsUrl(triageId), {
      method: 'POST',
      body: JSON.stringify({ reason: reason.trim() }),
    })
      .then((response) => {
        if (!response.ok) {
          return parseErrorResponse(response)
        }
        return response.json()
      })
      .then((data) => {
        const count = data?.closed_regression_ids?.length || 0
        setAlertText(
          `Successfully force closed ${count} regression${
            count === 1 ? '' : 's'
          }`
        )
        setAlertSeverity('success')
        setSubmitting(false)
        setOpen(false)
        // Signal the parent to re-fetch so the newly closed regressions render.
        setIsUpdated(true)
      })
      .catch((err) => {
        setAlertText('Error force closing regressions: ' + err.toString())
        setAlertSeverity('error')
        setSubmitting(false)
      })
  }

  const formatDate = (dateString) =>
    dateString ? formatDateToSeconds(dateString) : 'None'

  const renderRegressionTable = (regressions, emptyText) => {
    if (!regressions || regressions.length === 0) {
      return (
        <Typography variant="body2" color="textSecondary">
          {emptyText}
        </Typography>
      )
    }
    return (
      <Box className={classes.tableContainer}>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>Test Name</TableCell>
              <TableCell>Last Failure Before Resolution</TableCell>
              <TableCell>First Failure After Resolution</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {regressions.map((regression) => (
              <TableRow key={regression.regression_id}>
                <TableCell>
                  <div className="test-name">{regression.test_name}</div>
                </TableCell>
                <TableCell>
                  {formatDate(regression.last_failure_before_resolution)}
                </TableCell>
                <TableCell>
                  {regression.first_failure_after_resolution ? (
                    <Tooltip title="Failures were observed after the triage resolution date. Force closing anyway will close this regression.">
                      <Box className={classes.dateCell}>
                        <Warning color="warning" fontSize="small" />
                        {formatDate(regression.first_failure_after_resolution)}
                      </Box>
                    </Tooltip>
                  ) : (
                    <Tooltip title="No failures observed after the triage resolution date.">
                      <Box className={classes.dateCell}>
                        <CheckCircle color="success" fontSize="small" />
                        None
                      </Box>
                    </Tooltip>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Box>
    )
  }

  // The button is hidden entirely when the user lacks the write capability.
  if (!writeEndpointsEnabled) {
    return null
  }

  const tooltipTitle = resolved
    ? ''
    : 'Triage must be resolved before regressions can be force closed'

  const wouldClose = preview?.would_close || []
  const wouldNotClose = preview?.would_not_close || []
  const nothingToClose =
    preview !== null && !loading && !error && wouldClose.length === 0
  const confirmDisabled =
    loading || submitting || error !== '' || wouldClose.length === 0

  return (
    <Fragment>
      <Snackbar
        open={alertText.length > 0}
        autoHideDuration={10000}
        onClose={handleAlertClose}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        <Alert onClose={handleAlertClose} severity={alertSeverity}>
          {alertText}
        </Alert>
      </Snackbar>

      <Tooltip title={tooltipTitle}>
        <span>
          <Button
            onClick={handleOpen}
            variant="contained"
            color="warning"
            disabled={!resolved}
            startIcon={<Block />}
            className={classes.triggerButton}
          >
            Force Close Regressions
          </Button>
        </span>
      </Tooltip>

      <Dialog open={open} onClose={handleClose} maxWidth="lg" fullWidth>
        <DialogTitle>Force Close Regressions</DialogTitle>
        <DialogContent>
          {loading && (
            <Box className={classes.loadingContainer}>
              <CircularProgress />
              <Typography variant="body1" className={classes.loadingText}>
                Loading preview...
              </Typography>
            </Box>
          )}

          {!loading && error && (
            <Alert severity="error" className={classes.alert}>
              {error}
            </Alert>
          )}

          {!loading && !error && preview && (
            <Fragment>
              <DialogContentText className={classes.description}>
                Review the regressions below before force closing. Force closing
                marks the eligible regressions as closed. This operation is
                idempotent; regressions that are already closed are skipped.
              </DialogContentText>

              <Typography variant="h6" className={classes.sectionHeading}>
                Would Close ({wouldClose.length})
              </Typography>
              {renderRegressionTable(
                wouldClose,
                'No regressions are eligible to be force closed (they may already be closed).'
              )}

              <Typography variant="h6" className={classes.sectionHeading}>
                Would Not Close ({wouldNotClose.length})
              </Typography>
              {renderRegressionTable(
                wouldNotClose,
                'All associated regressions are eligible to be force closed.'
              )}

              {nothingToClose ? (
                <Alert severity="info" className={classes.spacingTop}>
                  There are no regressions to force close.
                </Alert>
              ) : (
                <TextField
                  autoFocus
                  required
                  name="reason"
                  label="Reason"
                  value={reason}
                  onChange={(e) => {
                    setReason(e.target.value)
                    if (e.target.value.trim().length > 0) {
                      setReasonError('')
                    }
                  }}
                  fullWidth
                  multiline
                  minRows={2}
                  error={reasonError.length > 0}
                  helperText={
                    reasonError ||
                    'Required. Explain why these regressions are being force closed.'
                  }
                  disabled={submitting}
                  className={classes.spacingTop}
                />
              )}
            </Fragment>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose} color="secondary" disabled={submitting}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirm}
            variant="contained"
            color="warning"
            disabled={confirmDisabled}
            startIcon={submitting ? <CircularProgress size={16} /> : null}
          >
            {submitting ? 'Force Closing...' : 'Force Close'}
          </Button>
        </DialogActions>
      </Dialog>
    </Fragment>
  )
}

ForceCloseRegressionsModal.propTypes = {
  triageId: PropTypes.oneOfType([PropTypes.number, PropTypes.string])
    .isRequired,
  resolved: PropTypes.bool,
  setIsUpdated: PropTypes.func.isRequired,
}
