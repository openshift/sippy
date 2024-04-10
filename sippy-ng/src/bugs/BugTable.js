import {
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material'
import { relativeTime, safeEncodeURIComponent } from '../helpers'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { useEffect } from 'react'

export default function BugTable(props) {
  const [isLoaded, setLoaded] = React.useState(false)
  const [bugs, setBugs] = React.useState([])
  const [fetchError, setFetchError] = React.useState('')

  const fetchData = () => {
    Promise.all([
      fetch(
        `${
          process.env.REACT_APP_API_URL
        }/api/tests/bugs?test=${safeEncodeURIComponent(props.testName)}`
      ),
    ])
      .then(([bugs]) => {
        if (bugs.status !== 200) {
          throw new Error('server returned ' + bugs.status)
        }
        return Promise.all([bugs.json()])
      })
      .then(([bugs]) => {
        setBugs(bugs)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError('Could not retrieve bug data ' + error)
      })
  }

  useEffect(() => {
    fetchData()
  }, [])

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
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

BugTable.propTypes = {
  testName: PropTypes.string,
  classes: PropTypes.object,
}
