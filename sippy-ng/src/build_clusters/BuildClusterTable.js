import { BUILD_CLUSTER_THRESHOLDS } from '../constants'
import { CircularProgress, Tooltip } from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { generateClasses } from '../datagrid/utils'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { safeEncodeURIComponent, SafeJSONParam } from '../helpers'
import { withStyles } from '@mui/styles'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

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
  },
}))

function BuildClusterTable(props) {
  const gridClasses = useStyles()
  const { classes } = props

  // place to store state (i.e., our table data, error message, etc)
  const [rows, setRows] = React.useState([])
  const [error, setError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)

  const [period = props.period, setPeriod] = useQueryParam(
    'period',
    StringParam
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

  const [filterModel = props.filterModel, setFilterModel] = useQueryParam(
    'filters',
    SafeJSONParam
  )

  // define table columns
  const columns = [
    {
      field: 'cluster',
      headerName: 'Cluster',
      flex: 1,
      renderCell: (params) => {
        return (
          <div className="cluster-name">
            <Tooltip title={params.value}>
              <Link to={`/build_clusters/${params.value}`}>{params.value}</Link>
            </Tooltip>
          </div>
        )
      },
    },
    {
      field: 'current_pass_percentage',
      headerName: 'Current pass percentage',
      renderCell: (params) => {
        return Number(params.value).toFixed(2) + '%'
      },
      flex: 1,
    },
    {
      field: 'net_improvement',
      headerName: 'Net improvement',
      flex: 0.5,
      renderCell: (params) => {
        return <PassRateIcon tooltip={true} improvement={params.value} />
      },
    },
    {
      field: 'previous_pass_percentage',
      headerName: 'Previous pass percentage',
      renderCell: (params) => {
        return Number(params.value).toFixed(2) + '%'
      },
      flex: 1,
    },
  ]

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

  // fetch data from api
  const fetchData = () => {
    let queryString = ''
    if (period) {
      queryString += '?period=' + safeEncodeURIComponent(period)
    }

    fetch(
      process.env.REACT_APP_API_URL + '/api/health/build_cluster' + queryString
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
        setError('Could not retrieve build cluster health: ' + error)
      })
  }

  useEffect(() => {
    fetchData()
  }, [period])

  if (error !== '') {
    return <Alert severity="error">{error}</Alert>
  }

  // loading message
  if (!isLoaded) {
    return <CircularProgress color="secondary" />
  }

  // what we return
  return (
    <DataGrid
      className={gridClasses.root}
      components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
      rows={rows}
      columns={columns}
      autoHeight={true}
      disableColumnFilter={props.briefTable}
      disableColumnMenu={true}
      pageSize={pageSize}
      onPageSizeChange={(newPageSize) => setPageSize(newPageSize)}
      rowsPerPageOptions={props.rowsPerPageOptions}
      checkboxSelection={false}
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
      getRowClassName={(params) =>
        classes['row-percent-' + Math.round(params.row.current_pass_percentage)]
      }
      componentsProps={{
        toolbar: {
          columns: columns,
          period: period,
          selectPeriod: setPeriod,
          addFilters: addFilters,
          filterModel: filterModel,
          setFilterModel: setFilterModel,
          downloadDataFunc: () => {
            return rows
          },
          downloadFilePrefix: 'clusters',
        },
      }}
    />
  )
}

export default withStyles(generateClasses(BUILD_CLUSTER_THRESHOLDS))(
  BuildClusterTable
)

BuildClusterTable.defaultProps = {
  briefTable: false,
  hideControls: false,
  pageSize: 25,
  period: 'default',
  rowsPerPageOptions: [5, 10, 25, 50, 100],
  filterModel: {
    items: [],
  },
}

BuildClusterTable.propTypes = {
  briefTable: PropTypes.bool,
  classes: PropTypes.object,
  hideControls: PropTypes.bool,
  pageSize: PropTypes.number,
  period: PropTypes.string,
  rowsPerPageOptions: PropTypes.array,
  sort: PropTypes.string,
  sortField: PropTypes.string,
  filterModel: PropTypes.object,
}
