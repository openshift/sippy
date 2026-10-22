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
import { Link } from 'react-router'
import {
  pathForExactTestAnalysis,
  relativeTime,
  safeEncodeURIComponent,
} from '../helpers'
import { styled } from '@mui/material/styles'
import { StyledInfoIcon } from './releaseStyledComponents'
import Alert from '@mui/material/Alert'
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

const ContentCard = styled(Card)(({ theme }) => ({
  padding: theme.spacing(2.5),
}))

const CardHeader = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  marginBottom: theme.spacing(2),
}))

const SummaryBar = styled(Box)(({ theme }) => ({
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
}))

const OutputCard = styled(Box)(({ theme }) => ({
  padding: theme.spacing(1.5),
  marginBottom: theme.spacing(1),
  borderRadius: theme.shape.borderRadius,
  border: '1px solid',
  borderColor: theme.palette.divider,
}))

const OutputPre = styled('pre')(({ theme }) => ({
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
}))

const TestNameBox = styled(Box, {
  shouldForwardProp: (prop) => prop !== 'clickable',
})(({ clickable }) => ({
  maxWidth: 500,
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
  ...(clickable && {
    cursor: 'pointer',
    '&:hover': { textDecoration: 'underline' },
  }),
}))

const OutputDetailBar = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  gap: theme.spacing(2),
}))

const JobNameText = styled(Typography)({
  fontWeight: 500,
  flex: 1,
  minWidth: 0,
})

const CenteredBox = styled(Box)(({ theme }) => ({
  textAlign: 'center',
  paddingTop: theme.spacing(4),
  paddingBottom: theme.spacing(4),
}))

const EmptyStateIcon = styled(ExpandMoreIcon)(({ theme }) => ({
  fontSize: 48,
  opacity: 0.3,
  marginBottom: theme.spacing(1),
}))

function FailureRow({ row, release, defaultExpanded }) {
  const [open, setOpen] = React.useState(defaultExpanded)
  const hasOutputs = row.outputs && row.outputs.length > 0

  return (
    <Fragment>
      <TableRow
        hover
        sx={{
          '& > *': { borderBottom: 'unset' },
          cursor: hasOutputs ? 'pointer' : 'default',
        }}
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
            <TestNameBox
              clickable={hasOutputs}
              onClick={(e) => {
                e.stopPropagation()
                if (hasOutputs) setOpen(!open)
              }}
            >
              <Typography variant="body2" color="primary" noWrap>
                {row.test_name}
              </Typography>
            </TestNameBox>
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
            sx={{ fontWeight: 'bold' }}
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
          <TableCell colSpan={5} sx={{ paddingTop: 0, paddingBottom: 0 }}>
            <Collapse in={open} timeout="auto" unmountOnExit>
              <Box sx={(theme) => ({ margin: theme.spacing(1) })}>
                <SummaryBar>
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
                  <Box sx={{ flex: 1 }} />
                  <Typography variant="caption" color="text.secondary">
                    {row.outputs.length} failure
                    {row.outputs.length !== 1 ? 's' : ''} recorded
                  </Typography>
                </SummaryBar>
                {row.outputs.slice(0, 5).map((output, idx) => (
                  <OutputCard key={idx}>
                    <OutputDetailBar>
                      <JobNameText variant="body2" noWrap>
                        {output.prow_job_name}
                      </JobNameText>
                      <Tooltip
                        title={new Date(output.failed_at).toLocaleString()}
                      >
                        <Typography
                          variant="body2"
                          color="text.secondary"
                          sx={{ whiteSpace: 'nowrap' }}
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
                          sx={{ minWidth: 'auto' }}
                        />
                      </Tooltip>
                    </OutputDetailBar>
                    {output.output ? (
                      <OutputPre>
                        {output.output.substring(0, 500)}
                        {output.output.length > 500 ? '...' : ''}
                      </OutputPre>
                    ) : (
                      <Typography variant="caption" color="text.secondary">
                        No output captured
                      </Typography>
                    )}
                  </OutputCard>
                ))}
                {row.outputs.length > 5 && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={(theme) => ({
                      display: 'block',
                      marginTop: theme.spacing(1),
                    })}
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
  defaultExpanded: PropTypes.bool,
  release: PropTypes.string.isRequired,
  row: PropTypes.object.isRequired,
}

export default function RecentTestFailures(props) {
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
      import.meta.env.VITE_API_URL +
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
      <ContentCard elevation={5}>
        <Alert severity="error">{fetchError}</Alert>
      </ContentCard>
    )
  }

  const title = props.title || 'New Test Failures'

  return (
    <ContentCard elevation={5}>
      <CardHeader>
        <ErrorOutlineIcon
          sx={(theme) => ({
            marginRight: theme.spacing(1),
            color: theme.palette.error.main,
          })}
        />
        <Typography
          variant="h6"
          sx={{ textDecoration: 'none', color: 'inherit' }}
        >
          {title}
        </Typography>
        <Tooltip
          title={`Tests that failed in the last ${period} but did not fail in the ${previousPeriod} before that. This helps surface new regressions.`}
        >
          <StyledInfoIcon />
        </Tooltip>
        <Box sx={{ flex: 1 }} />
        {isLoaded && (
          <Chip
            label={`${data.total_rows} test${data.total_rows !== 1 ? 's' : ''}`}
            size="small"
            variant="outlined"
          />
        )}
      </CardHeader>

      {!isLoaded ? (
        <CenteredBox>
          <CircularProgress size={36} />
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            Loading recent failures...
          </Typography>
        </CenteredBox>
      ) : data.rows === null || data.rows.length === 0 ? (
        <Box
          sx={(theme) => ({ textAlign: 'center', padding: theme.spacing(4) })}
        >
          <EmptyStateIcon />
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
    </ContentCard>
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
