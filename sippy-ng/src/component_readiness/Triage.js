import { Box, Button, Tooltip } from '@mui/material'
import { CapabilitiesContext } from '../App'
import { CheckCircle, Error as ErrorIcon } from '@mui/icons-material'
import { CompReadyVarsContext } from './CompReadyVars'
import { formatDateToSeconds, relativeTime } from '../helpers'
import {
  getTriagesAPIUrl,
  hasFailedFixRegression,
  jiraUrlPrefix,
} from './CompReadyUtils'
import { useGlobalChat } from '../chat/useGlobalChat'
import { useTheme } from '@mui/material/styles'
import AskSippyButton from '../chat/AskSippyButton'
import CompSeverityIcon from './CompSeverityIcon'
import LaunderedLink from '../components/Laundry'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'
import TriageAuditLogsModal from './TriageAuditLogsModal'
import TriagedRegressionTestList from './TriagedRegressionTestList'
import TriagePotentialMatches from './TriagePotentialMatches'
import UpsertTriageModal from './UpsertTriageModal'

export default function Triage({ id }) {
  const theme = useTheme()
  const { updatePageContext } = useGlobalChat()
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [triage, setTriage] = React.useState({})
  const [message, setMessage] = React.useState('')
  const [isUpdated, setIsUpdated] = React.useState(false)
  const capabilitiesContext = React.useContext(CapabilitiesContext)
  const triageEnabled = capabilitiesContext.includes('write_endpoints')
  const localDBEnabled = capabilitiesContext.includes('local_db')
  // The view is needed in order to formulate test_details links on the frontend, this is still necessary as they are not
  // always included in the server response. If the triage has regressions from multiple views included on it, and the
  // page is loaded from the context of one of those component_reports, then the regressions from the other view will not
  // load properly. This is a better result than not being able to formulate any URLs when links aren't provided.
  // TODO(sgoeddel): Make it so links are *always* provided, and remove this (https://issues.redhat.com/browse/TRT-2356)
  const { view } = useContext(CompReadyVarsContext)

  React.useEffect(() => {
    setIsLoaded(false)
    setIsUpdated(false)

    let triageFetch
    // triage entries will only be available when there is a postgres connection
    if (localDBEnabled) {
      triageFetch = fetch(`${getTriagesAPIUrl(id)}?expand=regressions`).then(
        (response) => {
          if (response.status !== 200) {
            throw new Error('API server returned ' + response.status)
          }
          return response.json()
        }
      )
    } else {
      triageFetch = Promise.resolve({})
    }

    triageFetch
      .then((t) => {
        setTriage(t)
        setIsLoaded(true)
        document.title = 'Triage "' + t.description + '" (' + t.id + ')'
      })
      .catch((error) => {
        setMessage(error.toString())
      })
  }, [isUpdated, localDBEnabled, id])

  // Update page context for chat
  React.useEffect(() => {
    if (!isLoaded || !triage.id) return

    // Generate test details links for regressed tests
    const regressedTestsWithLinks = []
    if (triage.regressed_tests && triage.regressed_tests.length > 0) {
      triage.regressed_tests.forEach((regressedTest) => {
        regressedTestsWithLinks.push({
          test_name: regressedTest.test_name,
          component: regressedTest.component,
          capability: regressedTest.capability,
          environment: regressedTest.environment,
          test_id: regressedTest.test_id,
          status: regressedTest.status,
          explanations: regressedTest.explanations || [],
          test_details_api_url: regressedTest.links?.test_details || null,
          regression_id: regressedTest.regression?.id,
          regression_opened: regressedTest.regression?.opened,
          regression_closed: regressedTest.regression?.closed?.valid
            ? regressedTest.regression.closed.time
            : null,
        })
      })
    }

    const contextData = {
      page: 'triage-details',
      url: window.location.href,
      instructions: `You are viewing a triage record that links test regressions to Jira issues.

**Choose the appropriate analysis based on what the user is asking:**

**For questions about test failures, patterns, or technical analysis:**
- Use the get_test_details_report tool with each test's test_details_api_url as the query_params parameter
- Use get_prow_job_summary with the failed_job_run_ids to analyze specific job failures
- Compare failure patterns across tests to identify common causes
- Compare up to 10 failed job run IDs to analyze specific job failures, choosing a sampling from each test_details report
- Look for consistency in failures (same root cause vs varied issues)
- Determine if regressions are related or independent

**For questions about fix status, timeline, or Jira issue:**
- Use the get_jira_issue_analysis tool with the issue_key from the jira data
- Review the recent comments to assess fix readiness and timeline
- Look for indicators of fix completion, testing status, deployment readiness, or blockers
- Pay attention to the most recent comments as they reflect current status
- Check the issue status, assignee, and fix versions for additional context

**For questions about potential matches or which additional tests to add:**
- ALWAYS follow this process to determine match likelihood:
  1. Use get_triage_potential_matches with triage_id and view to get candidate tests sorted by confidence
  2. From the existing triaged tests (in regressed_tests), select 2-4 representative tests that provide a fair sampling of the set (consider different components, test types, or failure patterns if diverse; otherwise just pick the first few). Use get_test_details_report with their test_details_api_url to get failed_job_run_ids
  3. For the top 2-4 potential matches, use get_test_details_report with regressed_test.links.test_details as the query_params to get failed_job_run_ids
  4. Compare the failed_job_run_ids: calculate how many job runs each potential match shares with the sampled existing triaged tests
  5. A potential match with high overlap (many common job runs) is very likely to have the same root cause
- Tests failing in the same job runs are strong indicators of related failures
- Prioritize recommendations by: (high API confidence) AND (high job run overlap with existing tests)
- Present results showing which specific triaged tests each potential match shares job runs with, making sure to include the regression_id (regressed_test.regression.id) for each test

**Do not perform all analyses unless the user specifically asks for each of them. Focus on what they're actually asking about.**

**Do not summarize triage metadata** (resolution status, dates, etc.) as this information is already displayed on the page.`,
      suggestedQuestions: [
        'What are the common failure patterns across these regressed tests?',
        'What is the current status of the fix in the Jira issue?',
        'Are there any other regressions that should be added to this triage?',
      ],
      data: {
        triage_id: triage.id,
        view: view,
        description: triage.description,
        type: triage.type,
        resolved: triage.resolved?.Valid
          ? {
              time: triage.resolved.Time,
              reason: triage.resolution_reason,
            }
          : null,
        created_at: triage.created_at,
        updated_at: triage.updated_at,
        jira: {
          issue_key: extractJiraIssueKey(triage.url),
          status: triage.bug?.status,
          target_versions: triage.bug?.target_versions,
          affects_versions: triage.bug?.affects_versions,
          release_blocker: triage.bug?.release_blocker,
          last_change_time: triage.bug?.last_change_time,
        },
        regressed_tests: regressedTestsWithLinks,
        regressed_tests_count: triage.regressed_tests?.length || 0,
        regressions_count: triage.regressions?.length || 0,
        has_failed_fix: hasFailedFixRegression(triage, triage.regressed_tests),
      },
    }

    updatePageContext(contextData)

    // Cleanup: Clear context when component unmounts
    return () => {
      updatePageContext(null)
    }
  }, [isLoaded, triage, updatePageContext])

  const deleteTriage = () => {
    const confirmed = window.confirm(
      'Are you sure you want to delete this triage record?'
    )
    if (confirmed) {
      fetch(triage.links.self, {
        method: 'DELETE',
      })
        .then((response) => {
          if (response.status !== 200) {
            throw new Error('API server returned ' + response.status)
          }

          setMessage('Triage record has been deleted.')
        })
        .catch((error) => {
          setMessage(error.toString())
        })
    }
  }

  const extractJiraIssueKey = (url) => {
    if (!url) return null
    return url.startsWith(jiraUrlPrefix) ? url.slice(jiraUrlPrefix.length) : url
  }

  if (message !== '') {
    return <h2>{message}</h2>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    <Fragment>
      <Box
        display="flex"
        justifyContent="space-between"
        alignItems="center"
        mb={2}
      >
        <h2 style={{ margin: 0 }}>Triage Details</h2>
        <Box display="flex" alignItems="center" gap={1}>
          <AskSippyButton
            question="What is the primary cause of these test failures?"
            tooltip="Ask Sippy AI to analyze this triage record's test failures"
          />
          {localDBEnabled && <TriageAuditLogsModal triage={triage} />}
          {triageEnabled && (
            <Fragment>
              <UpsertTriageModal
                triage={triage}
                buttonText={'Update'}
                setComplete={setIsUpdated}
              />
              <Button
                onClick={deleteTriage}
                variant="contained"
                color="secondary"
                sx={{ marginLeft: '10px' }}
              >
                Delete
              </Button>
            </Fragment>
          )}
        </Box>
      </Box>
      <Table>
        <TableBody>
          <TableRow>
            <TableCell>Resolved</TableCell>
            <TableCell>
              {triage.resolved?.Valid ? (
                <Tooltip
                  title={`${relativeTime(
                    new Date(triage.resolved.Time),
                    new Date()
                  )} (${formatDateToSeconds(triage.resolved.Time)})`}
                >
                  {hasFailedFixRegression(triage, triage.regressed_tests) ? (
                    <CompSeverityIcon status={-1000} />
                  ) : (
                    <CheckCircle
                      style={{ color: theme.palette.success.light }}
                    />
                  )}
                </Tooltip>
              ) : (
                <Tooltip title="Not resolved">
                  <ErrorIcon style={{ color: theme.palette.error.light }} />
                </Tooltip>
              )}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Description</TableCell>
            <TableCell>{triage.description}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Type</TableCell>
            <TableCell>{triage.type}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Record Created</TableCell>
            <TableCell>
              {triage.created_at ? (
                <Tooltip
                  title={relativeTime(new Date(triage.created_at), new Date())}
                >
                  <span>{formatDateToSeconds(triage.created_at)}</span>
                </Tooltip>
              ) : (
                ''
              )}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Record Updated</TableCell>
            <TableCell>
              {triage.updated_at ? (
                <Tooltip
                  title={relativeTime(new Date(triage.updated_at), new Date())}
                >
                  <span>{formatDateToSeconds(triage.updated_at)}</span>
                </Tooltip>
              ) : (
                ''
              )}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Resolution Date</TableCell>
            <TableCell>
              {triage.resolved?.Valid ? (
                <Tooltip
                  title={relativeTime(
                    new Date(triage.resolved.Time),
                    new Date()
                  )}
                >
                  <span>{formatDateToSeconds(triage.resolved.Time)}</span>
                </Tooltip>
              ) : (
                ''
              )}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Resolution Reason</TableCell>
            <TableCell>{triage.resolution_reason}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Jira</TableCell>
            <TableCell>
              <LaunderedLink address={triage.url}>
                {extractJiraIssueKey(triage.url)}
              </LaunderedLink>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Jira State</TableCell>
            <TableCell>{triage.bug?.status}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Jira Version</TableCell>
            <TableCell>
              {triage.bug?.target_versions || triage.bug?.affects_versions}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Jira Release Blocker</TableCell>
            <TableCell>{triage.bug?.release_blocker}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Jira updated</TableCell>
            <TableCell>
              {triage.bug?.last_change_time ? (
                <Tooltip
                  title={relativeTime(
                    new Date(triage.bug.last_change_time),
                    new Date()
                  )}
                >
                  <span>
                    {formatDateToSeconds(triage.bug.last_change_time)}
                  </span>
                </Tooltip>
              ) : (
                ''
              )}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <h2>Included Tests</h2>
      <TriagedRegressionTestList
        allRegressedTests={triage.regressed_tests}
        regressions={triage.regressions}
        filterVals={`?view=${view}`}
      />
      {triageEnabled && (
        <TriagePotentialMatches
          triage={triage}
          setMessage={setMessage}
          setLinkingComplete={setIsUpdated}
        />
      )}
    </Fragment>
  )
}

Triage.propTypes = {
  id: PropTypes.string.isRequired,
}
