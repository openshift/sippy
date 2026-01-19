import './ComponentReadiness.css'
import { BooleanParam, StringParam, useQueryParam } from 'use-query-params'
import {
  cancelledDataTable,
  formColumnName,
  getColumns,
  getCRMainAPIUrl,
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
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useParams,
} from 'react-router-dom'
import ComponentReadinessHelp from './ComponentReadinessHelp'
import ComponentReadinessToolBar from './ComponentReadinessToolBar'
import CompReadyCancelled from './CompReadyCancelled'
import CompReadyEnvCapabilities from './CompReadyEnvCapabilities'
import CompReadyEnvCapability from './CompReadyEnvCapability'
import CompReadyEnvCapabilityTest from './CompReadyEnvCapabilityTest'
import CompReadyPageTitle from './CompReadyPageTitle'
import CompReadyProgress from './CompReadyProgress'
import CompReadyRow from './CompReadyRow'
import CopyPageURL from './CopyPageURL'
import GeneratedAt from './GeneratedAt'
import React, { Fragment, useContext, useEffect, useState } from 'react'
import Sidebar from './Sidebar'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import TestDetailsReport from './TestDetailsReport'
import Triage from './Triage'
import TriageList from './TriageList'
import WarningsBanner from './WarningsBanner'

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
  selectedJobRun: {
    borderStyle: 'solid',
    borderWidth: '1px',
    borderRadius: '5px',
    // borderColor: theme.palette.mode === 'dark' ? grey[200] : grey['A800'],
    marginRight: '1px',
  },
  unselectedJobRun: {
    marginRight: '1px',
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
    verticalAlign: 'top',
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

function TriageWrapper() {
  const { triageId } = useParams()
  return <Triage id={triageId} />
}

export const ComponentReadinessStyleContext = React.createContext({})
export const TestCapabilitiesContext = React.createContext([])
export const TestLifecyclesContext = React.createContext([])

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

  // Get test-related parameters from context instead of local state
  const { testId, testName, testBasisRelease } =
    useContext(CompReadyVarsContext)

  const location = useLocation()
  const currentPath = location.pathname

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [warnings, setWarnings] = React.useState([])

  const [triageActionTaken, setTriageActionTaken] = React.useState(false)

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
    let apiCallStr = getCRMainAPIUrl()

    if (varsContext.view != null && varsContext.view !== '') {
      apiCallStr += '?view=' + varsContext.view
    } else {
      apiCallStr += getUpdatedUrlParts(varsContext)
    }
    return makeRFC3339Time(apiCallStr)
  }

  // Only calculate columnNames and keepColumnsList for main route
  const isMainRoute =
    location.pathname.endsWith('/component_readiness/main') ||
    location.pathname === '/component_readiness'
  const columnNames = isMainRoute ? getColumns(data) : []
  const keepColumnsList =
    isMainRoute && data?.rows?.length > 0
      ? getKeeperColumns(data, columnNames, redOnlyChecked)
      : []

  // Handle special cases for main route
  if (
    isMainRoute &&
    columnNames.length > 0 &&
    (columnNames[0] === 'Cancelled' || columnNames[0] === 'None')
  ) {
    return (
      <CompReadyCancelled
        message={columnNames[0]}
        apiCallStr={showValuesForReport()}
      />
    )
  }

  const fetchData = (fresh) => {
    // prevent a slightly expensive duplicate request when user navs to /main with no query params,
    // and we're still in the process of setting the default view to use
    // Only skip if we have views AND we're in the process of setting a default view
    // (indicated by missing query params since any normal report request would specify baseRelease or view)
    if (
      varsContext.views !== undefined &&
      varsContext.views.length > 0 &&
      varsContext.view === undefined &&
      varsContext.urlParams.view === undefined &&
      varsContext.urlParams.baseRelease === undefined
    ) {
      return
    }

    let formattedApiCallStr = showValuesForReport()
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
          setWarnings([])
        } else {
          setData(json)
          // Extract warnings from the API response
          setWarnings(json.warnings || [])
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

  // Helper function to get only report-related parameters (excluding UI state params)
  const getReportParams = () => {
    const params = new URLSearchParams(location.search)
    const uiOnlyParams = [
      'regressedModal',
      'regressedModalTab',
      'regressedModalRow',
      'regressedModalPage',
      'regressedModalTestRow',
      'regressedModalTestPage',
      'regressedModalFilters',
      'regressedModalTestFilters',
      'triageFilters',
      'searchComponent',
      'searchColumn',
      'searchRow',
      'redOnly',
    ]

    // Remove UI-only parameters
    uiOnlyParams.forEach((param) => params.delete(param))

    return params.toString()
  }

  const reportParams = getReportParams()

  const [testCapabilities, setTestCapabilities] = React.useState([])
  function fetchCapabilities() {
    fetch(process.env.REACT_APP_API_URL + '/api/tests/capabilities')
      .then((testCapabilities) => {
        if (testCapabilities.status !== 200) {
          throw new Error('server returned ' + testCapabilities.status)
        }
        return testCapabilities.json()
      })
      .then((testCapabilities) => {
        setTestCapabilities(testCapabilities)
      })
      .catch((error) => {
        setFetchError('could not retrieve data:' + error)
      })
  }

  const [testLifecycles, setTestLifecycles] = React.useState([])
  function fetchLifecycles() {
    fetch(process.env.REACT_APP_API_URL + '/api/tests/lifecycles')
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((lifecycles) => {
        // Ensure we always set an array
        setTestLifecycles(Array.isArray(lifecycles) ? lifecycles : [])
      })
      .catch((error) => {
        // Don't fail the whole page for lifecycle fetch errors, just log it
        console.error('could not retrieve lifecycles:', error)
        setTestLifecycles([])
      })
  }

  useEffect(() => {
    fetchCapabilities()
    fetchLifecycles()
    setIsLoaded(false)
    if (
      location.pathname.endsWith('/component_readiness/main') ||
      location.pathname === '/component_readiness'
    ) {
      fetchData()
    } else {
      setIsLoaded(true)
    }
    setTriageActionTaken(false)
  }, [triageActionTaken, location.pathname, reportParams])

  if (!isLoaded) {
    return <CompReadyProgress apiLink={showValuesForReport()} />
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
      <TestCapabilitiesContext.Provider value={testCapabilities}>
        <TestLifecyclesContext.Provider value={testLifecycles}>
          <Fragment>
            <Grid
              container
              justifyContent="center"
              size="xl"
              className="cr-view"
            ></Grid>
            {isMainRoute && <WarningsBanner warnings={warnings} />}
            {/* eslint-disable react/prop-types */}
            <Routes>
              <Route index element={<Navigate to="main" replace />} />
              <Route
                path="help"
                element={<ComponentReadinessHelp key="cr-help" />}
              />
              <Route
                path="test_details"
                element={
                  <TestDetailsReport
                    key="testreport"
                    filterVals={getUpdatedUrlParts(varsContext)}
                    component={varsContext.component}
                    capability={varsContext.capability}
                    environment={varsContext.environment}
                    testId={testId}
                    testName={testName}
                    testBasisRelease={testBasisRelease}
                  />
                }
              />
              <Route
                path="test"
                element={
                  <CompReadyEnvCapabilityTest
                    key="capabilitytest"
                    filterVals={getUpdatedUrlParts(varsContext)}
                    component={varsContext.component}
                    capability={varsContext.capability}
                    testId={testId}
                  />
                }
              />
              <Route
                path="env_test"
                element={
                  <CompReadyEnvCapabilityTest
                    key="capabilitytest"
                    filterVals={getUpdatedUrlParts(varsContext)}
                    component={varsContext.component}
                    capability={varsContext.capability}
                    testId={testId}
                    environment={varsContext.environment}
                    theme={theme}
                  />
                }
              />
              <Route
                path="capability"
                element={
                  <CompReadyEnvCapability
                    key="capabilities"
                    filterVals={getUpdatedUrlParts(varsContext)}
                    component={varsContext.component}
                    capability={varsContext.capability}
                    theme={theme}
                  />
                }
              />
              <Route
                path="env_capability"
                element={
                  <CompReadyEnvCapability
                    key="capabilities"
                    filterVals={getUpdatedUrlParts(varsContext)}
                    component={varsContext.component}
                    capability={varsContext.capability}
                    environment={varsContext.environment}
                    theme={theme}
                  />
                }
              />
              <Route
                path="capabilities"
                element={
                  <CompReadyEnvCapabilities
                    filterVals={getUpdatedUrlParts(varsContext)}
                    component={varsContext.component}
                    theme={theme}
                  />
                }
              />
              <Route
                path="env_capabilities"
                element={
                  <CompReadyEnvCapabilities
                    filterVals={getUpdatedUrlParts(varsContext)}
                    component={varsContext.component}
                    environment={varsContext.environment}
                    theme={theme}
                  />
                }
              />
              <Route path="/triages/:triageId" element={<TriageWrapper />} />
              <Route path="/triages" element={<TriageList />} />
              <Route
                path="main"
                element={
                  <div className="cr-view">
                    <Sidebar
                      controlsOpts={{
                        filterByCapabilities: true,
                        filterByLifecycles: true,
                      }}
                    />
                    <CompReadyPageTitle
                      pageTitle={pageTitle}
                      apiCallStr={showValuesForReport()}
                    />
                    {data === initialPageTable ? (
                      <Typography variant="h6" style={{ textAlign: 'left' }}>
                        To get started, make your filter selections on the left,
                        left, then click Generate Report
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
                          filterVals={getUpdatedUrlParts(varsContext)}
                          setTriageActionTaken={setTriageActionTaken}
                          forceRefresh={forceRefresh}
                        />
                        <TableContainer
                          component="div"
                          className="cr-table-wrapper"
                        >
                          <Table className="cr-comp-read-table">
                            <TableHead>
                              <TableRow>
                                <TableCell className={classes.crColResultFull}>
                                  <Typography className={classes.crCellName}>
                                    Name
                                  </Typography>
                                </TableCell>
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
                                              'Single row report for ' + column
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
                              {data.rows
                                ? Object.keys(data.rows)
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
                                        ? data.rows[
                                            componentIndex
                                          ].columns.some(
                                            // Filter for rows where any of their columns have status <= -2 and accepted by the regex.
                                            (column) =>
                                              column.status <= -2 &&
                                              formColumnName(column).match(
                                                new RegExp(
                                                  escapeRegex(
                                                    searchColumnRegex
                                                  ),
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
                                            ) &&
                                            keepColumnsList &&
                                            keepColumnsList[idx]
                                        )}
                                        columnNames={columnNames.filter(
                                          (column, idx) =>
                                            column.match(
                                              new RegExp(
                                                escapeRegex(searchColumnRegex),
                                                'i'
                                              )
                                            ) &&
                                            keepColumnsList &&
                                            keepColumnsList[idx]
                                        )}
                                        grayFactor={redOnlyChecked ? 100 : 0}
                                        filterVals={getUpdatedUrlParts(
                                          varsContext
                                        )}
                                      />
                                    ))
                                : null}
                            </TableBody>
                          </Table>
                        </TableContainer>
                        <GeneratedAt time={data.generated_at} />
                        <CopyPageURL apiCallStr={showValuesForReport()} />
                      </div>
                    )}
                  </div>
                }
              />
            </Routes>
          </Fragment>
        </TestLifecyclesContext.Provider>
      </TestCapabilitiesContext.Provider>
    </ComponentReadinessStyleContext.Provider>
  )
}
