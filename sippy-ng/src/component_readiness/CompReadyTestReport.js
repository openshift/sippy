import './ComponentReadiness.css'
import {
  Box,
  Button,
  Grid,
  Popover,
  Tab,
  Tabs,
  Typography,
} from '@mui/material'
import {
  cancelledDataTable,
  generateTestReport,
  getColumns,
  getStatusAndIcon,
  getTestDetailsAPIUrl,
  gotFetchError,
  makePageTitle,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
import { FileCopy, Help, InsertLink } from '@mui/icons-material'
import { Link } from 'react-router-dom'
import { ReleasesContext } from '../App'
import { safeEncodeURIComponent } from '../helpers'
import { Tooltip } from '@mui/material'
import BugButton from '../bugs/BugButton'
import BugTable from '../bugs/BugTable'
import CompReadyCancelled from './CompReadyCancelled'
import CompReadyPageTitle from './CompReadyPageTitle'
import CompReadyProgress from './CompReadyProgress'
import CompReadyTestPanel from './CompReadyTestPanel'
import CopyPageURL from './CopyPageURL'
import GeneratedAt from './GeneratedAt'
import IconButton from '@mui/material/IconButton'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { Fragment, useContext, useEffect } from 'react'
import Sidebar from './Sidebar'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'

// Big query requests take a while so give the user the option to
// abort in case they inadvertently requested a huge dataset.
let abortController = new AbortController()
const cancelFetch = () => {
  console.log('Aborting page5a')
  abortController.abort()
}

function tabProps(index) {
  return {
    id: `test-report-tab-${index}`,
    'aria-controls': `test-report-tabpanel-${index}`,
  }
}

TestsReportTabPanel.propTypes = {
  children: PropTypes.node,
  index: PropTypes.number.isRequired,
  activeIndex: PropTypes.number.isRequired,
}

function TestsReportTabPanel(props) {
  const { children, activeIndex, index, ...other } = props

  return (
    <div
      role="tabpanel"
      hidden={activeIndex !== index}
      id={`test-report-tabpanel-${index}`}
      aria-labelledby={`test-report-tab-${index}`}
      {...other}
    >
      {activeIndex === index && (
        <Box sx={{ p: 3 }}>
          <Typography>{children}</Typography>
        </Box>
      )}
    </div>
  )
}

// This component runs when we see /component_readiness/test_details
// This is page 5 which runs when you click a test cell on the right of page 4 or page 4a
export default function CompReadyTestReport(props) {
  const classes = useContext(ComponentReadinessStyleContext)

  const [activeTabIndex, setActiveTabIndex] = React.useState(0)

  const handleTabChange = (event, newValue) => {
    setActiveTabIndex(newValue)
  }

  const {
    filterVals,
    component,
    capability,
    testId,
    environment,
    testName,
    testBasisRelease,
    accessibilityMode,
  } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [versions, setVersions] = React.useState({})
  const releases = useContext(ReleasesContext)

  // Set the browser tab title
  document.title =
    'Sippy > Component Readiness > Capabilities > Tests > Capability Tests > Test Details' +
    (environment ? `Env` : '')
  const safeComponent = safeEncodeURIComponent(component)
  const safeCapability = safeEncodeURIComponent(capability)
  const safeTestId = safeEncodeURIComponent(testId)
  const safeTestBasisRelease = safeEncodeURIComponent(testBasisRelease)

  const { expandEnvironment } = useContext(CompReadyVarsContext)

  // Helpers for copying the test ID to clipboard
  const [copyPopoverEl, setCopyPopoverEl] = React.useState(null)
  const copyPopoverOpen = Boolean(copyPopoverEl)
  const copyTestID = (event) => {
    event.preventDefault()
    navigator.clipboard.writeText(testId)
    setCopyPopoverEl(event.currentTarget)
    setTimeout(() => setCopyPopoverEl(null), 2000)
  }

  const handleCopy = async (event) => {
    try {
      await navigator.clipboard.writeText(testId)
      setAnchorEl(event.currentTarget)
      setTimeout(() => setAnchorEl(null), 1500) // Close popover after 1.5 seconds
    } catch (err) {
      setAnchorEl(event.currentTarget)
      setTimeout(() => setAnchorEl(null), 1500) // Close popover after 1.5 seconds
    }
  }

  const apiCallStr =
    getTestDetailsAPIUrl() +
    makeRFC3339Time(filterVals) +
    `&component=${safeComponent}` +
    `&capability=${safeCapability}` +
    `&testId=${safeTestId}` +
    `&testBasisRelease=${safeTestBasisRelease}` +
    (environment ? expandEnvironment(environment) : '')

  useEffect(() => {
    setIsLoaded(false)

    fetch(apiCallStr, { signal: abortController.signal })
      .then((response) => response.json())
      .then((data) => {
        if (data.code < 200 || data.code >= 300) {
          const errorMessage = data.message
            ? `${data.message}`
            : 'No error message'
          throw new Error(`Return code = ${data.code} (${errorMessage})`)
        }
        return data
      })
      .then((json) => {
        // If the basics are not present, consider it no data
        if (!json.component || !json.sample_stats) {
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
          setFetchError(`API call failed: ${apiCallStr}\n${error}`)
        }
      })
      .finally(() => {
        setIsLoaded(true)
      })
  }, [])

  useEffect(() => {
    let tmpRelease = {}
    releases.releases
      .filter((aVersion) => {
        // We won't process Presubmits or 3.11
        return aVersion !== 'Presubmits' && aVersion != '3.11'
      })
      .forEach((r) => {
        tmpRelease[r] = releases.ga_dates[r]
      })
    setVersions(tmpRelease)
  }, [releases])

  // this backhand way of recording the query dates keeps their display
  // from re-rendering to match the controls until the controls update the report
  const [loadedParams, setLoadedParams] = React.useState({})
  const datesEnv = useContext(CompReadyVarsContext)
  useEffect(() => setLoadedParams(datesEnv), [])

  if (fetchError !== '') {
    return gotFetchError(fetchError)
  }

  const pageTitle = makePageTitle(
    'Test Details Report',
    environment ? 'page 5a' : 'page 5',
    `component: ${component}`,
    `capability: ${capability}`,
    `testId: ${testId}`,
    `testName: ${testName}`,
    `environment: ${environment}`
  )

  if (!isLoaded) {
    return <CompReadyProgress apiLink={apiCallStr} cancelFunc={cancelFetch} />
  }

  const columnNames = getColumns(data)
  if (columnNames[0] === 'Cancelled' || columnNames[0] == 'None') {
    return (
      <CompReadyCancelled message={columnNames[0]} apiCallStr={apiCallStr} />
    )
  }

  const handleFailuresOnlyChange = (event) => {
    setShowOnlyFailures(event.target.checked)
  }

  const [statusStr, assessmentIcon] = getStatusAndIcon(
    data.status,
    0,
    accessibilityMode
  )
  const significanceTitle = `Test results for individual Prow Jobs may not be statistically
  significant, but when taken in aggregate, there may be a statistically
  significant difference compared to the historical basis
  `

  let url
  if (apiCallStr.startsWith('/')) {
    // In production mode, there is no hostname so we add it so that 'new URL' will work
    // for both production and development modes.
    url = new URL('http://sippy.dptools.openshift.org' + apiCallStr)
  } else {
    url = new URL(apiCallStr)
  }

  const params = new URLSearchParams(url.search)
  const baseRelease = params.get('baseRelease')

  let isBaseOverride = false
  let baseReleaseTabLabel = baseRelease + ' Basis'
  let overrideReleaseTabLabel = ''
  if (
    data &&
    data.base_stats &&
    data.base_stats.release &&
    data.base_stats.release !== baseRelease &&
    data.base_override_report
  ) {
    isBaseOverride = true
    overrideReleaseTabLabel = baseReleaseTabLabel
    baseReleaseTabLabel = data.base_stats.release + ' Basis'
  }

  const printStatsText = (statsLabel, stats, from, to) => {
    if (stats === undefined) {
      return `
          Insufficient pass rate`
    }
    return `
${statsLabel} Release: ${stats.release}
Start Time: ${from}
End Time: ${to}
Success Rate: ${(stats.success_rate * 100).toFixed(2)}%
Successes: ${stats.success_count}
Failures: ${stats.failure_count}
Flakes: ${stats.flake_count}`
  }

  return (
    <Fragment>
      <Sidebar isTestDetails={true} />
      <Box
        display="flex"
        justifyContent="right"
        alignItems="right"
        width="100%"
      >
        <Tooltip title="Frequently Asked Questions">
          <Link
            to="/component_readiness/help"
            style={{ textDecoration: 'none' }}
          >
            <IconButton>
              <Help />
            </IconButton>
          </Link>
        </Tooltip>
      </Box>
      <CompReadyPageTitle pageTitle={pageTitle} apiCallStr={apiCallStr} />
      <h3>
        <Link to="/component_readiness">
          / {environment} &gt; {component}
          &gt; {testName}
        </Link>
      </h3>
      <div align="center" style={{ marginTop: 50 }}>
        <h2>{testName}</h2>
      </div>
      <Grid container>
        <Grid>
          <h2>Linked Bugs</h2>
          <BugTable testName={testName} />
          <Box
            sx={{
              display: 'flex',
              marginTop: 2,
              alignItems: 'center',
              gap: 2,
            }}
          >
            {data.base_stats ? (
              <BugButton
                testName={testName}
                component={component}
                capability={capability}
                jiraComponentID={data.jira_component_id}
                labels={['component-regression']}
                context={`
(_Feel free to update this bug's summary to be more specific._)
Component Readiness has found a potential regression in the following test:

{code:none}${testName}{code}

${data.explanations.join('\n')}
${printStatsText(
  'Sample (being evaluated)',
  data.sample_stats,
  data.sample_stats.Start,
  data.sample_stats.End
)}
${printStatsText(
  'Base (historical)',
  data.base_stats,
  data.base_stats.Start,
  data.base_stats.End
)}

View the [test details report|${document.location.href}] for additional context.
            `}
              />
            ) : (
              <BugButton
                testName={testName}
                component={component}
                capability={capability}
                jiraComponentID={data.jira_component_id}
                labels={['component-regression']}
                context={`
(_Feel free to update this bug's summary to be more specific._)
Component Readiness has found a potential regression in the following test:

{code:none}${testName}{code}

${data.explanations.join('\n')}
${printStatsText(
  'Sample (being evaluated)',
  data.sample_stats,
  data.sample_stats.Start,
  data.sample_stats.End
)}

View the [test details report|${document.location.href}] for additional context.
            `}
              />
            )}
            <Button
              variant="contained"
              color="secondary"
              href="https://issues.redhat.com/issues/?filter=12432468"
            >
              View other open regressions
            </Button>
          </Box>
        </Grid>
      </Grid>

      <h2>Regression Report</h2>

      <Table>
        <TableBody>
          <TableRow>
            <TableCell>Test ID:</TableCell>
            <TableCell>
              {testId}
              <IconButton
                aria-label="Copy test ID"
                color="inherit"
                onClick={copyTestID}
              >
                <Tooltip title="Copy test ID">
                  <FileCopy />
                </Tooltip>
              </IconButton>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Environment:</TableCell>
            <TableCell>{environment}</TableCell>
          </TableRow>
          {isBaseOverride ? (
            <TableRow>
              <TableCell>{baseRelease} Override:</TableCell>
              <TableCell>Earlier release had a higher threshold</TableCell>
            </TableRow>
          ) : (
            <Fragment />
          )}
          <TableRow>
            <TableCell>Assessment:</TableCell>
            <TableCell>
              <Tooltip title={statusStr}>{assessmentIcon}</Tooltip>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Explanations:</TableCell>
            <TableCell>{data.explanations.join('\n')}</TableCell>
          </TableRow>
        </TableBody>
      </Table>
      {isBaseOverride ? (
        <Fragment>
          <Tabs
            value={activeTabIndex}
            onChange={handleTabChange}
            aria-label="Test Report Tabs"
          >
            <Tab label={baseReleaseTabLabel} {...tabProps(0)} />
            <Tab label={overrideReleaseTabLabel} {...tabProps(1)} />
          </Tabs>
          <TestsReportTabPanel activeIndex={activeTabIndex} index={0}>
            <CompReadyTestPanel
              data={data}
              versions={versions}
              isOverride={false}
              loadedParams={loadedParams}
            />
          </TestsReportTabPanel>
          <TestsReportTabPanel activeIndex={activeTabIndex} index={1}>
            <CompReadyTestPanel
              data={data.base_override_report}
              versions={versions}
              isOverride={true}
              loadedParams={loadedParams}
            />
          </TestsReportTabPanel>
        </Fragment>
      ) : (
        <CompReadyTestPanel
          data={data}
          versions={versions}
          isOverride={false}
          loadedParams={loadedParams}
        />
      )}
      <Popover
        id="copyPopover"
        open={copyPopoverOpen}
        anchorEl={copyPopoverEl}
        onClose={() => setCopyPopoverEl(null)}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'center',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'center',
        }}
      >
        ID copied!
      </Popover>
      <GeneratedAt time={data.generated_at} />
      <CopyPageURL apiCallStr={apiCallStr} />
    </Fragment>
  )
}

CompReadyTestReport.propTypes = {
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  environment: PropTypes.string.isRequired,
  testName: PropTypes.string.isRequired,
  testBasisRelease: PropTypes.string.isRequired,
  accessibilityMode: PropTypes.bool.isRequired,
}
