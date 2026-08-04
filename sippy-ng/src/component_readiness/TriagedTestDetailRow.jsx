import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { Fragment, useContext } from 'react'
import { Link, Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'

export default function TriagedTestDetailRow(props) {
  const classes = useContext(ComponentReadinessStyleContext)

  const { element, idx } = props

  const testJobDetailCell = (jobRuns) => {
    return (
      <TableCell>
        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
          }}
        >
          {jobRuns &&
            jobRuns.length > 0 &&
            jobRuns.slice().map((jobRun, jobRunIndex) => {
              return (
                <Link
                  href={jobRun.url}
                  key={jobRunIndex}
                  style={{
                    marginRight: '4px',
                  }}
                >
                  <Typography className={classes.crCellName}>
                    {jobRun.jobRunId}
                  </Typography>
                </Link>
              )
            })}
        </div>
      </TableCell>
    )
  }

  return (
    <Fragment>
      <TableRow key={idx}>
        <TableCell
          className={classes.crColResult}
          key={'column' + '-' + idx}
          width={'25%'}
        >
          <Typography className={classes.crCellName}>
            {element.job_name}
          </Typography>
        </TableCell>
        {testJobDetailCell(element.job_runs)}
      </TableRow>
    </Fragment>
  )
}

TriagedTestDetailRow.propTypes = {
  element: PropTypes.object.isRequired,
  idx: PropTypes.number.isRequired,
}
