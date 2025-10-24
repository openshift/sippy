import { Box, Button, Tooltip } from '@mui/material'
import { CheckCircle, Error as ErrorIcon } from '@mui/icons-material'
import { CompReadyVarsContext } from './CompReadyVars'
import { formatDateToSeconds, relativeTime } from '../helpers'
import {
  getTriagesAPIUrl,
  hasFailedFixRegression,
  jiraUrlPrefix,
} from './CompReadyUtils'
import { SippyCapabilitiesContext } from '../App'
import { usePageContextForChat } from '../chat/store/useChatStore'
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
  const { setPageContextForChat, unsetPageContextForChat } =
    usePageContextForChat()
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [triage, setTriage] = React.useState({})
  const [message, setMessage] = React.useState('')
  const [isUpdated, setIsUpdated] = React.useState(false)
  const capabilitiesContext = React.useContext(SippyCapabilitiesContext)
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
      suggestions: [
        {
          prompt: 'triage-failure-analysis',
          label: 'Analyze failure patterns across regressed tests',
          args: {
            triage_id: triage.id,
            view: view,
          },
        },
        {
          prompt: 'triage-fix-status',
          label: 'Check Jira fix status and timeline',
          args: {
            issue_key: extractJiraIssueKey(triage.url),
          },
        },
        {
          prompt: 'triage-potential-matches',
          label: 'Find potential test matches to add',
          args: {
            triage_id: triage.id,
            view: view,
          },
        },
      ],
      data: {
        triage_id: triage.id,
        view: view,
        jira_issue_key: extractJiraIssueKey(triage.url),
        regressed_tests: regressedTestsWithLinks,
        has_failed_fix: hasFailedFixRegression(triage, triage.regressed_tests),
      },
    }

    setPageContextForChat(contextData)

    // Cleanup: Clear context when component unmounts
    return () => {
      unsetPageContextForChat()
    }
  }, [isLoaded, triage, setPageContextForChat, unsetPageContextForChat])

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
            slashCommand="triage-failure-analysis"
            commandArgs={{ triage_id: triage.id, view: view }}
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
