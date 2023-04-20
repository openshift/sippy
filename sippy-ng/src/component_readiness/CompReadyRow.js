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
  const { columnNames, componentName, results, release } = props

  // columnNames includes the "Name" column
  // componentName will be the name of the component and be under the "Name" column
  // results will contain the status value per columnName

  // Put the component name on the left side with a link to a component specific
  // report.
  const componentNameColumn = (
    <TableCell className={'component-name'} key={componentName}>
      <Tooltip title={'Capabilities report for' + componentName}>
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
        {columnNames.map((column, idx) => {
          // We already printed the componentName earlier so skip it here.
          if (column !== 'Name') {
            return (
              <CompReadyCell
                key={'testName-' + idx}
                status={results[column]}
                release={release}
                variant={column}
                testName={componentName}
              />
            )
          }
        })}
      </TableRow>
    </Fragment>
  )
}

CompReadyRow.propTypes = {
  results: PropTypes.object,
  columnNames: PropTypes.array.isRequired,
  componentName: PropTypes.string.isRequired,
  release: PropTypes.string.isRequired,
}
