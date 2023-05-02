import './ComponentReadiness.css'
import { Fragment } from 'react'
import { Link } from 'react-router-dom'
import { Tooltip, Typography } from '@material-ui/core'
import CompReadyCell from './CompReadyCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@material-ui/core/TableCell'
import TableRow from '@material-ui/core/TableRow'

export default function CompCapRow(props) {
  // componentName is the name of the component
  // results is an array of columns and contains the status value per columnName
  // columnNames is the calculated array of columns
  const { componentName, results, columnNames } = props

  // Put the component name on the left side with a link to a component specific
  // capabilities report.  The left side link will go back to the main page.
  const componentNameColumn = (
    <TableCell className={'cr-component-name'} key={componentName}>
      <Tooltip title={'Capabilities report for ' + componentName}>
        <Typography className="cr-cell-name">
          <Link to="/component_readiness">{componentName}</Link>
        </Typography>
      </Tooltip>
    </TableCell>
  )

  return (
    <Fragment>
      <TableRow>
        {componentNameColumn}
        {results.map((columnVal, idx) => (
          <CompReadyCell
            key={'testName-' + idx}
            status={columnVal.status}
            columnVal={columnNames[idx]}
            componentName={componentName}
            filterVals=""
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

CompCapRow.propTypes = {
  componentName: PropTypes.string.isRequired,
  results: PropTypes.array.isRequired,
  columnNames: PropTypes.array.isRequired,
}
