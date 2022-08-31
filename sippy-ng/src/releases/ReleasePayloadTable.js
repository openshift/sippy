import './ReleasePayloadTable.css'
import { Box, Button, Container, Tooltip, Typography } from '@material-ui/core'
import {
  CheckCircle,
  CompareArrows,
  Error,
  Help,
  Warning,
} from '@material-ui/icons'
import { createTheme, makeStyles } from '@material-ui/core/styles'
import { DataGrid } from '@material-ui/data-grid'
import {
  getReportStartDate,
  relativeTime,
  safeEncodeURIComponent,
  SafeJSONParam,
} from '../helpers'
import { Link } from 'react-router-dom'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { ReportEndContext } from '../App'
import Alert from '@material-ui/lab/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    rowPhaseAccepted: {
      backgroundColor: theme.palette.success.light,
    },
    rowPhaseRejected: {
      backgroundColor: theme.palette.error.light,
    },
    rowPhaseForced: {
      backgroundColor: theme.palette.warning.light,
    },
  }),
  { defaultTheme }
)

function ReleasePayloadTable(props) {
  const classes = useStyles()
  const theme = defaultTheme
  const startDate = getReportStartDate(React.useContext(ReportEndContext))
  const columns = [
    {
      field: 'phase',
      headerName: 'Phase',
      flex: 0.75,
      align: 'center',
      renderCell: (params) => {
        if (params.row.phase === 'Accepted') {
          if (params.row.forced === true) {
            return (
              <Tooltip title="This payload was manually force accepted.">
                <CheckCircle style={{ fill: 'green' }} />
              </Tooltip>
            )
          } else {
            return (
              <Tooltip title="This payload was accepted.">
                <CheckCircle style={{ fill: 'green' }} />
              </Tooltip>
            )
          }
        } else if (params.row.phase === 'Rejected') {
          if (params.row.forced === true) {
            return (
              <Tooltip title="This payload was manually force rejected.">
                <Error style={{ fill: 'maroon' }} />
              </Tooltip>
            )
          } else {
            return (
              <Tooltip title="This payload was rejected.">
                <Error style={{ fill: 'maroon' }} />
              </Tooltip>
            )
          }
        } else {
          return (
            <Fragment>
              <Tooltip title="This payload has an unknown status">
                <Help style={{ fill: 'gray' }} />
              </Tooltip>
            </Fragment>
          )
        }
      },
    },
    {
      field: 'forced',
      headerName: 'Forced',
      align: 'center',
      flex: 0.75,
      hide: props.briefTable,
      renderCell: (params) => {
        if (params.value === true) {
          if (params.row.phase === 'Accepted') {
            return (
              <Tooltip title="This payload was manually force accepted.">
                <Warning style={{ color: theme.palette.warning.dark }} />
              </Tooltip>
            )
          } else if (params.row.phase === 'Rejected') {
            return (
              <Tooltip title="This payload was manually force rejected">
                <Warning style={{ color: theme.palette.warning.dark }} />
              </Tooltip>
            )
          } else {
            return (
              <Tooltip title="This payload was manually forced but has an unknown status.">
                <Warning style={{ color: theme.palette.warning.dark }} />
              </Tooltip>
            )
          }
        } else {
          return ' '
        }
      },
    },
    {
      field: 'reject_reason',
      headerName: 'Reject reason',
      flex: 1.5,
      hide: props.briefTable,
      renderCell: (params) => {
        return (
          <Tooltip title={`${params.row.reject_reason_note}`}>
            <Typography>{params.value}</Typography>
          </Tooltip>
        )
      },
    },
    {
      field: 'release_tag',
      headerName: 'Tag',
      flex: 4,
      renderCell: (params) => {
        return (
          <Link
            to={`/release/${params.row.release}/tags/${params.row.release_tag}`}
          >
            {params.value}
          </Link>
        )
      },
    },
    {
      field: 'architecture',
      headerName: 'Architecture',
      flex: 1.5,
      hide: props.briefTable,
    },
    {
      field: 'stream',
      headerName: 'Stream',
      flex: 1.5,
      hide: props.briefTable,
    },
    {
      field: 'release_time',
      headerName: 'Time',
      flex: 2,
      type: 'date',
      valueFormatter: (params) => {
        return new Date(params.value)
      },
      renderCell: (params) => {
        if (!params.value) {
          return
        }

        return (
          <Tooltip title={params.value}>
            <p>{relativeTime(new Date(params.value), startDate)}</p>
          </Tooltip>
        )
      },
    },
    {
      field: 'kubernetes_version',
      headerName: 'Kubernetes version',
      flex: 1.5,
      hide: props.briefTable,
    },
    {
      field: 'current_os_version',
      headerName: 'Current OS Version',
      flex: 3,
      renderCell: (params) => {
        return <a href={params.row.current_os_url}>{params.value}</a>
      },
      hide: props.briefTable,
    },
    {
      field: 'os_diff_url',
      headerName: 'Diff',
      flex: 1.25,
      align: 'center',
      headerAlign: 'center',
      renderCell: (params) => {
        if (params.row.previous_os_version !== '') {
          return (
            <Tooltip title="See diff between these two OS releases">
              <Button
                style={{ justifyContent: 'center' }}
                startIcon={<CompareArrows />}
                href={params.row.current_os_url}
              />
            </Tooltip>
          )
        }
      },
      hide: props.briefTable,
    },
    {
      field: 'previous_os_version',
      headerName: 'Previous OS Version',
      flex: 3,
      renderCell: (params) => {
        if (params.value !== '') {
          return <a href={params.row.previous_os_version}>{params.value}</a>
        }
      },
      hide: props.briefTable,
    },
    {
      field: 'failed_job_names',
      headerName: 'Failed jobs',
      sortable: false,
      filterable: false,
      flex: 4,
      renderCell: (params) => {
        if (params.value && params.value.length > 0) {
          return (
            <Tooltip
              title={`${params.value.length} jobs failed: ${params.value.join(
                ', '
              )}`}
            >
              <Box
                component={Link}
                to={`/release/${props.release}/tags/${params.row.release_tag}`}
                className="clamped"
              >
                {params.value.join(', ')}
              </Box>
            </Tooltip>
          )
        }
        return ''
      },
    },
  ]

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

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

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'release_tag'
    )
    currentFilters.items.push({
      id: 99,
      columnField: 'release_tag',
      operatorValue: 'contains',
      value: searchValue,
    })
    setFilterModel(currentFilters)
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

  const fetchData = () => {
    let queryString = ''
    if (filterModel && filterModel.items.length > 0) {
      queryString +=
        '&filter=' + safeEncodeURIComponent(JSON.stringify(filterModel))
    }

    if (props.release !== '') {
      queryString += '&release=' + safeEncodeURIComponent(props.release)
    }

    if (props.limit > 0) {
      queryString += '&limit=' + safeEncodeURIComponent(props.limit)
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/releases/tags?' +
        queryString.substring(1)
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
        setFetchError('Could not retrieve tags ' + error)
      })
  }

  useEffect(() => {
    fetchData()
  }, [filterModel, sort, sortField])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return <p>Loading...</p>
  }

  return (
    <DataGrid
      components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
      rows={rows}
      columns={columns}
      rowHeight={70}
      autoHeight={true}
      disableColumnFilter={props.briefTable}
      disableColumnMenu={true}
      pageSize={pageSize}
      onPageSizeChange={(newPageSize) => setPageSize(newPageSize)}
      rowsPerPageOptions={[5, 10, 25, 50, 100]}
      getRowClassName={(params) =>
        params.row.forced === true
          ? classes.rowPhaseForced
          : classes['rowPhase' + params.row.phase]
      }
      filterMode="server"
      sortingMode="server"
      sortingOrder={['desc', 'asc']}
      sortModel={[
        {
          field: sortField,
          sort: sort,
        },
      ]}
      onSortModelChange={(m) => updateSortModel(m)}
      componentsProps={{
        toolbar: {
          columns: columns,
          clearSearch: () => requestSearch(''),
          doSearch: requestSearch,
          addFilters: addFilters,
          filterModel: filterModel,
          setFilterModel: setFilterModel,
        },
      }}
    />
  )
}

ReleasePayloadTable.defaultProps = {
  limit: 0,
  hideControls: false,
  pageSize: 25,
  briefTable: false,
  filterModel: {
    items: [],
  },
  sortField: 'release_time',
  sort: 'desc',
}

ReleasePayloadTable.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  filterModel: PropTypes.object,
  release: PropTypes.string,
  sort: PropTypes.string,
  sortField: PropTypes.string,
}

export default ReleasePayloadTable
