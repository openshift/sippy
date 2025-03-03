import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { Fragment, useContext } from 'react'
import { getAPIUrl, makeRFC3339Time } from './CompReadyUtils'
import { grey } from '@mui/material/colors'
import { Link } from 'react-router-dom'
import { makeStyles, useTheme } from '@mui/styles'
import { safeEncodeURIComponent } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip, Typography } from '@mui/material'
import CompReadyCell from './CompReadyCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'

// This is used when a user clicks on a component on the left side of the table.
function capabilitiesReport(filterVals, componentName) {
  const safeComponentName = safeEncodeURIComponent(componentName)
  const retUrl =
    '/component_readiness/capabilities' +
    filterVals +
    `&component=${safeComponentName}`
  return retUrl
}

export default function CompReadyRow(props) {
  const classes = useContext(ComponentReadinessStyleContext)

  // componentName is the name of the component
  // results is an array of columns and contains the status value per columnName
  // columnNames is the calculated array of columns
  // filterVals: the parts of the url containing input values
  const {
    componentName,
    results,
    columnNames,
    filterVals,
    grayFactor,
    accessibilityMode,
  } = props

  const [componentParam, setComponentParam] = useQueryParam(
    'component',
    StringParam
  )

  // Put the componentName on the left side with a link to a component specific
  // capabilities report.
  const componentNameColumn = (
    <TableCell className={classes.componentName} key={componentName}>
      <Tooltip title={'Component report for ' + componentName}>
        <Typography className={classes.crCellName}>
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
            environment={columnNames[idx]}
            componentName={componentName}
            filterVals={filterVals}
            grayFactor={grayFactor}
            regressedCount={
              columnVal.regressed_tests ? columnVal.regressed_tests.length : 0
            }
            accessibilityMode={accessibilityMode}
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
  grayFactor: PropTypes.number.isRequired,
  accessibilityMode: PropTypes.bool.isRequired,
}
