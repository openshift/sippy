import {
  Backdrop,
  Button,
  CircularProgress,
  Container,
  Tooltip,
  Typography,
} from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { DirectionsBoat, GitHub } from '@mui/icons-material'
import {
  getReportStartDate,
  pathForExactJob,
  relativeTime,
  safeEncodeURIComponent,
  SafeJSONParam,
} from '../helpers'
import { Link } from 'react-router-dom'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { ReportEndContext } from '../App'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

/**
 * JobRunsTable shows the list of all job runs matching any selected filters.
 */
export default function JobRunsTable(props) {
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [apiResult, setApiResult] = React.useState([])

  const [filterModel = props.filterModel, setFilterModel] = useQueryParam(
    'filters',
    SafeJSONParam
  )

  const [sortField = props.sortField, setSortField] = useQueryParam(
    'sortField',
    StringParam
  )
  const [sort = props.sort, setSort] = useQueryParam('sort', StringParam)

  const [pageSize = props.pageSize, setPageSize] = useQueryParam(
    'pageSize',
    NumberParam
  )
  const [page = 0, setPage] = useQueryParam('page', NumberParam)
  const [pageFlip, setPageFlip] = React.useState(false)

  const tooltips = {
    S: 'Success',
    F: 'Failure (e2e)',
    f: 'failure (other tests)',
    A: 'Aborted',
    U: 'upgrade failure',
    I: 'setup failure (installer)',
    N: 'setup failure (infrastructure)',
    n: 'failure before setup (infra)',
    R: 'running',
  }

  const startDate = getReportStartDate(React.useContext(ReportEndContext))

  const columns = [
    {
      field: 'id',
      hide: true,
      type: 'number',
      filterable: false,
    },
    {
      field: 'timestamp',
      headerName: 'Date / Time',
      filterable: true,
      flex: 1.25,
      type: 'date',
      valueFormatter: (params) => {
        return new Date(params.value)
      },
      renderCell: (params) => {
        return (
          <Tooltip title={relativeTime(new Date(params.value), startDate)}>
            <p>{new Date(params.value).toLocaleString()}</p>
          </Tooltip>
        )
      },
    },
    {
      field: 'job',
      autocomplete: 'jobs',
      release: props.release,
      headerName: 'Job name',
      flex: props.briefTable ? 1 : 3,
      renderCell: (params) => {
        return (
          <div
            style={{
              display: 'block',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            <Tooltip title={params.value}>
              <Link to={pathForExactJob(props.release, params.value)}>
                {props.briefTable ? params.row.brief_name : params.value}
              </Link>
            </Tooltip>
          </div>
        )
      },
    },
    {
      field: 'test_failures',
      headerName: 'Failures',
      type: 'number',
      flex: 0.6,
    },
    {
      field: 'test_flakes',
      headerName: 'Flakes',
      type: 'number',
      flex: 0.6,
    },
    {
      field: 'overall_result',
      headerName: 'Result',
      flex: 0.5,
      renderCell: (params) => {
        return (
          <Tooltip title={tooltips[params.value]}>
            <div
              className={'result result-' + params.value}
              style={{ width: '100%', textAlign: 'center' }}
            >
              {params.value}
            </div>
          </Tooltip>
        )
      },
    },
    {
      field: 'url',
      headerName: ' ',
      flex: 0.4,
      renderCell: (params) => {
        return (
          <Tooltip title="View in Prow">
            <Button
              color="inherit"
              style={{ justifyContent: 'center' }}
              target="_blank"
              startIcon={<DirectionsBoat />}
              href={params.value}
            />
          </Tooltip>
        )
      },
      filterable: false,
    },
    {
      field: 'pull_request_link',
      headerName: ' ',
      flex: 0.4,
      renderCell: (params) => {
        if (params.value === undefined || params.value === '') {
          return ''
        }

        return (
          <Tooltip title="View pull request">
            <Button
              color="inherit"
              style={{ justifyContent: 'center' }}
              target="_blank"
              startIcon={<GitHub />}
              href={params.value}
            />
          </Tooltip>
        )
      },
    },
    {
      field: 'variants',
      type: 'array',
      autocomplete: 'variants',
      headerName: 'Variants',
      hide: true,
    },
    {
      field: 'failed_test_names',
      type: 'array',
      autocomplete: 'tests',
      headerName: 'Failed tests',
      hide: true,
    },
    {
      field: 'flaked_test_names',
      type: 'array',
      autocomplete: 'tests',
      headerName: 'Flaked tests',
      hide: true,
    },
    {
      field: 'pull_request_author',
      autocomplete: 'authors',
      headerName: 'Pull request author',
      hide: true,
    },
    {
      field: 'pull_request_repo',
      autocomplete: 'repos',
      headerName: 'Pull request repo',
      hide: true,
    },
    {
      field: 'pull_request_org',
      autocomplete: 'orgs',
      headerName: 'Pull request org',
      hide: true,
    },
    {
      field: 'pull_request_sha',
      headerName: 'Pull request SHA',
      hide: true,
    },
    // These are fields on the job, not the run - but we can
    // filter by them.
    {
      field: 'name',
      autocomplete: 'jobs',
      release: props.release,
      headerName: 'Name',
      type: 'string',
      hide: 'true',
    },
    {
      field: 'cluster',
      autocomplete: 'cluster',
      headerName: 'Build cluster',
      type: 'string',
      hide: 'true',
    },
  ]

  const fetchData = () => {
    let queryString = ''
    if (props.release !== '') {
      queryString += '&release=' + props.release
    }

    if (filterModel && filterModel.items.length > 0) {
      queryString +=
        '&filter=' + safeEncodeURIComponent(JSON.stringify(filterModel))
    }

    if (props.limit > 0) {
      queryString += '&limit=' + safeEncodeURIComponent(props.limit)
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)
    queryString += '&perPage=' + safeEncodeURIComponent(pageSize)
    queryString += '&page=' + safeEncodeURIComponent(page)

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/jobs/runs?' +
        queryString.substring(1)
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setApiResult(json)
        setLoaded(true)
        setPageFlip(false)
      })
      .catch((error) => {
        setFetchError('Could not retrieve jobs ' + props.release + ', ' + error)
      })
  }

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'job'
    )
    currentFilters.items.push({
      id: 99,
      columnField: 'job',
      operatorValue: 'contains',
      value: searchValue,
    })
    setFilterModel(currentFilters)
  }

  useEffect(() => {
    fetchData()
  }, [filterModel, sort, sortField, page, pageSize])

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
      <Backdrop open={!isLoaded}>
        Fetching data...
        <CircularProgress color="inherit" />
      </Backdrop>
    )
  }

  const addFilters = (filter) => {
    const currentFilters = filterModel.items.filter((item) => item.value !== '')

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

  const legend = (
    <div>
      <span className="legend-item">
        <span className="results results-demo">
          <span className="result result-S">S</span>
        </span>{' '}
        success
      </span>
      <span className="legend-item">
        <span className="results results-demo">
          <span className="result result-F">F</span>
        </span>{' '}
        failure (e2e)
      </span>
      <span className="legend-item">
        <span className="results results-demo">
          <span className="result result-f">f</span>
        </span>{' '}
        failure (other tests)
      </span>
      <span className="legend-item">
        <span className="results results-demo">
          <span className="result result-U">U</span>
        </span>{' '}
        upgrade failure
      </span>
      <span className="legend-item">
        <span className="results results-demo">
          <span className="result result-I">I</span>
        </span>{' '}
        setup failure (installer)
      </span>
      <span className="legend-item">
        <span className="results results-demo">
          <span className="result result-N">N</span>
        </span>{' '}
        setup failure (infra)
      </span>
      <span className="legend-item">
        <span className="results results-demo">
          <span className="result result-n">n</span>
        </span>{' '}
        failure before setup (infra)
      </span>
      <span className="legend-item">
        <span className="results results-demo">
          <span className="result result-R">R</span>
        </span>{' '}
        running
      </span>
    </div>
  )

  const changePage = (newPage) => {
    setPageFlip(true)
    setPage(newPage)
  }

  const table = (
    <DataGrid
      components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
      rows={apiResult.rows}
      rowCount={apiResult.total_rows}
      loading={pageFlip}
      pagination
      paginationMode="server"
      onPageChange={(newPage) => changePage(newPage)}
      columns={columns}
      autoHeight={true}
      // Filtering:
      filterMode="server"
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
      pageSize={pageSize}
      onPageSizeChange={(newPageSize) => setPageSize(newPageSize)}
      disableColumnMenu={true}
      rowsPerPageOptions={[5, 10, 25, 50, 100]}
      componentsProps={{
        toolbar: {
          columns: columns,
          clearSearch: () => requestSearch(''),
          doSearch: requestSearch,
          filterModel: filterModel,
          setFilterModel: setFilterModel,
          addFilters: (m) => addFilters(m),
        },
      }}
    />
  )

  if (props.briefTable) {
    return table
  }

  /* eslint-disable react/prop-types */
  return (
    <Fragment>
      {pageTitle()}
      <br />
      <br />
      {legend}
      <Container size="xl" style={{ marginTop: 20 }}>
        {table}
      </Container>
    </Fragment>
  )
}

JobRunsTable.defaultProps = {
  briefTable: false,
  hideControls: false,
  pageSize: 25,
  release: '',
  filterModel: {
    items: [],
  },
  sortField: 'timestamp',
  sort: 'desc',
}

JobRunsTable.propTypes = {
  briefTable: PropTypes.bool,
  classes: PropTypes.object,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  release: PropTypes.string,
  title: PropTypes.string,
  hideControls: PropTypes.bool,
  period: PropTypes.string,
  job: PropTypes.string,
  filterModel: PropTypes.object,
  sort: PropTypes.string,
  sortField: PropTypes.string,
}
