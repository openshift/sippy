import './ComponentReadiness.css'
import { AccessibilityModeContext } from '../components/AccessibilityModeProvider'
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
  getColumns,
  getStatusAndIcon,
  getTestDetailsAPIUrl,
  gotFetchError,
  makePageTitle,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { CapabilitiesContext, ReleasesContext } from '../App'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
import { FileCopy, Help } from '@mui/icons-material'
import { Link } from 'react-router-dom'
import {
  pathForExactTestAnalysisWithFilter,
  safeEncodeURIComponent,
} from '../helpers'
import { Tooltip } from '@mui/material'
import BugButton from '../bugs/BugButton'
import BugTable from '../bugs/BugTable'
import CompReadyCancelled from './CompReadyCancelled'
import CompReadyPageTitle from './CompReadyPageTitle'
import CompReadyProgress from './CompReadyProgress'
import CompReadyTestPanel from './CompReadyTestPanel'
import CopyPageURL from './CopyPageURL'
import FileBug from '../bugs/FileBug'
import GeneratedAt from './GeneratedAt'
import IconButton from '@mui/material/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment, useContext, useEffect } from 'react'
import Sidebar from './Sidebar'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'
import TriagedTestsPanel from './TriagedTestsPanel'
import UpsertTriageModal from './UpsertTriageModal'

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
export default function TestDetailsReport(props) {
  const classes = useContext(ComponentReadinessStyleContext)
  const { accessibilityModeOn } = useContext(AccessibilityModeContext)

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
  } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [regressionId, setRegressionId] = React.useState(0)
  const [versions, setVersions] = React.useState({})
  const [triageEntries, setTriageEntries] = React.useState([])
  const releases = useContext(ReleasesContext)

  // Set the browser tab title
  document.title =
    'Sippy > Component Readiness > Capabilities > Tests > Capability Tests > Test Details' +
    (environment ? `Env` : '')
  const safeComponent = safeEncodeURIComponent(component)
  const safeCapability = safeEncodeURIComponent(capability)
  const safeTestId = safeEncodeURIComponent(testId)
  const safeTestBasisRelease = safeEncodeURIComponent(testBasisRelease)

  const capabilitiesContext = React.useContext(CapabilitiesContext)
  const writeEndpointsEnabled = capabilitiesContext.includes('write_endpoints')
  const { expandEnvironment, sampleRelease } = useContext(CompReadyVarsContext)

  // Helpers for copying the test ID to clipboard
  const [copyPopoverEl, setCopyPopoverEl] = React.useState(null)
  const copyPopoverOpen = Boolean(copyPopoverEl)
  const copyTestID = (event) => {
    event.preventDefault()
    navigator.clipboard.writeText(testId)
    setCopyPopoverEl(event.currentTarget)
    setTimeout(() => setCopyPopoverEl(null), 2000)
  }

  const [hasBeenTriaged, setHasBeenTriaged] = React.useState(false)

  const testDetailsApiCall =
    getTestDetailsAPIUrl() +
    makeRFC3339Time(filterVals) +
    `&component=${safeComponent}` +
    `&capability=${safeCapability}` +
    `&testId=${safeTestId}` +
    `&testBasisRelease=${safeTestBasisRelease}` +
    (environment ? expandEnvironment(environment) : '')

  useEffect(() => {
    setIsLoaded(false)
    setHasBeenTriaged(false)

    // fetch the test_details data followed by any triage records that match the regressionId (if found)
    fetch(testDetailsApiCall, { signal: abortController.signal })
      .then((response) => response.json())
      .then((data) => {
        if (data.code < 200 || data.code >= 300) {
          const errorMessage = data.message
            ? `${data.message}`
            : 'No error message'
          throw new Error(
            `API call failed: ${testDetailsApiCall}\n Return code = ${data.code} (${errorMessage})`
          )
        }
        return data
      })
      .then((json) => {
        // If the basics are not present, consider it no data
        if (
          !json.component ||
          !json.analyses ||
          !json.analyses[0].sample_stats
        ) {
          // The api call returned 200 OK but the data was empty
          setData(noDataTable)
        } else {
          setData(json)
          const regression = json.analyses[0].regression
          if (regression) {
            setRegressionId(regression.id)
            setTriageEntries(regression.triages)
          }
        }
      })
      .catch((error) => {
        if (error.name === 'AbortError') {
          setData(cancelledDataTable)

          // Once this fired, we need a new one for the next button click.
          abortController = new AbortController()
        } else {
          setFetchError(error)
        }
      })
      .finally(() => {
        setIsLoaded(true)
      })
  }, [hasBeenTriaged])

  useEffect(() => {
    let tmpRelease = {}
    releases.releases
      .filter((aVersion) => {
        return !releases.release_attrs[aVersion].capabilities.componentReadiness
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
    return (
      <CompReadyProgress
        apiLink={testDetailsApiCall}
        cancelFunc={cancelFetch}
      />
    )
  }

  const columnNames = getColumns(data)
  if (columnNames[0] === 'Cancelled' || columnNames[0] === 'None') {
    return (
      <CompReadyCancelled
        message={columnNames[0]}
        apiCallStr={testDetailsApiCall}
      />
    )
  }

  let status = data.analyses[0].status
  const [statusStr, assessmentIcon] = getStatusAndIcon(
    status,
    0,
    accessibilityModeOn
  )
  const significanceTitle = `Test results for individual Prow Jobs may not be statistically
  significant, but when taken in aggregate, there may be a statistically
  significant difference compared to the historical basis
  `

  let url
  if (testDetailsApiCall.startsWith('/')) {
    // In production mode, there is no hostname so we add it so that 'new URL' will work
    // for both production and development modes.
    url = new URL('http://sippy.dptools.openshift.org' + testDetailsApiCall)
  } else {
    url = new URL(testDetailsApiCall)
  }

  const params = new URLSearchParams(url.search)
  const baseRelease = params.get('baseRelease')

  let isBaseOverride = false
  let baseReleaseTabLabel = baseRelease + ' Basis'
  let overrideReleaseTabLabel = ''
  if (data && data.analyses && data.analyses.length > 1) {
    isBaseOverride = true
    overrideReleaseTabLabel = baseReleaseTabLabel
    baseReleaseTabLabel = data.analyses[0].base_stats.release + ' Basis'
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

  const getBugFilingComponent = () => {
    const hasBaseStats = data.analyses[0].base_stats
    const contextWithStats = `
(_Feel free to update this bug's summary to be more specific._)
Component Readiness has found a potential regression in the following test:

{code:none}${testName}{code}

${(data.analyses[0].explanations || []).join('\n')}
${printStatsText(
  'Sample (being evaluated)',
  data.analyses[0].sample_stats,
  data.analyses[0].sample_stats.Start,
  data.analyses[0].sample_stats.End
)}${
      hasBaseStats
        ? printStatsText(
            'Base (historical)',
            data.analyses[0].base_stats,
            data.analyses[0].base_stats.Start,
            data.analyses[0].base_stats.End
          )
        : ''
    }

View the [test details report|${document.location.href}] for additional context.
            `

    const commonProps = {
      testName,
      component,
      capability,
      labels: ['component-regression'],
      context: contextWithStats,
    }

    if (writeEndpointsEnabled) {
      return (
        <FileBug
          {...commonProps}
          regressionId={regressionId}
          version={sampleRelease}
          jiraComponentID={Number(data.jira_component_id)}
          setHasBeenTriaged={setHasBeenTriaged}
        />
      )
    } else {
      return (
        <BugButton
          {...commonProps}
          jiraComponentID={String(data.jira_component_id)}
        />
      )
    }
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
      <CompReadyPageTitle
        pageTitle={pageTitle}
        apiCallStr={testDetailsApiCall}
      />
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
        <Grid item xs={12}>
          {triageEntries.length > 0 && (
            <Fragment>
              <h2>Triages</h2>
              <TriagedTestsPanel triageEntries={triageEntries} />
            </Fragment>
          )}

          {writeEndpointsEnabled && regressionId > 0 && (
            <UpsertTriageModal
              regressionIds={[regressionId]}
              setComplete={setHasBeenTriaged}
              buttonText="Triage"
              submissionDelay={2000}
            />
          )}

          <h2>Bugs Mentioning This Test</h2>
          <BugTable
            testName={testName}
            writeEndpointsEnabled={writeEndpointsEnabled}
            regressionId={regressionId}
            setHasBeenTriaged={setHasBeenTriaged}
          />
          <Box
            sx={{
              display: 'flex',
              marginTop: 2,
              alignItems: 'center',
              gap: 2,
            }}
          >
            {getBugFilingComponent()}
            <Button
              variant="contained"
              color="secondary"
              href="https://issues.redhat.com/issues/?filter=12432468"
            >
              View other open regressions
            </Button>
            <Link
              to={pathForExactTestAnalysisWithFilter(sampleRelease, testName, {
                items: [],
              })}
              style={{ textDecoration: 'none' }}
            >
              <Button variant="contained" color="secondary">
                View Test Analysis
              </Button>
            </Link>
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
          {isBaseOverride && (
            <TableRow>
              <TableCell>{baseRelease} Override:</TableCell>
              <TableCell>Earlier release had a higher threshold</TableCell>
            </TableRow>
          )}
          <TableRow>
            <TableCell>Assessment:</TableCell>
            <TableCell>
              <Tooltip title={statusStr}>{assessmentIcon}</Tooltip>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Explanations:</TableCell>
            <TableCell>
              {(data.analyses[0].explanations || []).join('\n')}
            </TableCell>
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
              data={data.analyses[0]}
              versions={versions}
              loadedParams={loadedParams}
              testName={testName}
              environment={environment}
              component={component}
            />
          </TestsReportTabPanel>
          <TestsReportTabPanel activeIndex={activeTabIndex} index={1}>
            <CompReadyTestPanel
              data={data.analyses[1]}
              versions={versions}
              loadedParams={loadedParams}
              testName={testName}
              environment={environment}
              component={component}
            />
          </TestsReportTabPanel>
        </Fragment>
      ) : (
        <CompReadyTestPanel
          data={data.analyses[0]}
          versions={versions}
          loadedParams={loadedParams}
          testName={testName}
          environment={environment}
          component={component}
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
      <CopyPageURL apiCallStr={testDetailsApiCall} />
    </Fragment>
  )
}

TestDetailsReport.propTypes = {
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  environment: PropTypes.string.isRequired,
  testName: PropTypes.string.isRequired,
  testBasisRelease: PropTypes.string.isRequired,
}
