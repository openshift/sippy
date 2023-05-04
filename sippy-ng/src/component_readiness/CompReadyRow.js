import './ComponentReadiness.css'
import { Fragment } from 'react'
import { getAPIUrl, makeRFC3339Time } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { Tooltip, Typography } from '@material-ui/core'
import CompReadyCell from './CompReadyCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@material-ui/core/TableCell'
import TableRow from '@material-ui/core/TableRow'

// This is used when a user clicks on a component on the left side of the table.
function capabilitiesReport(filterVals, componentName) {
  const safeComponentName = safeEncodeURIComponent(componentName)
  const retUrl =
    '/component_readiness/capabilities' +
    filterVals +
    `&component=${safeComponentName}`

  const apiCallStr = getAPIUrl() + makeRFC3339Time(retUrl)
  //console.log('apiCallStrR: ', apiCallStr)
  return retUrl
}

export default function CompReadyRow(props) {
  // componentName is the name of the component
  // results is an array of columns and contains the status value per columnName
  // columnNames is the calculated array of columns
  // filterVals: the parts of the url containing input values
  const { componentName, results, columnNames, filterVals } = props

  // Put the component name on the left side with a link to a component specific
  // capabilities report.
  const componentNameColumn = (
    <TableCell className={'cr-component-name'} key={componentName}>
      <Tooltip title={'Component report for ' + componentName}>
        <Typography className="cr-cell-name">
          <Link to={capabilitiesReport(filterVals, componentName)}>
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
            columnVal={columnNames[idx]}
            componentName={componentName}
            filterVals={filterVals}
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

CompReadyRow.propTypes = {
  componentName: PropTypes.string.isRequired,
  results: PropTypes.array.isRequired,
  columnNames: PropTypes.array.isRequired,
  filterVals: PropTypes.string.isRequired,
}
