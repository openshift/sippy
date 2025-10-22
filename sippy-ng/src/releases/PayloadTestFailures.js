import './PayloadTestFailures.css'
import { Box, Card, Grid, Tooltip, Typography } from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { generateClasses } from '../datagrid/utils'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { apiFetch, safeEncodeURIComponent, SafeJSONParam } from '../helpers'
import { withStyles } from '@mui/styles'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

function PayloadTestFailures(props) {
  const { classes } = props

  // Most things not filterable here, as we are not querying them directly from db,
  // but test name is, and that is likely the most important.
  const columns = [
    {
      field: 'id',
      headerName: 'Test ID',
      hide: true,
      filterable: false,
      sortable: false,
    },
    {
      field: 'name',
      headerName: 'Test',
      flex: 5,
      sortable: false,
    },
    {
      field: 'failure_count',
      headerName: 'Failures',
      flex: 1,
      filterable: false,
      sortable: false,
    },
    {
      // We're working with this field, but in this case there will be only one item in the list of failed payloads,
      // and we want to render the failed jobs beneath it.
      field: 'failed_payloads',
      headerName: 'Failed Jobs',
      sortable: false,
      filterable: false,
      flex: 4,
      renderCell: (params) => {
        const renderJobRunLinks = () => {
          let pl = []
          for (let key of Object.keys(params.value)) {
            var i
            for (i = 0; i < params.value[key].failed_jobs.length; i++) {
              /* Assume the jobs and job runs are the same length and match positionally */
              pl.push(
                <Fragment>
                  <a href={params.value[key].failed_job_runs[i]}>
                    {params.value[key].failed_jobs[i] + '  '}
                  </a>
                  <br />
                </Fragment>
              )
            }
          }
          return pl
        }

        return <Box className="clamped">{renderJobRunLinks()}</Box>
      },
    },
  ]

  const [payload = props.payload, setPayload] = useQueryParam(
    'payload',
    StringParam
  )
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

    Promise.all([
      apiFetch(`/api/payloads/test_failures?payload=${payload}` + queryString),
    ])
      .then(([analysis]) => {
        if (analysis.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }

        return Promise.all([analysis.json()])
      })
      .then(([analysis]) => {
        setRows(analysis)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve payload analysis for ' +
            props.payload +
            ': ' +
            error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [filterModel, sort, sortField])

  if (fetchError !== '') {
    return <Alert severity="error">Failed to load data, {fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  if (rows.length === 0) {
    return <Typography variant="h5">Analysis not found.</Typography>
  }

  return (
    <Grid item md={12}>
      <Card className="test-failure-card" elevation={5}>
        <Typography variant="h5">
          Payload Test Failures
          <Tooltip
            title={
              <p>
                Displays all test failures in this payload across all jobs, both
                blocking and informing.
              </p>
            }
          >
            <InfoIcon />
          </Tooltip>
        </Typography>
        <DataGrid
          components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
          rows={rows}
          columns={columns}
          autoHeight={true}
          rowHeight={125}
          disableColumnFilter={props.briefTable}
          disableColumnMenu={true}
          pageSize={pageSize}
          onPageSizeChange={(newPageSize) => setPageSize(newPageSize)}
          rowsPerPageOptions={[5, 10, 25, 50]}
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
            classes['row-percent-' + params.row.failure_count]
          }
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
      </Card>
    </Grid>
  )
}

PayloadTestFailures.defaultProps = {
  limit: 0,
  hideControls: false,
  pageSize: 25,
  briefTable: false,
  filterModel: {
    items: [],
  },
  sortField: 'kind',
  sort: 'asc',
}

PayloadTestFailures.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  filterModel: PropTypes.object,
  sort: PropTypes.string,
  sortField: PropTypes.string,
  classes: PropTypes.object,

  payload: PropTypes.string,
}

const TEST_FAILURES_IN_PAYLOAD_THRESHOLDS = {
  success: 0,
  warning: 2,
  error: 5,
}

export default withStyles(
  generateClasses(TEST_FAILURES_IN_PAYLOAD_THRESHOLDS, true)
)(PayloadTestFailures)
