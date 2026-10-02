import { DataGrid } from '@mui/x-data-grid'
import { makeStyles, useTheme } from '@mui/styles'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { safeEncodeURIComponent, useStableJSONQueryParam } from '../helpers'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

const useStyles = makeStyles((theme) => ({
  rowPhaseAccepted: {
    backgroundColor: theme.palette.success.light,
  },
  rowPhaseRejected: {
    backgroundColor: theme.palette.error.light,
  },
  title: {
    textAlign: 'center',
  },
}))

function ReleasePayloadPullRequests({
  limit = 0,
  hideControls = false,
  pageSize: pageSizeDefault = 25,
  briefTable = false,
  filterModel: filterModelDefault = { items: [] },
  sortField: sortFieldDefault = 'pull_request_id',
  sort: sortDefault = 'asc',
  ...props
}) {
  const theme = useTheme()
  const classes = useStyles(theme)

  const columns = [
    {
      field: 'release_tag',
      headerName: 'Tag',
      hide: true,
      flex: 1,
    },
    {
      field: 'pull_request_id',
      headerName: 'PR',
      flex: 0.5,
      renderCell: (params) => {
        return <a href={params.row.url}>{params.value}</a>
      },
    },
    {
      field: 'name',
      headerName: 'Repo',
      flex: 0.5,
      renderCell: (params) => {
        return <a href={params.row.url}>{params.value}</a>
      },
    },
    {
      field: 'description',
      headerName: 'Description',
      flex: 3,
      renderCell: (params) => {
        return <a href={params.row.url}>{params.value}</a>
      },
    },
  ]

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

  const [filterModel, setFilterModel] = useStableJSONQueryParam(
    'filters',
    filterModelDefault
  )

  const [sortField = sortFieldDefault, setSortField] = useQueryParam(
    'sortField',
    StringParam
  )
  const [sort = sortDefault, setSort] = useQueryParam('sort', StringParam)

  const [pageSize = pageSizeDefault, setPageSize] = useQueryParam(
    'pageSize',
    NumberParam
  )

  const [page, setPage] = React.useState(0)

  const requestSearch = (searchValue) => {
    const newItems = filterModel.items.filter((f) => f.field !== 'release_tag')
    newItems.push({
      id: 99,
      field: 'release_tag',
      operator: 'contains',
      value: searchValue,
    })
    setFilterModel({
      ...filterModel,
      items: newItems,
    })
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
      logicOperator: filterModel.logicOperator || 'and',
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

    if (props.release && props.release !== '') {
      queryString += '&release=' + safeEncodeURIComponent(props.release)
    }

    if (limit > 0) {
      queryString += '&limit=' + safeEncodeURIComponent(limit)
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    fetch(
      import.meta.env.VITE_API_URL +
        '/api/releases/pull_requests?' +
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
      slots={{ toolbar: hideControls ? '' : GridToolbar }}
      rows={rows}
      columns={columns}
      autoHeight={true}
      disableColumnFilter={briefTable}
      disableColumnMenu={true}
      paginationModel={{ page, pageSize }}
      onPaginationModelChange={(model) => {
        setPage(model.page)
        setPageSize(model.pageSize)
      }}
      pageSizeOptions={[5, 10, 25, 50]}
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
      slotProps={{
        toolbar: {
          columns: columns,
          clearSearch: () => requestSearch(''),
          doSearch: requestSearch,
          searchField: 'release_tag',
          addFilters: addFilters,
          filterModel: filterModel,
          setFilterModel: setFilterModel,
        },
      }}
    />
  )
}

ReleasePayloadPullRequests.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  filterModel: PropTypes.object,
  release: PropTypes.string,
  sort: PropTypes.string,
  sortField: PropTypes.string,
}

export default ReleasePayloadPullRequests
