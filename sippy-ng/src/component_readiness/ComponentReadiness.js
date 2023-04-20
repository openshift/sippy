import './ComponentReadiness.css'
import { Alert, TabContext } from '@material-ui/lab'
import { ArrayParam, StringParam, useQueryParam } from 'use-query-params'
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
import { useTheme } from '@material-ui/core/styles'
import Button from '@material-ui/core/Button'
import CheckBoxList from './CheckboxList'
import ChevronLeftIcon from '@material-ui/icons/ChevronLeft'
import ChevronRightIcon from '@material-ui/icons/ChevronRight'
import clsx from 'clsx'
import CompReadyRow from './CompReadyRow'
import IconButton from '@material-ui/core/IconButton'
import MenuIcon from '@material-ui/icons/Menu'
import React from 'react'
import ReleaseSelector from './ReleaseSelector'
import Table from '@material-ui/core/Table'
import TableBody from '@material-ui/core/TableBody'
import TableCell from '@material-ui/core/TableCell'
import TableHead from '@material-ui/core/TableHead'
import TableRow from '@material-ui/core/TableRow'

// This is used when the user clicks on one of the columns at the top of the table
function topSideReport(columnName) {
  return '/componentreadiness/' + safeEncodeURIComponent(columnName) + '/tests'
}

export default function ComponentReadiness(props) {
  console.log('ComponentReadiness start')
  const classes = useStyles()
  const theme = useTheme()

  // Extract the url and get the parameters from it
  const location = useLocation()
  const groupByParameters = new URLSearchParams(location.search)

  const params = {}
  groupByParameters.forEach((value, key) => {
    params[key] = value
  })
  console.log('params from url: ', JSON.stringify(params))

  const [drawerOpen, setDrawerOpen] = React.useState(true)
  const handleDrawerOpen = () => {
    setDrawerOpen(true)
  }

  const handleDrawerClose = () => {
    setDrawerOpen(false)
  }

  const groupByList = ['Platform', 'Arch', 'Network', 'Upgrade', 'Variant']
  let tmp = groupByParameters.get('group_by')
  let initGroupBy = []
  if (tmp !== null) {
    initGroupBy = tmp.split(',')
  }
  const [groupByCheckedItems, setGroupByCheckedItems] = React.useState(
    // Extract the 'group_by=platform,arch,network' from the url to be the initial
    // groupBy array of checked values.
    initGroupBy
  )
  console.log('initGroupBy: ', initGroupBy)
  console.log('groupByCheckedItems: ', groupByCheckedItems)

  // TODO: Get these from single place.
  const excludeCloudsList = [
    'aws',
    'gcp',
    'azure',
    'libvirt',
    'ovirt',
    'vsphere',
    'metal',
    'IBM Cloud',
    'Alibaba',
    'Unknown',
  ]
  tmp = groupByParameters.get('excluded_platforms')
  let initExcludeCloudsList = []
  if (tmp !== null && tmp !== '') {
    initExcludeCloudsList = tmp.split(',')
  }
  const [excludeCloudsCheckedItems, setExcludeCloudsCheckedItems] =
    React.useState(
      // Extract the 'excluded_platforms=aws' from the url to be the initial
      // Exclude Clouds array of checked values.
      initExcludeCloudsList
    )
  console.log('initExcludeCloudsList: ', initExcludeCloudsList)
  console.log('excludeCloudsCheckedItems: ', excludeCloudsCheckedItems)

  // TODO: Get these from single place.
  const excludeArchesList = ['amd64', 'arm64', 'ppc64le', 's390x', 'multi']
  tmp = groupByParameters.get('excluded_arches')
  let initExcludeArchesList = []
  if (tmp !== null) {
    initExcludeArchesList = tmp.split(',')
  }
  const [excludeArchesCheckedItems, setExcludeArchesCheckedItems] =
    React.useState(initExcludeArchesList)
  console.log('initExcludeArchesList: ', initExcludeArchesList)
  console.log('excludeArchesCheckedItems: ', excludeArchesCheckedItems)

  const excludeNetworksList = ['ovn', 'sdn']
  tmp = groupByParameters.get('excluded_networks')
  let initExcludeNetworksList = []
  if (tmp !== null) {
    initExcludeNetworksList = tmp.split(',')
  }
  const [excludeNetworksCheckedItems, setExcludeNetworksCheckedItems] =
    React.useState(initExcludeNetworksList)
  console.log('initExcludeNetworksList: ', initExcludeNetworksList)
  console.log('excludeNetworksCheckedItems: ', excludeNetworksCheckedItems)

  const [historicalRelease = '4.13', setHistoricalRelease] = useQueryParam(
    'historicalRelease',
    StringParam
  )
  const [sampleRelease = '4.14', setSampleRelease] = useQueryParam(
    'sampleRelease',
    StringParam
  )

  const days = 24 * 60 * 60 * 1000
  const initialTime = new Date()
  const initialFromTime = new Date(initialTime.getTime() - 30 * days)
  const initialToTime = new Date(initialTime.getTime())

  const [sampleReleaseFrom, setSampleReleaseFrom] = useQueryParam(
    'sampleReleaseFrom',
    StringParam
  )
  const [sampleReleaseTo, setSampleReleaseTo] = useQueryParam(
    'sampleReleaseTo',
    StringParam
  )

  const [historicalReleaseFrom, setHistoricalReleaseFrom] = useQueryParam(
    'historicalReleaseFrom',
    StringParam
  )
  const [historicalReleaseTo, setHistoricalReleaseTo] = useQueryParam(
    'historicalReleaseTo',
    StringParam
  )

  const excludeUpgradesList = [
    'No Upgrade',
    'Y-Stream Upgrade',
    'Z-Stream Upgrade',
  ]

  tmp = groupByParameters.get('excluded_upgrades')
  let initExcludeUpgradesList = []
  if (tmp !== null) {
    initExcludeUpgradesList = groupByParameters.tmp.split(',')
  }
  const [excludeUpgradesCheckedItems, setExcludeUpgradesCheckedItems] =
    React.useState(initExcludeUpgradesList)
  console.log('initExcludeUpgradesList: ', initExcludeUpgradesList)
  console.log('excludeUpgradesCheckedItems: ', excludeUpgradesCheckedItems)

  const excludeVariantsList = [
    'Standard',
    'Assisted',
    'FIPs',
    'MicroShift',
    'Serial',
    'Real-Time',
    'Tech Preview',
    'Compact',
    'Hypershift',
    'OSD',
    'Proxy',
    'Single Node',
  ]
  tmp = groupByParameters.get('excluded_variants')
  let initExcludeVariantsList = []
  if (tmp !== null) {
    initExcludeVariantsList = groupByParameters.tmp.split(',')
  }
  const [excludeVariantsCheckedItems, setExcludeVariantsCheckedItems] =
    React.useState(initExcludeVariantsList)
  console.log('initExcludeVariantsList: ', initExcludeVariantsList)
  console.log('excludeVariantsCheckedItems: ', excludeVariantsCheckedItems)

  const pageTitle = (
    <Typography variant="h4" style={{ margin: 20, textAlign: 'center' }}>
      Component Readiness for {sampleRelease}
    </Typography>
  )

  const { path, url } = useRouteMatch()

  console.count('path: ' + path)
  console.log('url: ', url)
  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  //const fetchData = () => {
  //  const fromFile = true
  //  if (fromFile) {
  //    //const json = require('./output.json')
  //    //const json = require('./5-rows-map.json')
  //    //const json = require('./5-rows-map2.json')
  //    const json = require('./5-rows-comp.json')
  //    setData(json)
  //    console.log('json:', json)
  //    setLoaded(true)
  //  } else {
  //    fetch(
  //      // This will call the new API for component readiness
  //      process.env.REACT_APP_API_URL +
  //        '/api/upgrade?release=' +
  //        { historicalRelease }
  //    )
  //      .then((response) => {
  //        if (response.status !== 200) {
  //          throw new Error('server returned ' + response.status)
  //        }
  //        return response.json()
  //      })
  //      .then((json) => {
  //        setData(json)
  //        setLoaded(true)
  //      })
  //      .catch((error) => {
  //        setFetchError(
  //          'Could not retrieve component readiness data' + ', ' + error
  //        )
  //      })
  //  }
  //}
  //useEffect(() => {
  //  document.title = `Sippy > Component Readiness`
  //  fetchData()
  //}, [])
  useEffect(() => {
    document.title = `Sippy > Component Readiness`
    setData({
      column_names: ['Name', 'Empty'],
      tests: {
        Empty: {
          Empty: 10,
          Name: 1,
        },
      },
    })
    setLoaded(true)
  }, [])

  document.title = `Sippy > Component Readiness`
  if (fetchError !== '') {
    return (
      <Alert severity="error">
        Failed to load component readiness data, {fetchError}
      </Alert>
    )
  }

  if (!isLoaded) {
    return <p>Loading component readiness data ...</p>
  }

  console.log('data: ', data)
  if (Object.keys(data).length === 0 || data.tests.length === 0) {
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

  if (data.column_names.length === 0) {
    return (
      <Typography variant="h6" style={{ marginTop: 50 }}>
        No per-variant data found.
      </Typography>
    )
  }

  //const handleClick = (e) => {
  //  //console.log('historicalReleaseFrom', historicalReleaseFrom)
  //  //console.log('historicalReleaseTo', historicalReleaseTo)
  //  //console.log('sampleReleaseFrom', sampleReleaseFrom)
  //  //console.log('sampleReleaseTo', sampleReleaseTo)
  //  //console.log('groupBy: ', groupByCheckedItems)
  //  //console.log('excludeClouds: ', excludeCloudsCheckedItems)
  //  //console.log('excludeArches', excludeArchesCheckedItems)
  //  //console.log('excludeNetworks', excludeNetworksCheckedItems)
  //  //console.log('excludeUpgrades', excludeUpgradesCheckedItems)
  //  //console.log('excludeVariants', excludeVariantsCheckedItems)
  //  //console.log('historicalRelease', historicalRelease)
  //  //console.log('sampleRelease', sampleRelease)

  //  const history = useHistory()
  //  console.log('here')
  //  history.pushState(
  //    'componentreadiness/?basis_release=4.14&basis_start_dt=2023-04-10:00:00&basis_end_dt=2023-04-13:00:00basis_release=4.14&basis_start_dt=2023-04-10:00:00&basis_end_dt=2023-04-13:00:00'
  //  )
  //}
  const handleGenerateReport = () => {
    console.log('--------------- handleGenerateReport ------------------')
    console.log('historicalRelease', historicalRelease)
    console.log('historicalReleaseFrom', historicalReleaseFrom)
    console.log('historicalReleaseTo', historicalReleaseTo)
    console.log('sampleRelease', sampleRelease)
    console.log('sampleReleaseFrom', sampleReleaseFrom)
    console.log('sampleReleaseTo', sampleReleaseTo)
    console.log('groupBy: ', groupByCheckedItems)
    console.log('excludeClouds: ', excludeCloudsCheckedItems)
    console.log('excludeArches', excludeArchesCheckedItems)
    console.log('excludeNetworks', excludeNetworksCheckedItems)
    console.log('excludeUpgrades', excludeUpgradesCheckedItems)
    console.log('excludeVariants', excludeVariantsCheckedItems)

    const apiCallStr =
      process.env.REACT_APP_API_URL +
      '/api/comp_read' +
      getUpdatedUrlParts(
        historicalRelease,
        historicalReleaseFrom,
        historicalReleaseTo,
        sampleRelease,
        sampleReleaseFrom,
        sampleReleaseTo,
        groupByCheckedItems,
        excludeCloudsCheckedItems,
        excludeArchesCheckedItems,
        excludeNetworksCheckedItems,
        excludeUpgradesCheckedItems,
        excludeVariantsCheckedItems
      )
    const params = new URLSearchParams(apiCallStr.split('?')[1])

    console.log('*** API Call: ')
    params.forEach((value, key) => {
      console.log(`${key}: ${value}`)
    })
    const fromFile = true
    if (fromFile) {
      //const json = require('./output.json')
      //const json = require('./5-rows-map.json')
      //const json = require('./5-rows-map2.json')
      const json = require('./5-rows-comp.json')
      setData(json)
      console.log('json:', json)
      setLoaded(true)
    } else {
      fetch(
        // This will call the new API for component readiness
        process.env.REACT_APP_API_URL +
          '/api/upgrade?release=' +
          { historicalRelease }
      )
        .then((response) => {
          if (response.status !== 200) {
            throw new Error('server returned ' + response.status)
          }
          return response.json()
        })
        .then((json) => {
          setData(json)
          setLoaded(true)
        })
        .catch((error) => {
          setFetchError(
            'Could not retrieve component readiness data' + ', ' + error
          )
        })
    }
  }

  //'/componentreadiness/' +
  //getUpdatedUrlParts(
  //  historicalRelease,
  //  historicalReleaseFrom,
  //  historicalReleaseTo,
  //  sampleRelease,
  //  sampleReleaseFrom,
  //  sampleReleaseTo,
  //  groupByCheckedItems,
  //  excludeCloudsCheckedItems,
  //  excludeArchesCheckedItems,
  //  excludeNetworksCheckedItems,
  //  excludeUpgradesCheckedItems,
  //  excludeVariantsCheckedItems
  //)

  console.log('ComponentReadiness end')
  return (
    <Fragment>
      <Route
        path="/"
        render={({ location }) => (
          <TabContext value={path}>
            {pageTitle}
            <Grid
              container
              justifyContent="center"
              size="xl"
              className="view"
            ></Grid>
            <Switch>
              <Route path={path}>
                <div className="view" width="100%">
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
                    <div>
                      <Button
                        size="large"
                        variant="contained"
                        color="primary"
                        onClick={handleGenerateReport}
                      >
                        Generate Report
                      </Button>
                    </div>
                    <br></br>
                    <div className="release-sample">
                      <ReleaseSelector
                        label="Current Release"
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
                          format="yyyy-MM-dd-HH:mm"
                          ampm={false}
                          value={sampleReleaseFrom}
                          onChange={(e) => {
                            const formattedTime = format(e, 'yyyy-MM-dd:HH:mm')
                            console.log('sample text: ', formattedTime)
                            setSampleReleaseFrom(formattedTime)
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
                          format="yyyy-MM-dd-HH:mm"
                          ampm={false}
                          value={sampleReleaseTo}
                          onChange={(e) => {
                            setSampleReleaseTo(e.getTime())
                          }}
                        />
                      </MuiPickersUtilsProvider>
                    </div>
                    <div className="release-historical">
                      <ReleaseSelector
                        version={historicalRelease}
                        label="Historical Release"
                        onChange={setHistoricalRelease}
                      ></ReleaseSelector>
                      <MuiPickersUtilsProvider
                        utils={GridToolbarFilterDateUtils}
                      >
                        <DateTimePicker
                          showTodayButton
                          disableFuture
                          label="From"
                          format="yyyy-MM-dd-HH:mm"
                          ampm={false}
                          value={historicalReleaseFrom}
                          onChange={(e) => {
                            setHistoricalReleaseFrom(e.getTime())
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
                          format="yyyy-MM-dd-HH:mm"
                          ampm={false}
                          value={historicalReleaseTo}
                          onChange={(e) => {
                            setHistoricalReleaseTo(e.getTime())
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
                    </div>
                    <div>
                      <CheckBoxList
                        headerName="Exclude Variants"
                        displayList={excludeVariantsList}
                        checkedItems={excludeVariantsCheckedItems}
                        setCheckedItems={setExcludeVariantsCheckedItems}
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
                    </div>
                  </Drawer>
                  <TableContainer component="div" className="wrapper">
                    <Table className="comp-read-table">
                      <TableHead>
                        <TableRow>
                          {
                            <TableCell className={'col-result-full'}>
                              <Typography className="cell-name">
                                Name
                              </Typography>
                            </TableCell>
                          }
                          {data.column_names.map((column, idx) => {
                            if (column !== 'Name') {
                              return (
                                <TableCell
                                  className={'col-result'}
                                  key={'column' + '-' + idx}
                                >
                                  <Tooltip
                                    title={'Topside report for ' + column}
                                  >
                                    <Typography className="cell-name">
                                      <Link to={topSideReport(column)}>
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
                        {Object.keys(data.tests).map((test) => (
                          <CompReadyRow
                            key={test}
                            componentName={test}
                            columnNames={data.column_names}
                            results={data.tests[test]}
                            release={historicalRelease}
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

// Take the list of default values and create a string of parameters that we
// use from the Sidebar when calling the ComponentReadiness component.
export function getCompReadDefaultUrlPart() {
  // This is here as an example of what the POC UI used for API calls
  const sample = {
    group_by: ['cloud', 'arch', 'network'],
    sample_release: '4.14',
    basis_release: '4.14',
    sample_start_dt: '2023-04-16:00:00',
    sample_end_dt: '2023-04-18:00:00',
    basis_start_dt: '2023-04-13:00:00',
    basis_end_dt: '2023-04-16:00:00',
    exclude_platforms: ['aws'],
    exclude_arches: ['amd64'],
    exclude_networks: ['ovn'],
    exclude_upgrades: ['micro'],
    exclude_variants: ['techpreview'],
  }

  const days = 24 * 60 * 60 * 1000
  const dateFormat = 'yyyy-MM-dd:HH:mm'
  const initialTime = new Date()
  const initialFromTime = new Date(initialTime.getTime() - 30 * days)
  const initialToTime = new Date(initialTime.getTime())

  const releaseAndDates = {
    sampleRelease: '4.14',
    historicalRelease: '4.13',
    sampleReleaseFrom: format(initialFromTime, dateFormat),
    sampleReleaseTo: format(initialToTime, dateFormat),
    historicalReleaseFrom: format(initialFromTime, dateFormat),
    historicalReleaseTo: format(initialToTime, dateFormat),
  }

  let retVal = '?'

  retVal = retVal + 'group_by=Platform,Network'
  retVal = retVal + '&excluded_platforms='
  retVal = retVal + '&exclude_arches='
  retVal = retVal + '&exclude_networks='
  retVal = retVal + '&exclude_upgrades='
  retVal = retVal + '&exclude_variants='

  // Turn my map into a list of key/value pairs so we can use map() on it.
  const fieldList = Object.entries(releaseAndDates)
  fieldList.map(([key, value]) => {
    retVal = retVal + '&' + key + '=' + value
  })

  console.log('filter: ', retVal)
  return retVal
}

export function getUpdatedUrlParts(
  historicalRelease,
  historicalReleaseFrom,
  historicalReleaseTo,
  sampleRelease,
  sampleReleaseFrom,
  sampleReleaseTo,
  groupByCheckedItems,
  excludeCloudsCheckedItems,
  excludeArchesCheckedItems,
  excludeNetworksCheckedItems,
  excludeUpgradesCheckedItems,
  excludeVariantsCheckedItems
) {
  const filtersMap = {
    group_by: groupByCheckedItems,
    basis_release: historicalRelease,
    basis_start_dt: historicalReleaseFrom,
    basis_end_dt: historicalReleaseTo,
    sample_release: sampleRelease,
    sample_start_dt: sampleReleaseFrom,
    sample_end_dt: sampleReleaseTo,
    exclude_platforms: excludeCloudsCheckedItems,
    exclude_arches: excludeArchesCheckedItems,
    exclude_networks: excludeNetworksCheckedItems,
    exclude_upgrades: excludeUpgradesCheckedItems,
    exclude_variants: excludeVariantsCheckedItems,
  }
  let retVal = '/?'
  const fieldList = Object.entries(filtersMap)
  fieldList.map(([key, value]) => {
    let amper = '&'
    if (key === 'group_by') {
      amper = ''
    }
    console.log('key: ')
    retVal = retVal + amper + key + '=' + value
  })
  console.log('retVal: ', retVal)
  return retVal
}
