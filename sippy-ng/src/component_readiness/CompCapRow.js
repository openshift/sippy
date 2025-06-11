import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
import { Fragment, useContext } from 'react'
import { generateRegressionCount, sortQueryParams } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip, Typography } from '@mui/material'
import CompReadyCapsCell from './CompReadyCapsCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'

// Represents a row when you clicked a cell from page 1
// We display capabilities on the left.
export default function CompCapRow(props) {
  const classes = useContext(ComponentReadinessStyleContext)

  // capabilityName is the name of the capability
  // results is an array of columns and contains the status value per columnName
  // columnNames is the calculated array of columns
  // filterVals: the parts of the url containing input values
  const { capabilityName, results, columnNames, filterVals, environment } =
    props

  const [capabilityParam, setCapabilityParam] = useQueryParam(
    'capability',
    StringParam
  )

  const { expandEnvironment } = useContext(CompReadyVarsContext)

  // After clicking a capability name on page 2, we add that capability name
  // to the api call along with all the other parts we already have.
  function capabilityLink(filterVals, capabilityName, environment) {
    const retVal =
      '/component_readiness/capability' +
      filterVals +
      expandEnvironment(environment) +
      `&capability=${capabilityName}`

    return sortQueryParams(retVal)
  }

  // Put the capabilityName on the left side with a link to a capability specific
  // capabilities report.
  const capabilityNameColumn = (
    <TableCell className={classes.componentName} key={capabilityName}>
      <Tooltip title={'Capabilities report for ' + capabilityName}>
        <Typography className={classes.crCellName}>
          <Link to={capabilityLink(filterVals, capabilityName, environment)}>
            {capabilityName}
          </Link>
        </Typography>
      </Tooltip>
    </TableCell>
  )

  return (
    <Fragment>
      <TableRow>
        {capabilityNameColumn}
        {results.map((columnVal, idx) => (
          <CompReadyCapsCell
            key={'testName-' + idx}
            status={columnVal.status}
            environment={columnNames[idx]}
            capabilityName={capabilityName}
            filterVals={filterVals}
            regressedCount={generateRegressionCount(
              columnVal.regressed_tests,
              columnVal.triaged_incidents
            )}
          />
        ))}
      </TableRow>
    </Fragment>
  )
}

CompCapRow.propTypes = {
  capabilityName: PropTypes.string.isRequired,
  results: PropTypes.array.isRequired,
  columnNames: PropTypes.array.isRequired,
  filterVals: PropTypes.string.isRequired,
  environment: PropTypes.string,
}
