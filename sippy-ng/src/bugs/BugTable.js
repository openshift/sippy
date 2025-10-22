import {
  Button,
  Link,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material'
import { getTriagesAPIUrl } from '../component_readiness/CompReadyUtils'
import { apiFetch, relativeTime, safeEncodeURIComponent } from '../helpers'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

export default function BugTable(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [bugs, setBugs] = React.useState([])
  const [bugToPotentialTriage, setBugToPotentialTriage] = React.useState({})
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    let bugsURL = props.bugsURL
    if (!bugsURL || bugsURL.length === 0) {
      bugsURL = `/api/tests/bugs?test=${safeEncodeURIComponent(props.testName)}`
    }

    Promise.all([apiFetch(bugsURL)])
      .then(([bugs]) => {
        if (bugs.status !== 200) {
          throw new Error('server returned when fetching bugs' + bugs.status)
        }
        return Promise.all([bugs.json()])
      })
      .then(([bugs]) => {
        setBugs(bugs)
        // If we have a regressionId and bugs, we can try to find a matching triage entry
        // for each bug that could potentially be used to triage the regression
        if (
          props.writeEndpointsEnabled &&
          props.regressionId &&
          bugs.length > 0
        ) {
          return fetch(getTriagesAPIUrl())
            .then((res) => {
              if (res.status !== 200) {
                throw new Error(
                  'server returned when fetching triages' + res.status
                )
              }
              return res.json()
            })
            .then((triages) => {
              let btpt = {}
              bugs.forEach((bug) => {
                let matchedTriage = undefined
                for (const triage of triages) {
                  const alreadyAssociatedWithTriage = triage.regressions.some(
                    (regression) => regression.id === props.regressionId
                  )
                  if (!alreadyAssociatedWithTriage && triage.url === bug.url) {
                    // If we have multiple matches, we should not add the button to triage to any of them
                    if (matchedTriage !== undefined) {
                      matchedTriage = undefined
                      break
                    }
                    matchedTriage = triage
                  }
                }

                if (matchedTriage !== undefined) {
                  btpt[bug.id] = matchedTriage
                }
              })
              setBugToPotentialTriage(btpt)
            })
        }
      })
      .then(() => {
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve bug data ' + error)
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

  const addToTriage = (triage) => {
    triage.regressions.push({ id: props.regressionId })
    fetch(getTriagesAPIUrl(triage.id), {
      method: 'PUT',
      body: JSON.stringify(triage),
    }).then((res) => {
      if (res.status !== 200) {
        setFetchError(
          'Could not add to triage ' + triage.url + ' error ' + res.status
        )
      }
      // this will refresh the entire test_details report page, resulting in the newly associated triage showing
      props.setHasBeenTriaged(true)
    })
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }
  if (fetchError !== '') {
    return <Alert severity="error">{fetchError}</Alert>
  }

  if (!bugs || bugs.length === 0) {
    return <Typography>None found</Typography>
  }

  return (
    <TableContainer component={Paper} style={{ marginTop: 20 }}>
      <Table size="small" aria-label="bug-table">
        <TableHead>
          <TableRow>
            <TableCell>Issue</TableCell>
            <TableCell>Summary</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>Component</TableCell>
            <TableCell>Affects Versions</TableCell>
            <TableCell>Last Modified</TableCell>
            <TableCell></TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {bugs.map((bug) => (
            <TableRow key={'bug-row-' + bug.id}>
              <TableCell scope="row">
                <a href={bug.url}>{bug.key}</a>
              </TableCell>
              <TableCell>
                <a href={bug.url}>{bug.summary}</a>
              </TableCell>
              <TableCell>{bug.status}</TableCell>
              <TableCell>
                {bug.components ? bug.components.join(',') : ''}
              </TableCell>
              <TableCell>
                {bug.affects_versions ? bug.affects_versions.join(',') : ''}
              </TableCell>
              <TableCell>
                {relativeTime(new Date(bug.last_change_time), new Date())}
              </TableCell>
              <TableCell>
                {bugToPotentialTriage[bug.id] && (
                  <Tooltip
                    title={
                      <div>
                        Add this regression to triage entry matching bug:{' '}
                        {bug.key}
                        <br />
                        <Link
                          href={`/sippy-ng/component_readiness/triages/${
                            bugToPotentialTriage[bug.id].id
                          }`}
                          color="inherit"
                          sx={{ textDecoration: 'underline' }}
                          target="_blank"
                        >
                          View triage details →
                        </Link>
                      </div>
                    }
                  >
                    <Button
                      variant="contained"
                      color="secondary"
                      onClick={() => addToTriage(bugToPotentialTriage[bug.id])}
                    >
                      Triage
                    </Button>
                  </Tooltip>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

BugTable.propTypes = {
  bugsURL: PropTypes.string,
  testName: PropTypes.string,
  classes: PropTypes.object,
  writeEndpointsEnabled: PropTypes.bool,
  regressionId: PropTypes.number,
  setHasBeenTriaged: PropTypes.func,
}
