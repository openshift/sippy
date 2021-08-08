
import { Button, Container, Tooltip } from '@material-ui/core'
import IconButton from '@material-ui/core/IconButton'
import { createTheme } from '@material-ui/core/styles'
import TextField from '@material-ui/core/TextField'
import {
  DataGrid,
  GridToolbarDensitySelector,
  GridToolbarFilterButton
} from '@material-ui/data-grid'
import { BugReport, Search } from '@material-ui/icons'
import ClearIcon from '@material-ui/icons/Clear'
import SearchIcon from '@material-ui/icons/Search'
import Alert from '@material-ui/lab/Alert'
import { makeStyles, withStyles } from '@material-ui/styles'
import clsx from 'clsx'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { ArrayParam, NumberParam, StringParam, useQueryParam } from 'use-query-params'
import BugzillaDialog from '../bugzilla/BugzillaDialog'
import GridToolbarPeriodSelector from '../datagrid/GridToolbarPeriodSelector'
import PassRateIcon from '../components/PassRateIcon'
import GridToolbarQueriesMenu from '../datagrid/GridToolbarQueriesMenu'
import { bugColor, weightedBugComparator } from '../bugzilla/BugzillaUtils'
import { TEST_THRESHOLDS } from '../constants'

function escapeRegExp (value) {
  return value.replace(/[-[\]{}()*+?.,\\^$|#\s]/g, '\\$&')
};

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    root: {
      padding: theme.spacing(0.5, 0.5, 0),
      justifyContent: 'space-between',
      display: 'flex',
      alignItems: 'flex-start',
      flexWrap: 'wrap'
    },
    textField: {
      [theme.breakpoints.down('xs')]: {
        width: '100%'
      },
      margin: theme.spacing(1, 0.5, 1.5),
      '& .MuiSvgIcon-root': {
        marginRight: theme.spacing(0.5)
      },
      '& .MuiInput-underline:before': {
        borderBottom: `1px solid ${theme.palette.divider}`
      }
    }
  }),
  { defaultTheme }
)

function TestSearchToolbar (props) {
  const classes = useStyles()

  return (
    <div className={classes.root}>
      <div>
        <GridToolbarFilterButton />
        <GridToolbarDensitySelector />
        <GridToolbarPeriodSelector
            selectPeriod={props.selectPeriod}
            period={props.period}
        />

        <GridToolbarQueriesMenu
            initialFilters={props.initialFilters}
            setFilters={props.requestFilter}
            allowedFilters={[
              {
                title: 'Has a linked bug',
                filter: 'hasBug',
                conflictsWith: 'noBug'
              },
              {
                title: 'No bug',
                filter: 'noBug',
                conflictsWith: 'hasBug'
              },
              {
                title: 'Install-related tests',
                filter: 'install',
                conflictsWith: 'upgrade'
              },
              {
                title: 'Upgrade-related tests',
                filter: 'upgrade',
                conflictsWith: 'install'
              },
              {
                title: 'More than 10 runs',
                filter: 'runs'
              },
              {
                title: 'Curated by TRT',
                filter: 'trt'
              }
            ]}

        />
      </div>
      <TextField
        variant="standard"
        value={props.value}
        onChange={props.onChange}
        placeholder="Search…"
        className={classes.textField}
        InputProps={{
          startAdornment: <SearchIcon fontSize="small" />,
          endAdornment: (
            <IconButton
              title="Clear"
              aria-label="Clear"
              size="small"
              style={{ visibility: props.value ? 'visible' : 'hidden' }}
              onClick={props.clearSearch}
            >
              <ClearIcon fontSize="small" />
            </IconButton>
          )
        }}
      />
    </div>
  )
}

TestSearchToolbar.propTypes = {
  selectPeriod: PropTypes.func.isRequired,
  period: PropTypes.string,
  clearSearch: PropTypes.func.isRequired,
  onChange: PropTypes.func.isRequired,
  value: PropTypes.string,
  initialFilters: PropTypes.array,
  requestFilter: PropTypes.func
}

const styles = {
  good: {
    backgroundColor: defaultTheme.palette.success.light,
    color: defaultTheme.palette.success.contrastText
  },
  ok: {
    backgroundColor: defaultTheme.palette.warning.light,
    color: defaultTheme.palette.warning.contrastText
  },
  failing: {
    backgroundColor: defaultTheme.palette.error.light,
    color: defaultTheme.palette.warning.contrastText
  }
}

function TestTable (props) {
  const { classes } = props

  const columns = [
    {
      field: 'name',
      headerName: 'Name',
      flex: 3,
      renderCell: (params) => (
        <div style={{ display: 'block', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          <Tooltip title={params.value}>
            <Link to={'/tests/' + props.release + '/details?test=' + params.row.name}>{params.value}</Link>
          </Tooltip>
        </div>
      )
    },
    {
      field: 'current_pass_percentage',
      headerName: 'Current Period',
      type: 'number',
      flex: 1,
      renderCell: (params) => (
        <Tooltip title={params.row.current_runs + ' runs'}>
          <p>
            {Number(params.value).toFixed(2).toLocaleString()}%
          </p>
        </Tooltip>
      )
    },
    {
      field: 'net_improvement',
      headerName: 'Improvement',
      type: 'number',
      flex: 0.5,
      renderCell: (params) => {
        return <PassRateIcon tooltip={true} improvement={params.value} />
      }
    },
    {
      field: 'previous_pass_percentage',
      headerName: 'Previous Period',
      flex: 1,
      type: 'number',
      renderCell: (params) => (
        <Tooltip title={params.row.previous_runs + ' runs'}>
          <p>
            {Number(params.value).toFixed(2).toLocaleString()}%
          </p>
        </Tooltip>
      )
    },
    {
      field: 'link',
      headerName: ' ',
      flex: 0.40,
      filterable: false,
      renderCell: (params) => {
        return (
          <Button target="_blank" startIcon={<Search />} href={'https://search.ci.openshift.org/?search=' + encodeURIComponent(params.row.name) + '&maxAge=336h&context=1&type=bug%2Bjunit&name=&excludeName=&maxMatches=5&maxBytes=20971520&groupBy=job'} />
        )
      },
      hide: props.briefTable
    },
    {
      field: 'bugs',
      headerName: ' ',
      flex: 0.40,
      filterable: false,
      renderCell: (params) => {
        return (
          <Tooltip title={params.value.length + ' linked bugs,' + params.row.associated_bugs.length + ' associated bugs'}>
            <Button style={{ color: bugColor(params.row) }} startIcon={<BugReport />} onClick={() => openBugzillaDialog(params.row)} />
          </Tooltip>
        )
      },
      // Weight linked bugs more than associated bugs, but associated bugs are ranked more than not having one at all.
      sortComparator: (v1, v2, param1, param2) => weightedBugComparator(
        param1.api.getCellValue(param1.id, 'bugs'),
        param1.api.getCellValue(param1.id, 'associated_bugs'),
        param2.api.getCellValue(param2.id, 'bugs'),
        param2.api.getCellValue(param2.id, 'associated_bugs')),
      hide: props.briefTable
    },

    // These are here just to allow filtering
    {
      field: 'current_runs',
      headerName: 'Current runs',
      hide: true,
      type: 'number'
    },
    {
      field: 'previous_runs',
      headerName: 'Previous runs',
      hide: true,
      type: 'number'
    }
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
  const [tests, setTests] = React.useState([])
  const [rows, setRows] = React.useState([])
  const [selectedTests, setSelectedTests] = React.useState([])

  const [runs = props.runs] = useQueryParam('runs', NumberParam)
  const [filterBy = props.filterBy, setFilterBy] = useQueryParam('filterBy', ArrayParam)
  const [sortBy = props.sortBy] = useQueryParam('sortBy', StringParam)
  const [limit = props.limit] = useQueryParam('limit, StringParam')
  const [period = props.period, setPeriod] = useQueryParam('period', StringParam)

  const [searchText, setSearchText] = useQueryParam('searchText', StringParam)
  const [testNames = []] = useQueryParam('test', ArrayParam)

  const fetchData = () => {
    let queryString = ''
    if (filterBy) {
      filterBy.forEach((filter) => {
        if (filter === 'runs' && !runs) {
          queryString += '&runs=10'
        }
        queryString += '&filterBy=' + encodeURIComponent(filter)
      })
    }

    testNames.forEach((test) => {
      queryString += '&test=' + encodeURIComponent(test)
    })

    if (runs) {
      queryString += '&runs=' + encodeURIComponent(runs)
    }

    if (sortBy && sortBy !== '') {
      queryString += '&sortBy=' + encodeURIComponent(sortBy)
    }

    if (limit) {
      queryString += '&limit=' + encodeURIComponent(limit)
    }

    if (period) {
      queryString += '&period=' + encodeURIComponent(period)
    }

    fetch(process.env.REACT_APP_API_URL + '/api/tests?release=' + props.release + queryString)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then(json => {
        setTests(json)
        setRows(json)
        setLoaded(true)
      }).catch(error => {
        setFetchError('Could not retrieve tests ' + props.release + ', ' + error)
      })
  }

  useEffect(() => {
    fetchData()
  }, [filterBy, period])

  const requestSearch = (searchValue) => {
    setSearchText(searchValue)
    const searchRegex = new RegExp(escapeRegExp(searchValue), 'i')
    const filteredRows = tests.filter((row) => {
      return Object.keys(row).some((field) => {
        return searchRegex.test(row[field].toString())
      })
    })
    setRows(filteredRows)
  }

  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (isLoaded === false) {
    return <p>Loading...</p>
  }

  const createTestNameQuery = () => {
    const selectedIDs = new Set(selectedTests)
    let tests = rows.filter((row) =>
      selectedIDs.has(row.id)
    )
    tests = tests.map((test) =>
      'test=' + encodeURIComponent(test.name)
    )
    return tests.join('&')
  }

  const detailsButton = (
    <Button component={Link} to={'/tests/' + props.release + '/details?' + createTestNameQuery()} variant="contained" color="primary" style={{ margin: 10 }}>Get Details</Button>
  )

  return (
    <Container size="xl">
      <DataGrid
        components={{ Toolbar: props.hideControls ? '' : TestSearchToolbar }}
        rows={rows}
        columns={columns}
        autoHeight={true}
        disableColumnFilter={props.briefTable}
        disableColumnMenu={true}
        pageSize={props.pageSize}
        rowsPerPageOptions={[5, 10, 25, 50]}
        checkboxSelection={!props.hideControls}
        onSelectionModelChange={(rows) =>
          setSelectedTests(rows)
        }
        getRowClassName={(params =>
          clsx({
            [classes.good]: (params.row.current_pass_percentage >= TEST_THRESHOLDS.success),
            [classes.ok]: (params.row.current_pass_percentage >= TEST_THRESHOLDS.warning && params.row.current_pass_percentage < TEST_THRESHOLDS.success),
            [classes.failing]: (params.row.current_pass_percentage >= TEST_THRESHOLDS.error && params.row.current_pass_percentage < TEST_THRESHOLDS.warning)
          })
        )}
        componentsProps={{
          toolbar: {
            value: searchText,
            onChange: (event) => requestSearch(event.target.value),
            requestFilter: (f) => setFilterBy(f),
            initialFilters: filterBy,
            clearSearch: () => requestSearch(''),
            period: period,
            selectPeriod: setPeriod
          }
        }}
      />

      {props.hideControls ? '' : detailsButton}

      <BugzillaDialog item={testDetails} isOpen={isBugzillaDialogOpen} close={closeBugzillaDialog} />
    </Container>
  )
}

TestTable.defaultProps = {
  hideControls: false,
  pageSize: 25,
  briefTable: false,
  filterBy: []
}

TestTable.propTypes = {
  briefTable: PropTypes.bool,
  filterBy: PropTypes.array,
  hideControls: PropTypes.bool,
  limit: PropTypes.number,
  pageSize: PropTypes.number,
  release: PropTypes.string.isRequired,
  runs: PropTypes.number,
  sortBy: PropTypes.string,
  classes: PropTypes.object,
  period: PropTypes.string
}
export default withStyles(styles)(TestTable)
