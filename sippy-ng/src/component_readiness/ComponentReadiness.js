import './ComponentReadiness.css'
import { Alert, TabContext } from '@material-ui/lab'
import { CircularProgress } from '@material-ui/core'
import {
  Drawer,
  Grid,
  TableContainer,
  Tooltip,
  Typography,
} from '@material-ui/core'
import {
  formatLongDate,
  getAPIUrl,
  getColumns,
  getUpdatedUrlParts,
  makeRFC3339Time,
  singleRowReport,
} from './CompReadyUtils'
import { Fragment, useEffect } from 'react'
import {
  Link,
  Route,
  Switch,
  useLocation,
  useRouteMatch,
} from 'react-router-dom'
import { useStyles } from '../App'
import { useTheme } from '@material-ui/core/styles'
import Button from '@material-ui/core/Button'
import Capabilities from './Capabilities'
import ChevronLeftIcon from '@material-ui/icons/ChevronLeft'
import ChevronRightIcon from '@material-ui/icons/ChevronRight'
import clsx from 'clsx'
import CompReadyCapabilities from './CompReadyCapabilities'
import CompReadyMainInputs from './CompReadyMainInputs'
import CompReadyRow from './CompReadyRow'
import IconButton from '@material-ui/core/IconButton'
import MenuIcon from '@material-ui/icons/Menu'
import React from 'react'
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

export default function ComponentReadiness(props) {
  console.log('ComponentReadiness start')
  const classes = useStyles()
  const theme = useTheme()

  // Extract the url and get the parameters from it
  const location = useLocation()
  const groupByParameters = new URLSearchParams(location.search)

  const [drawerOpen, setDrawerOpen] = React.useState(true)
  const handleDrawerOpen = () => {
    setDrawerOpen(true)
  }

  const handleDrawerClose = () => {
    setDrawerOpen(false)
  }

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
  //console.log('groupByCheckedItems: ', groupByCheckedItems)

  tmp = groupByParameters.get('component')
  const [component, setComponent] = React.useState(tmp)

  tmp = groupByParameters.get('environment')
  const [environment, setEnvironment] = React.useState(tmp)

  //console.log('component, environment: ', component, ', ', environment)

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
  //console.log('excludeCloudsCheckedItems: ', excludeCloudsCheckedItems)

  tmp = groupByParameters.get('exclude_arches')
  let initExcludeArchesList = []
  if (tmp !== null && tmp !== '') {
    initExcludeArchesList = tmp.split(',')
  }
  const [excludeArchesCheckedItems, setExcludeArchesCheckedItems] =
    React.useState(initExcludeArchesList)
  //console.log('initExcludeArchesList: ', initExcludeArchesList)
  //console.log('excludeArchesCheckedItems: ', excludeArchesCheckedItems)

  tmp = groupByParameters.get('exclude_networks')
  //console.log('url excludedNetworks:', tmp)
  let initExcludeNetworksList = []
  if (tmp !== null && tmp !== '') {
    initExcludeNetworksList = tmp.split(',')
  }
  const [excludeNetworksCheckedItems, setExcludeNetworksCheckedItems] =
    React.useState(initExcludeNetworksList)
  //console.log('initExcludeNetworksList: ', initExcludeNetworksList)
  //console.log('excludeNetworksCheckedItems: ', excludeNetworksCheckedItems)

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

  tmp = groupByParameters.get('exclude_upgrades')
  let initExcludeUpgradesList = []
  if (tmp !== null && tmp !== '') {
    initExcludeUpgradesList = tmp.split(',')
  }
  const [excludeUpgradesCheckedItems, setExcludeUpgradesCheckedItems] =
    React.useState(initExcludeUpgradesList)
  //console.log('initExcludeUpgradesList: ', initExcludeUpgradesList)
  //console.log('excludeUpgradesCheckedItems: ', excludeUpgradesCheckedItems)

  tmp = groupByParameters.get('exclude_variants')
  let initExcludeVariantsList = []
  if (tmp !== null && tmp !== '') {
    initExcludeVariantsList = tmp.split(',')
  }
  const [excludeVariantsCheckedItems, setExcludeVariantsCheckedItems] =
    React.useState(initExcludeVariantsList)
  //console.log('initExcludeVariantsList: ', initExcludeVariantsList)
  //console.log('excludeVariantsCheckedItems: ', excludeVariantsCheckedItems)

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

  // Show the current state of the filter variables and the url.
  // Create API call string and return it.
  const showValuesForReport = () => {
    console.log('--------------- showValuesForReport ------------------')
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
    console.log('environment', environment)

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
    const formattedApiCallStr = makeRFC3339Time(apiCallStr)
    console.log('formatted api call: ')
    formattedApiCallStr
      .split('?')[1]
      .split('&')
      .map((item) => {
        console.log('   ', item)
      })
    console.log('apiurl: ', formattedApiCallStr)
    return formattedApiCallStr
  }

  if (!isLoaded) {
    const formattedApiCallStr = showValuesForReport()
    return (
      <Fragment>
        <p>
          Loading component readiness data ... If you asked for a huge dataset,
          it may take minutes.
        </p>
        <br />
        Here is the API call in case you are interested:
        <br />
        <h3>
          <a href={formattedApiCallStr}>{formattedApiCallStr}</a>
        </h3>
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

  const handleClick = () => {
    window.location.href = '/component_readiness'
  }
  const columnNames = getColumns(data)
  if (columnNames[0] === 'Cancelled') {
    return (
      <Fragment>
        <p>Operation cancelled</p>
        <button onClick={handleClick}>Start Over</button>
      </Fragment>
    )
  }

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

  // This runs when someone pushes the "Generate Report" button.
  // We form an api string and then call the api.
  const handleGenerateReport = () => {
    const formattedApiCallStr = showValuesForReport()

    setIsLoaded(false)
    const fromFile = true
    if (fromFile) {
      //const json = require('./api_page1.json')
      const json = require('./api_page1-big.json')
      setData(json)
      console.log('json:', json)
      setIsLoaded(true)
    } else {
      console.log('about to fetch: ', formattedApiCallStr)
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
            setData({})

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

  console.log('ComponentReadiness end')

  return (
    <Fragment>
      <Route
        path={path}
        render={({ location }) => (
          <TabContext value={path}>
            <Grid
              container
              justifyContent="center"
              size="xl"
              className="cr-view"
            ></Grid>
            {/* eslint-disable react/prop-types */}
            <Switch>
              <Route
                path="/component_readiness/:component/tests"
                render={(props) => (
                  <Capabilities
                    key="capabilities"
                    component={props.match.params.component}
                  ></Capabilities>
                )}
              />
              <Route
                path="/component_readiness/capabilities"
                render={(props) => {
                  return (
                    <CompReadyCapabilities
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
                    ></CompReadyCapabilities>
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
                    <CompReadyMainInputs
                      baseRelease={baseRelease}
                      baseStartTime={formatLongDate(baseStartTime)}
                      baseEndTime={formatLongDate(baseEndTime)}
                      sampleRelease={sampleRelease}
                      sampleStartTime={formatLongDate(sampleStartTime)}
                      sampleEndTime={formatLongDate(sampleEndTime)}
                      groupByCheckedItems={groupByCheckedItems}
                      excludeCloudsCheckedItems={excludeCloudsCheckedItems}
                      excludeArchesCheckedItems={excludeArchesCheckedItems}
                      excludeNetworksCheckedItems={excludeNetworksCheckedItems}
                      excludeUpgradesCheckedItems={excludeUpgradesCheckedItems}
                      excludeVariantsCheckedItems={excludeVariantsCheckedItems}
                      component={component}
                      environment={environment}
                      setBaseRelease={setBaseRelease}
                      setSampleRelease={setSampleRelease}
                      setBaseStartTime={setBaseStartTime}
                      setBaseEndTime={setBaseEndTime}
                      setSampleStartTime={setSampleStartTime}
                      setSampleEndTime={setSampleEndTime}
                      setGroupByCheckedItems={setGroupByCheckedItems}
                      setExcludeArchesCheckedItems={
                        setExcludeArchesCheckedItems
                      }
                      setExcludeNetworksCheckedItems={
                        setExcludeNetworksCheckedItems
                      }
                      setExcludeCloudsCheckedItems={
                        setExcludeCloudsCheckedItems
                      }
                      setExcludeUpgradesCheckedItems={
                        setExcludeUpgradesCheckedItems
                      }
                      setExcludeVariantsCheckedItems={
                        setExcludeVariantsCheckedItems
                      }
                      handleGenerateReport={handleGenerateReport}
                      showValuesForReport={showValuesForReport}
                    ></CompReadyMainInputs>
                  </Drawer>
                  {pageTitle}
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
                              null
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
