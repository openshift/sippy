import './ComponentReadiness.css'
import { Checkbox, Tooltip, Typography } from '@mui/material'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { Fragment, useContext } from 'react'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'

import { getTestStatus } from '../helpers'

const getJobRunColor = (jobRun) => {
  return getTestStatus(jobRun.test_stats, 'purple', 'red', 'green')
}

// Represents a row on page 5a when you clicked a status cell on page4 or page4a
export default function CompReadyTestDetailRow(props) {
  const classes = useContext(ComponentReadinessStyleContext)

  // element: a test detail element
  // idx: array index of test detail element
  // showOnlyFailures: says to focus on job failures
  const {
    element,
    idx,
    showOnlyFailures,
    searchJobArtifacts,
    searchJobRunIds,
    setSearchJobRunIds,
  } = props

  const handlerForJobRunSelect = (jobId) => {
    return (element) => {
      if (searchJobRunIds.has(jobId)) {
        setSearchJobRunIds(searchJobRunIds.difference(new Set([jobId])))
      } else {
        setSearchJobRunIds(searchJobRunIds.union(new Set([jobId])))
      }
    }
  }

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

  const testJobDetailCell = (jobStats, statsKind) => {
    let jobRuns = []
    if (statsKind === 'base') {
      jobRuns = jobStats.base_job_run_stats || []
    } else if (statsKind === 'sample') {
      jobRuns = jobStats.sample_job_run_stats || []
    } else {
      console.log('ERROR in testDetailJobRow: unknown statsKind ' + statsKind)
    }

    let failureJobRuns = jobRuns.filter(
      (jstat) => jstat.test_stats.failure_count > 0
    ) //lol
    jobRuns = showOnlyFailures ? failureJobRuns : jobRuns

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
          {jobRuns &&
            jobRuns.length > 0 &&
            jobRuns
              .slice()
              .reverse()
              .map((jobRun, jobRunIndex) => {
                var content = (
                  <Tooltip
                    title={
                      new Date(jobRun.start_time).toUTCString() +
                      ' (#' +
                      jobRun.job_run_id +
                      ')'
                    }
                  >
                    <Typography className={classes.crCellName}>
                      {jobRun.test_stats.failure_count > 0 ? 'F' : 'S'}
                    </Typography>
                  </Tooltip>
                )
                var selectHandler = handlerForJobRunSelect(jobRun.job_run_id)

                return searchJobArtifacts ? (
                  <a
                    className={
                      searchJobRunIds.has(jobRun.job_run_id)
                        ? classes.selectedJobRun
                        : classes.unselectedJobRun
                    }
                    onClick={selectHandler}
                    key={jobRunIndex}
                    style={{
                      color: getJobRunColor(jobRun),
                      marginRight: '1px',
                    }}
                  >
                    {' '}
                    {content}{' '}
                  </a>
                ) : (
                  // not searching artifacts, show normal job run with link
                  <a
                    href={jobRun.job_url}
                    key={jobRunIndex}
                    style={{
                      color: getJobRunColor(jobRun),
                      marginRight: '1px',
                    }}
                  >
                    {' '}
                    {content}{' '}
                  </a>
                )
              })}
        </div>
      </TableCell>
    )
  }

  // determine checkbox statuses for the failed job runs in each row
  function getSelectingCheckBox(jobRunStats) {
    let failures = new Set(
      (jobRunStats || [])
        .filter((jstat) => jstat.test_stats.failure_count > 0)
        .map((jobRun) => jobRun.job_run_id)
    )
    let selectedFailures = failures.intersection(searchJobRunIds)
    let allSelected =
      failures.size > 0 && failures.size === selectedFailures.size
    let someSelected =
      failures.size > 0 &&
      selectedFailures.size > 0 &&
      selectedFailures.size < failures.size
    let clickHandler = (event) => {
      event.stopPropagation()
      if (selectedFailures.size > 0) {
        // if any are selected, deselect them
        setSearchJobRunIds(searchJobRunIds.difference(failures))
      } else {
        setSearchJobRunIds(searchJobRunIds.union(failures))
      }
    }
    return (
      <Tooltip title="Select Failed Job Runs">
        <Checkbox
          sx={{ padding: 0 }}
          checked={allSelected}
          indeterminate={someSelected}
          disabled={failures.size === 0}
          size="small"
          label="Select Failed"
          onClick={clickHandler}
        />
      </Tooltip>
    )
  }

  return (
    <Fragment>
      <TableRow key={'jobheader-' + idx}>
        <TableCell
          className={classes.crColJobName}
          key={'basisJob-' + idx}
          colSpan={3}
        >
          <Typography className={classes.crCellName}>
            {element.base_job_name ? (
              <Fragment>
                {element.base_job_name}
                {searchJobArtifacts &&
                  getSelectingCheckBox(element.base_job_run_stats)}
              </Fragment>
            ) : (
              '(No basis equivalent)'
            )}
          </Typography>
        </TableCell>
        <TableCell
          className={classes.crColJobName}
          key={'sampleJob-' + idx}
          colSpan={3}
        >
          <Typography className={classes.crCellName}>
            {element.sample_job_name ? (
              <Fragment>
                {element.sample_job_name}
                {searchJobArtifacts &&
                  getSelectingCheckBox(element.sample_job_run_stats)}
              </Fragment>
            ) : (
              '(No sample equivalent)'
            )}
          </Typography>
        </TableCell>
      </TableRow>
      <TableRow key={idx}>
        <TableCell style={{ verticalAlign: 'top' }}>
          {element.base_job_name ? infoCell(element.base_stats) : ''}
        </TableCell>
        {testJobDetailCell(element, 'base')}
        <TableCell></TableCell>
        <TableCell style={{ verticalAlign: 'top' }}>
          {element.sample_job_name ? infoCell(element.sample_stats) : ''}
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
  searchJobArtifacts: PropTypes.bool.isRequired,
  searchJobRunIds: PropTypes.object.isRequired,
  setSearchJobRunIds: PropTypes.func.isRequired,
}
