import { Button, Container, Tooltip, Typography } from '@material-ui/core'
import { CheckCircle, CompareArrows, Error, Help } from '@material-ui/icons'
import { createTheme, makeStyles } from '@material-ui/core/styles'
import { DataGrid } from '@material-ui/data-grid'
import { JsonParam, StringParam, useQueryParam } from 'use-query-params'
import { Link } from 'react-router-dom'
import { relativeTime } from '../helpers'
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
  }),
  { defaultTheme }
)

function ReleasePayloadTable(props) {
  const classes = useStyles()

  const columns = [
    {
      field: 'phase',
      headerName: 'Phase',
      flex: 0.75,
      align: 'center',
      renderCell: (params) => {
        if (params.row.phase === 'Accepted') {
          return (
            <Tooltip title="This payload was accepted.">
              <CheckCircle style={{ fill: 'green' }} />
            </Tooltip>
          )
        } else if (params.row.phase === 'Rejected') {
          return (
            <Tooltip title="This payload was rejected.">
              <Error style={{ fill: 'maroon' }} />
            </Tooltip>
          )
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
      field: 'releaseTag',
      headerName: 'Tag',
      flex: 5,
      renderCell: (params) => {
        return (
          <Link
            to={`/release/${params.row.release}/tags/${params.row.releaseTag}`}
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
      field: 'releaseTime',
      headerName: 'Time',
      flex: 2.5,
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
            <p>{relativeTime(new Date(params.value))}</p>
          </Tooltip>
        )
      },
    },
    {
      field: 'kubernetesVersion',
      headerName: 'Kubernetes version',
      flex: 1.5,
      hide: props.briefTable,
    },
    {
      field: 'currentOSVersion',
      headerName: 'Current OS Version',
      flex: 3,
      renderCell: (params) => {
        return <a href={params.row.currentOSURL}>{params.value}</a>
      },
      hide: props.briefTable,
    },
    {
      field: 'osDiffURL',
      headerName: 'Diff',
      flex: 1.25,
      align: 'center',
      headerAlign: 'center',
      renderCell: (params) => {
        if (params.row.previousOSVersion !== '') {
          return (
            <Tooltip title="See diff between these two OS releases">
              <Button
                style={{ justifyContent: 'center' }}
                startIcon={<CompareArrows />}
                href={params.row.osDiffURL}
              />
            </Tooltip>
          )
        }
      },
      hide: props.briefTable,
    },
    {
      field: 'previousOSVersion',
      headerName: 'Previous OS Version',
      flex: 3,
      renderCell: (params) => {
        if (params.value !== '') {
          return <a href={params.row.previousOSURL}>{params.value}</a>
        }
      },
      hide: props.briefTable,
    },
  ]

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

  const [filterModel = props.filterModel, setFilterModel] = useQueryParam(
    'filters',
    JsonParam
  )

  const [sortField = props.sortField, setSortField] = useQueryParam(
    'sortField',
    StringParam
  )
  const [sort = props.sort, setSort] = useQueryParam('sort', StringParam)

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'releaseTag'
    )
    currentFilters.items.push({
      id: 99,
      columnField: 'releaseTag',
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
        '&filter=' + encodeURIComponent(JSON.stringify(filterModel))
    }

    if (props.release !== '') {
      queryString += '&release=' + encodeURIComponent(props.release)
    }

    if (props.limit > 0) {
      queryString += '&limit=' + encodeURIComponent(props.limit)
    }

    queryString += '&sortField=' + encodeURIComponent(sortField)
    queryString += '&sort=' + encodeURIComponent(sort)

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
      autoHeight={true}
      disableColumnFilter={props.briefTable}
      disableColumnMenu={true}
      pageSize={props.pageSize}
      rowsPerPageOptions={[5, 10, 25, 50]}
      getRowClassName={(params) => classes['rowPhase' + params.row.phase]}
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
  sortField: 'releaseTime',
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
