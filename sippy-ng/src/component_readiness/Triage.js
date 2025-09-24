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
import { useTheme } from '@mui/material/styles'
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
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [triage, setTriage] = React.useState({})
  const [message, setMessage] = React.useState('')
  const [isUpdated, setIsUpdated] = React.useState(false)
  const capabilitiesContext = React.useContext(CapabilitiesContext)
  const triageEnabled = capabilitiesContext.includes('write_endpoints')
  const localDBEnabled = capabilitiesContext.includes('local_db')
  const { view } = useContext(CompReadyVarsContext)

  React.useEffect(() => {
    // can't do anything if we have no view set (this is sometimes not true on initial page load)
    if (view !== undefined) {
      setIsLoaded(false)
      setIsUpdated(false)

      let triageFetch
      // triage entries will only be available when there is a postgres connection
      if (localDBEnabled) {
        triageFetch = fetch(
          `${getTriagesAPIUrl(id)}?view=${view}&expand=regressions`
        ).then((response) => {
          if (response.status !== 200) {
            throw new Error('API server returned ' + response.status)
          }
          return response.json()
        })
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
    }
  }, [isUpdated, localDBEnabled, view])

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

  if (message !== '') {
    return <h2>{message}</h2>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  const displayUrl = triage.url.startsWith(jiraUrlPrefix)
    ? triage.url.slice(jiraUrlPrefix.length)
    : triage.url

  return (
    <Fragment>
      <Box
        display="flex"
        justifyContent="space-between"
        alignItems="center"
        mb={2}
      >
        <h2 style={{ margin: 0 }}>Triage Details</h2>
        <Box>
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
              <LaunderedLink address={triage.url}>{displayUrl}</LaunderedLink>
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
