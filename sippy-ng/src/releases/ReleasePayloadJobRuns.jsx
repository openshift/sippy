import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  List,
  ListItem,
  ListItemText,
  Tooltip,
} from '@mui/material'
import { Check, DirectionsBoat } from '@mui/icons-material'
import { DataGrid } from '@mui/x-data-grid'
import { makeStyles, useTheme } from '@mui/styles'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { safeEncodeURIComponent, useStableJSONQueryParam } from '../helpers'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'
import ReactMarkdown from 'react-markdown'

const useStyles = makeStyles((theme) => ({
  rowPhaseSucceeded: {
    backgroundColor: theme.palette.success.light,
  },
  rowPhaseFailed: {
    backgroundColor: theme.palette.error.light,
  },
  title: {
    textAlign: 'center',
  },
}))

function ReleasePayloadJobRuns(props) {
  const theme = useTheme()
  const classes = useStyles(theme)

  const columns = [
    {
      field: 'release_tag',
      headerName: 'Tag',
      hide: true,
    },
    {
      field: 'job_name',
      headerName: 'Job name',
      flex: 3,
    },

    {
      field: 'upgrades_from',
      headerName: 'Upgrades from',
      flex: 3,
    },
    {
      field: 'upgrades_to',
      headerName: 'Upgrades to',
      flex: 3,
    },
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
    {
      field: 'labels',
      autocomplete: 'labels',
      headerName: 'Labels',
      type: 'array',
      flex: 0.5,
      sortable: false,
      renderCell: (params) => {
        if (!params.value || params.value.length === 0) {
          return ''
        }
        const labelTitles = params.value.map((labelId) => {
          const label = allLabels[labelId]
          return label ? label.label_title : labelId
        })
        return (
          <Tooltip
            title={
              <div>
                {labelTitles.map((title, idx) => (
                  <div key={idx}>{title}</div>
                ))}
              </div>
            }
          >
            <Button
              color="inherit"
              variant="text"
              onClick={() => {
                setSelectedLabels(params.value)
                setSelectedJobRun(params.row)
                setLabelsDialogOpen(true)
              }}
            >
              {params.value.length}
            </Button>
          </Tooltip>
        )
      },
    },
    {
      field: 'url',
      headerName: ' ',
      flex: 0.75,
      filterable: false,
      renderCell: (params) => {
        return (
          <Tooltip title="View in Prow">
            <Button
              style={{ justifyContent: 'center' }}
              target="_blank"
              startIcon={<DirectionsBoat />}
              href={params.value}
            />
          </Tooltip>
        )
      },
    },
  ]

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])
  const [labelsDialogOpen, setLabelsDialogOpen] = React.useState(false)
  const [selectedLabels, setSelectedLabels] = React.useState([])
  const [selectedJobRun, setSelectedJobRun] = React.useState(null)
  const [allLabels, setAllLabels] = React.useState({})

  const [filterModel, setFilterModel] = useStableJSONQueryParam(
    'filters',
    props.filterModel
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
    const newItems = filterModel.items.filter(
      (f) => f.columnField !== 'release_tag'
    )
    newItems.push({
      id: 99,
      columnField: 'release_tag',
      operatorValue: 'contains',
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

    if (props.release && props.release !== '') {
      queryString += '&release=' + safeEncodeURIComponent(props.release)
    }

    if (props.limit > 0) {
      queryString += '&limit=' + safeEncodeURIComponent(props.limit)
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    fetch(
      import.meta.env.VITE_API_URL +
        '/api/releases/job_runs?' +
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
  }, [filterModel, sort, sortField, pageSize])

  // Fetch label definitions
  useEffect(() => {
    fetch(import.meta.env.VITE_API_URL + '/api/jobs/labels')
      .then((response) => {
        if (response.status !== 200) {
          console.warn(
            'Labels API returned unsuccessful status: ' + response.statusText
          )
          return {}
        }
        return response.json()
      })
      .then((labels) => {
        const labelMap = {}
        if (labels && Array.isArray(labels)) {
          labels.forEach((label) => {
            labelMap[label.id] = label
          })
        }
        setAllLabels(labelMap)
      })
      .catch((error) => {
        console.error('Could not fetch labels:', error)
      })
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return <p>Loading...</p>
  }

  const labelsDialog = (
    <Dialog
      open={labelsDialogOpen}
      onClose={() => setLabelsDialogOpen(false)}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>
        {selectedJobRun
          ? `Labels for ${selectedJobRun.job_name}`
          : 'Job Run Labels'}
      </DialogTitle>
      <DialogContent>
        <List>
          {selectedLabels.map((labelId) => {
            const label = allLabels[labelId]
            return (
              <ListItem key={labelId}>
                <ListItemText
                  primary={label ? label.label_title : labelId}
                  secondary={
                    label ? (
                      <ReactMarkdown>{label.explanation}</ReactMarkdown>
                    ) : (
                      'Label not found'
                    )
                  }
                />
              </ListItem>
            )
          })}
        </List>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => setLabelsDialogOpen(false)} color="primary">
          Close
        </Button>
      </DialogActions>
    </Dialog>
  )

  return (
    <>
      <DataGrid
        components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
        rows={rows}
        columns={columns}
        autoHeight={true}
        getRowClassName={(params) => classes['rowPhase' + params.row.state]}
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
        componentsProps={{
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
      {labelsDialog}
    </>
  )
}

ReleasePayloadJobRuns.defaultProps = {
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

ReleasePayloadJobRuns.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  filterModel: PropTypes.object,
  release: PropTypes.string,
  sort: PropTypes.string,
  sortField: PropTypes.string,
}

export default ReleasePayloadJobRuns
