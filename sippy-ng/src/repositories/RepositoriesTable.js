import {
  Backdrop,
  Button,
  CircularProgress,
  Link,
  Tooltip,
  Typography,
} from '@mui/material'
import { DataGrid } from '@mui/x-data-grid'
import { generateClasses } from '../datagrid/utils'
import { GridView } from '../datagrid/GridView'
import { Launch } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import { MERGE_FAILURE_THERSHOLDS } from '../constants'
import {
  apiFetch,
  pathForRepository,
  safeEncodeURIComponent,
  SafeJSONParam,
} from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { useNavigate } from 'react-router-dom'
import { withStyles } from '@mui/styles'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

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
    backdrop: {
      zIndex: 999999,
      color: '#fff',
    },
  },
}))

function RepositoriesTable(props) {
  const PREMERGE_JOB_FAILURES_TOOLTIP =
    'Premerge job failures shows the average number of failures for the worst performing job. ' +
    'Failures exclude developer pushes or successful retests due to code changes.  It only looks ' +
    'at failures on merged commit shas.'

  const REVERT_COUNT_TOOLTIP =
    'Revert count is our best guess for how many reverts this repo has had merged over the last 90 days.'

  const { classes } = props
  const gridClasses = useStyles()
  const navigate = useNavigate()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])

  const [filterModel = props.filterModel, setFilterModel] = useQueryParam(
    'filters',
    SafeJSONParam
  )

  const [view = props.view, setView] = useQueryParam('view', StringParam)

  const [sortField = props.sortField, setSortField] = useQueryParam(
    'sortField',
    StringParam
  )
  const [sort = props.sort, setSort] = useQueryParam('sort', StringParam)

  const views = {
    Default: {
      sortField: 'worst_premerge_job_failures',
      sort: 'desc',
      fieldOrder: [
        {
          field: 'org',
          flex: 1,
        },
        {
          field: 'repo',
          flex: 1,
        },
        {
          field: 'job_count',
          flex: 2,
        },
        {
          field: 'revert_count',
          flex: 2,
        },
        {
          field: 'worst_premerge_job_failures',
          flex: 2,
        },
        {
          field: 'link',
          flex: 0.5,
        },
      ],
    },
  }

  const columns = {
    org: {
      field: 'org',
      headerName: 'Org',
      autocomplete: 'orgs',
    },
    repo: {
      field: 'repo',
      headerName: 'Repo',
      autocomplete: 'repos',
    },
    job_count: {
      field: 'job_count',
      headerName: 'Job count',
      type: 'number',
    },
    revert_count: {
      field: 'revert_count',
      headerName: (
        <Fragment>
          <Tooltip title={REVERT_COUNT_TOOLTIP}>
            <Typography>
              Revert count
              <InfoIcon />
            </Typography>
          </Tooltip>
        </Fragment>
      ),
      type: 'number',
    },
    worst_premerge_job_failures: {
      field: 'worst_premerge_job_failures',
      headerName: (
        <Fragment>
          <Tooltip title={PREMERGE_JOB_FAILURES_TOOLTIP}>
            <Typography>
              Premerge job failures
              <InfoIcon />
            </Typography>
          </Tooltip>
        </Fragment>
      ),
      type: 'number',
      renderCell: (params) => Number(params.value).toFixed(1).toLocaleString(),
    },
    link: {
      field: 'link',
      headerName: ' ',
      filterable: false,
      sortable: false,
      renderCell: (params) => {
        return (
          <Tooltip title="Details">
            <Button
              color="inherit"
              component={Link}
              target="_blank"
              startIcon={<Launch />}
              to={pathForRepository(
                props.release,
                params.row.org,
                params.row.repo
              )}
            />
          </Tooltip>
        )
      },
    },
  }

  const gridView = new GridView(columns, views, view)

  const selectView = (v) => {
    setLoaded(false)
    setView(v)
    gridView.setView(v)
    setSort(gridView.view.sort)
    setSortField(gridView.view.sortField)
  }

  const fetchData = () => {
    let queryString = ''
    if (filterModel && filterModel.items.length > 0) {
      queryString +=
        '&filter=' + safeEncodeURIComponent(JSON.stringify(filterModel))
    }

    if (props.limit > 0) {
      queryString += '&limit=' + safeEncodeURIComponent(props.limit)
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    apiFetch(
      '/api/repositories?release=' +
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
        if (json != null) {
          setRows(json)
        } else {
          setRows([])
        }
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve tests ' + props.release + ', ' + error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [filterModel, sort, sortField, view])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    if (props.briefTable) {
      return <p>Loading...</p>
    } else {
      return (
        <Backdrop className={gridClasses.backdrop} open={true}>
          <CircularProgress color="inherit" />
        </Backdrop>
      )
    }
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

  const requestSearch = (searchValue) => {
    const currentFilters = filterModel
    currentFilters.items = currentFilters.items.filter(
      (f) => f.columnField !== 'repo'
    )
    currentFilters.items.push({
      id: 99,
      columnField: 'repo',
      operatorValue: 'contains',
      value: searchValue,
    })
    setFilterModel(currentFilters)
  }

  return (
    /* eslint-disable react/prop-types */
    <Fragment>
      <DataGrid
        className={gridClasses.root}
        components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
        rows={rows}
        density="compact"
        columns={gridView.columns}
        autoHeight={true}
        rowHeight={100}
        disableColumnFilter={props.briefTable}
        disableColumnMenu={true}
        pageSize={props.pageSize}
        rowsPerPageOptions={props.rowsPerPageOptions}
        getRowClassName={(params) =>
          classes[
            'row-percent-' + Math.round(params.row.worst_premerge_job_failures)
          ]
        }
        onRowClick={(e) =>
          navigate(`/repositories/${props.release}/${e.row.org}/${e.row.repo}`)
        }
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
        componentsProps={{
          toolbar: {
            doSearch: requestSearch,
            clearSearch: () => requestSearch(''),
            views: gridView.views,
            view: view,
            selectView: selectView,
            columns: gridView.filterColumns,
            addFilters: addFilters,
            filterModel: filterModel,
            setFilterModel: setFilterModel,
            downloadDataFunc: () => {
              return rows
            },
            downloadFilePrefix: 'repositories',
          },
        }}
      />
    </Fragment>
  )
}

RepositoriesTable.defaultProps = {
  collapse: true,
  limit: 0,
  hideControls: false,
  pageSize: 25,
  view: 'Default',
  rowsPerPageOptions: [5, 10, 25, 50, 100],
  briefTable: false,
  filterModel: {
    items: [],
  },
  sortField: 'worst_premerge_job_failures',
  sort: 'desc',
}

RepositoriesTable.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  release: PropTypes.string.isRequired,
  classes: PropTypes.object,
  filterModel: PropTypes.object,
  sort: PropTypes.string,
  sortField: PropTypes.string,
  rowsPerPageOptions: PropTypes.array,
  view: PropTypes.string,
}

export default withStyles(generateClasses(MERGE_FAILURE_THERSHOLDS))(
  RepositoriesTable
)
