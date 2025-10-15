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
  getAPIUrl,
  getColumns,
  getStatusAndIcon,
  gotFetchError,
  makePageTitle,
  makeRFC3339Time,
  noDataTable,
} from './CompReadyUtils'
import { CapabilitiesContext, ReleasesContext } from '../App'
import { CompReadyVarsContext } from './CompReadyVars'
import { FileCopy, Help } from '@mui/icons-material'
import { Link } from 'react-router-dom'
import { pathForExactTestAnalysisWithFilter } from '../helpers'
import { Tooltip } from '@mui/material'
import { usePageContextForChat } from '../chat/store/useChatStore'
import AskSippyButton from '../chat/AskSippyButton'
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

// Map status code to description based on the status codes used in getStatusAndIcon
const getStatusDescription = (statusCode) => {
  const statusMap = {
    '-1000': 'Failed fix detected',
    '-500': 'ExtremeRegression detected (>15% pass rate change)',
    '-400': 'SignificantRegression detected',
    '-300': 'ExtremeTriagedRegression detected (>15% pass rate change)',
    '-200': 'SignificantTriagedRegression detected',
    '-150': 'Fixed (hopefully) regression detected',
    '-100': 'Missing Sample (sample data missing)',
    0: 'NoSignificantDifference detected',
    100: 'Missing Basis (basis data missing)',
    200: 'Missing Basis And Sample (basis and sample data missing)',
    300: 'SignificantImprovement detected (improved sample rate)',
  }

  return statusMap[statusCode.toString()] || `Unknown Status (${statusCode})`
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
  const { accessibilityModeOn } = useContext(AccessibilityModeContext)
  const { setPageContextForChat, unsetPageContextForChat } =
    usePageContextForChat()

  const [activeTabIndex, setActiveTabIndex] = React.useState(0)

  const handleTabChange = (event, newValue) => {
    setActiveTabIndex(newValue)
  }

  const { component, capability, testId, environment } = props

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [data, setData] = React.useState({})
  const [regressionId, setRegressionId] = React.useState(0)
  const [versions, setVersions] = React.useState({})
  const [triageEntries, setTriageEntries] = React.useState([])
  const releases = useContext(ReleasesContext)
  const hasSetContextRef = React.useRef(false)

  // Set the browser tab title
  document.title =
    'Sippy > Component Readiness > Capabilities > Tests > Capability Tests > Test Details' +
    (environment ? `Env` : '')

  const capabilitiesContext = React.useContext(CapabilitiesContext)
  const writeEndpointsEnabled = capabilitiesContext.includes('write_endpoints')
  const { sampleRelease, urlParams } = useContext(CompReadyVarsContext)

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

  // This is the inverse of the hack in CompReadyUtils.generateTestDetailsReportLink.
  // We are assuming the API query params are identical to the UI query params, but we have to adjust the host port and prefix from
  // http://localhost:3000/sippy-ng/ to http://localhost:8080/api/
  // This hack allows us to keep the param generation logic in one place. (server side)
  const currentUrl = window.location.href
  const sippyNgIndex = currentUrl.indexOf('/sippy-ng/')
  let testDetailsApiCall
  if (sippyNgIndex !== -1) {
    const pathAfterSippyNg = currentUrl.substring(sippyNgIndex + 10) // +10 to skip '/sippy-ng/'
    // We have to format the url to RFC3339Time in case the date picker has been used to update report params
    testDetailsApiCall = makeRFC3339Time(getAPIUrl(pathAfterSippyNg))
  } else {
    console.error('No /sippy-ng/ found in URL, this is a bug')
  }

  useEffect(() => {
    setIsLoaded(false)
    setHasBeenTriaged(false)
    if (!testId) return // wait until the vars are initialized from params

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
  }, [hasBeenTriaged, urlParams, testId])

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

  // Update page context for chat
  useEffect(() => {
    if (
      !isLoaded ||
      !data.analyses ||
      !data.analyses[0] ||
      hasSetContextRef.current
    )
      return

    hasSetContextRef.current = true

    const firstAnalysis = data.analyses[0]

    // Helper to format date range
    const formatDateRange = (stats) => {
      if (!stats || !stats.Start || !stats.End) return null
      try {
        const start = new Date(stats.Start)
        const end = new Date(stats.End)
        const duration = Math.floor(
          (end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24)
        )
        return {
          start: start.toISOString().split('T')[0],
          end: end.toISOString().split('T')[0],
          duration_days: duration,
        }
      } catch {
        return null
      }
    }

    // Collect sample job failure IDs (limit to 10 for context management)
    const failedJobRunIds = []
    if (firstAnalysis.job_stats) {
      for (const jobStat of firstAnalysis.job_stats) {
        if (
          jobStat.sample_job_run_stats &&
          Array.isArray(jobStat.sample_job_run_stats)
        ) {
          for (const sampleJobRun of jobStat.sample_job_run_stats) {
            if (
              sampleJobRun.test_stats &&
              sampleJobRun.test_stats.failure_count > 0 &&
              sampleJobRun.job_run_id
            ) {
              failedJobRunIds.push(sampleJobRun.job_run_id)
              if (failedJobRunIds.length >= 10) break
            }
          }
          if (failedJobRunIds.length >= 10) break
        }
      }
    }

    const sampleDateRange = formatDateRange(firstAnalysis.sample_stats)
    const baseDateRange = formatDateRange(firstAnalysis.base_stats)

    const contextData = {
      page: 'component-readiness-test-details',
      url: window.location.href,
      instructions: `You are viewing a Component Readiness test regression analysis.

**How to analyze test regressions:**

1. **Summarize the regression status:**
   - Report when it was opened (regression.opened) - format as human-readable date (e.g., "January 15, 2024") and include how many days ago this was. 
     Do not include the actual time.
   - Report if it's closed (regression.closed) or still ongoing - if closed, format the date similarly and show days ago
   - Explain the status code (e.g., -400 = SignificantRegression, -500 = ExtremeRegression >15% change)

2. **Compare sample vs base statistics:**
   - Calculate the pass rate change: (sample_stats.success_rate - base_stats.success_rate) * 100
   - Note the time periods being compared (check date_range for each) - display date ranges in human-readable format with days ago context
   - Identify if this is a recent degradation or long-term issue
   - Note if the BaseRelease appears to be more than one minor version ahead of the sample release.
     This would indicate we found a better pass rate in earlier releases, so we compared against
     that release instead of the one prior to prevent a test gradually getting worse.
   - Check if we see base_stats.flake_count > 0 and base_stats.failure_count close to 0,
     but the sample_stats.failure_count > 0 and sample_stats.flake_count = 0.
     This may indicate a test that had it's ability to flake removed. (a dangerous policy we're slowly working to remove)
     Note the flake rate in the base_stats compared to the fail rate in the sample_stats for the user,
     and see if they are comparable so we can know if the test is actually getting significantly worse.

3. **Investigate the root cause:**
   - Use the sample_failed_job_runs.sample_ids to dig into specific failures
   - Call get_prow_job_summary **in parallel** for multiple job run IDs to understand failure patterns
   - Look for common failure reasons across the job runs
   - Check if failures are consistent or intermittent

4. **Check for triages:**
   - If triages_count > 0, the regression has been attributed to a known Jira issue
   - Note whether the triage has a resolved timestamp
   - If there are failures after the resolved timestamp, this indicates a failed fix

5. **Determine if tests are failing for the same reasons:**
   - Look for patterns in the set of tests that fail in each job run.
     If the same tests are failing in each job, this indicates they may all be related to the same failure.
     If a job suffers from mass failures (more than 10 failed tests), this may indicate a systemic
     problem in the cluster and perhaps the test is not at fault.
     If the test is the only failure in each run, this more likely indicates a problem with this
     specific test or the feature it is testing.
   - Analyze the test failure outputs from multiple failed job runs using get_prow_job_summary
   - Compare error messages, stack traces, and failure patterns
   - Report whether it's a consistent failure (same root cause) or multiple different issues
   
6. **Remind the user about the importance of regressions:**
   - Regressions represent the line of quality we're willing to ship in the product or not.
   - We treat regressions as release blockers for this reason.
   - We will not ship a release with a regression unless the relevant team is willing to submit
     an SBAR to the leadership team and BU for approval.`,
      suggestedQuestions: [
        'Why is this test regressed?',
        'Show me sample outputs from test failures',
        'What other tests are failing together?',
      ],
      data: {
        test_id: testId,
        test_name: data.test_name,
        component: component,
        capability: capability,
        environment: environment,
        regression: firstAnalysis.regression
          ? {
              id: firstAnalysis.regression.id,
              opened: firstAnalysis.regression.opened,
              closed:
                firstAnalysis.regression.closed &&
                firstAnalysis.regression.closed.valid
                  ? firstAnalysis.regression.closed.time
                  : null,
            }
          : null,
        status: {
          code: firstAnalysis.status,
          description: getStatusDescription(firstAnalysis.status),
        },
        explanations: firstAnalysis.explanations || [],
        sample_stats: firstAnalysis.sample_stats
          ? {
              release: firstAnalysis.sample_stats.release,
              success_count: firstAnalysis.sample_stats.success_count,
              failure_count: firstAnalysis.sample_stats.failure_count,
              flake_count: firstAnalysis.sample_stats.flake_count,
              success_rate: firstAnalysis.sample_stats.success_rate,
              date_range: sampleDateRange,
            }
          : null,
        base_stats: firstAnalysis.base_stats
          ? {
              release: firstAnalysis.base_stats.release,
              success_count: firstAnalysis.base_stats.success_count,
              failure_count: firstAnalysis.base_stats.failure_count,
              flake_count: firstAnalysis.base_stats.flake_count,
              success_rate: firstAnalysis.base_stats.success_rate,
              date_range: baseDateRange,
            }
          : null,
        sample_failed_job_runs: {
          count: failedJobRunIds.length,
          sample_ids: failedJobRunIds,
          note:
            failedJobRunIds.length >= 10
              ? 'Limited to 10 sample job run IDs'
              : null,
        },
        triages_count: triageEntries.length,
      },
    }

    setPageContextForChat(contextData)

    // Cleanup: Clear context when component unmounts
    return () => {
      unsetPageContextForChat()
    }
  }, [
    isLoaded,
    data,
    testId,
    component,
    capability,
    environment,
    triageEntries.length,
    setPageContextForChat,
    unsetPageContextForChat,
  ])

  if (fetchError !== '') {
    return gotFetchError(fetchError)
  }

  const pageTitle = makePageTitle(
    'Test Details Report',
    environment ? 'page 5a' : 'page 5',
    `component: ${component}`,
    `capability: ${capability}`,
    `testId: ${testId}`,
    `testName: ${data.test_name}`,
    `environment: ${environment}`
  )

  if (!isLoaded) {
    return <CompReadyProgress apiLink={testDetailsApiCall} />
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

{code:none}${data.test_name}{code}

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
      testName: data.test_name,
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
          jiraComponentName={data.jira_component}
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
        alignItems="center"
        gap={1}
        width="100%"
      >
        <AskSippyButton
          question="Please summarize this test details report."
          tooltip="Ask Sippy AI to summarize this test report"
        />
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
          &gt; {data.test_name}
        </Link>
      </h3>
      <div align="center" style={{ marginTop: 50 }}>
        <h2>{data.test_name}</h2>
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
            testName={data.test_name}
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
              to={pathForExactTestAnalysisWithFilter(
                sampleRelease,
                data.test_name,
                {
                  items: [],
                }
              )}
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
              testName={data.test_name}
              environment={environment}
              component={component}
            />
          </TestsReportTabPanel>
          <TestsReportTabPanel activeIndex={activeTabIndex} index={1}>
            <CompReadyTestPanel
              data={data.analyses[1]}
              versions={versions}
              loadedParams={loadedParams}
              testName={data.test_name}
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
          testName={data.test_name}
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
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  environment: PropTypes.string.isRequired,
}
