import { BLOCKER_SCORE_THRESHOLDS } from '../constants'
import { Box, Card, Grid, Tooltip, Typography } from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { generateClasses } from '../datagrid/utils'
import { Link } from 'react-router-dom'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { apiFetch, safeEncodeURIComponent, SafeJSONParam } from '../helpers'
import { withStyles } from '@mui/styles'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

function PayloadStreamTestFailures(props) {
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
      field: 'blocker_score',
      filterable: false,
      sortable: false,
      headerName: 'Blocker Score',
      flex: 1,
      renderCell: (params) => {
        return (
          <div>
            {Number(params.value)}%
            <Tooltip title={params.row.blocker_score_reasons.join(' - ')}>
              <InfoIcon />
            </Tooltip>
          </div>
        )
      },
    },
    {
      field: 'failed_payloads',
      headerName: 'Failed Payloads',
      sortable: false,
      filterable: false,
      flex: 4,
      renderCell: (params) => {
        const renderPayloadLinks = () => {
          let pl = []
          for (let key of Object.keys(params.value)) {
            pl.push(
              <Link key={key} to={`/release/${params.release}/tags/${key}`}>
                {key + '  '}
              </Link>
            )
          }
          return pl
        }

        return (
          <Tooltip
            title={`Failed in ${Object.keys(params.value).length} payloads`}
          >
            <Box className="clamped">{renderPayloadLinks()}</Box>
          </Tooltip>
        )
      },
    },
  ]

  const [release = props.release, setRelease] = useQueryParam(
    'release',
    StringParam
  )
  const [arch = props.arch, setArch] = useQueryParam('arch', StringParam)
  const [stream = props.stream, setStream] = useQueryParam(
    'stream',
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
      apiFetch(
        `/api/releases/test_failures?release=${release}&arch=${arch}&stream=${stream}` +
          queryString
      ),
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
          'Could not retrieve payload stream analysis for ' +
            props.release +
            ' ' +
            props.arch +
            ' ' +
            props.stream +
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
          Test Failures Over 7 Days of Payloads
          <Tooltip
            title={
              <p>
                Displays all test failures in blocking payload jobs for this
                stream over the last 10 attempts. Test failures in informing
                jobs are not considered. Blocker score indicates how likely the
                test is to be fully blocking payloads now.
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
            classes['row-percent-' + params.row.blocker_score]
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

PayloadStreamTestFailures.defaultProps = {
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

PayloadStreamTestFailures.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  filterModel: PropTypes.object,
  sort: PropTypes.string,
  sortField: PropTypes.string,
  classes: PropTypes.object,

  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}

export default withStyles(generateClasses(BLOCKER_SCORE_THRESHOLDS, true))(
  PayloadStreamTestFailures
)
