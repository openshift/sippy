import './JobTable.css'
import { BOOKMARKS, JOB_THRESHOLDS } from '../constants'
import { BugReport, DirectionsRun, GridOn } from '@mui/icons-material'
import { Button, Container, Tooltip, Typography } from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import {
  escapeRegex,
  pathForExactJobAnalysis,
  pathForExactJobRuns,
  relativeTime,
  safeEncodeURIComponent,
  SafeJSONParam,
} from '../helpers'
import { generateClasses } from '../datagrid/utils'
import { GridView } from '../datagrid/GridView'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { usePageContextForChat } from '../chat/store/useChatStore'
import { withStyles } from '@mui/styles'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

const bookmarks = [
  { name: 'New jobs (no previous runs)', model: [BOOKMARKS.NEW_JOBS] },
  { name: 'Low frequency jobs', model: [BOOKMARKS.RUN_FEW] },
  { name: 'High frequency jobs', model: [BOOKMARKS.RUN_7] },
  { name: 'Upgrade related', model: [BOOKMARKS.UPGRADE] },
]

export const getColumns = (config, openBugzillaDialog) => {
  return {
    name: {
      field: 'name',
      headerName: 'Name',
      flex: 3.5,
      renderCell: (params) => {
        return (
          <div align="left" className="job-name">
            <Tooltip title={params.value}>
              <Link to={pathForExactJobAnalysis(config.release, params.value)}>
                {config.briefTable ? params.row.brief_name : params.value}
              </Link>
            </Tooltip>
          </div>
        )
      },
    },
    current_pass_percentage: {
      field: 'current_pass_percentage',
      headerName: 'Current pass percentage',
      type: 'number',
      flex: 0.75,
      renderCell: (params) => (
        <div className="percentage-cell">
          {Number(params.value).toFixed(1).toLocaleString()}%<br />
          <small>({params.row.current_runs} runs)</small>
        </div>
      ),
    },
    net_improvement: {
      field: 'net_improvement',
      headerName: 'Improvement',
      type: 'number',
      flex: 0.5,
      renderCell: (params) => {
        return <PassRateIcon tooltip={true} improvement={params.value} />
      },
    },
    previous_pass_percentage: {
      field: 'previous_pass_percentage',
      headerName: 'Previous pass percentage',
      flex: 0.75,
      type: 'number',
      renderCell: (params) => (
        <div className="percentage-cell">
          {Number(params.value).toFixed(1).toLocaleString()}%<br />
          <small>({params.row.previous_runs} runs)</small>
        </div>
      ),
    },
    last_pass: {
      field: 'last_pass',
      headerName: 'Last pass',
      filterable: true,
      flex: 1.25,
      type: 'date',
      valueFormatter: (params) => {
        return new Date(params.value)
      },
      renderCell: (params) => {
        if (params.value === undefined || params.value === '') {
          return (
            <Tooltip title="Job has never passed, or pass predates Sippy's history (typically about 90 days)">
              <Fragment>-</Fragment>
            </Tooltip>
          )
        }

        return (
          <Tooltip title={params.value}>
            <Fragment>
              {relativeTime(new Date(params.value), new Date())}
            </Fragment>
          </Tooltip>
        )
      },
    },
    open_bugs: {
      field: 'open_bugs',
      headerName: 'Bugs',
      type: 'number',
      flex: 0.5,
      renderCell: (params) => (
        <div>
          <Link to={pathForExactJobAnalysis(config.release, params.row.name)}>
            {params.value}
          </Link>
        </div>
      ),
    },
    test_grid_url: {
      field: 'test_grid_url',
      headerName: ' ',
      flex: 0.4,
      renderCell: (params) => {
        if (params.value === undefined || params.value === '') {
          return
        }

        return (
          <Tooltip title="TestGrid">
            <Button
              color="inherit"
              style={{ justifyContent: 'center' }}
              target="_blank"
              startIcon={<GridOn />}
              href={params.value}
            />
          </Tooltip>
        )
      },
      filterable: false,
      hide: config.briefTable,
    },
    job_runs: {
      field: 'job_runs',
      headerName: ' ',
      flex: 0.4,
      renderCell: (params) => {
        return (
          <Tooltip title="See all job runs">
            <Button
              color="inherit"
              component={Link}
              style={{ justifyContent: 'center' }}
              startIcon={<DirectionsRun />}
              to={pathForExactJobRuns(config.release, params.row.name)}
            />
          </Tooltip>
        )
      },
      filterable: false,
      hide: config.briefTable,
    },
    link: {
      field: 'link',
      sortable: false,
      headerName: ' ',
      flex: 0.4,

      filterable: false,
      hide: config.briefTable,
      renderCell: (params) => {
        return (
          <Tooltip title="Find Bugs">
            <Button
              target="_blank"
              startIcon={<BugReport />}
              href={
                'https://search.dptools.openshift.org/?search=' +
                safeEncodeURIComponent(escapeRegex(params.row.name)) +
                '&maxAge=336h&context=1&type=bug&name=&excludeName=&maxMatches=5&maxBytes=20971520&groupBy=job'
              }
            />
          </Tooltip>
        )
      },
    },
    average_retests_to_merge: {
      field: 'average_retests_to_merge',
      type: 'number',
      headerName: 'Average premerge failures',
      renderCell: (params) => Number(params.value).toFixed(1).toLocaleString(),
    },
    // These are here just to allow filtering
    org: {
      field: 'org',
      autocomplete: 'orgs',
      type: 'string',
      headerName: 'GitHub Org',
      hide: true,
    },
    repo: {
      field: 'repo',
      autocomplete: 'repos',
      type: 'string',
      headerName: 'GitHub Repo',
      hide: true,
    },
    variants: {
      field: 'variants',
      autocomplete: 'variants',
      type: 'array',
      headerName: 'Variants',
      hide: true,
      renderCell: (params) => (
        <Tooltip
          sx={{ whiteSpace: 'pre' }}
          title={params.value ? params.value.join('\n') : ''}
        >
          <div className="variants-list">
            {params.value
              ? params.value
                  .filter((item) => !item.endsWith(':default'))
                  .join('\n')
              : ''}
          </div>
        </Tooltip>
      ),
    },
    current_runs: {
      field: 'current_runs',
      headerName: 'Current runs',
      hide: true,
      type: 'number',
    },
    previous_runs: {
      field: 'previous_runs',
      headerName: 'Previous runs',
      hide: true,
      type: 'number',
    },
  }
}

export const getViews = (props) => {
  return {
    Default: {
      sortField: 'current_pass_percentage',
      sort: 'asc',
      fieldOrder: [
        {
          field: 'name',
          flex: 3.5,
        },
        {
          field: 'current_pass_percentage',
          flex: 0.75,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'net_improvement',
          flex: 0.5,
        },
        {
          field: 'previous_pass_percentage',
          flex: 0.75,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'open_bugs',
          flex: 0.5,
          hide: props.briefTable,
        },
        {
          field: 'test_grid_url',
          flex: 0.4,
          hide: props.briefTable,
        },
        {
          field: 'job_runs',
          flex: 0.4,
          hide: props.briefTable,
        },
      ],
    },
    Variants: {
      sortField: 'name',
      sort: 'asc',
      fieldOrder: [
        {
          field: 'name',
          flex: 3.5,
        },
        {
          field: 'current_pass_percentage',
          flex: 0.75,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'net_improvement',
          flex: 0.5,
        },
        {
          field: 'previous_pass_percentage',
          flex: 0.75,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'variants',
          flex: 2,
        },
      ],
    },
    'Pull Requests': {
      sortField: 'average_retests_to_merge',
      sort: 'desc',
      fieldOrder: [
        {
          field: 'name',
          flex: 3.5,
        },
        {
          field: 'current_pass_percentage',
          flex: 1.5,
          headerClassName: 'wrapHeader',
        },
        {
          field: 'average_retests_to_merge',
          flex: 1,
          headerClassName: 'wrapHeader',
        },
      ],
    },
    'Last passing': {
      sortField: 'current_runs',
      sort: 'desc',
      fieldOrder: [
        {
          field: 'name',
          flex: 3.5,
        },
        {
          field: 'current_pass_percentage',
          flex: 0.75,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'net_improvement',
          flex: 0.5,
        },
        {
          field: 'previous_pass_percentage',
          flex: 0.75,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'last_pass',
          flex: 1.0,
        },
        {
          field: 'open_bugs',
          flex: 0.5,
          hide: props.briefTable,
        },
        {
          field: 'test_grid_url',
          flex: 0.4,
          hide: props.briefTable,
        },
        {
          field: 'job_runs',
          flex: 0.4,
          hide: props.briefTable,
        },
      ],
    },
  }
}

const useStyles = makeStyles((theme) => ({
  root: {
    '& .wrapHeader .MuiDataGrid-columnHeaderTitle': {
      textOverflow: 'ellipsis',
      display: '-webkit-box',
      '-webkit-line-clamp': 2,
      '-webkit-box-orient': 'vertical',
      overflow: 'hidden',
      overflowWrap: 'break-word',
      lineHeight: '20px',
      whiteSpace: 'normal',
    },
    backdrop: {
      zIndex: 999999,
      color: '#fff',
    },
  },
}))

/**
 * JobTable shows the list of all jobs matching any selected filters,
 * including current and previous pass percentages, net improvement, and
 * bug links.
 */
function JobTable(props) {
  const { classes } = props
  const gridClasses = useStyles()
  const { setPageContextForChat, unsetPageContextForChat } =
    usePageContextForChat()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])
  const [selectedJobs, setSelectedJobs] = React.useState([])

  const [view = props.view, setView] = useQueryParam('view', StringParam)

  const [period = props.period, setPeriod] = useQueryParam(
    'period',
    StringParam
  )

  const [filterModel = props.filterModel, setFilterModel] = useQueryParam(
    'filters',
    SafeJSONParam
  )

  const [sortField = props.sortField, setSortField] = useQueryParam(
    'sortField',
    StringParam
  )

  const [pageSize = props.pageSize, setPageSize] = useQueryParam(
    'pageSize',
    NumberParam
  )

  const [sort = props.sort, setSort] = useQueryParam('sort', StringParam)

  const [jobDetails, setJobDetails] = React.useState({ bugs: [] })

  // Update page context for chat (only if this is the main page, not embedded)
  useEffect(() => {
    // Don't set context if this is an embedded table (e.g., in ReleaseOverview)
    if (!isLoaded || rows.length === 0 || props.briefTable) return

    // Send all rows on current page
    const visibleJobs = rows.map((job) => ({
      name: job.name,
      current_pass_percentage: job.current_pass_percentage,
      current_runs: job.current_runs,
      previous_pass_percentage: job.previous_pass_percentage,
      previous_runs: job.previous_runs,
      net_improvement: job.net_improvement,
    }))

    setPageContextForChat({
      page: 'jobs-table',
      url: window.location.href,
      data: {
        release: props.release,
        totalJobs: rows.length,
        period: period,
        view: view,
        sortField: sortField,
        sortOrder: sort,
        filters: filterModel,
        selectedJobsCount: selectedJobs.length,
        visibleJobs: visibleJobs,
      },
    })

    // Cleanup: Clear context when component unmounts
    return () => {
      unsetPageContextForChat()
    }
  }, [
    rows,
    selectedJobs.length,
    isLoaded,
    props.release,
    props.briefTable,
    period,
    view,
    sortField,
    sort,
    filterModel,
    setPageContextForChat,
    unsetPageContextForChat,
  ])

  const fetchData = () => {
    let queryString = ''
    if (filterModel && filterModel.items.length > 0) {
      queryString +=
        '&filter=' + safeEncodeURIComponent(JSON.stringify(filterModel))
    }

    if (props.limit > 0) {
      queryString += '&limit=' + safeEncodeURIComponent(props.limit)
    }

    if (period) {
      queryString += '&period=' + safeEncodeURIComponent(period)
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/jobs?release=' +
        props.release +
        queryString
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setRows(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve jobs ' + props.release + ', ' + error)
      })
  }

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'name'
    )
    currentFilters.items.push({
      id: 99,
      columnField: 'name',
      operatorValue: 'contains',
      value: searchValue,
    })
    setFilterModel(currentFilters)
  }

  useEffect(() => {
    fetchData()
  }, [period, filterModel, sort, sortField])

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
    return <p>Loading...</p>
  }

  const addFilters = (filter) => {
    const currentFilters = filterModel.items.filter((item) => {
      for (let i = 0; i < filter.length; i++) {
        if (filter[i].columnField === item.columnField) {
          return false
        }
      }

      return item.value !== ''
    })

    filter.forEach((item) => {
      if (item.value && item.value !== '') {
        currentFilters.push(item)
      }
    })
    setFilterModel({
      items: currentFilters,
      linkOperator: filterModel.linkOperator || 'and',
    })
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

  const selectedJobNames = () => {
    let jobs = rows
    if (selectedJobs.length !== 0) {
      const selectedIDs = new Set(selectedJobs)
      jobs = rows.filter((row) => selectedIDs.has(row.id))
    }
    return jobs.map((job) => job.name).join('\n')
  }

  const createFilter = () => {
    if (selectedJobs.length === rows.length || selectedJobs.length === 0) {
      return safeEncodeURIComponent(JSON.stringify(filterModel))
    }

    const selectedIDs = new Set(selectedJobs)
    let jobs = rows.filter((row) => selectedIDs.has(row.id))
    jobs = jobs.map((job, id) => {
      return {
        id: id,
        columnField: 'name',
        operatorValue: 'equals',
        value: job.name,
      }
    })
    return safeEncodeURIComponent(
      JSON.stringify({ items: jobs, linkOperator: 'or' })
    )
  }

  const detailsButton = (
    <Button
      component={Link}
      to={`/jobs/${props.release}/analysis?filters=${createFilter()}`}
      variant="contained"
      color="primary"
      style={{ margin: 10 }}
    >
      Analyze
    </Button>
  )

  const copyButton = (
    <Button
      onClick={() => navigator.clipboard.writeText(selectedJobNames())}
      variant="contained"
      color="secondary"
      style={{ margin: 10 }}
    >
      Copy names
    </Button>
  )

  const gridView = new GridView(getColumns(props), getViews(props), view)

  const selectView = (v) => {
    setLoaded(false)
    setView(v)
    gridView.setView(v)
    setSort(gridView.view.sort)
    setSortField(gridView.view.sortField)
  }

  return (
    /* eslint-disable react/prop-types */
    <Container size="xl">
      {pageTitle()}
      <DataGrid
        className={gridClasses.root}
        components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
        rows={rows}
        columns={gridView.columns}
        autoHeight={true}
        getRowHeight={() => 'auto'}
        sortingOrder={['desc', 'asc']}
        sortModel={[
          {
            field: sortField,
            sort: sort,
          },
        ]}
        // Sorting:
        onSortModelChange={(m) => updateSortModel(m)}
        sortingMode="server"
        disableColumnFilter={props.briefTable}
        disableColumnMenu={true}
        checkboxSelection={!props.briefTable && !props.hideControls}
        onSelectionModelChange={(rows) => setSelectedJobs(rows)}
        rowsPerPageOptions={props.rowsPerPageOptions}
        pageSize={pageSize}
        onPageSizeChange={(newPageSize) => setPageSize(newPageSize)}
        getRowClassName={(params) =>
          classes[
            'row-percent-' + Math.round(params.row.current_pass_percentage)
          ]
        }
        componentsProps={{
          toolbar: {
            bookmarks: bookmarks,
            views: gridView.views,
            view: view,
            selectView: selectView,
            columns: gridView.filterColumns,
            clearSearch: () => requestSearch(''),
            doSearch: requestSearch,
            period: period,
            selectPeriod: setPeriod,
            addFilters: (m) => addFilters(m),
            filterModel: filterModel,
            setFilterModel: setFilterModel,
            downloadDataFunc: () => {
              return rows
            },
            downloadFilePrefix: 'jobs',
          },
        }}
      />
      {props.briefTable || props.hideControls ? '' : detailsButton}
      {props.briefTable || props.hideControls ? '' : copyButton}
    </Container>
  )
}

JobTable.defaultProps = {
  hideControls: false,
  pageSize: 25,
  period: 'default',
  briefTable: false,
  rowsPerPageOptions: [5, 10, 25, 50, 100],
  filterModel: {
    items: [],
  },
  sortField: 'current_pass_percentage',
  sort: 'asc',
  view: 'Default',
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
  filterModel: PropTypes.object,
  sort: PropTypes.string,
  sortField: PropTypes.string,
  rowsPerPageOptions: PropTypes.array,
  view: PropTypes.string,
}

export default withStyles(generateClasses(JOB_THRESHOLDS))(JobTable)
