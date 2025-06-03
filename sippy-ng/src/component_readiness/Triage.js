import { Button } from '@mui/material'
import { CapabilitiesContext } from '../App'
import { getTriagesAPIUrl, jiraUrlPrefix } from './CompReadyUtils'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SecureLink from '../components/SecureLink'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'
import TriagedRegressionTestList from './TriagedRegressionTestList'
import UpsertTriageModal from './UpsertTriageModal'

export default function Triage({ id }) {
  const [isLoaded, setIsLoaded] = React.useState(false)
  const [triage, setTriage] = React.useState({})
  const [message, setMessage] = React.useState('')
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

    triageFetch
      .then((t) => {
        setTriage(t)
        setIsLoaded(true)
        document.title = 'Triage: ' + t.id
      })
      .catch((error) => {
        setMessage(error.toString())
      })
  }, [isUpdated])

  const deleteTriage = () => {
    const confirmed = window.confirm(
      'Are you sure you want to delete this triage record?'
    )
    if (confirmed) {
      fetch(getTriagesAPIUrl(id), {
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
      <h2>Triage Details</h2>
      <Table>
        <TableBody>
          <TableRow>
            <TableCell>Description</TableCell>
            <TableCell>{triage.description}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Type</TableCell>
            <TableCell>{triage.type}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Jira</TableCell>
            <TableCell>
              <SecureLink address={triage.url}>{displayUrl}</SecureLink>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Resolution Date</TableCell>
            <TableCell>
              {triage.resolved?.Valid ? triage.resolved?.Time : ''}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>State</TableCell>
            <TableCell>{triage.bug?.status}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Version</TableCell>
            <TableCell>
              {triage.bug?.target_versions || triage.bug?.affects_versions}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Last Change</TableCell>
            <TableCell>{triage.bug?.last_change_time}</TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <h2>Included Tests</h2>
      <TriagedRegressionTestList regressions={triage.regressions} />
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
    </Fragment>
  )
}

Triage.propTypes = {
  id: PropTypes.string.isRequired,
}
