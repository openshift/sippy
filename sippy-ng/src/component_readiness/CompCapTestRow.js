import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { Fragment, useContext } from 'react'
import { Tooltip, Typography } from '@mui/material'
import CompReadyTestCell from './CompReadyTestCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'

// Represents a row on page 4 or page4a when you clicked a testName on page3
export default function CompCapTestRow(props) {
  const classes = useContext(ComponentReadinessStyleContext)

  // regressedTestCols is the full test data structure
  // columnNames is the calculated array of columns
  // filterVals: the parts of the url containing input values
  const { regressedTestCols, columnNames, filterVals } = props

  // Put the testName on the left side with no link.
  const testNameColumn = (
    <TableCell
      className={classes.componentName}
      key={regressedTestCols.test_name}
    >
      <Tooltip title={regressedTestCols.test_id}>
        <Typography className={classes.crCellName}>
          {[regressedTestCols.test_suite, regressedTestCols.test_name]
            .filter(Boolean)
            .join('.')}
        </Typography>
      </Tooltip>
    </TableCell>
  )

  return (
    <Fragment>
      <TableRow>
        {testNameColumn}
        {regressedTestCols.columns.map((columnVal, idx) => (
          <CompReadyTestCell
            key={'testName-' + idx}
            status={columnVal.status}
            environment={columnNames[idx]}
            filterVals={filterVals}
            regressedTest={columnVal.regressed_tests?.[0] || null}
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

CompCapTestRow.propTypes = {
  regressedTestCols: PropTypes.object.isRequired,
  columnNames: PropTypes.array.isRequired,
  filterVals: PropTypes.string.isRequired,
}
