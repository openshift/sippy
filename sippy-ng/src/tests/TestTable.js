import './TestTable.css'
import { AcUnit, Error as ErrorIcon, Search } from '@mui/icons-material'
import {
  Backdrop,
  Badge,
  Box,
  CircularProgress,
  Grid,
  Tooltip,
} from '@mui/material'
import { BOOKMARKS, TEST_THRESHOLDS } from '../constants'
import {
  escapeRegex,
  filterFor,
  pathForExactTestAnalysisWithFilter,
  pathForJobRunsWithTestFailure,
  pathForJobRunsWithTestFlake,
  safeEncodeURIComponent,
  SafeJSONParam,
  withSort,
} from '../helpers'
import { generateClasses } from '../datagrid/utils'
import { GridView } from '../datagrid/GridView'
import { Link, useLocation } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { NumberParam, StringParam, useQueryParam } from 'use-query-params'
import { StyledDataGrid } from '../datagrid/StyledDataGrid'
import { useCookies } from 'react-cookie'
import { withStyles } from '@mui/styles'
import Alert from '@mui/material/Alert'
import GridToolbar from '../datagrid/GridToolbar'
import IconButton from '@mui/material/IconButton'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect, useRef } from 'react'

const overallTestName = 'Overall'

const bookmarks = [
  {
    name: 'Runs > 10',
    model: [BOOKMARKS.RUN_10],
  },
]

const useStyles = makeStyles((theme) => ({
  backdrop: {
    zIndex: 999999,
    color: '#fff',
  },
}))

function TestTable(props) {
  const { classes } = props
  const gridClasses = useStyles()
  const location = useLocation().pathname

  const [testDetails, setTestDetails] = React.useState({ bugs: [] })

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [rows, setRows] = React.useState([])
  const [selectedTests, setSelectedTests] = React.useState([])

  const [period = props.period, setPeriod] = useQueryParam(
    'period',
    StringParam
  )

  const [view = props.view, setView] = useQueryParam('view', StringParam)

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

  const [cookies, setCookie] = useCookies(['testTableDBSource'])
  const testTableDBSource = cookies['testTableDBSource']

  const views = {
    Working: {
      sortField: 'current_working_percentage',
      sort: 'asc',
      rowColor: {
        field: 'current_working_percentage',
      },
      fieldOrder: [
        {
          field: 'name',
          flex: 3.5,
        },
        {
          field: 'suite_name',
          flex: 1.0,
          hide: props.collapse,
        },
        {
          field: 'variants',
          flex: 1.75,
          hide: props.collapse,
        },
        {
          field: 'delta_from_working_average',
          flex: 0.75,
          hide: props.collapse,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'working_average',
          flex: 0.75,
          hide: props.collapse || props.briefTable,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'working_standard_deviation',
          flex: 0.75,
          hide: true,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'current_working_percentage',
          flex: 0.75,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'net_working_improvement',
          flex: 0.5,
          hide: !props.collapse && props.briefTable,
        },
        {
          field: 'previous_working_percentage',
          flex: 0.75,
          hide: !props.collapse && props.briefTable,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'open_bugs',
          flex: 0.5,
          hide: props.briefTable,
        },
        {
          field: 'link',
          flex: props.collapse ? 1.25 : 2,
          hide: props.briefTable,
        },
      ],
    },
    Passing: {
      sortField: 'current_pass_percentage',
      sort: 'asc',
      rowColor: {
        field: 'current_pass_percentage',
      },
      fieldOrder: [
        {
          field: 'name',
          flex: 3.5,
        },
        {
          field: 'suite_name',
          flex: 1.0,
          hide: true,
        },
        {
          field: 'variants',
          flex: 1.75,
          hide: props.collapse,
        },
        {
          field: 'delta_from_passing_average',
          flex: 0.75,
          hide: props.collapse,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'passing_average',
          flex: 0.75,
          hide: props.collapse,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'passing_standard_deviation',
          flex: 0.75,
          hide: props.collapse,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'current_pass_percentage',
          flex: 0.75,
          headerClassName: 'wrapHeader',
        },
        {
          field: 'net_improvement',
          flex: 0.5,
        },
        {
          field: 'previous_pass_percentage',
          flex: 0.75,
          headerClassName: 'wrapHeader',
        },
        {
          field: 'open_bugs',
          flex: 0.5,
          hide: props.briefTable,
        },
        {
          field: 'link',
          flex: 0.75,
          hide: props.briefTable,
        },
      ],
    },
    Flakes: {
      sortField: 'current_flake_percentage',
      sort: 'desc',
      rowColor: {
        field: 'current_flake_percentage',
        inverted: true,
      },
      fieldOrder: [
        {
          field: 'name',
          flex: 3.5,
        },
        {
          field: 'suite_name',
          flex: 1.0,
          hide: true,
        },
        {
          field: 'variants',
          flex: 1.75,
          hide: props.collapse,
        },
        {
          field: 'delta_from_flake_average',
          flex: 0.75,
          hide: props.collapse,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'flake_average',
          flex: 0.75,
          hide: props.collapse,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'flake_standard_deviation',
          flex: 0.75,
          hide: props.collapse,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'current_flake_percentage',
          flex: 0.75,
          headerClassName: 'wrapHeader',
        },
        {
          field: 'net_flake_improvement',
          flex: 0.5,
        },
        {
          field: 'previous_flake_percentage',
          flex: 0.75,
          headerClassName: 'wrapHeader',
        },
        {
          field: 'open_bugs',
          flex: 0.5,
          hide: props.briefTable,
        },
        {
          field: 'link',
          flex: 0.75,
          hide: props.briefTable,
        },
      ],
    },
    'Jira component': {
      sortField: 'current_working_percentage',
      sort: 'asc',
      rowColor: {
        field: 'current_working_percentage',
      },
      fieldOrder: [
        {
          field: 'jira_component',
          flex: 1,
        },
        {
          field: 'name',
          flex: 3.5,
        },
        {
          field: 'current_working_percentage',
          flex: 0.75,
          headerClassName: props.briefTable ? '' : 'wrapHeader',
        },
        {
          field: 'open_bugs',
          flex: 0.5,
          hide: props.briefTable,
        },
        {
          field: 'link',
          flex: props.collapse ? 1.25 : 2,
          hide: props.briefTable,
        },
      ],
    },
  }

  const currentPercentageRender = (params) => (
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
          <small>({params.row.current_runs.toLocaleString()} runs)</small>
        </Box>
      </Tooltip>
    </div>
  )

  const previousPercentageRender = (params) => (
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
          <small>({params.row.previous_runs.toLocaleString()} runs)</small>
        </Box>
      </Tooltip>
    </div>
  )

  const columns = {
    name: {
      field: 'name',
      headerName: 'Name',
      renderCell: (params) => {
        if (params.value === overallTestName) {
          return params.value
        }
        return (
          <div align="left" className="test-name">
            <Tooltip title={params.value}>
              <Link
                to={pathForExactTestAnalysisWithFilter(
                  props.release,
                  params.row.name,
                  filterModel
                )}
              >
                {params.value}
              </Link>
            </Tooltip>
          </div>
        )
      },
    },
    suite_name: {
      field: 'suite_name',
      hide: true,
      headerName: 'Suite',
      autocomplete: 'suite_name',
      type: 'string',
      renderCell: (params) => <div className="test-name">{params.value}</div>,
    },
    jira_component: {
      field: 'jira_component',
      headerName: 'Jira component',
    },
    variants: {
      field: 'variants',
      headerName: 'Variants',
      autocomplete: 'variants',
      type: 'array',
      renderCell: (params) => (
        <Tooltip
          sx={{ whiteSpace: 'pre' }}
          title={params.value ? params.value.join('\n') : ''}
        >
          <div className="variants-list">
            {params.value
              ? params.value
                  .slice(0, 8)
                  .filter((item) => !item.endsWith(':default'))
                  .join('\n')
              : ''}
          </div>
        </Tooltip>
      ),
    },
    delta_from_working_average: {
      field: 'delta_from_working_average',
      headerName: 'Delta (working)',
      type: 'number',
      renderCell: (params) => {
        return (
          <div className="percentage-cell">
            {params.value
              ? Number(params.value).toFixed(1).toLocaleString() + '%'
              : ''}
          </div>
        )
      },
    },
    working_average: {
      field: 'working_average',
      headerName: 'Average (working)',
      type: 'number',
      renderCell: (params) => (
        <div className="percentage-cell">
          {params.value
            ? Number(params.value).toFixed(1).toLocaleString() + '%'
            : ''}
        </div>
      ),
    },
    working_standard_deviation: {
      field: 'working_standard_deviation',
      headerName: 'Standard Deviation (working)',
      type: 'number',
    },
    delta_from_passing_average: {
      field: 'delta_from_passing_average',
      headerName: 'Delta (pass)',
      filterable: false,
      type: 'number',
      renderCell: (params) => {
        return (
          <div className="percentage-cell">
            {params.value
              ? Number(params.value).toFixed(1).toLocaleString() + '%'
              : ''}
          </div>
        )
      },
    },
    passing_average: {
      field: 'passing_average',
      headerName: 'Average (pass)',
      filterable: false,
      type: 'number',
      renderCell: (params) => (
        <div className="percentage-cell">
          {params.value
            ? Number(params.value).toFixed(1).toLocaleString() + '%'
            : ''}
        </div>
      ),
    },
    passing_standard_deviation: {
      field: 'passing_standard_deviation',
      headerName: 'Standard Deviation (pass)',
      filterable: false,
      type: 'number',
    },
    delta_from_flake_average: {
      field: 'delta_from_flake_average',
      headerName: 'Delta (flake)',
      filterable: false,
      type: 'number',
      renderCell: (params) => {
        return (
          <div className="percentage-cell">
            {params.value
              ? Number(params.value).toFixed(1).toLocaleString() + '%'
              : ''}
          </div>
        )
      },
    },
    flake_average: {
      field: 'flake_average',
      headerName: 'Average (flake)',
      filterable: false,
      type: 'number',
      renderCell: (params) => (
        <div className="percentage-cell">
          {params.value
            ? Number(params.value).toFixed(1).toLocaleString() + '%'
            : ''}
        </div>
      ),
    },
    flake_standard_deviation: {
      field: 'flake_standard_deviation',
      headerName: 'Standard Deviation (flake)',
      filterable: false,
      type: 'number',
    },
    current_working_percentage: {
      field: 'current_working_percentage',
      headerName: 'Current working percentage',
      type: 'number',
      renderCell: currentPercentageRender,
    },
    net_improvement: {
      field: 'net_improvement',
      headerName: 'Improvement (pass)',
      filterable: false,
      type: 'number',
      renderCell: (params) => {
        return <PassRateIcon tooltip={true} improvement={params.value} />
      },
    },
    net_working_improvement: {
      field: 'net_working_improvement',
      headerName: 'Improvement (working)',
      type: 'number',
      renderCell: (params) => {
        return <PassRateIcon tooltip={true} improvement={params.value} />
      },
    },
    net_flake_improvement: {
      field: 'net_flake_improvement',
      headerName: 'Improvement (flake)',
      filterable: false,
      type: 'number',
      renderCell: (params) => {
        return (
          <PassRateIcon
            tooltip={true}
            inverted={true}
            improvement={params.value}
          />
        )
      },
    },
    net_failure_improvement: {
      field: 'net_failure_improvement',
      headerName: 'Improvement (failure)',
      filterable: false,
      type: 'number',
      renderCell: (params) => {
        return <PassRateIcon tooltip={true} improvement={params.value} />
      },
    },
    previous_working_percentage: {
      field: 'previous_working_percentage',
      headerName: 'Previous working percentage',
      type: 'number',
      renderCell: previousPercentageRender,
    },
    open_bugs: {
      field: 'open_bugs',
      headerName: 'Bugs',
      type: 'number',
      renderCell: (params) => (
        <div>
          <Link
            to={
              '/tests/' +
              props.release +
              '/analysis?test=' +
              safeEncodeURIComponent(params.row.name)
            }
          >
            {params.value}
          </Link>
        </div>
      ),
    },
    link: {
      field: 'link',
      headerName: ' ',
      filterable: false,
      sortable: false,
      renderCell: (params) => {
        if (params.row.name === overallTestName) {
          return ''
        }

        let jobRunsFilter = {
          items: [...filterModel.items],
        }
        if (params.row.variants && params.row.variants.length > 0) {
          params.row.variants.forEach((f) => {
            if (!jobRunsFilter.items.find((i) => i.value === f)) {
              jobRunsFilter.items.push(filterFor('variants', 'contains', f))
            }
          })
        }

        return (
          <Grid container justifyContent="space-evenly">
            <Tooltip title="Search CI Logs">
              <IconButton
                target="_blank"
                href={
                  'https://search.dptools.openshift.org/?search=' +
                  safeEncodeURIComponent(escapeRegex(params.row.name)) +
                  '&maxAge=336h&context=1&type=bug%2Bjunit&name=&excludeName=&maxMatches=5&maxBytes=20971520&groupBy=job'
                }
                size="large"
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
                    jobRunsFilter
                  ),
                  'timestamp',
                  'desc'
                )}
                size="large"
              >
                <Badge
                  badgeContent={
                    params.row.current_failures + params.row.previous_failures
                  }
                  color="error"
                >
                  <ErrorIcon />
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
                    jobRunsFilter
                  ),
                  'timestamp',
                  'desc'
                )}
                size="large"
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
          </Grid>
        )
      },
    },
    // These are here just to allow filtering
    current_runs: {
      field: 'current_runs',
      headerName: 'Current runs',
      type: 'number',
    },
    current_failures: {
      field: 'current_failures',
      headerName: 'Current failures',
      type: 'number',
    },
    current_flakes: {
      field: 'current_flakes',
      headerName: 'Current failures',
      type: 'number',
    },
    current_pass_percentage: {
      field: 'current_pass_percentage',
      headerName: 'Current pass percentage',
      type: 'number',
      renderCell: currentPercentageRender,
    },
    current_flake_percentage: {
      field: 'current_flake_percentage',
      headerName: 'Current flake percentage',
      type: 'number',
      renderCell: currentPercentageRender,
    },
    current_failure_percentage: {
      field: 'current_failure_percentage',
      headerName: 'Current failure percentage',
      type: 'number',
    },
    previous_runs: {
      field: 'previous_runs',
      headerName: 'Previous runs',
      type: 'number',
    },
    previous_failures: {
      field: 'previous_failures',
      headerName: 'Previous failures',
      type: 'number',
    },
    previous_flakes: {
      field: 'previous_flakes',
      headerName: 'Previous failures',
      type: 'number',
    },
    previous_pass_percentage: {
      field: 'previous_pass_percentage',
      headerName: 'Previous pass percentage',
      type: 'number',
      renderCell: previousPercentageRender,
    },
    previous_flake_percentage: {
      field: 'previous_flake_percentage',
      headerName: 'Previous flake percentage',
      type: 'number',
      renderCell: previousPercentageRender,
    },
    previous_failure_percentage: {
      field: 'previous_failure_percentage',
      headerName: 'Previous failure percentage',
      type: 'number',
      renderCell: previousPercentageRender,
    },
    tags: {
      field: 'tags',
      headerName: 'Tags',
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

    if (props.overall !== undefined) {
      queryString += '&overall=' + safeEncodeURIComponent(props.overall)
    }

    if (period) {
      queryString += '&period=' + safeEncodeURIComponent(period)
    }

    queryString += '&sortField=' + safeEncodeURIComponent(sortField)
    queryString += '&sort=' + safeEncodeURIComponent(sort)

    queryString += '&collapse=' + safeEncodeURIComponent(props.collapse)

    let testAPI = '/api/tests?release='
    if (testTableDBSource === 'bigquery') {
      testAPI = '/api/tests/v2?release='
    }

    fetch(process.env.REACT_APP_API_URL + testAPI + props.release + queryString)
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

  const prevLocation = useRef()

  useEffect(() => {
    if (prevLocation.current !== location) {
      setRows([])
      setLoaded(false)
    }
    fetchData()
    prevLocation.current = location
  }, [
    period,
    filterModel,
    sort,
    sortField,
    props.collapse,
    props.briefTable,
    view,
  ])

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
    if (props.briefTable || props.simpleLoading) {
      return <p>Loading...</p>
    } else {
      return (
        <Backdrop className={gridClasses.backdrop} open={true}>
          <CircularProgress color="inherit" />
        </Backdrop>
      )
    }
  }

  const createTestNameQuery = () => {
    const selectedIDs = new Set(selectedTests)
    let tests = rows.filter((row) => selectedIDs.has(row.id))
    tests = tests.map((test) => 'test=' + safeEncodeURIComponent(test.name))
    return tests.join('&')
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
      <StyledDataGrid
        components={{ Toolbar: props.hideControls ? '' : GridToolbar }}
        rows={rows}
        columns={gridView.columns}
        autoHeight={true}
        getRowHeight={() => (props.collapse ? 100 : 'auto')}
        disableColumnFilter={props.briefTable}
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
        onSelectionModelChange={(rows) => setSelectedTests(rows)}
        getRowClassName={(params) => {
          let rowClass = []
          if (params.row.name === overallTestName) {
            rowClass.push(classes['overall'])
          }

          // Force the test tables to extremes; it's an up or down thing. Either a test
          // is good enough, or it's not.  Green is > 95%, otherwise it's red.  For flakes
          // (inverted) it's 5% or less is green.
          let adjustedPercentage =
            params.row[gridView.view.rowColor.field] >= 95 ? 100 : 0
          if (gridView.view.rowColor.inverted) {
            adjustedPercentage =
              params.row[gridView.view.rowColor.field] < 5 ? 100 : 0
          }

          let className = 'row-percent-' + adjustedPercentage

          rowClass.push(classes[className])

          return rowClass.join(' ')
        }}
        componentsProps={{
          toolbar: {
            bookmarks: bookmarks,
            views: gridView.views,
            view: view,
            selectView: selectView,
            columns: gridView.filterColumns,
            clearSearch: () => requestSearch(''),
            doSearch: requestSearch,
            period: period,
            selectPeriod: setPeriod,
            addFilters: addFilters,
            filterModel: filterModel,
            setFilterModel: setFilterModel,
            downloadDataFunc: () => {
              return rows
            },
            downloadFilePrefix: 'tests',
          },
        }}
      />
    </Fragment>
  )
}

TestTable.defaultProps = {
  collapse: true,
  limit: 0,
  hideControls: false,
  pageSize: 25,
  period: 'default',
  view: 'Passing',
  rowsPerPageOptions: [5, 10, 25, 50, 100],
  briefTable: false,
  simpleLoading: false,
  filterModel: {
    items: [],
  },
  sortField: 'current_pass_percentage',
  sort: 'asc',
}

TestTable.propTypes = {
  briefTable: PropTypes.bool,
  collapse: PropTypes.bool,
  overall: PropTypes.bool,
  hideControls: PropTypes.bool,
  simpleLoading: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  release: PropTypes.string.isRequired,
  classes: PropTypes.object,
  period: PropTypes.string,
  filterModel: PropTypes.object,
  sort: PropTypes.string,
  sortField: PropTypes.string,
  rowsPerPageOptions: PropTypes.array,
  view: PropTypes.string,
}

export default withStyles(generateClasses(TEST_THRESHOLDS))(TestTable)
