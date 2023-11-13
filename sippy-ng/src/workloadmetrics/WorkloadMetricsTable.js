// TODO: separate css for this file?
import '../jobs/JobTable.css'
import { DataGrid } from '@material-ui/data-grid'
import { Grid } from '@mui/material'
import { SafeJSONParam } from '../helpers'
import { useQueryParam } from 'use-query-params'
import Alert from '@mui/lab/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

const columns = [
  {
    field: 'workload',
    headerName: 'Workload',
    flex: 1.5,
    renderCell: (params) => {
      return <div className="job-name">{params.value}</div>
    },
  },
  {
    field: 'platform',
    headerName: 'Platform',
    flex: 0.8,
    renderCell: (params) => {
      return <div className="job-name">{params.value}</div>
    },
  },
  {
    field: 'networkType',
    headerName: 'Network',
    flex: 0.8,
    renderCell: (params) => {
      return <div className="job-name">{params.value}</div>
    },
  },
  {
    field: 'machineCounts',
    headerName: 'Nodes',
    flex: 0.8,
    filterable: false,
    renderCell: (params) => {
      return <div className="job-name">{params.value}</div>
    },
  },
  {
    field: 'currentAvgCPU',
    headerName: 'Avg CPU Current (Millicores)',
    flex: 1,
    type: 'number',
    renderCell: (params) => {
      return (
        <div className="job-name">
          {Math.round(params.value * 100) / 100} mc
        </div>
      )
    },
  },
  {
    field: 'previousAvgCPU',
    headerName: 'Avg CPU Previous (Millicores)',
    flex: 1,
    type: 'number',
    renderCell: (params) => {
      return (
        <div className="job-name">
          {Math.round(params.value * 100) / 100} mc
        </div>
      )
    },
  },
  {
    field: 'currentMaxCPU',
    headerName: 'Max CPU Current (Millicores)',
    flex: 1,
    type: 'number',
    renderCell: (params) => {
      return (
        <div className="job-name">
          {Math.round(params.value * 100) / 100} mc
        </div>
      )
    },
  },
  {
    field: 'previousMaxCPU',
    headerName: 'Max CPU Previous (Millicores)',
    flex: 1,
    type: 'number',
    renderCell: (params) => {
      return (
        <div className="job-name">
          {Math.round(params.value * 100) / 100} mc
        </div>
      )
    },
  },
  {
    field: 'currentAvgMem',
    headerName: 'Avg Memory Current (Mb)',
    flex: 1,
    type: 'number',
    renderCell: (params) => {
      return (
        <div className="job-name">
          {Math.round(params.value / 1024 / 1024)} Mb
        </div>
      )
    },
  },
  {
    field: 'previousAvgMem',
    headerName: 'Avg Memory Previous (Mb)',
    flex: 1,
    type: 'number',
    renderCell: (params) => {
      return (
        <div className="job-name">
          {Math.round(params.value / 1024 / 1024)} Mb
        </div>
      )
    },
  },
  {
    field: 'currentMaxMem',
    headerName: 'Max Memory Current (Mb)',
    flex: 1,
    type: 'number',
    renderCell: (params) => {
      return (
        <div className="job-name">
          {Math.round(params.value / 1024 / 1024)} Mb
        </div>
      )
    },
  },
  {
    field: 'previousMaxMem',
    headerName: 'Max Memory Previous (Mb)',
    flex: 1,
    type: 'number',
    renderCell: (params) => {
      return (
        <div className="job-name">
          {Math.round(params.value / 1024 / 1024)} Mb
        </div>
      )
    },
  },
]

/**
 * WorkloadMetricsTable shows a table of workload metrics (avg/max CPU, current vs previous) for OpenShift perfscale team job runs.
 */
function WorkloadMetricsTable(props) {
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

  const [filterModel = props.filterModel, setFilterModel] = useQueryParam(
    'filters',
    SafeJSONParam
  )

  useEffect(() => {
    fetchData()
  }, [])

  const fetchData = () => {
    let queryString = ''
    if (filterModel && filterModel.items.length > 0) {
      queryString +=
        '&filter=' + encodeURIComponent(JSON.stringify(filterModel))
    }

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/perfscalemetrics?release=' +
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
        let i = 1
        for (const r of json) {
          // Add a fake column with the controlplane/infra/worker counts we want to see.
          r[
            'machineCounts'
          ] = `${r.controlPlaneCount}/${r.infraCount}/${r.workerCount}`
        }
        setRows(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve scale jobs ' + props.release + ', ' + error
        )
      })
  }

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'workload'
    )
    currentFilters.items.push({
      id: 99,
      columnField: 'workload',
      operatorValue: 'contains',
      value: searchValue,
    })
    setFilterModel(currentFilters)
  }

  useEffect(() => {
    fetchData()
  }, [filterModel])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    /* eslint-disable react/prop-types */
    <Grid container spacing={3} alignItems="stretch">
      <Grid item md={12}>
        <DataGrid
          components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
          rows={rows}
          columns={columns}
          autoHeight={true}
          componentsProps={{
            toolbar: {
              clearSearch: () => requestSearch(''),
              doSearch: requestSearch,
              filterModel: filterModel,
              setFilterModel: setFilterModel,
              columns: columns,
              addFilters: (m) => addFilters(m),
            },
          }}
        />
      </Grid>
    </Grid>
  )
}

WorkloadMetricsTable.defaultProps = {
  hideControls: false,
  pageSize: 25,
  limit: 10,
  filterModel: {
    items: [],
  },
}

WorkloadMetricsTable.propTypes = {
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  release: PropTypes.string.isRequired,
  title: PropTypes.string,
  hideControls: PropTypes.bool,
  job: PropTypes.string,
  filterModel: PropTypes.object,
}

export default WorkloadMetricsTable
