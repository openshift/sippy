import { CheckCircle, Error as ErrorIcon, Help } from '@mui/icons-material'
import { DataGrid } from '@mui/x-data-grid'
import { Link } from 'react-router-dom'
import { makeStyles, useTheme } from '@mui/styles'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { apiFetch, safeEncodeURIComponent, SafeJSONParam } from '../helpers'
import { Tooltip } from '@mui/material'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

const useStyles = makeStyles((theme) => ({
  rowPhaseAccepted: {
    backgroundColor: theme.palette.success.light,
  },
  rowPhaseRejected: {
    backgroundColor: theme.palette.error.light,
  },
  rowPhaseForced: {
    backgroundColor: theme.palette.warning.light,
  },
}))

function PayloadStreamsTable(props) {
  const theme = useTheme()
  const classes = useStyles(theme)

  const columns = [
    {
      field: 'phase',
      headerName: 'Phase',
      flex: 0.75,
      align: 'center',
      renderCell: (params) => {
        if (params.row.last_phase === 'Accepted') {
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
        } else if (params.row.last_phase === 'Rejected') {
          if (params.row.forced === true) {
            return (
              <Tooltip title="This payload was manually force rejected.">
                <ErrorIcon style={{ fill: 'maroon' }} />
              </Tooltip>
            )
          } else {
            return (
              <Tooltip title="This payload was rejected.">
                <ErrorIcon style={{ fill: 'maroon' }} />
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
      field: 'stream',
      headerName: 'Stream',
      flex: 1.5,
      hide: props.briefTable,
      renderCell: (params) => {
        return (
          <Link
            to={`/release/${params.row.release}/streams/${params.row.architecture}/${params.row.stream}`}
          >
            {params.row.architecture} {params.value}
          </Link>
        )
      },
    },
    {
      field: 'release_tag',
      headerName: 'Last Accepted Tag',
      flex: 4,
    },
    {
      field: 'count',
      headerName: 'Last Phase',
      flex: 4,
      renderCell: (params) => {
        return (
          <div>
            Last {params.value} payloads have been {params.row.last_phase}
          </div>
        )
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

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    apiFetch('/api/releases/health?' + queryString.substring(1))
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
      rowsPerPageOptions={[]}
      getRowClassName={(params) =>
        params.row.forced === true
          ? classes.rowPhaseForced
          : classes['rowPhase' + params.row.last_phase]
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

PayloadStreamsTable.defaultProps = {
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

PayloadStreamsTable.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  filterModel: PropTypes.object,
  release: PropTypes.string,
  sort: PropTypes.string,
  sortField: PropTypes.string,
}

export default PayloadStreamsTable
