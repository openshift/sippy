import './ComponentReadiness.css'
import { BooleanParam, StringParam, useQueryParam } from 'use-query-params'
import {
  cancelledDataTable,
  formColumnName,
  getAPIUrl,
  getColumns,
  getKeeperColumns,
  getUpdatedUrlParts,
  gotFetchError,
  initialPageTable,
  makePageTitle,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { CompReadyVarsContext } from './CompReadyVars'
import { escapeRegex } from '../helpers'
import { grey } from '@mui/material/colors'
import { Grid, TableContainer, Tooltip, Typography } from '@mui/material'
import { makeStyles, useTheme } from '@mui/styles'
import { Route, Switch, useRouteMatch } from 'react-router-dom'
import { useCookies } from 'react-cookie'
import ComponentReadinessHelp from './ComponentReadinessHelp'
import ComponentReadinessToolBar from './ComponentReadinessToolBar'
import CompReadyCancelled from './CompReadyCancelled'
import CompReadyEnvCapabilities from './CompReadyEnvCapabilities'
import CompReadyEnvCapability from './CompReadyEnvCapability'
import CompReadyEnvCapabilityTest from './CompReadyEnvCapabilityTest'
import CompReadyPageTitle from './CompReadyPageTitle'
import CompReadyProgress from './CompReadyProgress'
import CompReadyRow from './CompReadyRow'
import CompReadyTestReport from './CompReadyTestReport'
import CopyPageURL from './CopyPageURL'
import GeneratedAt from './GeneratedAt'
import React, { Fragment, useContext, useEffect, useState } from 'react'
import Sidebar from './Sidebar'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'

const drawerWidth = 240

const useStyles = makeStyles((theme) => ({
  root: {
    display: 'flex',
    flexGrow: 1,
  },
  title: {
    flexGrow: 1,
  },
  appBar: {
    transition: theme.transitions.create(['margin', 'width'], {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.leavingScreen,
    }),
  },
  appBarShift: {
    width: `calc(100% - ${drawerWidth}px)`,
    marginLeft: drawerWidth,
    transition: theme.transitions.create(['margin', 'width'], {
      easing: theme.transitions.easing.easeOut,
      duration: theme.transitions.duration.enteringScreen,
    }),
  },
  backdrop: {
    zIndex: theme.zIndex.drawer + 1,
    color: '#fff',
  },
  menuButton: {
    marginRight: theme.spacing(2),
  },
  hide: {
    display: 'none',
  },
  drawer: {
    width: drawerWidth,
    flexShrink: 0,
  },
  drawerPaper: {
    width: drawerWidth,
  },
  drawerHeader: {
    display: 'flex',
    alignItems: 'center',
    padding: theme.spacing(0, 1),
    // necessary for content to be below app bar
    ...theme.mixins.toolbar,
    justifyContent: 'flex-end',
  },
  content: {
    maxWidth: '100%',
    flexGrow: 1,
    padding: theme.spacing(3),
    transition: theme.transitions.create('margin', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.leavingScreen,
    }),
    marginLeft: -drawerWidth,
  },
  contentShift: {
    transition: theme.transitions.create('margin', {
      easing: theme.transitions.easing.easeOut,
      duration: theme.transitions.duration.enteringScreen,
    }),
    marginLeft: 0,
  },

  // Table styling

  crColResultFull: {
    backgroundColor: theme.palette.mode === 'dark' ? grey[800] : 'whitesmoke',
    fontWeight: 'bold',
    position: 'sticky',
    top: 0,
    left: 0,
    zIndex: 1,
  },
  crColResult: {
    hyphens: 'auto',
    verticalAlign: 'top !important',
    backgroundColor: theme.palette.mode === 'dark' ? grey[800] : 'whitesmoke',
    fontWeight: 'bold',
    position: 'sticky',
    top: 0,
    left: 0,
    zIndex: 1,
  },
  crColJobName: {
    verticalAlign: 'bottom',
    backgroundColor: theme.palette.mode === 'dark' ? grey[900] : grey['A200'],
  },
  componentName: {
    width: 175,
    minWidth: 175,
    maxWidth: 175,
    backgroundColor: theme.palette.mode === 'dark' ? grey[800] : 'whitesmoke',
    fontWeight: 'bold',
    position: 'sticky',
    left: 0,
    zIndex: 1,
  },
  crCellResult: {
    backgroundColor: theme.palette.mode === 'dark' ? grey[100] : 'white',
    height: 50,
    width: 50,
    padding: '5px !important',
    lineHeight: '13px !important',
    border: '1px solid #EEE',
  },
  crCellName: {
    fontSize: '11px !important',
  },
  crCellCapabCol: {
    fontSize: '11px !important',
    width: '300px',
  },
}))

export const ComponentReadinessStyleContext = React.createContext({})

// Big query requests take a while so give the user the option to
// abort in case they inadvertently requested a huge dataset.
let abortController = new AbortController()
const cancelFetch = () => {
  abortController.abort()
}

export default function ComponentReadiness(props) {
  const theme = useTheme()
  const classes = useStyles(theme)

  const [searchRowRegexURL, setSearchRowRegexURL] = useQueryParam(
    'searchRow',
    StringParam
  )
  const [searchRowRegex, setSearchRowRegex] = useState(searchRowRegexURL)
  const handleSearchRowRegexChange = (event) => {
    const searchValue = event.target.value
    setSearchRowRegex(searchValue)
  }

  const [searchColumnRegexURL, setSearchColumnRegexURL] = useQueryParam(
    'searchColumn',
    StringParam
  )
  const [searchColumnRegex, setSearchColumnRegex] =
    useState(searchColumnRegexURL)
  const handleSearchColumnRegexChange = (event) => {
    const searchValue = event.target.value
    setSearchColumnRegex(searchValue)
  }

  const [redOnlyURL = false, setRedOnlyURL] = useQueryParam(
    'redOnly',
    BooleanParam
  )
  const [redOnlyChecked, setRedOnlyChecked] = React.useState(redOnlyURL)
  const handleRedOnlyCheckboxChange = (event) => {
    setRedOnlyChecked(event.target.checked)
  }

  const varsContext = useContext(CompReadyVarsContext)

  const [testIdParam, setTestIdParam] = useQueryParam('testId', StringParam)
  const [testNameParam, setTestNameParam] = useQueryParam('testName', String)
  const [testBasisReleaseParam, setTestBasisReleaseParam] = useQueryParam(
    'testBasisRelease',
    String
  )

  const [testId, setTestId] = React.useState(testIdParam)
  const [testName, setTestName] = React.useState(testNameParam)
  const [testBasisRelease, setTestBasisRelease] = React.useState(
    testBasisReleaseParam
  )

  const { path, url } = useRouteMatch()

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  const [copyPopoverEl, setCopyPopoverEl] = React.useState(null)
  const copyPopoverOpen = Boolean(copyPopoverEl)

  const linkToReport = () => {
    const currentUrl = new URL(window.location.href)
    if (searchRowRegex && searchRowRegex !== '') {
      currentUrl.searchParams.set('searchComponent', searchRowRegex)
    }

    if (searchColumnRegex && searchColumnRegex !== '') {
      currentUrl.searchParams.set('searchColumn', searchColumnRegex)
    }

    if (redOnlyChecked) {
      currentUrl.searchParams.set('redOnly', '1')
    }

    return currentUrl.href
  }

  const copyLinkToReport = (event) => {
    event.preventDefault()
    navigator.clipboard.writeText(linkToReport())
    setCopyPopoverEl(event.currentTarget)
    setTimeout(() => setCopyPopoverEl(null), 2000)
  }

  const clearSearches = () => {
    setSearchRowRegex('')
    if (searchRowRegexURL && searchRowRegexURL !== '') {
      setSearchRowRegexURL('')
    }

    setSearchColumnRegex('')
    if (searchColumnRegexURL && searchColumnRegexURL !== '') {
      setSearchColumnRegexURL('')
    }

    if (setRedOnlyChecked) {
      setRedOnlyURL(false)
    }
    setRedOnlyChecked(false)
  }

  document.title = `Sippy > Component Readiness`
  if (fetchError !== '') {
    return gotFetchError(fetchError)
  }

  // Show the current state of the filter variables and the url.
  // Create API call string and return it.
  const showValuesForReport = () => {
    let apiCallStr = getAPIUrl()

    if (varsContext.view != null && varsContext.view !== '') {
      apiCallStr += '?view=' + varsContext.view
    } else {
      apiCallStr += getUpdatedUrlParts(varsContext)
    }
    return makeRFC3339Time(apiCallStr)
  }

  const columnNames = getColumns(data)
  if (columnNames[0] === 'Cancelled' || columnNames[0] === 'None') {
    return (
      <CompReadyCancelled
        message={columnNames[0]}
        apiCallStr={showValuesForReport()}
      />
    )
  }

  const keepColumnsList =
    data &&
    data.rows &&
    data.rows.length > 1 &&
    getKeeperColumns(data, columnNames, redOnlyChecked)

  const fetchData = (fresh) => {
    let formattedApiCallStr = showValuesForReport()

    // prevent a slightly expensive duplicate request when user navs to /main with no query params,
    // and we're still in the process of setting the default view to use
    if (
      varsContext.views !== undefined &&
      varsContext.views.length > 0 &&
      varsContext.view === undefined &&
      varsContext.baseReleaseParam === undefined
    ) {
      return
    }

    console.log('fetchData api call str: ' + formattedApiCallStr)
    if (fresh) {
      formattedApiCallStr += '&forceRefresh=true'
    }
    fetch(formattedApiCallStr, { signal: abortController.signal })
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('API server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        if (
          Object.keys(json).length === 0 ||
          json.rows === undefined ||
          json.rows.length === 0
        ) {
          // The api call returned 200 OK but the data was empty
          setData(noDataTable)
        } else {
          setData(json)
        }
      })
      .catch((error) => {
        if (error.name === 'AbortError') {
          setData(cancelledDataTable)

          // Once this fired, we need a new one for the next button click.
          abortController = new AbortController()
        } else {
          setFetchError(`API call failed: ${formattedApiCallStr}\n${error}`)
        }
      })
      .finally(() => {
        // Mark the attempt as finished whether successful or not.
        setIsLoaded(true)
      })
  }

  const forceRefresh = () => {
    setIsLoaded(false)
    fetchData(true)
  }

  useEffect(() => {
    if (window.location.pathname.includes('/component_readiness/main')) {
      fetchData()
    } else {
      setIsLoaded(true)
    }
  }, [])

  if (!isLoaded) {
    return (
      <CompReadyProgress
        apiLink={showValuesForReport()}
        cancelFunc={cancelFetch}
      />
    )
  }

  const pageTitle = makePageTitle(
    `Component Readiness for ${varsContext.sampleRelease} vs. ${varsContext.baseRelease}`,
    `page 1`,
    `rows: ${data && data.rows ? data.rows.length : 0}, columns: ${
      data && data.rows && data.rows[0] && data.rows[0].columns
        ? data.rows[0].columns.length
        : 0
    }`
  )

  return (
    <ComponentReadinessStyleContext.Provider value={classes}>
      <Route
        path={path}
        render={({ location }) => (
          <Fragment>
            <Grid
              container
              justifyContent="center"
              size="xl"
              className="cr-view"
            ></Grid>
            {/* eslint-disable react/prop-types */}
            <Switch>
              <Route
                path="/component_readiness/help"
                render={(props) => {
                  return <ComponentReadinessHelp key="cr-help" />
                }}
              />
              <Route
                path="/component_readiness/test_details"
                render={(props) => {
                  // We need to pass the testId and testName
                  const filterVals = getUpdatedUrlParts(varsContext)
                  varsContext.setComponentParam(varsContext.component)
                  varsContext.setCapabilityParam(varsContext.capability)
                  setTestIdParam(testId)
                  setTestNameParam(testName)
                  setTestBasisReleaseParam(testBasisRelease)
                  varsContext.setEnvironmentParam(varsContext.environment)
                  return (
                    <CompReadyTestReport
                      key="testreport"
                      filterVals={filterVals}
                      component={varsContext.component}
                      capability={varsContext.capability}
                      environment={varsContext.environment}
                      testId={testId}
                      testName={testName}
                      testBasisRelease={testBasisRelease}
                    ></CompReadyTestReport>
                  )
                }}
              />
              <Route
                path="/component_readiness/test"
                render={(props) => {
                  // We need to pass the testId
                  const filterVals = getUpdatedUrlParts(varsContext)
                  varsContext.setComponentParam(varsContext.component)
                  varsContext.setCapabilityParam(varsContext.capability)
                  setTestIdParam(testId)
                  return (
                    <CompReadyEnvCapabilityTest
                      key="capabilitytest"
                      filterVals={filterVals}
                      component={varsContext.component}
                      capability={varsContext.capability}
                      testId={testId}
                    ></CompReadyEnvCapabilityTest>
                  )
                }}
              />
              <Route
                path="/component_readiness/env_test"
                render={(props) => {
                  // We need to pass the environment and testId
                  const filterVals = getUpdatedUrlParts(varsContext)
                  varsContext.setComponentParam(varsContext.component)
                  varsContext.setCapabilityParam(varsContext.capability)
                  varsContext.setEnvironmentParam(varsContext.environment)
                  setTestIdParam(testId)
                  return (
                    <CompReadyEnvCapabilityTest
                      key="capabilitytest"
                      filterVals={filterVals}
                      component={varsContext.component}
                      capability={varsContext.capability}
                      testId={testId}
                      environment={varsContext.environment}
                      theme={theme}
                    ></CompReadyEnvCapabilityTest>
                  )
                }}
              />
              <Route
                path="/component_readiness/capability"
                render={(props) => {
                  // We need the component and capability from url
                  const filterVals = getUpdatedUrlParts(varsContext)
                  varsContext.setComponentParam(varsContext.component)
                  varsContext.setCapabilityParam(varsContext.capability)
                  return (
                    <CompReadyEnvCapability
                      key="capabilities"
                      filterVals={filterVals}
                      component={varsContext.component}
                      capability={varsContext.capability}
                      theme={theme}
                    ></CompReadyEnvCapability>
                  )
                }}
              />
              <Route
                path="/component_readiness/env_capability"
                render={(props) => {
                  // We need the component and capability and environment from url
                  const filterVals = getUpdatedUrlParts(varsContext)
                  return (
                    <CompReadyEnvCapability
                      key="capabilities"
                      filterVals={filterVals}
                      component={varsContext.component}
                      capability={varsContext.capability}
                      environment={varsContext.environment}
                      theme={theme}
                    ></CompReadyEnvCapability>
                  )
                }}
              />
              <Route
                path="/component_readiness/capabilities"
                render={(props) => {
                  const filterVals = getUpdatedUrlParts(varsContext)
                  return (
                    <CompReadyEnvCapabilities
                      filterVals={filterVals}
                      component={varsContext.component}
                      theme={theme}
                    ></CompReadyEnvCapabilities>
                  )
                }}
              />
              <Route
                path="/component_readiness/env_capabilities"
                render={(props) => {
                  const filterVals = getUpdatedUrlParts(varsContext)
                  varsContext.setComponentParam(varsContext.component)
                  varsContext.setEnvironmentParam(varsContext.environment)
                  // We normally would get the environment and pass it but it doesn't work
                  return (
                    <CompReadyEnvCapabilities
                      filterVals={filterVals}
                      component={varsContext.component}
                      environment={varsContext.environment}
                      theme={theme}
                    ></CompReadyEnvCapabilities>
                  )
                }}
              />
              <Route
                path={'/component_readiness/main'}
                render={(props) => {
                  const filterVals = getUpdatedUrlParts(varsContext)
                  return (
                    <div className="cr-view">
                      <Sidebar />
                      <CompReadyPageTitle
                        pageTitle={pageTitle}
                        apiCallStr={showValuesForReport()}
                      />
                      {data === initialPageTable ? (
                        <Typography variant="h6" style={{ textAlign: 'left' }}>
                          To get started, make your filter selections on the
                          left, left, then click Generate Report
                        </Typography>
                      ) : (
                        <div>
                          <ComponentReadinessToolBar
                            searchRowRegex={searchRowRegex}
                            handleSearchRowRegexChange={
                              handleSearchRowRegexChange
                            }
                            searchColumnRegex={searchColumnRegex}
                            handleSearchColumnRegexChange={
                              handleSearchColumnRegexChange
                            }
                            redOnlyChecked={redOnlyChecked}
                            handleRedOnlyCheckboxChange={
                              handleRedOnlyCheckboxChange
                            }
                            clearSearches={clearSearches}
                            data={data}
                            filterVals={filterVals}
                            forceRefresh={forceRefresh}
                          />
                          <TableContainer
                            component="div"
                            className="cr-table-wrapper"
                          >
                            <Table className="cr-comp-read-table">
                              <TableHead>
                                <TableRow>
                                  {
                                    <TableCell
                                      className={classes.crColResultFull}
                                    >
                                      <Typography
                                        className={classes.crCellName}
                                      >
                                        Name
                                      </Typography>
                                    </TableCell>
                                  }
                                  {columnNames
                                    .filter(
                                      (column, idx) =>
                                        column.match(
                                          new RegExp(
                                            escapeRegex(searchColumnRegex),
                                            'i'
                                          )
                                        ) && keepColumnsList[idx]
                                    )

                                    .map((column, idx) => {
                                      if (column !== 'Name') {
                                        return (
                                          <TableCell
                                            className={classes.crColResult}
                                            key={'column' + '-' + idx}
                                          >
                                            <Tooltip
                                              title={
                                                'Single row report for ' +
                                                column
                                              }
                                            >
                                              <Typography
                                                className={classes.crCellName}
                                              >
                                                {' '}
                                                {column}
                                              </Typography>
                                            </Tooltip>
                                          </TableCell>
                                        )
                                      }
                                    })}
                                </TableRow>
                              </TableHead>
                              <TableBody>
                                {Object.keys(data.rows)
                                  .filter((componentIndex) =>
                                    data.rows[componentIndex].component.match(
                                      new RegExp(
                                        escapeRegex(searchRowRegex),
                                        'i'
                                      )
                                    )
                                  )
                                  .filter((componentIndex) =>
                                    redOnlyChecked
                                      ? data.rows[componentIndex].columns.some(
                                          // Filter for rows where any of their columns have status <= -2 and accepted by the regex.
                                          (column) =>
                                            column.status <= -2 &&
                                            formColumnName(column).match(
                                              new RegExp(
                                                escapeRegex(searchColumnRegex),
                                                'i'
                                              )
                                            )
                                        )
                                      : true
                                  )
                                  .map((componentIndex) => (
                                    <CompReadyRow
                                      key={componentIndex}
                                      componentName={
                                        data.rows[componentIndex].component
                                      }
                                      results={data.rows[
                                        componentIndex
                                      ].columns.filter(
                                        (column, idx) =>
                                          formColumnName(column).match(
                                            new RegExp(
                                              escapeRegex(searchColumnRegex),
                                              'i'
                                            )
                                          ) && keepColumnsList[idx]
                                      )}
                                      columnNames={columnNames.filter(
                                        (column, idx) =>
                                          column.match(
                                            new RegExp(
                                              escapeRegex(searchColumnRegex),
                                              'i'
                                            )
                                          ) && keepColumnsList[idx]
                                      )}
                                      grayFactor={redOnlyChecked ? 100 : 0}
                                      filterVals={filterVals}
                                    />
                                  ))}
                              </TableBody>
                            </Table>
                          </TableContainer>
                          <GeneratedAt time={data.generated_at} />
                          <CopyPageURL apiCallStr={showValuesForReport()} />
                        </div>
                      )}
                    </div>
                  )
                }}
              />
            </Switch>
          </Fragment>
        )}
      />
    </ComponentReadinessStyleContext.Provider>
  )
}
