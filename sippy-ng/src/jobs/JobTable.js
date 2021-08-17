import { Backdrop, Box, Button, CircularProgress, Container, Tooltip, Typography } from '@material-ui/core'
import {
  DataGrid
} from '@material-ui/data-grid'
import { BugReport, GridOn } from '@material-ui/icons'
import Alert from '@material-ui/lab/Alert'
import { withStyles } from '@material-ui/styles'
import clsx from 'clsx'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { StringParam, useQueryParam } from 'use-query-params'
import BugzillaDialog from '../bugzilla/BugzillaDialog'
import { bugColor, weightedBugComparator } from '../bugzilla/BugzillaUtils'
import PassRateIcon from '../components/PassRateIcon'
import { BOOKMARKS, JOB_THRESHOLDS } from '../constants'
import GridToolbar from '../datagrid/GridToolbar'
import { ROW_STYLES } from '../datagrid/utils'

const bookmarks = [
  { name: 'Runs > 10', model: [BOOKMARKS.RUN_10] },
  { name: 'Upgrade related', model: [BOOKMARKS.UPGRADE] },
  { name: 'Has a linked bug', model: [BOOKMARKS.LINKED_BUG] },
  { name: 'Has no linked bug', model: [BOOKMARKS.NO_LINKED_BUG] },
  { name: 'Has an associated bug', model: [BOOKMARKS.ASSOCIATED_BUG] },
  { name: 'Has no associated bug', model: [BOOKMARKS.NO_ASSOCIATED_BUG] }
]

/**
 * JobTable shows the list of all jobs matching any selected filters,
 * including current and previous pass percentages, net improvement, and
 * bug links.
 */
function JobTable (props) {
  const { classes } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

  const [period = props.period, setPeriod] = useQueryParam('period', StringParam)

  const [filterModel, setFilterModel] = React.useState(props.filterModel)
  const [filters = JSON.stringify(props.filterModel), setFilters] = useQueryParam('filters', StringParam)

  const [sortField = 'net_improvement', setSortField] = useQueryParam('sortField', StringParam)
  const [sort = 'asc', setSort] = useQueryParam('sort', StringParam)

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
      filterable: true,
      renderCell: (params) => {
        return (
          <Tooltip title={params.value.length + ' linked bugs,' + params.row.associated_bugs.length + ' associated bugs'}>
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
    },
    {
      field: 'associated_bugs',
      headerName: 'Associated bugs',
      type: 'number',
      hide: true
    },
    {
      field: 'tags',
      headerName: 'Tags',
      hide: true
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
    if (filters && filters !== '') {
      queryString += '&filter=' + encodeURIComponent(filters)
    }

    if (props.limit > 0) {
      queryString += '&limit=' + encodeURIComponent(props.limit)
    }

    if (period) {
      queryString += '&period=' + encodeURIComponent(period)
    }

    queryString += '&sortField=' + encodeURIComponent(sortField)
    queryString += '&sort=' + encodeURIComponent(sort)

    fetch(process.env.REACT_APP_API_URL + '/api/jobs?release=' + props.release + queryString)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then(json => {
        setRows(json)
        setLoaded(true)
      }).catch(error => {
        setFetchError('Could not retrieve jobs ' + props.release + ', ' + error)
      })
  }

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter((f) => f.columnField !== 'name')
    currentFilters.items.push({ id: 99, columnField: 'name', operatorValue: 'contains', value: searchValue })
    setFilters(JSON.stringify(currentFilters))
  }

  useEffect(() => {
    if (filters && filters !== '') {
      setFilterModel(JSON.parse(filters))
    }

    fetchData()
  }, [period, filters, sort, sortField])

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

  const addFilters = (filter) => {
    const currentFilters = filterModel
    filter.forEach((item) => {
      currentFilters.items.push(item)
    })
    setFilters(JSON.stringify(currentFilters))
  }

  const updateSortModel = (model) => {
    if (model.length === 0) {
      return
    }

    if (sort !== model[0].sort) {
      setSort(model[0].sort)
    }

    if (sortField !== model[0].field) {
      setSortField(model[0].field)
    }
  }

  return (
    /* eslint-disable react/prop-types */
    <Container size="xl">
      {pageTitle()}
      <DataGrid
        components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
        rows={rows}
        columns={columns}
        autoHeight={true}

        // Filtering:
        filterMode="server"
        filterModel={filterModel}
        onFilterModelChange={(m) => setFilters(JSON.stringify(m))}
        sortingOrder={['desc', 'asc']}
        sortModel={[{
          field: sortField,
          sort: sort
        }]}

        // Sorting:
        onSortModelChange={(m) => updateSortModel(m)}
        sortingMode="server"
        pageSize={props.pageSize}
        disableColumnFilter={props.briefTable}
        disableColumnMenu={true}

        rowsPerPageOptions={[5, 10, 25, 50]}
        getRowClassName={(params =>
          clsx({
            [classes.rowSuccess]: (
              params.row.current_pass_percentage >= JOB_THRESHOLDS.success
            ),
            [classes.rowWarning]: (
              params.row.current_pass_percentage >= JOB_THRESHOLDS.warning &&
              params.row.current_pass_percentage < JOB_THRESHOLDS.success
            ),
            [classes.rowError]: (
              params.row.current_pass_percentage >= JOB_THRESHOLDS.error &&
              params.row.current_pass_percentage < JOB_THRESHOLDS.warning
            )
          })
        )}
        componentsProps={{
          toolbar: {
            bookmarks: bookmarks,
            clearSearch: () => requestSearch(''),
            doSearch: requestSearch,
            period: period,
            selectPeriod: setPeriod,
            setFilterModel: (m) => addFilters(m)
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
  briefTable: false,
  filterModel: {
    items: []
  }
}

JobTable.propTypes = {
  briefTable: PropTypes.bool,
  classes: PropTypes.object,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  release: PropTypes.string.isRequired,
  title: PropTypes.string,
  hideControls: PropTypes.bool,
  period: PropTypes.string,
  job: PropTypes.string,
  filterModel: PropTypes.object
}

export default withStyles(ROW_STYLES)(JobTable)
