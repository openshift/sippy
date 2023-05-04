import './ComponentReadiness.css'
import { Fragment } from 'react'
import { Link } from 'react-router-dom'
import { Tooltip, Typography } from '@material-ui/core'
import CompReadyCell from './CompReadyCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@material-ui/core/TableCell'
import TableRow from '@material-ui/core/TableRow'

// Represents a row when you clicked a capability on page2
// We display tests on the left and results on the right.
export default function CompTestRow(props) {
  // testName is the name of the test (called test_name)
  // results is an array of columns and contains the status value per columnName
  // columnNames is the calculated array of columns
  // filterVals: the parts of the url containing input values
  const { testName, results, columnNames, filterVals } = props

  // Put the capability name on the left side with a link to a capability specific
  // capabilities report.  The left side link will go back to the main page.
  const testNameColumn = (
    <TableCell className={'cr-component-name'} key={testName}>
      <Tooltip title={'Capabilities report for ' + testName}>
        <Typography className="cr-cell-name">
          <Link to="/component_rediness">{testName}</Link>
        </Typography>
      </Tooltip>
    </TableCell>
  )

  return (
    <Fragment>
      <TableRow>
        {testNameColumn}
        {results.map((columnVal, idx) => (
          <CompReadyCell
            key={'testName-' + idx}
            status={columnVal.status}
            columnVal={columnNames[idx]}
            componentName={testName}
            filterVals={filterVals}
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

CompTestRow.propTypes = {
  testName: PropTypes.string.isRequired,
  results: PropTypes.array.isRequired,
  columnNames: PropTypes.array.isRequired,
  filterVals: PropTypes.string.isRequired,
}
