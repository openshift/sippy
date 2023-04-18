import './ComponentReadiness.css'
import { Alert, TabContext } from '@material-ui/lab'
import { DateTimePicker, MuiPickersUtilsProvider } from '@material-ui/pickers'
import {
  Drawer,
  Grid,
  TableContainer,
  Tooltip,
  Typography,
} from '@material-ui/core'
import { Fragment, useEffect } from 'react'
import { GridToolbarFilterDateUtils } from '../datagrid/GridToolbarFilterDateUtils'
import { Link, Redirect, Route, Switch, useRouteMatch } from 'react-router-dom'
import { useStyles } from '../App'
import { useTheme } from '@material-ui/core/styles'
import Button from '@material-ui/core/Button'
import CheckBoxList from './CheckboxList'
import ChevronLeftIcon from '@material-ui/icons/ChevronLeft'
import ChevronRightIcon from '@material-ui/icons/ChevronRight'
import clsx from 'clsx'
import EmojiEmotionsOutlinedIcon from '@material-ui/icons/EmojiEmotionsOutlined'
import FireplaceIcon from '@material-ui/icons/Fireplace'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import IconButton from '@material-ui/core/IconButton'
import MenuIcon from '@material-ui/icons/Menu'
import PropTypes from 'prop-types'
import React from 'react'
import ReleaseSelector from './ReleaseSelector'
import Table from '@material-ui/core/Table'
import TableBody from '@material-ui/core/TableBody'
import TableCell from '@material-ui/core/TableCell'
import TableHead from '@material-ui/core/TableHead'
import TableRow from '@material-ui/core/TableRow'

function SeverityIcon(props) {
  const theme = useTheme()
  const status = props.status

  let icon = ''

  if (status > 8) {
    icon = (
      <EmojiEmotionsOutlinedIcon
        data-icon="EmojiEmotionsOutlinedIcon"
        fontSize="large"
        style={{ color: theme.palette.success.main }}
      />
    )
  } else if (status > 5) {
    icon = (
      <FireplaceIcon
        data-icon="FireplaceIcon"
        fontSize="small"
        style={{
          color: theme.palette.error.main,
        }}
      />
    )
  } else {
    icon = (
      <FireplaceIcon
        data-icon="FireplaceIcon"
        fontSize="large"
        style={{ color: theme.palette.error.main }}
      />
    )
  }

  return <Tooltip title={status}>{icon}</Tooltip>
}

SeverityIcon.propTypes = {
  status: PropTypes.number,
}

function Cell(props) {
  const status = props.status
  const theme = useTheme()

  if (status === undefined) {
    return (
      <Tooltip title="No data">
        <TableCell
          className="cell-result"
          style={{
            textAlign: 'center',
            backgroundColor: theme.palette.text.disabled,
          }}
        >
          <HelpOutlineIcon style={{ color: theme.palette.text.disabled }} />
        </TableCell>
      </Tooltip>
    )
  } else {
    return (
      <TableCell
        className="cell-result"
        style={{
          textAlign: 'center',
          backgroundColor: 'white',
        }}
      >
        <SeverityIcon status={status} />
      </TableCell>
    )
  }
}

Cell.propTypes = {
  status: PropTypes.number,
}

function groupByReport(columnName, release) {
  return `/tests/${release}?columnName=${columnName}`
}

function componentReport(componentName, release) {
  return `/tests/${release}?componentName=${componentName}`
}
function Row(props) {
  const { columnNames, componentName, results, release } = props

  // columnNames includes the "Name" column
  // componentName will be the name of the component and be under the "Name" column
  // results will contain the status value per columnName

  // Put the component name on the left side with a link to a component specific
  // report.
  const componentNameColumn = (
    <TableCell className={'component-name'} key={componentName}>
      <Tooltip title={'Specific report for' + componentName}>
        <Typography className="cell-name">
          <Link to={componentReport(componentName, { release })}>
            {componentName}
          </Link>
        </Typography>
      </Tooltip>
    </TableCell>
  )

  return (
    <Fragment>
      <TableRow>
        {componentNameColumn}
        {columnNames.map((column, idx) => {
          // We already printed the componentName earlier so skip it here.
          if (column !== 'Name') {
            return (
              <Cell
                key={'testName-' + idx}
                status={results[column]}
                release={release}
                variant={column}
                testName={componentName}
              />
            )
          }
        })}
      </TableRow>
    </Fragment>
  )
}

Row.propTypes = {
  results: PropTypes.object,
  columnNames: PropTypes.array.isRequired,
  componentName: PropTypes.string.isRequired,
  release: PropTypes.string.isRequired,
}

export default function ComponentReadiness(props) {
  const classes = useStyles()
  const theme = useTheme()

  const [drawerOpen, setDrawerOpen] = React.useState(true)
  const handleDrawerOpen = () => {
    setDrawerOpen(true)
  }

  const handleDrawerClose = () => {
    setDrawerOpen(false)
  }

  const groupByList = ['Platform', 'Arch', 'Network', 'Upgrade', 'Variant']
  const [groupByCheckedItems, setGroupByCheckedItems] = React.useState([
    'Platform',
    'Network',
  ])

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

  const [excludeCloudsCheckedItems, setExcludeCloudsCheckedItems] =
    React.useState(excludeCloudsList)

  // TODO: Get these from single place.
  const excludeArchesList = ['amd64', 'arm64', 'ppc64le', 's390x', 'multi']

  const [excludeArchesCheckedItems, setExcludeArchesCheckedItems] =
    React.useState(excludeArchesList)

  const excludeNetworksList = ['ovn', 'sdn']

  const [excludeNetworksCheckedItems, setExcludeNetworksCheckedItems] =
    React.useState(excludeNetworksList)

  const [historicalRelease, setHistoricalRelease] = React.useState('4.13')
  const [sampleRelease, setSampleRelease] = React.useState('4.14')

  const initialTime = new Date()
  const fromTime = new Date(initialTime.getTime() - 72 * 60 * 60 * 1000)

  const [historicalReleaseFrom, setHistoricalReleaseFrom] = React.useState(
    fromTime.toString()
  )
  const [historicalReleaseTo, setHistoricalReleaseTo] = React.useState(
    new Date(fromTime.getTime() + 3 * 60 * 60 * 1000)
  )

  const [sampleReleaseFrom, setSampleReleaseFrom] = React.useState(
    fromTime.toString()
  )
  const [sampleReleaseTo, setSampleReleaseTo] = React.useState(
    new Date(fromTime.getTime() + 3 * 60 * 60 * 1000)
  )

  const excludeUpgradesList = [
    'No Upgrade',
    'Y-Stream Upgrade',
    'Z-Stream Upgrade',
  ]

  const [excludeUpgradesCheckedItems, setExcludeUpgradesCheckedItems] =
    React.useState(excludeUpgradesList)

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

  const [excludeVariantsCheckedItems, setExcludeVariantsCheckedItems] =
    React.useState(excludeVariantsList)

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

  const fetchData = () => {
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

  useEffect(() => {
    document.title = `Sippy > Component Readiness`
    console.log('useEffect called')
    fetchData()
  }, [])

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

  if (data === undefined || data.tests.length === 0) {
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
  const handleClick = () => {
    console.log('historicalReleaseFrom', historicalReleaseFrom)
    console.log('historicalReleaseTo', historicalReleaseTo)
    console.log('sampleReleaseFrom', sampleReleaseFrom)
    console.log('sampleReleaseTo', sampleReleaseTo)
    console.log('groupBy: ', groupByCheckedItems)
    console.log('excludeClouds: ', excludeCloudsCheckedItems)
    console.log('excludeArches', excludeArchesCheckedItems)
    console.log('excludeNetworks', excludeNetworksCheckedItems)
    console.log('excludeUpgrades', excludeUpgradesCheckedItems)
    console.log('excludeVariants', excludeVariantsCheckedItems)
    console.log('historicalRelease', historicalRelease)
    console.log('sampleRelease', sampleRelease)
  }

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
                        onClick={handleClick}
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
                          format="yyyy-MM-dd HH:mm"
                          ampm={false}
                          value={sampleReleaseFrom}
                          onChange={(e) => {
                            setSampleReleaseFrom(e.getTime())
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
                          format="yyyy-MM-dd HH:mm"
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
                          format="yyyy-MM-dd HH:mm"
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
                          format="yyyy-MM-dd HH:mm"
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
                                    title={'Specific report for ' + column}
                                  >
                                    <Typography className="cell-name">
                                      <Link
                                        to={groupByReport(column, {
                                          historicalRelease,
                                        })}
                                      >
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
                          <Row
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
              <Redirect from="/" to={url} />
            </Switch>
          </TabContext>
        )}
      />
    </Fragment>
  )
}
