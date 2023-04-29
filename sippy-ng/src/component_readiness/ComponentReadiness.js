import './ComponentReadiness.css'
import { Alert, TabContext } from '@material-ui/lab'
import { ArrayParam, StringParam, useQueryParam } from 'use-query-params'
import { CircularProgress } from '@material-ui/core'
import { createTheme, makeStyles, useTheme } from '@material-ui/core/styles'
import {
  dateFormat,
  getAPIUrl,
  getUpdatedUrlParts,
  makeRFC3339Time,
} from './CompReadyUtils'
import { DateTimePicker, MuiPickersUtilsProvider } from '@material-ui/pickers'
import {
  Drawer,
  Grid,
  TableContainer,
  Tooltip,
  Typography,
} from '@material-ui/core'
import { format } from 'date-fns'
import { Fragment, useEffect } from 'react'
import { GridToolbarFilterDateUtils } from '../datagrid/GridToolbarFilterDateUtils'
import {
  Link,
  Route,
  Switch,
  useLocation,
  useRouteMatch,
} from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { useStyles } from '../App'
import Button from '@material-ui/core/Button'
import Capabilities from './Capabilities'
import CheckBoxList from './CheckboxList'
import ChevronLeftIcon from '@material-ui/icons/ChevronLeft'
import ChevronRightIcon from '@material-ui/icons/ChevronRight'
import clsx from 'clsx'
import CompReadyRow from './CompReadyRow'
import CompReadyTest from './CompReadyTest'
import IconButton from '@material-ui/core/IconButton'
import MenuIcon from '@material-ui/icons/Menu'
import React from 'react'
import ReleaseSelector from './ReleaseSelector'
import Table from '@material-ui/core/Table'
import TableBody from '@material-ui/core/TableBody'
import TableCell from '@material-ui/core/TableCell'
import TableHead from '@material-ui/core/TableHead'
import TableRow from '@material-ui/core/TableRow'

const days = 24 * 60 * 60 * 1000
const initialTime = new Date()
const initialStartTime = new Date(initialTime.getTime() - 30 * days)
const initialEndTime = new Date(initialTime.getTime())
const initialPrevStartTime = new Date(initialTime.getTime() - 30 * days)
const initialPrevEndTime = new Date(initialTime.getTime())

// Big query requests take a while so give the user the option to
// abort in case they inadvertently requested a huge dataset.
let abortController = new AbortController()
const cancelFetch = () => {
  abortController.abort()
}

// The API likes RFC3339 times and the date pickers don't.  So we use this
// function to convert for when we call the API.
// 4 digits, followed by a -, followed by 2 digits, and so on all wrapped in
// a group so we can refer to them as $1 and $2 respectively.
// We add a 'T' in the middle and a 'Z' on the end.

// Given the data pulled from the API server, calculate an array
// of columns using the first row.  Assumption: the number of columns
// is the same across all rows.
function getColumns(data) {
  const row0Columns = data.rows[0].columns

  let retVal = []
  row0Columns.forEach((column) => {
    let columnName = ''
    for (const key in column) {
      if (key !== 'status') {
        columnName = columnName + ' ' + column[key]
      }
    }
    retVal.push(columnName.trimStart())
  })
  return retVal
}

// This is used when the user clicks on one of the columns at the top of the table
function singleRowReport(columnName) {
  return '/component_readiness/' + safeEncodeURIComponent(columnName) + '/tests'
}

export default function ComponentReadiness(props) {
  console.log('ComponentReadiness start')
  const classes = useStyles()
  const theme = useTheme()

  // Extract the url and get the parameters from it
  const location = useLocation()
  const groupByParameters = new URLSearchParams(location.search)

  //const params = {}
  //groupByParameters.forEach((value, key) => {
  //  params[key] = value
  //})
  //console.log('params from url: ', JSON.stringify(params))

  const [drawerOpen, setDrawerOpen] = React.useState(true)
  const handleDrawerOpen = () => {
    setDrawerOpen(true)
  }

  const handleDrawerClose = () => {
    setDrawerOpen(false)
  }

  const groupByList = ['cloud', 'arch', 'network', 'upgrade', 'variant']
  let tmp = groupByParameters.get('group_by')
  let initGroupBy = ['cloud', 'arch', 'network']
  if (tmp !== null && tmp !== '') {
    initGroupBy = tmp.split(',')
  }
  const [groupByCheckedItems, setGroupByCheckedItems] = React.useState(
    // Extract the 'group_by=cloud,arch,network' from the url to be the initial
    // groupBy array of checked values.
    initGroupBy
  )
  //console.log('initGroupBy: ', initGroupBy)
  console.log('groupByCheckedItems: ', groupByCheckedItems)

  tmp = groupByParameters.get('component')
  const [component, setComponent] = React.useState(tmp)

  tmp = groupByParameters.get('environment')
  const [environment, setEnvironment] = React.useState(tmp)

  console.log('component, environment: ', component, ', ', environment)

  // TODO: Get these from single place.
  const excludeCloudsList = [
    'alibaba',
    'aws',
    'azure',
    'gcp',
    'ibmcloud',
    'libvirt',
    'metal-assisted',
    'metal-ipi',
    'openstack',
    'ovirt',
    'unknown',
    'vsphere',
    'vsphere-upi',
  ]
  tmp = groupByParameters.get('exclude_clouds')
  let initExcludeCloudsList = []
  if (tmp !== null && tmp !== '') {
    initExcludeCloudsList = tmp.split(',')
  }
  const [excludeCloudsCheckedItems, setExcludeCloudsCheckedItems] =
    React.useState(
      // Extract the 'exclude_clouds=aws' from the url to be the initial
      // Exclude Clouds array of checked values.
      initExcludeCloudsList
    )
  //console.log('initExcludeCloudsList: ', initExcludeCloudsList)
  console.log('excludeCloudsCheckedItems: ', excludeCloudsCheckedItems)

  // TODO: Get these from single place.
  const excludeArchesList = [
    'amd64',
    'arm64',
    'ppc64le',
    's390x',
    'heterogeneous',
  ]
  tmp = groupByParameters.get('exclude_arches')
  let initExcludeArchesList = []
  if (tmp !== null && tmp !== '') {
    initExcludeArchesList = tmp.split(',')
  }
  const [excludeArchesCheckedItems, setExcludeArchesCheckedItems] =
    React.useState(initExcludeArchesList)
  //console.log('initExcludeArchesList: ', initExcludeArchesList)
  console.log('excludeArchesCheckedItems: ', excludeArchesCheckedItems)

  const excludeNetworksList = ['ovn', 'sdn']
  tmp = groupByParameters.get('exclude_networks')
  console.log('url networks:', tmp)
  let initExcludeNetworksList = []
  if (tmp !== null && tmp !== '') {
    initExcludeNetworksList = tmp.split(',')
  }
  const [excludeNetworksCheckedItems, setExcludeNetworksCheckedItems] =
    React.useState(initExcludeNetworksList)
  //console.log('initExcludeNetworksList: ', initExcludeNetworksList)
  console.log('excludeNetworksCheckedItems: ', excludeNetworksCheckedItems)

  tmp = groupByParameters.get('baseRelease')
  let initBaseRelease = '4.14'
  if (tmp != null) {
    initBaseRelease = tmp
  }
  const [baseRelease, setBaseRelease] = React.useState(initBaseRelease)

  tmp = groupByParameters.get('sampleRelease')
  let initSampleRelease = '4.13'
  if (tmp != null) {
    initSampleRelease = tmp
  }
  const [sampleRelease, setSampleRelease] = React.useState(initSampleRelease)

  tmp = groupByParameters.get('baseStartTime')
  let initBaseStartTime = initialStartTime
  if (tmp != null) {
    initBaseStartTime = tmp
  }
  const [baseStartTime, setBaseStartTime] = React.useState(initBaseStartTime)

  tmp = groupByParameters.get('baseEndTime')
  let initBaseEndTime = initialEndTime
  if (tmp != null) {
    initBaseEndTime = tmp
  }
  const [baseEndTime, setBaseEndTime] = React.useState(initBaseEndTime)

  tmp = groupByParameters.get('sampleStartTime')
  let initSampleStartTime = initialPrevStartTime
  if (tmp != null) {
    initSampleStartTime = tmp
  }
  const [sampleStartTime, setSampleStartTime] =
    React.useState(initSampleStartTime)

  tmp = groupByParameters.get('sampleEndTime')
  let initSampleEndTime = initialPrevEndTime
  if (tmp != null) {
    initSampleEndTime = tmp
  }
  const [sampleEndTime, setSampleEndTime] = React.useState(initSampleEndTime)

  const excludeUpgradesList = [
    'no-upgrade',
    'none',
    'upgrade-micro',
    'upgrade-minor',
  ]

  tmp = groupByParameters.get('exclude_upgrades')
  let initExcludeUpgradesList = []
  if (tmp !== null && tmp !== '') {
    initExcludeUpgradesList = tmp.split(',')
  }
  const [excludeUpgradesCheckedItems, setExcludeUpgradesCheckedItems] =
    React.useState(initExcludeUpgradesList)
  //console.log('initExcludeUpgradesList: ', initExcludeUpgradesList)
  console.log('excludeUpgradesCheckedItems: ', excludeUpgradesCheckedItems)

  const excludeVariantsList = [
    'assisted',
    'compact',
    'fips',
    'hypershift',
    'microshift',
    'osd',
    'proxy',
    'rt',
    'serial',
    'single-node',
    'standard',
    'techpreview',
  ]
  tmp = groupByParameters.get('exclude_variants')
  let initExcludeVariantsList = []
  if (tmp !== null && tmp !== '') {
    initExcludeVariantsList = tmp.split(',')
  }
  const [excludeVariantsCheckedItems, setExcludeVariantsCheckedItems] =
    React.useState(initExcludeVariantsList)
  //console.log('initExcludeVariantsList: ', initExcludeVariantsList)
  console.log('excludeVariantsCheckedItems: ', excludeVariantsCheckedItems)

  const pageTitle = (
    <Typography variant="h4" style={{ margin: 20, textAlign: 'center' }}>
      Component Readiness for {baseRelease} vs. {sampleRelease}
    </Typography>
  )

  const { path, url } = useRouteMatch()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  // This is the table we use when the first page is initially rendered.
  const initialPageTable = {
    rows: [
      {
        component: 'None',
        columns: [
          {
            empty: 'None',
            status: 3, // Let's start with success
          },
        ],
      },
    ],
  }
  const noDataTable = {
    rows: [
      {
        component: 'No Data found',
        columns: [
          {
            empty: 'None',
            status: 3, // Let's start with success
          },
        ],
      },
    ],
  }
  useEffect(() => {
    setData(initialPageTable)
    setIsLoaded(true)
  }, [])

  document.title = `Sippy > Component Readiness`
  if (fetchError !== '') {
    return (
      <Alert severity="error">
        <h2>Failed to load component readiness data</h2>
        <h3>{fetchError}.</h3>
        <h3>Check, and possibly fix api server, then reload page to retry</h3>
      </Alert>
    )
  }

  if (!isLoaded) {
    return (
      <Fragment>
        <p>
          Loading component readiness data ... If you asked for a huge dataset,
          it may take minutes.
        </p>
        <CircularProgress />
        <div>
          <Button
            size="medium"
            variant="contained"
            color="secondary"
            onClick={cancelFetch}
          >
            Cancel
          </Button>
        </div>
      </Fragment>
    )
  }

  console.log('data: ', data)
  if (Object.keys(data).length === 0 || data.rows.length === 0) {
    return <p>No data.</p>
  }

  if (data.tests && Object.keys(data.tests).length === 0) {
    return (
      <Fragment>
        {pageTitle}
        <p>No Results.</p>
      </Fragment>
    )
  }

  if (data.length === 0) {
    return (
      <Typography variant="h6" style={{ marginTop: 50 }}>
        No per-variant data found.
      </Typography>
    )
  }

  // Show the current state of the filter variables and the url.
  // Create API call string and return it.
  const showValuesForReport = () => {
    console.log('--------------- handleGenerateReport ------------------')
    console.log('baseRelease', baseRelease)
    console.log('baseStartTime', baseStartTime)
    console.log('baseEndTime', baseEndTime)
    console.log('sampleRelease', sampleRelease)
    console.log('sampleStartTime', sampleStartTime)
    console.log('sampleEndTime', sampleEndTime)
    console.log('groupBy: ', groupByCheckedItems)
    console.log('excludeClouds: ', excludeCloudsCheckedItems)
    console.log('excludeArches', excludeArchesCheckedItems)
    console.log('excludeNetworks', excludeNetworksCheckedItems)
    console.log('excludeUpgrades', excludeUpgradesCheckedItems)
    console.log('excludeVariants', excludeVariantsCheckedItems)
    console.log('component', component)
    console.log('enviornment', environment)

    // process.env.REACT_APP_API_URL +
    const apiCallStr =
      getAPIUrl() +
      getUpdatedUrlParts(
        baseRelease,
        baseStartTime,
        baseEndTime,
        sampleRelease,
        sampleStartTime,
        sampleEndTime,
        groupByCheckedItems,
        excludeCloudsCheckedItems,
        excludeArchesCheckedItems,
        excludeNetworksCheckedItems,
        excludeUpgradesCheckedItems,
        excludeVariantsCheckedItems,
        component,
        environment
      )
    //const params = new URLSearchParams(apiCallStr.split('?')[1])

    //console.log('*** API Call: ')
    //params.forEach((value, key) => {
    //  console.log(`${key}: ${value}`)
    //})
    const formattedApiCallStr = makeRFC3339Time(apiCallStr)
    console.log('formatted api call: ')
    formattedApiCallStr
      .split('?')[1]
      .split('&')
      .map((item) => {
        console.log('   ', item)
      })
    return formattedApiCallStr
  }

  // This runs when someone pushes the "Generate Report" button.
  // We form an api string and then call the api.
  const handleGenerateReport = () => {
    const formattedApiCallStr = showValuesForReport()

    setIsLoaded(false)
    const fromFile = true
    if (fromFile) {
      const json = require('./api_page1.json')
      setData(json)
      console.log('json:', json)
      setIsLoaded(true)
    } else {
      fetch(formattedApiCallStr, { signal: abortController.signal })
        .then((response) => {
          if (response.status !== 200) {
            throw new Error('API server returned ' + response.status)
          }
          return response.json()
        })
        .then((json) => {
          console.log(json)
          if (Object.keys(json).length === 0 || json.rows.length === 0) {
            // The api call returned 200 OK but the data was empty
            setData(noDataTable)
          } else {
            setData(json)
          }
        })
        .catch((error) => {
          if (error.name === 'AbortError') {
            console.log('Request was cancelled')

            // Once this fired, we need a new one for the next button click.
            abortController = new AbortController()
          } else {
            setFetchError(`API call failed: ${formattedApiCallStr}` + error)
          }
        })
        .finally(() => {
          // Mark the attempt as finished whether successful or not.
          setIsLoaded(true)
        })
    }
  }

  const columnNames = getColumns(data)
  console.log('ComponentReadiness end: ', sampleRelease)

  const myPath = '/component_readiness'

  console.log('myPath:', myPath)
  console.log('path:', path)
  return (
    <Fragment>
      <Route
        path={myPath}
        render={({ location }) => (
          <TabContext value={path}>
            {pageTitle}
            <Grid
              container
              justifyContent="center"
              size="xl"
              className="cr-view"
            ></Grid>
            {/* eslint-disable react/prop-types */}
            <Switch>
              <Route
                path="/component_readiness/:component/capabilities"
                render={(props) => (
                  <Capabilities
                    key="capabilities"
                    component={props.match.params.component}
                  ></Capabilities>
                )}
              />
              <Route
                path="/component_readiness/tests"
                render={(props) => {
                  return (
                    <CompReadyTest
                      filterVals={getUpdatedUrlParts(
                        baseRelease,
                        baseStartTime,
                        baseEndTime,
                        sampleRelease,
                        sampleStartTime,
                        sampleEndTime,
                        groupByCheckedItems,
                        excludeCloudsCheckedItems,
                        excludeArchesCheckedItems,
                        excludeNetworksCheckedItems,
                        excludeUpgradesCheckedItems,
                        excludeVariantsCheckedItems,
                        component,
                        environment
                      )}
                    ></CompReadyTest>
                  )
                }}
              />
              <Route path={path}>
                <div className="cr-view">
                  <IconButton
                    color="inherit"
                    aria-label="open drawer"
                    onClick={handleDrawerOpen}
                    edge="start"
                    className={clsx(
                      classes.menuButton,
                      drawerOpen && classes.hide
                    )}
                  >
                    <MenuIcon />
                  </IconButton>
                  <Drawer
                    className={classes.drawer}
                    variant="persistent"
                    anchor="left"
                    open={drawerOpen}
                    classes={{
                      paper: classes.drawerPaper,
                    }}
                  >
                    <div className={classes.drawerHeader}>
                      <IconButton onClick={handleDrawerClose}>
                        {theme.direction === 'ltr' ? (
                          <ChevronLeftIcon />
                        ) : (
                          <ChevronRightIcon />
                        )}
                      </IconButton>
                    </div>
                    <div className="cr-report-button">
                      <Button
                        size="large"
                        variant="contained"
                        color="primary"
                        component={Link}
                        to={
                          '/component_readiness/' +
                          getUpdatedUrlParts(
                            baseRelease,
                            baseStartTime,
                            baseEndTime,
                            sampleRelease,
                            sampleStartTime,
                            sampleEndTime,
                            groupByCheckedItems,
                            excludeCloudsCheckedItems,
                            excludeArchesCheckedItems,
                            excludeNetworksCheckedItems,
                            excludeUpgradesCheckedItems,
                            excludeVariantsCheckedItems,
                            component,
                            environment
                          )
                        }
                        onClick={handleGenerateReport}
                      >
                        Generate Report
                      </Button>
                    </div>
                    <div className="cr-report-button">
                      <Button
                        size="large"
                        variant="contained"
                        color="primary"
                        component={Link}
                        to={
                          '/component_readiness/' +
                          getUpdatedUrlParts(
                            baseRelease,
                            baseStartTime,
                            baseEndTime,
                            sampleRelease,
                            sampleStartTime,
                            sampleEndTime,
                            groupByCheckedItems,
                            excludeCloudsCheckedItems,
                            excludeArchesCheckedItems,
                            excludeNetworksCheckedItems,
                            excludeUpgradesCheckedItems,
                            excludeVariantsCheckedItems,
                            component,
                            environment
                          )
                        }
                        onClick={handleGenerateReport}
                      >
                        Debug
                      </Button>
                    </div>
                    <div className="cr-release-historical">
                      <ReleaseSelector
                        version={baseRelease}
                        label="Historical"
                        onChange={setBaseRelease}
                      ></ReleaseSelector>
                      <MuiPickersUtilsProvider
                        utils={GridToolbarFilterDateUtils}
                      >
                        <DateTimePicker
                          showTodayButton
                          disableFuture
                          label="From"
                          format={dateFormat}
                          ampm={false}
                          value={baseStartTime}
                          onChange={(e) => {
                            const formattedTime = format(e, dateFormat)
                            setBaseStartTime(formattedTime)
                          }}
                        />
                      </MuiPickersUtilsProvider>
                      <MuiPickersUtilsProvider
                        utils={GridToolbarFilterDateUtils}
                      >
                        <DateTimePicker
                          showTodayButton
                          disableFuture
                          label="To"
                          format={dateFormat}
                          ampm={false}
                          value={baseEndTime}
                          onChange={(e) => {
                            const formattedTime = format(e, dateFormat)
                            setBaseEndTime(formattedTime)
                          }}
                        />
                      </MuiPickersUtilsProvider>
                    </div>
                    <div className="cr-release-sample">
                      <ReleaseSelector
                        label="Sample Release"
                        version={sampleRelease}
                        onChange={setSampleRelease}
                      ></ReleaseSelector>
                      <MuiPickersUtilsProvider
                        utils={GridToolbarFilterDateUtils}
                      >
                        <DateTimePicker
                          showTodayButton
                          disableFuture
                          label="From"
                          format={dateFormat}
                          ampm={false}
                          value={sampleStartTime}
                          onChange={(e) => {
                            const formattedTime = format(e, dateFormat)
                            setSampleStartTime(formattedTime)
                          }}
                        />
                      </MuiPickersUtilsProvider>
                      <MuiPickersUtilsProvider
                        utils={GridToolbarFilterDateUtils}
                      >
                        <DateTimePicker
                          showTodayButton
                          disableFuture
                          label="To"
                          format={dateFormat}
                          ampm={false}
                          value={sampleEndTime}
                          onChange={(e) => {
                            const formattedTime = format(e, dateFormat)
                            setSampleEndTime(formattedTime)
                          }}
                        />
                      </MuiPickersUtilsProvider>
                    </div>
                    <div>
                      <CheckBoxList
                        headerName="Group By"
                        displayList={groupByList}
                        checkedItems={groupByCheckedItems}
                        setCheckedItems={setGroupByCheckedItems}
                      ></CheckBoxList>
                      <CheckBoxList
                        headerName="Exclude Arches"
                        displayList={excludeArchesList}
                        checkedItems={excludeArchesCheckedItems}
                        setCheckedItems={setExcludeArchesCheckedItems}
                      ></CheckBoxList>
                      <CheckBoxList
                        headerName="Exclude Networks"
                        displayList={excludeNetworksList}
                        checkedItems={excludeNetworksCheckedItems}
                        setCheckedItems={setExcludeNetworksCheckedItems}
                      ></CheckBoxList>
                      <CheckBoxList
                        headerName="Exclude Clouds"
                        displayList={excludeCloudsList}
                        checkedItems={excludeCloudsCheckedItems}
                        setCheckedItems={setExcludeCloudsCheckedItems}
                      ></CheckBoxList>
                      <CheckBoxList
                        headerName="Exclude Upgrades"
                        displayList={excludeUpgradesList}
                        checkedItems={excludeUpgradesCheckedItems}
                        setCheckedItems={setExcludeUpgradesCheckedItems}
                      ></CheckBoxList>
                      <CheckBoxList
                        headerName="Exclude Variants"
                        displayList={excludeVariantsList}
                        checkedItems={excludeVariantsCheckedItems}
                        setCheckedItems={setExcludeVariantsCheckedItems}
                      ></CheckBoxList>
                    </div>
                  </Drawer>
                  <TableContainer component="div" className="cr-wrapper">
                    <Table className="cr-comp-read-table">
                      <TableHead>
                        <TableRow>
                          {
                            <TableCell className={'cr-col-result-full'}>
                              <Typography className="cr-cell-name">
                                Name
                              </Typography>
                            </TableCell>
                          }
                          {columnNames.map((column, idx) => {
                            if (column !== 'Name') {
                              return (
                                <TableCell
                                  className={'cr-col-result'}
                                  key={'column' + '-' + idx}
                                >
                                  <Tooltip
                                    title={'Single row report for ' + column}
                                  >
                                    <Typography className="cr-cell-name">
                                      <Link to={singleRowReport(column)}>
                                        {column}
                                      </Link>
                                    </Typography>
                                  </Tooltip>
                                </TableCell>
                              )
                            }
                          })}
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {Object.keys(data.rows).map((componentIndex) => (
                          <CompReadyRow
                            key={componentIndex}
                            componentName={data.rows[componentIndex].component}
                            results={data.rows[componentIndex].columns}
                            columnNames={columnNames}
                            filterVals={getUpdatedUrlParts(
                              baseRelease,
                              baseStartTime,
                              baseEndTime,
                              sampleRelease,
                              sampleStartTime,
                              sampleEndTime,
                              groupByCheckedItems,
                              excludeCloudsCheckedItems,
                              excludeArchesCheckedItems,
                              excludeNetworksCheckedItems,
                              excludeUpgradesCheckedItems,
                              excludeVariantsCheckedItems,
                              data.rows[componentIndex].component,
                              columnNames
                            )}
                          />
                        ))}
                      </TableBody>
                    </Table>
                  </TableContainer>
                </div>
              </Route>
            </Switch>
          </TabContext>
        )}
      />
    </Fragment>
  )
}

// Create a set of initial values when accessing ComponentReadiness component
// from the SideBar.  This provides an initial URL where the ComponentReadiness
// will pull these values from the URL.
// export function getInitialUrlParts() {
//   const releaseAndDates = {
//     sampleRelease: '4.13',
//     baseRelease: '4.14',
//     baseStartTime: format(initialStartTime, dateFormat),
//     baseEndTime: format(initialEndTime, dateFormat),
//     sampleStartTime: format(initialPrevStartTime, dateFormat),
//     sampleEndTime: format(initialPrevEndTime, dateFormat),
//   }

//   let retVal = '?'

//   retVal = retVal + 'group_by=cloud,network'
//   retVal = retVal + '&exclude_clouds='
//   retVal = retVal + '&exclude_arches='
//   retVal = retVal + '&exclude_networks='
//   retVal = retVal + '&exclude_upgrades='
//   retVal = retVal + '&exclude_variants='

//   // Turn my map into a list of key/value pairs so we can use map() on it.
//   const fieldList = Object.entries(releaseAndDates)
//   fieldList.map(([key, value]) => {
//     retVal = retVal + '&' + key + '=' + value
//   })

//   console.log('*** INITIALIZED ***')
//   initialized = true
//   return retVal
// }

//
