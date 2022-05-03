import './TestTable.css'
import {
  AcUnit,
  BugReport,
  DirectionsRun,
  Error,
  Search,
} from '@material-ui/icons'
import { Badge, Box, Button, Container, Grid, Tooltip } from '@material-ui/core'
import { BOOKMARKS, TEST_THRESHOLDS } from '../constants'
import { DataGrid } from '@material-ui/data-grid'
import {
  escapeRegex,
  pathForJobRunsWithTestFailure,
  pathForJobRunsWithTestFlake,
  safeEncodeURIComponent,
  SafeJSONParam,
  withSort,
} from '../helpers'
import { generateClasses } from '../datagrid/utils'
import { Link } from 'react-router-dom'
import { makeStyles } from '@material-ui/core/styles'
import { StringParam, useQueryParam } from 'use-query-params'
import { withStyles } from '@material-ui/styles'
import Alert from '@material-ui/lab/Alert'
import BugzillaDialog from '../bugzilla/BugzillaDialog'
import GridToolbar from '../datagrid/GridToolbar'
import IconButton from '@material-ui/core/IconButton'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

const bookmarks = [
  {
    name: 'Runs > 10',
    model: [BOOKMARKS.RUN_10],
  },
]

const useStyles = makeStyles({
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
})

function TestTable(props) {
  const { classes } = props
  const gridClasses = useStyles()

  const columns = [
    {
      field: 'name',
      headerName: 'Name',
      flex: 3.5,
      renderCell: (params) => (
        <div className="test-name">
          <Tooltip title={params.value}>
            <Link
              to={
                '/tests/' + props.release + '/analysis?test=' + params.row.name
              }
            >
              {params.value}
            </Link>
          </Tooltip>
        </div>
      ),
    },
    {
      field: 'current_working_percentage',
      headerName: 'Current working percentage',
      headerClassName: props.briefTable ? '' : 'wrapHeader',
      type: 'number',
      flex: 0.75,
      renderCell: (params) => (
        <div className="percentage-cell">
          <Tooltip
            title={
              <div>
                <b>Pass: </b>
                {Number(params.row.current_pass_percentage)
                  .toFixed(1)
                  .toLocaleString()}
                %
                <br />
                <b>Flake: </b>
                {Number(params.row.current_flake_percentage)
                  .toFixed(1)
                  .toLocaleString()}
                %
                <br />
                <b>Fail: </b>
                {Number(params.row.current_failure_percentage)
                  .toFixed(1)
                  .toLocaleString()}
                %
              </div>
            }
          >
            <Box>
              {Number(params.value).toFixed(1).toLocaleString()}%<br />
              <small>({params.row.current_runs} runs)</small>
            </Box>
          </Tooltip>
        </div>
      ),
    },
    {
      field: 'net_working_improvement',
      headerName: 'Improvement',
      type: 'number',
      flex: 0.5,
      renderCell: (params) => {
        return <PassRateIcon tooltip={true} improvement={params.value} />
      },
    },
    {
      field: 'previous_working_percentage',
      headerName: 'Previous working percentage',
      headerClassName: props.briefTable ? '' : 'wrapHeader',
      flex: 0.75,
      type: 'number',
      renderCell: (params) => (
        <div className="percentage-cell">
          <Tooltip
            title={
              <div>
                <b>Pass: </b>
                {Number(params.row.previous_pass_percentage)
                  .toFixed(1)
                  .toLocaleString()}
                %
                <br />
                <b>Flake: </b>
                {Number(params.row.previous_flake_percentage)
                  .toFixed(1)
                  .toLocaleString()}
                %
                <br />
                <b>Fail: </b>
                {Number(params.row.previous_failure_percentage)
                  .toFixed(1)
                  .toLocaleString()}
                %
              </div>
            }
          >
            <Box>
              {Number(params.value).toFixed(1).toLocaleString()}%<br />
              <small>({params.row.previous_runs} runs)</small>
            </Box>
          </Tooltip>
        </div>
      ),
    },
    {
      field: 'link',
      headerName: ' ',
      flex: 1.5,
      hide: props.briefTable,
      filterable: false,
      renderCell: (params) => {
        return (
          <Grid container justifyContent="space-between">
            <Tooltip title="Search CI Logs">
              <IconButton
                target="_blank"
                href={
                  'https://search.ci.openshift.org/?search=' +
                  safeEncodeURIComponent(escapeRegex(params.row.name)) +
                  '&maxAge=336h&context=1&type=bug%2Bjunit&name=&excludeName=&maxMatches=5&maxBytes=20971520&groupBy=job'
                }
              >
                <Search />
              </IconButton>
            </Tooltip>
            <Tooltip title="See job runs that failed this test">
              <IconButton
                component={Link}
                to={withSort(
                  pathForJobRunsWithTestFailure(
                    props.release,
                    params.row.name,
                    filterModel
                  ),
                  'timestamp',
                  'desc'
                )}
              >
                <Badge
                  badgeContent={
                    params.row.current_failures + params.row.previous_failures
                  }
                  color="error"
                >
                  <Error />
                </Badge>
              </IconButton>
            </Tooltip>
            <Tooltip title="See job runs that flaked on this test">
              <IconButton
                component={Link}
                to={withSort(
                  pathForJobRunsWithTestFlake(
                    props.release,
                    params.row.name,
                    filterModel
                  ),
                  'timestamp',
                  'desc'
                )}
              >
                <Badge
                  badgeContent={
                    params.row.current_flakes + params.row.previous_flakes
                  }
                  color="error"
                >
                  <AcUnit />
                </Badge>
              </IconButton>
            </Tooltip>
            <Tooltip title="Find Bugs">
              <IconButton
                target="_blank"
                href={
                  'https://search.ci.openshift.org/?search=' +
                  safeEncodeURIComponent(escapeRegex(params.row.name)) +
                  '&maxAge=336h&context=1&type=bug&name=&excludeName=&maxMatches=5&maxBytes=20971520&groupBy=job'
                }
              >
                <BugReport />
              </IconButton>
            </Tooltip>
          </Grid>
        )
      },
    },
    // These are here just to allow filtering
    {
      field: 'variants',
      headerName: 'Variants',
      hide: true,
      type: 'array',
    },
    {
      field: 'current_runs',
      headerName: 'Current runs',
      hide: true,
      type: 'number',
    },
    {
      field: 'current_failures',
      headerName: 'Current failures',
      hide: true,
      type: 'number',
    },
    {
      field: 'current_flakes',
      headerName: 'Current failures',
      hide: true,
      type: 'number',
    },
    {
      field: 'current_pass_percentage',
      headerName: 'Current pass percentage',
      hide: true,
      type: 'number',
    },
    {
      field: 'current_flake_percentage',
      headerName: 'Current flake percentage',
      hide: true,
      type: 'number',
    },
    {
      field: 'current_failure_percentage',
      headerName: 'Current failure percentage',
      hide: true,
      type: 'number',
    },
    {
      field: 'previous_runs',
      headerName: 'Previous runs',
      hide: true,
      type: 'number',
    },
    {
      field: 'previous_failures',
      headerName: 'Previous failures',
      hide: true,
      type: 'number',
    },
    {
      field: 'previous_flakes',
      headerName: 'Previous failures',
      hide: true,
      type: 'number',
    },
    {
      field: 'previous_pass_percentage',
      headerName: 'Previous pass percentage',
      hide: true,
      type: 'number',
    },
    {
      field: 'previous_flake_percentage',
      headerName: 'Previous flake percentage',
      hide: true,
      type: 'number',
    },
    {
      field: 'previous_failure_percentage',
      headerName: 'Previous failure percentage',
      hide: true,
      type: 'number',
    },
    {
      field: 'tags',
      headerName: 'Tags',
      hide: true,
    },
  ]

  const openBugzillaDialog = (test) => {
    setTestDetails(test)
    setBugzillaDialogOpen(true)
  }

  const closeBugzillaDialog = (details) => {
    setBugzillaDialogOpen(false)
  }

  const [isBugzillaDialogOpen, setBugzillaDialogOpen] = React.useState(false)
  const [testDetails, setTestDetails] = React.useState({ bugs: [] })

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])
  const [selectedTests, setSelectedTests] = React.useState([])

  const [period = props.period, setPeriod] = useQueryParam(
    'period',
    StringParam
  )

  const [filterModel = props.filterModel, setFilterModel] = useQueryParam(
    'filters',
    SafeJSONParam
  )

  const [sortField = props.sortField, setSortField] = useQueryParam(
    'sortField',
    StringParam
  )
  const [sort = props.sort, setSort] = useQueryParam('sort', StringParam)

  const fetchData = () => {
    let queryString = ''
    if (filterModel && filterModel.items.length > 0) {
      queryString +=
        '&filter=' + safeEncodeURIComponent(JSON.stringify(filterModel))
    }

    if (props.limit > 0) {
      queryString += '&limit=' + safeEncodeURIComponent(props.limit)
    }

    if (period) {
      queryString += '&period=' + safeEncodeURIComponent(period)
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    fetch(
      process.env.REACT_APP_API_URL +
        '/api/tests?release=' +
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
  }, [period, filterModel, sort, sortField])

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

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return <p>Loading...</p>
  }

  const createTestNameQuery = () => {
    const selectedIDs = new Set(selectedTests)
    let tests = rows.filter((row) => selectedIDs.has(row.id))
    tests = tests.map((test) => 'test=' + safeEncodeURIComponent(test.name))
    return tests.join('&')
  }

  const detailsButton = (
    <Button
      component={Link}
      to={'/tests/' + props.release + '/details?' + createTestNameQuery()}
      variant="contained"
      color="primary"
      style={{ margin: 10 }}
    >
      Get Details
    </Button>
  )

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
    <Container size="xl">
      <DataGrid
        className={gridClasses.root}
        components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
        rows={rows}
        columns={columns}
        autoHeight={true}
        rowHeight={100}
        disableColumnFilter={props.briefTable}
        disableColumnMenu={true}
        pageSize={props.pageSize}
        rowsPerPageOptions={props.rowsPerPageOptions}
        checkboxSelection={!props.hideControls}
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
        onSelectionModelChange={(rows) => setSelectedTests(rows)}
        getRowClassName={(params) =>
          classes[
            'row-percent-' + Math.round(params.row.current_working_percentage)
          ]
        }
        componentsProps={{
          toolbar: {
            bookmarks: bookmarks,
            columns: columns,
            clearSearch: () => requestSearch(''),
            doSearch: requestSearch,
            period: period,
            selectPeriod: setPeriod,
            addFilters: addFilters,
            filterModel: filterModel,
            setFilterModel: setFilterModel,
          },
        }}
      />

      {props.hideControls ? '' : detailsButton}

      <BugzillaDialog
        release={props.release}
        item={testDetails}
        isOpen={isBugzillaDialogOpen}
        close={closeBugzillaDialog}
      />
    </Container>
  )
}

TestTable.defaultProps = {
  limit: 0,
  hideControls: false,
  pageSize: 25,
  period: 'default',
  rowsPerPageOptions: [5, 10, 25, 50, 100],
  briefTable: false,
  filterModel: {
    items: [],
  },
  sortField: 'current_working_percentage',
  sort: 'asc',
}

TestTable.propTypes = {
  briefTable: PropTypes.bool,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  release: PropTypes.string.isRequired,
  classes: PropTypes.object,
  period: PropTypes.string,
  filterModel: PropTypes.object,
  sort: PropTypes.string,
  sortField: PropTypes.string,
  rowsPerPageOptions: PropTypes.array,
}

export default withStyles(generateClasses(TEST_THRESHOLDS))(TestTable)
