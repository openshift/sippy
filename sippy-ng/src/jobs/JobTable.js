import { Backdrop, Box, Button, CircularProgress, Container, Tooltip, Typography } from '@material-ui/core'
import IconButton from '@material-ui/core/IconButton'
import { createTheme } from '@material-ui/core/styles'
import TextField from '@material-ui/core/TextField'
import {
  DataGrid,
  GridToolbarDensitySelector,
  GridToolbarFilterButton
} from '@material-ui/data-grid'
import { BugReport, GridOn } from '@material-ui/icons'
import ClearIcon from '@material-ui/icons/Clear'
import SearchIcon from '@material-ui/icons/Search'
import Alert from '@material-ui/lab/Alert'
import { makeStyles, withStyles } from '@material-ui/styles'
import clsx from 'clsx'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { ArrayParam, NumberParam, StringParam, useQueryParam } from 'use-query-params'
import BugzillaDialog from '../bugzilla/BugzillaDialog'
import GridToolbarPeriodSelector from '../datagrid/GridToolbarPeriodSelector'
import PassRateIcon from '../components/PassRateIcon'
import GridToolbarQueriesMenu from '../datagrid/GridToolbarQueriesMenu'
import { bugColor, weightedBugComparator } from '../bugzilla/BugzillaUtils'
import { JOB_THRESHOLDS } from '../constants'

function escapeRegExp (value) {
  return value.replace(/[-[\]{}()*+?.,\\^$|#\s]/g, '\\$&')
};

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    root: {
      padding: theme.spacing(0.5, 0.5, 0),
      justifyContent: 'space-between',
      display: 'flex',
      alignItems: 'flex-start',
      flexWrap: 'wrap'
    },
    textField: {
      [theme.breakpoints.down('xs')]: {
        width: '100%'
      },
      margin: theme.spacing(1, 0.5, 1.5),
      '& .MuiSvgIcon-root': {
        marginRight: theme.spacing(0.5)
      },
      '& .MuiInput-underline:before': {
        borderBottom: `1px solid ${theme.palette.divider}`
      }
    }
  }),
  { defaultTheme }
)

const styles = {
  good: {
    backgroundColor: defaultTheme.palette.success.light,
    color: 'black'
  },
  ok: {
    backgroundColor: defaultTheme.palette.warning.light,
    color: 'black'
  },
  failing: {
    backgroundColor: defaultTheme.palette.error.light,
    color: 'black'
  }
}

function JobSearchToolbar (props) {
  const classes = useStyles()

  return (
    <div className={classes.root}>
      <div>
        <GridToolbarFilterButton />
        <GridToolbarDensitySelector />
        <GridToolbarPeriodSelector selectPeriod={props.selectPeriod} period={props.period} />

        <GridToolbarQueriesMenu
            initialFilters={props.initialFilters}
            setFilters={props.requestFilter}
            allowedFilters={[
              {
                title: 'Has a linked bug',
                filter: 'hasBug',
                conflictsWith: 'noBug'
              },
              {
                title: 'No bug',
                filter: 'noBug',
                conflictsWith: 'hasBug'
              },
              {
                title: 'Upgrade jobs',
                filter: 'upgrade'
              },
              {
                title: 'More than 10 Runs',
                filter: 'runs'
              }
            ]}

        />

      </div>
      <TextField
        variant="standard"
        value={props.value}
        onChange={props.onChange}
        placeholder="Search…"
        InputProps={{
          startAdornment: <SearchIcon fontSize="small" />,
          endAdornment: (
            <IconButton
              title="Clear"
              aria-label="Clear"
              size="small"
              onClick={props.clearSearch}
            >
              <ClearIcon fontSize="small" />
            </IconButton>
          )
        }}
      />
    </div>
  )
}

JobSearchToolbar.propTypes = {
  clearSearch: PropTypes.func.isRequired,
  onChange: PropTypes.func.isRequired,
  selectPeriod: PropTypes.func.isRequired,
  period: PropTypes.string,
  value: PropTypes.string,
  initialFilters: PropTypes.array,
  requestFilter: PropTypes.func
}

/**
 * JobTable shows the list of all jobs matching any selected filters,
 * including current and previous pass percentages, net improvement, and
 * bug links.
 */
function JobTable (props) {
  const { classes } = props
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [jobs, setJobs] = React.useState([])
  const [rows, setRows] = React.useState([])

  const [searchText, setSearchText] = React.useState('')
  const [filterBy = props.filterBy, setFilterBy] = useQueryParam('filterBy', ArrayParam)
  const [sortBy = props.sortBy] = useQueryParam('sortBy', StringParam)
  const [limit = props.limit] = useQueryParam('limit', NumberParam)
  const [runs = props.runs] = useQueryParam('runs', NumberParam)
  const [variant = props.variant] = useQueryParam('variant', StringParam)
  const [period = props.period, setPeriod] = useQueryParam('period', StringParam)

  const [job = props.job] = useQueryParam('job', StringParam)

  const [isBugzillaDialogOpen, setBugzillaDialogOpen] = React.useState(false)
  const [jobDetails, setJobDetails] = React.useState({ bugs: [] })

  const columns = [
    {
      field: 'name',
      headerName: 'Name',
      flex: 3,
      renderCell: (params) => {
        return (
          <div style={{ display: 'block', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
            <Tooltip title={params.value}>
              <Link to={'/jobs/' + props.release + '/detail?job=' + params.row.name}>
                {props.briefTable ? params.row.brief_name : params.value}
              </Link>
            </Tooltip>
          </div>
        )
      }
    },
    {
      field: 'current_pass_percentage',
      headerName: 'Current Period',
      type: 'number',
      flex: 1,
      renderCell: (params) => (
        <Tooltip title={params.row.current_runs + ' runs'}>
          <Box>
            {Number(params.value).toFixed(2).toLocaleString()}%
          </Box>
        </Tooltip>
      )
    },
    {
      field: 'net_improvement',
      headerName: 'Improvement',
      type: 'number',
      flex: 0.5,
      renderCell: (params) => {
        return (
          <PassRateIcon tooltip={true} improvement={params.value} />
        )
      }
    },
    {
      field: 'previous_pass_percentage',
      headerName: 'Previous Period',
      flex: 1,
      type: 'number',
      renderCell: (params) => (
        <Tooltip title={params.row.current_runs + ' runs'}>
          <Box>
            {Number(params.value).toFixed(2).toLocaleString()}%
          </Box>
        </Tooltip>
      )
    },
    {
      field: 'test_grid_url',
      headerName: ' ',
      flex: 0.40,
      renderCell: (params) => {
        return (
          <Tooltip title="TestGrid">
            <Button style={{ justifyContent: 'center' }} target="_blank" startIcon={<GridOn />} href={params.value} />
          </Tooltip>
        )
      },
      filterable: false,
      hide: props.briefTable
    },
    {
      field: 'bugs',
      headerName: 'Bugs',
      flex: 0.40,
      type: 'number',
      valueGetter: (params) => params.value.length,
      renderCell: (params) => {
        return (
          <Tooltip title={params.value + ' linked bugs,' + params.row.associated_bugs.length + ' associated bugs'}>
            <Button style={{ justifyContent: 'center', color: bugColor(params.row) }} startIcon={<BugReport />} onClick={() => openBugzillaDialog(params.row)} />
          </Tooltip>
        )
      },
      // Weight linked bugs more than associated bugs, but associated bugs are ranked more than not having one at all.
      sortComparator: (v1, v2, param1, param2) => weightedBugComparator(
        param1.api.getCellValue(param1.id, 'bugs'),
        param1.api.getCellValue(param1.id, 'associated_bugs'),
        param2.api.getCellValue(param2.id, 'bugs'),
        param2.api.getCellValue(param2.id, 'associated_bugs')),
      hide: props.briefTable
    },

    // These are here just to allow filtering
    {
      field: 'variants',
      headerName: 'Variants',
      hide: true
    },
    {
      field: 'current_runs',
      headerName: 'Current runs',
      hide: true,
      type: 'number'
    },
    {
      field: 'previous_runs',
      headerName: 'Previous runs',
      hide: true,
      type: 'number'
    }
  ]

  const openBugzillaDialog = (job) => {
    setJobDetails(job)
    setBugzillaDialogOpen(true)
  }

  const closeBugzillaDialog = (details) => {
    setBugzillaDialogOpen(false)
  }

  const fetchData = () => {
    let queryString = ''
    if (filterBy) {
      filterBy.forEach((filter) => {
        if (filter === 'runs' && !runs) {
          queryString += '&runs=10'
        }
        queryString += '&filterBy=' + encodeURIComponent(filter)
      })
    }

    if (sortBy && sortBy !== '') {
      queryString += '&sortBy=' + encodeURIComponent(sortBy)
    }

    if (limit && limit !== '') {
      queryString += '&limit=' + encodeURIComponent(limit)
    }

    if (job && job !== '') {
      queryString += '&job=' + encodeURIComponent(job)
    }

    if (runs) {
      queryString += '&runs=' + encodeURIComponent(runs)
    }

    if (variant && variant !== '') {
      queryString += '&variant=' + encodeURIComponent(variant)
    }

    if (period) {
      queryString += '&period=' + encodeURIComponent(period)
    }

    fetch(process.env.REACT_APP_API_URL + '/api/jobs?release=' + props.release + queryString)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then(json => {
        setJobs(json)
        setRows(json)
        setLoaded(true)
      }).catch(error => {
        setFetchError('Could not retrieve jobs ' + props.release + ', ' + error)
      })
  }

  const requestSearch = (searchValue) => {
    setSearchText(searchValue)
    const searchRegex = new RegExp(escapeRegExp(searchValue), 'i')
    const filteredRows = jobs.filter((row) => {
      return Object.keys(row).some((field) => {
        return searchRegex.test(row[field].toString())
      })
    })
    setRows(filteredRows)
  }

  useEffect(() => {
    fetchData()
  }, [period, filterBy])

  const pageTitle = () => {
    if (props.title) {
      return (
        <Typography align="center" style={{ margin: 20 }} variant="h4">
          {props.title}
        </Typography>
      )
    }
  }

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return (
      <Backdrop className={classes.backdrop} open={!isLoaded}>
        Fetching data...
        <CircularProgress color="inherit" />
      </Backdrop>
    )
  }

  return (
    <Container size="xl">
      {pageTitle()}
      <DataGrid
        components={{ Toolbar: props.hideControls ? '' : JobSearchToolbar }}
        rows={rows}
        columns={columns}
        autoHeight={true}
        pageSize={props.pageSize}
        disableColumnFilter={props.briefTable}
        disableColumnMenu={true}
        rowsPerPageOptions={[5, 10, 25, 50]}
        getRowClassName={(params =>
          clsx({
            [classes.good]: (params.row.current_pass_percentage >= JOB_THRESHOLDS.success),
            [classes.ok]: (params.row.current_pass_percentage >= JOB_THRESHOLDS.warning && params.row.current_pass_percentage < JOB_THRESHOLDS.success),
            [classes.failing]: (params.row.current_pass_percentage >= JOB_THRESHOLDS.error && params.row.current_pass_percentage < JOB_THRESHOLDS.warning)
          })
        )}
        componentsProps={{
          toolbar: {
            onChange: (event) => requestSearch(event.target.value),
            clearSearch: () => requestSearch(''),
            value: searchText,
            period: period,
            selectPeriod: setPeriod,
            requestFilter: setFilterBy,
            initialFilters: filterBy
          }
        }}

      />
      <BugzillaDialog item={jobDetails} isOpen={isBugzillaDialogOpen} close={closeBugzillaDialog} />
    </Container>
  )
}

JobTable.defaultProps = {
  hideControls: false,
  pageSize: 25,
  briefTable: false
}

JobTable.propTypes = {
  briefTable: PropTypes.bool,
  classes: PropTypes.object,
  filterBy: PropTypes.array,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  release: PropTypes.string.isRequired,
  runs: PropTypes.number,
  sortBy: PropTypes.string,
  title: PropTypes.string,
  hideControls: PropTypes.bool,
  variant: PropTypes.string,
  period: PropTypes.string,
  job: PropTypes.string
}

export default withStyles(styles)(JobTable)
