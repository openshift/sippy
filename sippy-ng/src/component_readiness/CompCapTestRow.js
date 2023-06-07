import './ComponentReadiness.css'
import { Fragment } from 'react'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip, Typography } from '@material-ui/core'
import CompReadyTestCell from './CompReadyTestCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@material-ui/core/TableCell'
import TableRow from '@material-ui/core/TableRow'

// Represents a row on page 4 or page4a when you clicked a testName on page3
export default function CompCapTestRow(props) {
  // testName is the name of the test (called testName)
  // testId is the unique test ID that maps to the testName
  // results is an array of columns and contains the status value per columnName
  // columnNames is the calculated array of columns
  // filterVals: the parts of the url containing input values
  // component, capability: these are passed because we need them in the /test endpoint
  const {
    testName,
    testSuite,
    testId,
    results,
    columnNames,
    filterVals,
    component,
    capability,
  } = props

  // Put the testName on the left side with no link.
  const testNameColumn = (
    <TableCell className={'cr-component-name'} key={testName}>
      <Tooltip title={testId}>
        <Typography className="cr-cell-name">
          {[testSuite, testName].filter(Boolean).join('.')}
        </Typography>
      </Tooltip>
    </TableCell>
  )

  return (
    <Fragment>
      <TableRow>
        {testNameColumn}
        {results.map((columnVal, idx) => (
          <CompReadyTestCell
            key={'testName-' + idx}
            status={columnVal.status}
            environment={columnNames[idx]}
            testId={testId}
            filterVals={filterVals}
            component={component}
            capability={capability}
            testName={testName}
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

CompCapTestRow.propTypes = {
  testName: PropTypes.string.isRequired,
  testSuite: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  results: PropTypes.array.isRequired,
  columnNames: PropTypes.array.isRequired,
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
}
