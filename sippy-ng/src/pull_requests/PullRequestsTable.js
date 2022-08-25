import { Backdrop, Button, CircularProgress, Tooltip } from '@material-ui/core'
import { BOOKMARKS } from '../constants'
import { DataGrid } from '@material-ui/data-grid'
import { Error, GitHub } from '@material-ui/icons'
import { GridView } from '../datagrid/GridView'
import { makeStyles } from '@material-ui/core/styles'
import { relativeTime, safeEncodeURIComponent, SafeJSONParam } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { useLocation } from 'react-router-dom'
import Alert from '@material-ui/lab/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

const overallTestName = 'Overall'

const bookmarks = [
  {
    name: 'Runs > 10',
    model: [BOOKMARKS.RUN_10],
  },
]

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

export default function PullRequestsTable(props) {
  const { classes } = props
  const gridClasses = useStyles()
  const location = useLocation().pathname

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
      sortField: 'merged_at',
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
          field: 'title',
          flex: 3,
        },
        {
          field: 'author',
          flex: 0.75,
        },
        {
          field: 'merged_at',
          flex: 1,
        },
        {
          field: 'link',
          flex: 0.5,
          hide: props.briefTable,
        },
      ],
    },
    Unmerged: {
      sortField: 'merged_at',
      sort: 'desc',
      fieldOrder: [
        {
          field: 'title',
          flex: 3,
        },
        {
          field: 'link',
          flex: 0.6,
          hide: props.briefTable,
        },
      ],
    },
    Summary: {
      sortField: 'merged_at',
      sort: 'desc',
      fieldOrder: [
        {
          field: 'title',
          flex: 3,
        },
        {
          field: 'merged_at',
          flex: 1.5,
        },
        {
          field: 'link',
          flex: 0.6,
          hide: props.briefTable,
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
    title: {
      field: 'title',
      headerName: 'Title',
    },
    author: {
      field: 'author',
      headerName: 'Author',
      autocomplete: 'authors',
    },
    sha: {
      field: 'sha',
      headerName: 'SHA',
    },
    merged_at: {
      type: 'date',
      field: 'merged_at',
      headerName: 'Merged at',
      renderCell: (params) => {
        if (!params.value) {
          return '—'
        }

        return (
          <Tooltip title={relativeTime(new Date(params.value))}>
            <p>{new Date(params.value).toLocaleString()}</p>
          </Tooltip>
        )
      },
    },
    link: {
      field: 'link',
      headerName: 'Link',
      renderCell: (params) => {
        if (params.value === undefined || params.value === '') {
          return ''
        }

        return (
          <Tooltip title="View pull request">
            <Button
              style={{ justifyContent: 'center' }}
              target="_blank"
              startIcon={<GitHub />}
              href={params.value}
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

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/pull_requests?release=' +
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
            bookmarks: bookmarks,
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
            downloadFilePrefix: 'pull_requests',
          },
        }}
      />
    </Fragment>
  )
}

PullRequestsTable.defaultProps = {
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
  sortField: 'merged_at',
  sort: 'desc',
}

PullRequestsTable.propTypes = {
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
