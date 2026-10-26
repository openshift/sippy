import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material'
import { CompactLoadingBox } from './TriageStyledComponents'
import { formatDateToSeconds, relativeTime } from '../helpers'
import { History } from '@mui/icons-material'
import { styled } from '@mui/material/styles'
import PropTypes from 'prop-types'
import React, { Fragment, useState } from 'react'

const DiffPaper = styled(Paper, {
  shouldForwardProp: (prop) => prop !== 'difftype',
})(({ theme, difftype }) => ({
  padding: 8,
  ...(difftype === 'original' && {
    marginBottom: 8,
    backgroundColor: theme.palette.error.light,
  }),
  ...(difftype === 'modified' && {
    backgroundColor: theme.palette.success.light,
  }),
}))

export default function TriageAuditLogsModal({ triage }) {
  const [auditLogs, setAuditLogs] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [open, setOpen] = useState(false)

  const fetchAuditLogs = async () => {
    setLoading(true)
    setError('')

    try {
      const response = await fetch(triage.links.audit_logs)
      if (response.status !== 200) {
        throw new Error(`API server returned ${response.status}`)
      }
      const data = await response.json()
      setAuditLogs(data || [])
    } catch (err) {
      setError(err.toString())
    } finally {
      setLoading(false)
    }
  }

  const handleOpen = () => {
    setOpen(true)
  }

  const handleClose = () => {
    setOpen(false)
  }

  // Fetch audit logs when modal opens
  React.useEffect(() => {
    if (open) {
      fetchAuditLogs()
    }
  }, [open])

  const formatOperation = (operation) => {
    switch (operation?.toLowerCase()) {
      case 'create':
        return <Chip label="Created" color="success" size="small" />
      case 'update':
        return <Chip label="Updated" color="primary" size="small" />
      case 'delete':
        return <Chip label="Deleted" color="error" size="small" />
      default:
        return (
          <Chip label={operation || 'Unknown'} color="default" size="small" />
        )
    }
  }

  const renderChangeData = (log) => {
    if (!log.changes || log.changes.length === 0) {
      return (
        <Typography variant="body2" color="textSecondary">
          No material fields modified
        </Typography>
      )
    }

    return (
      <Box>
        {log.changes.map((change, index) => (
          <Paper key={index} variant="outlined" sx={{ mb: 2, p: 2 }}>
            <Typography variant="subtitle2" fontWeight="bold" gutterBottom>
              {change.field_name}
            </Typography>
            <Box sx={{ ml: 1 }}>
              {change.original && (
                <DiffPaper variant="outlined" difftype="original">
                  <Typography
                    variant="caption"
                    color="text.primary"
                    fontFamily="monospace"
                  >
                    - {change.original}
                  </Typography>
                </DiffPaper>
              )}
              {change.modified && (
                <DiffPaper variant="outlined" difftype="modified">
                  <Typography
                    variant="caption"
                    color="text.primary"
                    fontFamily="monospace"
                  >
                    + {change.modified}
                  </Typography>
                </DiffPaper>
              )}
            </Box>
          </Paper>
        ))}
      </Box>
    )
  }

  return (
    <Fragment>
      <Button
        onClick={handleOpen}
        variant="outlined"
        startIcon={<History />}
        sx={{ mr: 1 }}
      >
        View Audit Logs
      </Button>

      <Dialog open={open} onClose={handleClose} maxWidth="lg" fullWidth>
        <DialogTitle>
          <Typography variant="h6">Audit Logs</Typography>
        </DialogTitle>
        <DialogContent>
          {loading && (
            <CompactLoadingBox>
              <CircularProgress />
              <Typography variant="body1" sx={{ ml: 2 }}>
                Loading audit logs...
              </Typography>
            </CompactLoadingBox>
          )}

          {error && (
            <Box p={2}>
              <Typography color="error">
                Error loading audit logs: {error}
              </Typography>
            </Box>
          )}

          {!loading && !error && auditLogs.length === 0 && (
            <Box p={2}>
              <Typography variant="body1">
                No audit logs found for this triage.
              </Typography>
            </Box>
          )}

          {!loading && !error && auditLogs.length > 0 && (
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>Time</TableCell>
                  <TableCell>User</TableCell>
                  <TableCell>Action</TableCell>
                  <TableCell>Change</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {auditLogs.map((log, index) => (
                  <TableRow key={index}>
                    <TableCell>
                      {log.created_at ? (
                        <Tooltip title={formatDateToSeconds(log.created_at)}>
                          <Typography variant="body2">
                            {relativeTime(new Date(log.created_at), new Date())}
                          </Typography>
                        </Tooltip>
                      ) : (
                        'Unknown'
                      )}
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">
                        {log.user || 'Unknown'}
                      </Typography>
                    </TableCell>
                    <TableCell>{formatOperation(log.operation)}</TableCell>
                    <TableCell>
                      <Box sx={{ maxHeight: 400, overflow: 'auto' }}>
                        {renderChangeData(log)}
                      </Box>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose} color="primary">
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Fragment>
  )
}

TriageAuditLogsModal.propTypes = {
  triage: PropTypes.object.isRequired,
}
