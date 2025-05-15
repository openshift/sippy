import { CapabilitiesContext } from '../App'
import { getTriagesAPIUrl, jiraUrlPrefix } from './CompReadyUtils'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'
import TriagedRegressionTestList from './TriagedRegressionTestList'
import UpsertTriageModal from './UpsertTriageModal'

export default function Triage({ id }) {
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [triage, setTriage] = React.useState({})
  const [isUpdated, setIsUpdated] = React.useState(false)
  const capabilitiesContext = React.useContext(CapabilitiesContext)
  const triageEnabled = capabilitiesContext.includes('write_endpoints')
  const localDBEnabled = capabilitiesContext.includes('local_db')

  React.useEffect(() => {
    setIsLoaded(false)
    setIsUpdated(false)

    let triageFetch
    // triage entries will only be available when there is a postgres connection
    if (localDBEnabled) {
      triageFetch = fetch(getTriagesAPIUrl(id)).then((response) => {
        if (response.status !== 200) {
          throw new Error('API server returned ' + response.status)
        }
        return response.json()
      })
    } else {
      triageFetch = Promise.resolve({})
    }

    triageFetch.then((t) => {
      setTriage(t)
      setIsLoaded(true)
    })
  }, [isUpdated])

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  const displayUrl = triage.url.startsWith(jiraUrlPrefix)
    ? triage.url.slice(jiraUrlPrefix.length)
    : triage.url

  return (
    <Fragment>
      <h2>Triage Details</h2>
      <Table>
        <TableBody>
          <TableRow>
            <TableCell>Description</TableCell>
            <TableCell>{triage.description}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Jira</TableCell>
            <TableCell>
              {/*TODO(sgoeddel): snyk doesn't like the link, bring it back in a followup*/}
              {displayUrl}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Type</TableCell>
            <TableCell>{triage.type}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Resolution Date</TableCell>
            <TableCell>
              {triage.resolved?.Valid ? triage.resolved?.Time : ''}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <h2>Included Tests</h2>
      <TriagedRegressionTestList regressions={triage.regressions} />
      {triageEnabled && (
        <UpsertTriageModal
          triage={triage}
          buttonText={'Update'}
          setComplete={setIsUpdated}
        />
      )}
    </Fragment>
  )
}

Triage.propTypes = {
  id: PropTypes.string.isRequired,
}
