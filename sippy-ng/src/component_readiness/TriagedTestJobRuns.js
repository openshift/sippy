import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { TableContainer, Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import TriagedTestDetailRow from './TriagedTestDetailRow'

export default function TriagedTestJobRuns(props) {
  const classes = useContext(ComponentReadinessStyleContext)
  const tableCell = (label, idx) => {
    return (
      <TableCell className={classes.crColResult} key={'column' + '-' + idx}>
        <Typography className={classes.crCellName}>{label}</Typography>
      </TableCell>
    )
  }

  return (
    <Fragment>
      <Typography>Failed Runs</Typography>
      <TableContainer component="div" className="cr-triage-run-table">
        <Table>
          <TableHead>
            <TableRow>
              {tableCell('ProwJob Name', 0)}
              {tableCell('Failed Job Runs', 1)}
            </TableRow>
          </TableHead>
          <TableBody>
            {props.jobRuns && props.jobRuns.length > 0 ? (
              props.jobRuns.map((element, idx) => {
                return (
                  <TriagedTestDetailRow
                    key={idx}
                    element={element}
                    idx={idx}
                  ></TriagedTestDetailRow>
                )
              })
            ) : (
              <TableRow>
                {/* No data to render, need to make a selection */}
                <TableCell align="center">Select Test Failure Row</TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Fragment>
  )
}

TriagedTestJobRuns.propTypes = {
  jobRuns: PropTypes.array.isRequired,
}
