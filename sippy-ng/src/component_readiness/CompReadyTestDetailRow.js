import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { Fragment, useContext } from 'react'
import { Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'

const getJobRunColor = (jobRun) => {
  if (jobRun.test_stats.flake_count > 0) {
    return 'purple'
  } else if (jobRun.test_stats.failure_count > 0) {
    return 'red'
  } else {
    return 'green'
  }
}

// Represents a row on page 5a when you clicked a status cell on page4 or page4a
export default function CompReadyTestDetailRow(props) {
  const classes = useContext(ComponentReadinessStyleContext)

  // element: a test detail element
  // idx: array index of test detail element
  // showOnlyFailures: says to focus on job failures
  const { element, idx, showOnlyFailures, triagedURLs } = props

  const infoCell = (stats) => {
    return (
      <Typography className={classes.crCellName}>
        pass rate=
        {(stats.success_rate * 100).toFixed(2)}%
        <br />
        successes={stats.success_count}
        <br />
        failures={stats.failure_count}
        <br />
        flakes={stats.flake_count}
      </Typography>
    )
  }

  const testJobDetailCell = (element, statsKind) => {
    let item
    if (statsKind === 'base') {
      item = element.base_job_run_stats
    } else if (statsKind === 'sample') {
      item = element.sample_job_run_stats
    } else {
      item = 'unknown statsKind in testDetailJobRow'
      console.log('ERROR in testDetailJobRow')
    }

    let filtered = item

    // If we only care to see failures, then remove anything else
    // Protect against empty/undefined data
    if (showOnlyFailures) {
      filtered = item
        ? item.filter((jstat) => jstat.test_stats.failure_count > 0)
        : []
    }

    // Print out the S and F letters for job runs (20 per line) in reverse order
    // so you see the most recent jobRuns first.
    return (
      <TableCell className="cr-jobrun-table-wrapper">
        <div
          style={{
            display: 'flex',
            maxWidth: '205px',
            flexWrap: 'wrap',
          }}
        >
          {filtered &&
            filtered.length > 0 &&
            filtered
              .slice()
              .reverse()
              .map((jobRun, jobRunIndex) => {
                return (
                  <a
                    href={jobRun.job_url}
                    key={jobRunIndex}
                    style={{
                      color: getJobRunColor(jobRun),
                      marginRight: '1px',
                    }}
                  >
                    <Typography className={classes.crCellName}>
                      {jobRun.test_stats.failure_count > 0
                        ? triagedURLs[jobRun.job_url] !== undefined
                          ? 'T'
                          : 'F'
                        : 'S'}
                    </Typography>
                  </a>
                )
              })}
        </div>
      </TableCell>
    )
  }

  return (
    <Fragment>
      <TableRow key={idx}>
        <TableCell className={classes.crColResult} key={'column' + '-' + idx}>
          <Typography className={classes.crCellName}>
            {element.job_name}
          </Typography>
        </TableCell>
        <TableCell style={{ verticalAlign: 'top' }}>
          {infoCell(element.base_stats)}
        </TableCell>
        {testJobDetailCell(element, 'base')}
        <TableCell style={{ verticalAlign: 'top' }}>
          {infoCell(element.sample_stats)}
        </TableCell>
        {testJobDetailCell(element, 'sample')}
        <TableCell style={{ verticalAlign: 'top' }}>
          <Typography className={classes.crCellName}>
            {element.significant ? 'True' : 'False'}
          </Typography>
        </TableCell>
      </TableRow>
    </Fragment>
  )
}

CompReadyTestDetailRow.propTypes = {
  element: PropTypes.object.isRequired,
  idx: PropTypes.number.isRequired,
  showOnlyFailures: PropTypes.bool.isRequired,
  triagedURLs: PropTypes.array.isRequired,
}
