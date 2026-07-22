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

  const { testCols, columnNames } = props

  // Put the testName on the left side with no link.
  const testNameColumn = (
    <TableCell className={classes.componentName} key={testCols.test_name}>
      <Tooltip title={testCols.test_id}>
        <Typography className={classes.crCellName}>
          {[testCols.test_suite, testCols.test_name].filter(Boolean).join('.')}
        </Typography>
      </Tooltip>
    </TableCell>
  )

  return (
    <Fragment>
      <TableRow>
        {testNameColumn}
        {testCols.columns.map((columnVal, idx) => (
          <CompReadyTestCell
            key={'testName-' + idx}
            status={columnVal.status}
            test={
              columnVal.all_tests?.[0] || columnVal.regressed_tests?.[0] || null
            }
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

CompCapTestRow.propTypes = {
  testCols: PropTypes.object.isRequired,
  columnNames: PropTypes.array.isRequired,
}
