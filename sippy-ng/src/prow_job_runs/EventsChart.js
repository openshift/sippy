import {
  Box,
  Button,
  Checkbox,
  CircularProgress,
  FormControl,
  FormControlLabel,
  InputLabel,
  Link,
  MenuItem,
  Paper,
  Select,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material'
import { makeStyles } from '@mui/styles'
import { Link as RouterLink, useParams } from 'react-router-dom'
import Alert from '@mui/material/Alert'
import LaunderedLink from '../components/Laundry'
import PropTypes from 'prop-types'
import React, {
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react'

const useStyles = makeStyles({
  filterRow: {
    padding: '10px 0',
    paddingBottom: '1rem',
  },
  controls: {
    marginBottom: '1rem',
  },
  columnToggle: {
    marginRight: '0.5rem',
    marginBottom: '0.5rem',
  },
})

const COLUMNS = [
  { id: 'firstTimestamp', label: 'First Timestamp', visible: true },
  { id: 'lastTimestamp', label: 'Last Timestamp', visible: true },
  { id: 'namespace', label: 'Namespace', visible: true },
  { id: 'kind', label: 'Kind', visible: true },
  { id: 'name', label: 'Name', visible: true },
  { id: 'type', label: 'Type', visible: true },
  { id: 'reason', label: 'Reason', visible: true },
  { id: 'message', label: 'Message', visible: true },
  { id: 'count', label: 'Count', visible: true },
  { id: 'source', label: 'Source', visible: false },
]

function formatTimestamp(ts) {
  if (!ts) return ''
  const d = new Date(ts)
  if (isNaN(d)) return ts
  return d.toISOString().replace('T', ' ').replace('.000Z', ' UTC')
}

function formatDateTimeLocalUTC(date) {
  const pad = (n) => n.toString().padStart(2, '0')
  return `${date.getUTCFullYear()}-${pad(date.getUTCMonth() + 1)}-${pad(
    date.getUTCDate()
  )}T${pad(date.getUTCHours())}:${pad(date.getUTCMinutes())}`
}

function buildIntervalsPath(jobrunid, jobname, repoinfo, pullnumber) {
  const parts = ['/job_runs', jobrunid]
  if (jobname) parts.push(jobname)
  if (repoinfo) parts.push(repoinfo)
  if (pullnumber) parts.push(pullnumber)
  return parts.join('/') + '/intervals'
}

export default function EventsChart(props) {
  const classes = useStyles()
  const { jobrunid, jobname, repoinfo, pullnumber } = useParams()
  const { jobRunID, jobName, repoInfo, pullNumber } = props
  const effectiveJobRunID = jobRunID ?? jobrunid
  const effectiveJobName = jobName ?? jobname
  const effectiveRepoInfo = repoInfo ?? repoinfo
  const effectivePullNumber = pullNumber ?? pullnumber

  const [fetchError, setFetchError] = useState('')
  const [isLoaded, setLoaded] = useState(false)
  const [allEvents, setAllEvents] = useState([])
  const [jobRunUrl, setJobRunUrl] = useState('')
  const [columns, setColumns] = useState(COLUMNS)
  const [sortColumn, setSortColumn] = useState('firstTimestamp')
  const [sortDirection, setSortDirection] = useState('desc')

  const [timeFrom, setTimeFrom] = useState('')
  const [timeTo, setTimeTo] = useState('')
  const [filterKind, setFilterKind] = useState('')
  const [filterNamespace, setFilterNamespace] = useState('')
  const [filterName, setFilterName] = useState('')
  const [filterType, setFilterType] = useState('')
  const [filterReason, setFilterReason] = useState('')
  const [filterMessage, setFilterMessage] = useState('')

  const fetchData = useCallback(() => {
    setFetchError('')
    const url =
      process.env.REACT_APP_API_URL +
      '/api/jobs/runs/events?prow_job_run_id=' +
      effectiveJobRunID +
      (effectiveJobName
        ? '&job_name=' + encodeURIComponent(effectiveJobName)
        : '') +
      (effectiveRepoInfo
        ? '&repo_info=' + encodeURIComponent(effectiveRepoInfo)
        : '') +
      (effectivePullNumber
        ? '&pull_number=' + encodeURIComponent(effectivePullNumber)
        : '')

    fetch(url)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        if (json != null) {
          setJobRunUrl(json.jobRunURL || '')
          setAllEvents(json.items || [])

          const events = json.items || []
          if (events.length > 0) {
            const timestamps = events
              .map((e) => new Date(e.firstTimestamp))
              .filter((d) => !isNaN(d))
            if (timestamps.length > 0) {
              const min = new Date(Math.min(...timestamps))
              const max = new Date(Math.max(...timestamps))
              setTimeFrom(formatDateTimeLocalUTC(min))
              setTimeTo(formatDateTimeLocalUTC(max))
            }
          }
        } else {
          setAllEvents([])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve events for jobRunID=' +
            jobRunID +
            ' jobName=' +
            jobName +
            ', ' +
            error
        )
        setLoaded(true)
      })
  }, [
    effectiveJobRunID,
    effectiveJobName,
    effectiveRepoInfo,
    effectivePullNumber,
  ])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const filteredEvents = useMemo(() => {
    const timeFromVal = timeFrom ? new Date(timeFrom + 'Z') : null
    const timeToVal = timeTo ? new Date(timeTo + 'Z') : null
    const filterNameLower = filterName.toLowerCase()
    const filterMessageLower = filterMessage.toLowerCase()

    return allEvents.filter((event) => {
      const eventTime = new Date(event.firstTimestamp)
      if (timeFromVal && eventTime < timeFromVal) return false
      if (timeToVal && eventTime > timeToVal) return false
      if (filterKind && event.kind !== filterKind) return false
      if (filterNamespace && event.namespace !== filterNamespace) return false
      if (filterName && !event.name?.toLowerCase().includes(filterNameLower))
        return false
      if (filterType && event.type !== filterType) return false
      if (filterReason && event.reason !== filterReason) return false
      if (
        filterMessage &&
        !event.message?.toLowerCase().includes(filterMessageLower)
      )
        return false
      return true
    })
  }, [
    allEvents,
    timeFrom,
    timeTo,
    filterKind,
    filterNamespace,
    filterName,
    filterType,
    filterReason,
    filterMessage,
  ])

  const sortedEvents = useMemo(() => {
    return [...filteredEvents].sort((a, b) => {
      let aVal = a[sortColumn]
      let bVal = b[sortColumn]

      if (sortColumn.includes('Timestamp')) {
        aVal = new Date(aVal || 0)
        bVal = new Date(bVal || 0)
      } else if (sortColumn === 'count') {
        aVal = parseInt(aVal) || 0
        bVal = parseInt(bVal) || 0
      }

      if (aVal < bVal) return sortDirection === 'asc' ? -1 : 1
      if (aVal > bVal) return sortDirection === 'asc' ? 1 : -1
      return 0
    })
  }, [filteredEvents, sortColumn, sortDirection])

  const filterOptions = useMemo(() => {
    const kinds = [...new Set(allEvents.map((e) => e.kind))]
      .filter(Boolean)
      .sort()
    const namespaces = [...new Set(allEvents.map((e) => e.namespace))]
      .filter(Boolean)
      .sort()
    const types = [...new Set(allEvents.map((e) => e.type))]
      .filter(Boolean)
      .sort()
    const reasons = [...new Set(allEvents.map((e) => e.reason))]
      .filter(Boolean)
      .sort()
    return { kinds, namespaces, types, reasons }
  }, [allEvents])

  const visibleColumns = columns.filter((c) => c.visible)

  const handleSort = (colId) => {
    if (sortColumn === colId) {
      setSortDirection((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortColumn(colId)
      setSortDirection('desc')
    }
  }

  const toggleColumn = (colId, visible) => {
    setColumns((prev) =>
      prev.map((c) => (c.id === colId ? { ...c, visible } : c))
    )
  }

  const clearFilters = () => {
    setFilterKind('')
    setFilterNamespace('')
    setFilterName('')
    setFilterType('')
    setFilterReason('')
    setFilterMessage('')
    if (allEvents.length > 0) {
      const timestamps = allEvents
        .map((e) => new Date(e.firstTimestamp))
        .filter((d) => !isNaN(d))
      if (timestamps.length > 0) {
        const min = new Date(Math.min(...timestamps))
        const max = new Date(Math.max(...timestamps))
        setTimeFrom(formatDateTimeLocalUTC(min))
        setTimeTo(formatDateTimeLocalUTC(max))
      }
    }
  }

  const warningCount = filteredEvents.filter((e) => e.type === 'Warning').length

  if (fetchError) {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return (
      <Fragment>
        <p>
          Loading events for job run: jobRunID={jobRunID}, jobName={jobName},
          pullNumber={pullNumber}, repoInfo={repoInfo}
        </p>
        <CircularProgress />
      </Fragment>
    )
  }

  return (
    <Fragment>
      <Box sx={{ mb: 2 }}>
        <Link
          component={RouterLink}
          to={buildIntervalsPath(
            effectiveJobRunID,
            effectiveJobName,
            effectiveRepoInfo,
            effectivePullNumber
          )}
        >
          View Intervals Chart
        </Link>
      </Box>
      <Typography variant="body1" gutterBottom>
        Loaded {allEvents.length} events from{' '}
        {jobRunUrl ? (
          <LaunderedLink address={jobRunUrl}>GCS job run</LaunderedLink>
        ) : (
          'GCS job run'
        )}
        , filtered down to {filteredEvents.length}.
      </Typography>

      <Paper className={classes.controls} sx={{ p: 2 }}>
        <Box className={classes.filterRow}>
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Time Range Filter (UTC)
          </Typography>
          <Box display="flex" gap={2} flexWrap="wrap" alignItems="flex-end">
            <TextField
              label="From (UTC)"
              type="datetime-local"
              value={timeFrom}
              onChange={(e) => setTimeFrom(e.target.value)}
              size="small"
              InputLabelProps={{ shrink: true }}
            />
            <TextField
              label="To (UTC)"
              type="datetime-local"
              value={timeTo}
              onChange={(e) => setTimeTo(e.target.value)}
              size="small"
              InputLabelProps={{ shrink: true }}
            />
          </Box>
        </Box>

        <Box className={classes.filterRow}>
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Filters
          </Typography>
          <Box display="flex" gap={2} flexWrap="wrap" alignItems="flex-end">
            <FormControl size="small" sx={{ minWidth: 120 }}>
              <InputLabel>Kind</InputLabel>
              <Select
                value={filterKind}
                label="Kind"
                onChange={(e) => setFilterKind(e.target.value)}
              >
                <MenuItem value="">All</MenuItem>
                {filterOptions.kinds.map((v) => (
                  <MenuItem key={v} value={v}>
                    {v}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 120 }}>
              <InputLabel>Namespace</InputLabel>
              <Select
                value={filterNamespace}
                label="Namespace"
                onChange={(e) => setFilterNamespace(e.target.value)}
              >
                <MenuItem value="">All</MenuItem>
                {filterOptions.namespaces.map((v) => (
                  <MenuItem key={v} value={v}>
                    {v}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <TextField
              label="Name (contains)"
              size="small"
              value={filterName}
              onChange={(e) => setFilterName(e.target.value)}
              placeholder="Search name..."
              sx={{ minWidth: 150 }}
            />
            <FormControl size="small" sx={{ minWidth: 100 }}>
              <InputLabel>Type</InputLabel>
              <Select
                value={filterType}
                label="Type"
                onChange={(e) => setFilterType(e.target.value)}
              >
                <MenuItem value="">All</MenuItem>
                {filterOptions.types.map((v) => (
                  <MenuItem key={v} value={v}>
                    {v}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 120 }}>
              <InputLabel>Reason</InputLabel>
              <Select
                value={filterReason}
                label="Reason"
                onChange={(e) => setFilterReason(e.target.value)}
              >
                <MenuItem value="">All</MenuItem>
                {filterOptions.reasons.map((v) => (
                  <MenuItem key={v} value={v}>
                    {v}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <TextField
              label="Message (contains)"
              size="small"
              value={filterMessage}
              onChange={(e) => setFilterMessage(e.target.value)}
              placeholder="Search message..."
              sx={{ minWidth: 200 }}
            />
            <Button variant="outlined" onClick={clearFilters}>
              Clear Filters
            </Button>
          </Box>
        </Box>

        <Box className={classes.filterRow}>
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Toggle Columns
          </Typography>
          <Box display="flex" flexWrap="wrap" gap={1}>
            {columns.map((col) => (
              <FormControlLabel
                key={col.id}
                control={
                  <Checkbox
                    checked={col.visible}
                    onChange={(e) => toggleColumn(col.id, e.target.checked)}
                  />
                }
                label={col.label}
                className={classes.columnToggle}
              />
            ))}
          </Box>
        </Box>

        <Box display="flex" gap={2} flexWrap="wrap" sx={{ mt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Total: {allEvents.length} events
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Filtered: {filteredEvents.length} events
          </Typography>
          <Typography variant="body2" color="error">
            Warnings: {warningCount}
          </Typography>
        </Box>
      </Paper>

      <TableContainer component={Paper} sx={{ maxHeight: '70vh' }}>
        <Table stickyHeader size="small">
          <TableHead>
            <TableRow>
              {visibleColumns.map((col) => (
                <TableCell
                  key={col.id}
                  onClick={() => handleSort(col.id)}
                  sx={{
                    cursor: 'pointer',
                    fontWeight: 600,
                    whiteSpace: 'nowrap',
                  }}
                >
                  {col.label}
                  {sortColumn === col.id &&
                    (sortDirection === 'asc' ? ' ▲' : ' ▼')}
                </TableCell>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {sortedEvents.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={visibleColumns.length}
                  align="center"
                  sx={{ py: 4 }}
                >
                  {allEvents.length === 0
                    ? 'No events.json found for this job run'
                    : 'No events match the current filters'}
                </TableCell>
              </TableRow>
            ) : (
              sortedEvents.map((event, idx) => (
                <TableRow key={idx} hover>
                  {visibleColumns.map((col) => {
                    let value = event[col.id]
                    if (col.id.includes('Timestamp')) {
                      value = formatTimestamp(value)
                    }
                    return (
                      <TableCell
                        key={col.id}
                        sx={{
                          ...(col.id === 'type' &&
                            event.type === 'Warning' && {
                              color: 'error.main',
                              fontWeight: 600,
                            }),
                          ...(col.id === 'message' && {
                            maxWidth: 400,
                            wordWrap: 'break-word',
                          }),
                          ...(col.id.includes('Timestamp') && {
                            fontFamily: 'monospace',
                            fontSize: '0.75rem',
                            whiteSpace: 'nowrap',
                          }),
                        }}
                      >
                        {value ?? ''}
                      </TableCell>
                    )
                  })}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Fragment>
  )
}

EventsChart.propTypes = {
  jobRunID: PropTypes.string.isRequired,
  jobName: PropTypes.string,
  repoInfo: PropTypes.string,
  pullNumber: PropTypes.string,
}
