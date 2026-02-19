import {
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  Collapse,
  IconButton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TableSortLabel,
  Tooltip,
  Typography,
} from '@mui/material'
import { DirectionsBoat, OpenInNew } from '@mui/icons-material'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import {
  pathForExactTestAnalysis,
  relativeTime,
  safeEncodeURIComponent,
} from '../helpers'
import Alert from '@mui/material/Alert'
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import InfoIcon from '@mui/icons-material/Info'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

const useStyles = makeStyles((theme) => ({
  card: {
    padding: theme.spacing(2.5),
  },
  cardHeader: {
    display: 'flex',
    alignItems: 'center',
    marginBottom: theme.spacing(2),
  },
  cardTitle: {
    textDecoration: 'none',
    color: 'inherit',
  },
  infoIcon: {
    marginLeft: theme.spacing(1),
    fontSize: 20,
    opacity: 0.6,
  },
  headerIcon: {
    marginRight: theme.spacing(1),
    color: theme.palette.error.main,
  },
  flexSpacer: {
    flex: 1,
  },
  failureChip: {
    fontWeight: 'bold',
  },
  expandableRow: {
    '& > *': { borderBottom: 'unset' },
  },
  testNameCell: {
    maxWidth: 500,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  clickableTestName: {
    cursor: 'pointer',
    '&:hover': {
      textDecoration: 'underline',
    },
  },
  collapseCell: {
    paddingTop: 0,
    paddingBottom: 0,
  },
  collapseContent: {
    margin: theme.spacing(1),
  },
  summaryBar: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    marginBottom: theme.spacing(1.5),
    padding: theme.spacing(1),
    borderRadius: theme.shape.borderRadius,
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255,255,255,0.04)'
        : 'rgba(0,0,0,0.03)',
  },
  outputCard: {
    padding: theme.spacing(1.5),
    marginBottom: theme.spacing(1),
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: theme.palette.divider,
  },
  outputHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(2),
  },
  jobName: {
    fontWeight: 500,
    flex: 1,
    minWidth: 0,
  },
  noWrap: {
    whiteSpace: 'nowrap',
  },
  prowButton: {
    minWidth: 'auto',
  },
  outputPre: {
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-all',
    fontSize: '0.75rem',
    maxHeight: 150,
    overflow: 'auto',
    backgroundColor: theme.palette.mode === 'dark' ? '#1a1a2e' : '#f5f5f5',
    padding: theme.spacing(1),
    borderRadius: theme.spacing(0.5),
    margin: 0,
    marginTop: theme.spacing(1),
  },
  moreText: {
    display: 'block',
    marginTop: theme.spacing(1),
  },
  loadingContainer: {
    textAlign: 'center',
    paddingTop: theme.spacing(4),
    paddingBottom: theme.spacing(4),
  },
  loadingText: {
    marginTop: theme.spacing(1),
  },
  emptyState: {
    textAlign: 'center',
    padding: theme.spacing(4),
  },
  emptyIcon: {
    fontSize: 48,
    opacity: 0.3,
    marginBottom: theme.spacing(1),
  },
}))

function FailureRow({ row, release, classes, defaultExpanded }) {
  const [open, setOpen] = React.useState(defaultExpanded)
  const hasOutputs = row.outputs && row.outputs.length > 0

  return (
    <Fragment>
      <TableRow
        hover
        className={classes.expandableRow}
        style={{ cursor: hasOutputs ? 'pointer' : 'default' }}
        onClick={() => {
          if (hasOutputs) setOpen(!open)
        }}
      >
        <TableCell padding="checkbox">
          {hasOutputs && (
            <IconButton size="small" onClick={() => setOpen(!open)}>
              {open ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
            </IconButton>
          )}
        </TableCell>
        <TableCell>
          <Tooltip title={row.test_name} enterDelay={500}>
            <Box
              className={`${classes.testNameCell} ${
                hasOutputs ? classes.clickableTestName : ''
              }`}
              onClick={(e) => {
                e.stopPropagation()
                if (hasOutputs) setOpen(!open)
              }}
            >
              <Typography variant="body2" color="primary" noWrap>
                {row.test_name}
              </Typography>
            </Box>
          </Tooltip>
          {row.suite_name && (
            <Typography
              variant="caption"
              color="text.secondary"
              display="block"
            >
              Suite: {row.suite_name}
            </Typography>
          )}
        </TableCell>
        <TableCell align="center">
          <Chip
            label={row.failure_count}
            color="error"
            size="small"
            className={classes.failureChip}
          />
        </TableCell>
        <TableCell>
          <Tooltip title={new Date(row.last_failure).toLocaleString()}>
            <Typography variant="body2">
              {relativeTime(new Date(row.last_failure), Date.now())}
            </Typography>
          </Tooltip>
        </TableCell>
        <TableCell align="center">
          <Tooltip title="View test details">
            <IconButton
              size="small"
              component={Link}
              to={pathForExactTestAnalysis(release, row.test_name)}
              onClick={(e) => e.stopPropagation()}
            >
              <OpenInNew fontSize="small" />
            </IconButton>
          </Tooltip>
        </TableCell>
      </TableRow>

      {hasOutputs && (
        <TableRow>
          <TableCell colSpan={5} className={classes.collapseCell}>
            <Collapse in={open} timeout="auto" unmountOnExit>
              <Box className={classes.collapseContent}>
                <Box className={classes.summaryBar}>
                  <Tooltip title={new Date(row.first_failure).toLocaleString()}>
                    <Chip
                      label={`First failure: ${relativeTime(
                        new Date(row.first_failure),
                        Date.now()
                      )}`}
                      size="small"
                      color="error"
                    />
                  </Tooltip>
                  <Tooltip title={new Date(row.last_failure).toLocaleString()}>
                    <Chip
                      label={`Last failure: ${relativeTime(
                        new Date(row.last_failure),
                        Date.now()
                      )}`}
                      size="small"
                      color="error"
                    />
                  </Tooltip>
                  {row.last_pass && (
                    <Tooltip title={new Date(row.last_pass).toLocaleString()}>
                      <Chip
                        label={`Last pass: ${relativeTime(
                          new Date(row.last_pass),
                          Date.now()
                        )}`}
                        size="small"
                        color="success"
                      />
                    </Tooltip>
                  )}
                  <Box className={classes.flexSpacer} />
                  <Typography variant="caption" color="text.secondary">
                    {row.outputs.length} failure
                    {row.outputs.length !== 1 ? 's' : ''} recorded
                  </Typography>
                </Box>
                {row.outputs.slice(0, 5).map((output, idx) => (
                  <Box key={idx} className={classes.outputCard}>
                    <Box className={classes.outputHeader}>
                      <Typography
                        variant="body2"
                        noWrap
                        className={classes.jobName}
                      >
                        {output.prow_job_name}
                      </Typography>
                      <Tooltip
                        title={new Date(output.failed_at).toLocaleString()}
                      >
                        <Typography
                          variant="body2"
                          color="text.secondary"
                          className={classes.noWrap}
                        >
                          {relativeTime(new Date(output.failed_at), Date.now())}
                        </Typography>
                      </Tooltip>
                      <Tooltip title="View in Prow">
                        <Button
                          size="small"
                          target="_blank"
                          startIcon={<DirectionsBoat />}
                          href={encodeURI(output.prow_job_url)}
                          onClick={(e) => e.stopPropagation()}
                          className={classes.prowButton}
                        />
                      </Tooltip>
                    </Box>
                    {output.output ? (
                      <pre className={classes.outputPre}>
                        {output.output.substring(0, 500)}
                        {output.output.length > 500 ? '...' : ''}
                      </pre>
                    ) : (
                      <Typography variant="caption" color="text.secondary">
                        No output captured
                      </Typography>
                    )}
                  </Box>
                ))}
                {row.outputs.length > 5 && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    className={classes.moreText}
                  >
                    ...and {row.outputs.length - 5} more
                  </Typography>
                )}
              </Box>
            </Collapse>
          </TableCell>
        </TableRow>
      )}
    </Fragment>
  )
}

FailureRow.propTypes = {
  classes: PropTypes.object.isRequired,
  defaultExpanded: PropTypes.bool,
  release: PropTypes.string.isRequired,
  row: PropTypes.object.isRequired,
}

export default function RecentTestFailures(props) {
  const classes = useStyles()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({ rows: [], total_rows: 0 })
  const [orderBy, setOrderBy] = React.useState(
    props.sortField || 'failure_count'
  )
  const [order, setOrder] = React.useState(props.sort || 'desc')
  const [page, setPage] = React.useState(0)
  const [rowsPerPage, setRowsPerPage] = React.useState(props.limit || 5)

  const period = props.period || '24h'
  const previousPeriod = props.previousPeriod || '72h'
  const includeOutputs =
    props.includeOutputs !== undefined ? props.includeOutputs : true

  const fetchData = () => {
    setLoaded(false)
    setFetchError('')
    let url =
      process.env.REACT_APP_API_URL +
      '/api/tests/recent_failures' +
      `?release=${safeEncodeURIComponent(props.release)}` +
      `&period=${safeEncodeURIComponent(period)}` +
      `&previousPeriod=${safeEncodeURIComponent(previousPeriod)}` +
      `&includeOutputs=${includeOutputs}` +
      `&sortField=${safeEncodeURIComponent(orderBy)}` +
      `&sort=${safeEncodeURIComponent(order)}` +
      `&perPage=${rowsPerPage}` +
      `&page=${page}`

    fetch(url)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setData(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve recent test failures for ' +
            props.release +
            ': ' +
            error
        )
        setLoaded(true)
      })
  }

  useEffect(() => {
    fetchData()
  }, [props.release, period, previousPeriod, orderBy, order, page, rowsPerPage])

  const handleSort = (field) => {
    const isAsc = orderBy === field && order === 'asc'
    setOrder(isAsc ? 'desc' : 'asc')
    setOrderBy(field)
    setPage(0)
  }

  if (fetchError !== '') {
    return (
      <Card elevation={5} className={classes.card}>
        <Alert severity="error">{fetchError}</Alert>
      </Card>
    )
  }

  const title = props.title || 'New Test Failures'

  return (
    <Card elevation={5} className={classes.card}>
      <Box className={classes.cardHeader}>
        <ErrorOutlineIcon className={classes.headerIcon} />
        <Typography variant="h6" className={classes.cardTitle}>
          {title}
        </Typography>
        <Tooltip
          title={`Tests that failed in the last ${period} but did not fail in the ${previousPeriod} before that. This helps surface new regressions.`}
        >
          <InfoIcon className={classes.infoIcon} />
        </Tooltip>
        <Box className={classes.flexSpacer} />
        {isLoaded && (
          <Chip
            label={`${data.total_rows} test${data.total_rows !== 1 ? 's' : ''}`}
            size="small"
            variant="outlined"
          />
        )}
      </Box>

      {!isLoaded ? (
        <Box className={classes.loadingContainer}>
          <CircularProgress size={36} />
          <Typography
            variant="body2"
            color="text.secondary"
            className={classes.loadingText}
          >
            Loading recent failures...
          </Typography>
        </Box>
      ) : data.rows === null || data.rows.length === 0 ? (
        <Box className={classes.emptyState}>
          <ExpandMoreIcon className={classes.emptyIcon} />
          <Typography variant="body1" color="text.secondary">
            No new test failures detected
          </Typography>
          <Typography variant="caption" color="text.secondary">
            No tests started failing in the last {period} that were not already
            failing in the prior {previousPeriod}.
          </Typography>
        </Box>
      ) : (
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell padding="checkbox" />
                <TableCell>
                  <TableSortLabel
                    active={orderBy === 'test_name'}
                    direction={orderBy === 'test_name' ? order : 'asc'}
                    onClick={() => handleSort('test_name')}
                  >
                    Test
                  </TableSortLabel>
                </TableCell>
                <TableCell align="center">
                  <TableSortLabel
                    active={orderBy === 'failure_count'}
                    direction={orderBy === 'failure_count' ? order : 'desc'}
                    onClick={() => handleSort('failure_count')}
                  >
                    Failures
                  </TableSortLabel>
                </TableCell>
                <TableCell>
                  <TableSortLabel
                    active={orderBy === 'last_failure'}
                    direction={orderBy === 'last_failure' ? order : 'desc'}
                    onClick={() => handleSort('last_failure')}
                  >
                    Last Seen
                  </TableSortLabel>
                </TableCell>
                <TableCell align="center">Details</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {data.rows.map((row, idx) => (
                <FailureRow
                  key={row.test_id || idx}
                  row={row}
                  release={props.release}
                  classes={classes}
                  defaultExpanded={false}
                />
              ))}
            </TableBody>
          </Table>
          <TablePagination
            component="div"
            count={data.total_rows}
            page={page}
            onPageChange={(e, newPage) => setPage(newPage)}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10))
              setPage(0)
            }}
            rowsPerPageOptions={[5, 10, 25]}
          />
        </TableContainer>
      )}
    </Card>
  )
}

RecentTestFailures.propTypes = {
  release: PropTypes.string.isRequired,
  period: PropTypes.string,
  previousPeriod: PropTypes.string,
  includeOutputs: PropTypes.bool,
  limit: PropTypes.number,
  sortField: PropTypes.string,
  sort: PropTypes.string,
  title: PropTypes.string,
}
