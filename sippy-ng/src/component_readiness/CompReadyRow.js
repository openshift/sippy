import './ComponentReadiness.css'
import { Fragment } from 'react'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { Tooltip, Typography } from '@material-ui/core'
import CompReadyCell from './CompReadyCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@material-ui/core/TableCell'
import TableRow from '@material-ui/core/TableRow'

// This is used when a user clicks on a component on the left side of the table
function capabilitiesReport(componentName, release) {
  return (
    '/componentreadiness/' +
    safeEncodeURIComponent(componentName) +
    '/capabilities'
  )
}

export default function CompReadyRow(props) {
  // componentName is the name of the component
  // results is an array of columns and contains the status value per columnName
  // release is the release (not sure if we need it)
  const { componentName, results, release } = props

  // Put the component name on the left side with a link to a component specific
  // capabilities report.
  const componentNameColumn = (
    <TableCell className={'component-name'} key={componentName}>
      <Tooltip title={'Capabilities report for ' + componentName}>
        <Typography className="cell-name">
          <Link to={capabilitiesReport(componentName, { release })}>
            {componentName}
          </Link>
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
            release={release}
            variant={columnVal}
            testName={componentName}
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

CompReadyRow.propTypes = {
  results: PropTypes.array.isRequired,
  componentName: PropTypes.string.isRequired,
  release: PropTypes.string.isRequired,
}
