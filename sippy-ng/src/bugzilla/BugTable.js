import {
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@material-ui/core'
import PropTypes from 'prop-types'
import React from 'react'

export default function BugTable(props) {
  if (!props.bugs || props.bugs.length === 0) {
    return <Typography>None found</Typography>
  }

  return (
    <TableContainer component={Paper} style={{ marginTop: 20 }}>
      <Table size="small" aria-label="bug-table">
        <TableHead>
          <TableRow>
            <TableCell>Bug ID</TableCell>
            <TableCell>Summary</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>Component</TableCell>
            <TableCell>Target Release</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {props.bugs.map((bug) => (
            <TableRow key={'bug-row-' + bug.id}>
              <TableCell scope="row">
                <a href={bug.url}>{bug.id}</a>
              </TableCell>
              <TableCell>{bug.summary}</TableCell>
              <TableCell>{bug.status}</TableCell>
              <TableCell>{bug.component}</TableCell>
              <TableCell>{bug.target_release}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

BugTable.propTypes = {
  bugs: PropTypes.array,
  classes: PropTypes.object,
}
