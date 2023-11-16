import './JobDetailTable.css'
import { Link } from 'react-router-dom'
import { pathForExactJob } from '../helpers'
import { TableContainer } from '@mui/material'
import JobDetailTestModal from './JobDetailTestModal'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'

/**
 * JobDetailTable shows the runs of the selected job(s) grouped by day, and
 * an icon indicating results that links to the prow job.
 */
export default function JobDetailTable(props) {
  const columns = props.columns
  const rows = props.rows

  const [isTestDialogOpen, setTestDialogOpen] = React.useState(false)
  const [testDetails, setTestDetails] = React.useState({
    name: '',
    failedTestNames: [],
  })

  const openTestDialog = (test) => {
    setTestDetails(test)
    setTestDialogOpen(true)
  }

  const closeTestDialog = (details) => {
    setTestDialogOpen(false)
  }

  return (
    <Fragment>
      <br />
      <br />
      <div>
        <span className="legend-item">
          <span className="results results-demo">
            <span className="result result-S">S</span>
          </span>{' '}
          success
        </span>
        <span className="legend-item">
          <span className="results results-demo">
            <span className="result result-F">F</span>
          </span>{' '}
          failure (e2e)
        </span>
        <span className="legend-item">
          <span className="results results-demo">
            <span className="result result-f">f</span>
          </span>{' '}
          failure (other tests)
        </span>
        <span className="legend-item">
          <span className="results results-demo">
            <span className="result result-U">U</span>
          </span>{' '}
          upgrade failure
        </span>
        <span className="legend-item">
          <span className="results results-demo">
            <span className="result result-I">I</span>
          </span>{' '}
          setup failure (installer)
        </span>
        <span className="legend-item">
          <span className="results results-demo">
            <span className="result result-N">N</span>
          </span>{' '}
          setup failure (infra)
        </span>
        <span className="legend-item">
          <span className="results results-demo">
            <span className="result result-n">n</span>
          </span>{' '}
          failure before setup (infra)
        </span>
        <span className="legend-item">
          <span className="results results-demo">
            <span className="result result-R">R</span>
          </span>{' '}
          running
        </span>
      </div>

      <div className="view">
        <TableContainer component="div" className="wrapper">
          <Table className="dashboard-table" aria-label="simple table">
            <TableHead>
              <TableRow>
                <TableCell className="col-name col-first">Name</TableCell>
                {columns.map((column, idx) => (
                  <TableCell key={'job-column' + idx} className="col-day">
                    {column}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={'job-' + row.name} className="row-item">
                  <TableCell
                    component="th"
                    scope="row"
                    className="col-name col-first"
                  >
                    <Link to={pathForExactJob(props.release, row.name)}>
                      {row.name}
                    </Link>
                  </TableCell>
                  {row.results.map((days, index) => (
                    <TableCell
                      className="col-day"
                      key={'ts-' + index}
                      style={{ verticalAlign: 'top' }}
                    >
                      {days.map((day, dayidx) => (
                        <Fragment key={`day-${index}-${dayidx}`}>
                          {dayidx % 5 === 0 ? <br /> : ''}
                          <a
                            key={day.id}
                            className={day.className}
                            onClick={() => openTestDialog(day)}
                          >
                            {day.text}
                          </a>
                        </Fragment>
                      ))}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        <JobDetailTestModal
          release={props.release}
          item={testDetails}
          isOpen={isTestDialogOpen}
          close={closeTestDialog}
        />
      </div>
    </Fragment>
  )
}

JobDetailTable.propTypes = {
  columns: PropTypes.array,
  rows: PropTypes.array,
  release: PropTypes.string.isRequired,
}
