import { Button, Tooltip, Typography } from '@material-ui/core'
import { Check, DirectionsBoat, Error } from '@material-ui/icons'
import { createTheme, makeStyles } from '@material-ui/core/styles'
import { DataGrid } from '@material-ui/data-grid'
import { safeEncodeURIComponent, SafeJSONParam } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import Alert from '@material-ui/lab/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    rowPhaseSucceeded: {
      backgroundColor: theme.palette.success.light,
    },
    rowPhaseFailed: {
      backgroundColor: theme.palette.error.light,
    },
    title: {
      textAlign: 'center',
    },
  }),
  { defaultTheme }
)

function ReleaseStreamAnalysis(props) {
  const classes = useStyles()

  const columns = [
    {
      field: 'id',
      headerName: 'Test ID',
      hide: true,
    },
    {
      field: 'name',
      headerName: 'Test',
      flex: 3,
    },
    {
      field: 'blocker_score',
      headerName: 'Blocker Score',
      flex: 3,
    },
    /*
    {
      field: 'kind',
      headerName: 'Blocking',
      flex: 1.25,
      renderCell: (params) => {
        if (params.value === 'Blocking') {
          return <Check />
        } else {
          return <></>
        }
      },
    },

       */
  ]

  // analysis stores the output from the health API
  const [analysis, setAnalysis] = React.useState({})

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

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'release_tag'
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
    /*
    const filter = safeEncodeURIComponent(
      JSON.stringify({
        items: [filterFor('release_tag', 'equals', releaseTag)],
      })
    )
     */

    Promise.all([
      fetch(
        `${process.env.REACT_APP_API_URL}/api/releases/stream_analysis?release=${release}&arch=${arch}&stream=${stream}`
      ),
    ])
      .then(([analysis]) => {
        if (analysis.status !== 200) {
          throw new Error('server returned ' + analysis.status)
        }

        return Promise.all([analysis.json()])
      })
      .then(([analysis]) => {
        if (analysis.length === 0) {
          return <Typography variant="h5">Analysis not found.</Typography>
        }
        console.log('GOT ANALYSIS 2')
        console.log(analysis)

        setAnalysis(analysis)
        setRows(analysis['test_failures'])
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
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">Failed to load data, {fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    <DataGrid
      components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
      rows={rows}
      columns={columns}
      autoHeight={true}
      getRowClassName={(params) => classes['rowPhase' + params.row.state]}
      disableColumnFilter={props.briefTable}
      disableColumnMenu={true}
      pageSize={props.pageSize}
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

ReleaseStreamAnalysis.defaultProps = {
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

ReleaseStreamAnalysis.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  filterModel: PropTypes.object,
  sort: PropTypes.string,
  sortField: PropTypes.string,

  release: PropTypes.string,
  arch: PropTypes.string.isRequired,
  stream: PropTypes.string.isRequired,
}

export default ReleaseStreamAnalysis
