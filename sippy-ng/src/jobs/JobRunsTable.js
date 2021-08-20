import { DataGrid } from '@material-ui/data-grid'
import Alert from '@material-ui/lab/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { StringParam, useQueryParam } from 'use-query-params'
import GridToolbar from '../datagrid/GridToolbar'
import { pathForExactJob, relativeTime } from '../helpers'
import { DirectionsBoat } from '@material-ui/icons'
import { Backdrop, Button, CircularProgress, Container, Tooltip, Typography } from '@material-ui/core'

/**
 * JobRunsTable shows the list of all job runs matching any selected filters.
 */
export default function JobRunsTable (props) {
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

  const [filterModel, setFilterModel] = React.useState(props.filterModel)
  const [filters = JSON.stringify(props.filterModel), setFilters] = useQueryParam('filters', StringParam)

  const [sortField = props.sortField, setSortField] = useQueryParam('sortField', StringParam)
  const [sort = props.sort, setSort] = useQueryParam('sort', StringParam)

  const tooltips = {
    S: 'Success',
    F: 'Failure (e2e)',
    f: 'failure (other tests)',
    U: 'upgrade failure',
    I: 'setup failure (installer)',
    N: 'setup failure (infrastructure)',
    n: 'failure before setup (infra)',
    R: 'running'
  }

  const columns = [
    {
      field: 'id',
      hide: true,
      type: 'number',
      filterable: false

    },
    {
      field: 'timestamp',
      headerName: 'Date / Time',
      filterable: false, // FIXME: probably need server-side date filtering
      flex: 1.5,
      type: 'date',
      valueFormatter: (params) => {
        return new Date(params.value)
      },
      renderCell: (params) => {
        return (
          <Tooltip title={relativeTime(new Date(params.value))}>
            <p>{new Date(params.value).toLocaleString()}</p>
          </Tooltip>
        )
      }
    },
    {
      field: 'job',
      headerName: 'Job name',
      flex: 3,
      renderCell: (params) => {
        return (
          <div style={{ display: 'block', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
            <Tooltip title={params.value}>
              <Link to={pathForExactJob(props.release, params.value)}>
                {params.value}
              </Link>
            </Tooltip>
          </div>
        )
      }
    },
    {
      field: 'testFailures',
      headerName: 'Test Failures',
      type: 'number',
      flex: 1
    },
    {
      field: 'result',
      headerName: 'Result',
      flex: 0.5,
      renderCell: (params) => {
        return (
          <Tooltip title={tooltips[params.value]}>
            <div className={'result result-' + params.value} style={{ width: '100%', textAlign: 'center' }}>
              {params.value}
            </div>
          </Tooltip>
        )
      }
    },
    {
      field: 'url',
      headerName: ' ',
      flex: 0.40,
      renderCell: (params) => {
        return (
          <Tooltip title="View in Prow">
            <Button style={{ justifyContent: 'center' }} target="_blank" startIcon={<DirectionsBoat />}
                    href={params.value} />
          </Tooltip>
        )
      },
      filterable: false
    },
    {
      field: 'variants',
      headerName: 'Variants',
      hide: true
    },
    {
      field: 'failedTestNames',
      headerName: 'Failed tests',
      hide: true
    }
  ]

  const fetchData = () => {
    let queryString = ''
    if (filters && filters !== '') {
      queryString += '&filter=' + encodeURIComponent(filters)
    }

    if (props.limit > 0) {
      queryString += '&limit=' + encodeURIComponent(props.limit)
    }

    queryString += '&sortField=' + encodeURIComponent(sortField)
    queryString += '&sort=' + encodeURIComponent(sort)

    fetch(process.env.REACT_APP_API_URL + '/api/jobs/runs?release=' + props.release + queryString)
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
    currentFilters.items = currentFilters.items.filter((f) => f.columnField !== 'job')
    currentFilters.items.push({ id: 99, columnField: 'job', operatorValue: 'contains', value: searchValue })
    setFilters(JSON.stringify(currentFilters))
  }

  useEffect(() => {
    if (filters && filters !== '') {
      setFilterModel(JSON.parse(filters))
    }

    fetchData()
  }, [filters, sort, sortField])

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

  /* eslint-disable react/prop-types */
  return (
    <Fragment>

      {pageTitle()}
      <br /><br />
      <div>
        <span className="legend-item"><span className="results results-demo"><span
          className="result result-S">S</span></span> success</span>
        <span className="legend-item"><span className="results results-demo"><span
          className="result result-F">F</span></span> failure (e2e)</span>
        <span className="legend-item"><span className="results results-demo"><span
          className="result result-f">f</span></span> failure (other tests)</span>
        <span className="legend-item"><span className="results results-demo"><span
          className="result result-U">U</span></span> upgrade failure</span>
        <span className="legend-item"><span className="results results-demo"><span
          className="result result-I">I</span></span> setup failure (installer)</span>
        <span className="legend-item"><span className="results results-demo"><span
          className="result result-N">N</span></span> setup failure (infra)</span>
        <span className="legend-item"><span className="results results-demo"><span
          className="result result-n">n</span></span> failure before setup (infra)</span>
        <span className="legend-item"><span className="results results-demo"><span
          className="result result-R">R</span></span> running</span>
      </div>
      <Container size="xl" style={{ marginTop: 20 }}>
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

          disableColumnMenu={true}
          rowsPerPageOptions={[5, 10, 25, 50]}
          componentsProps={{
            toolbar: {
              clearSearch: () => requestSearch(''),
              doSearch: requestSearch,
              setFilterModel: (m) => addFilters(m)
            }
          }}

        />
      </Container>
    </Fragment>
  )
}

JobRunsTable.defaultProps = {
  hideControls: false,
  pageSize: 25,
  filterModel: {
    items: []
  },
  sortField: 'testFailures',
  sort: 'desc'
}

JobRunsTable.propTypes = {
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
  sortField: PropTypes.string
}
