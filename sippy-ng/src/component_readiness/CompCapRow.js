import './ComponentReadiness.css'
import { Fragment } from 'react'
import { Link } from 'react-router-dom'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip, Typography } from '@material-ui/core'
import CompReadyCapsCell from './CompReadyCapsCell'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@material-ui/core/TableCell'
import TableRow from '@material-ui/core/TableRow'

// After clicking a capability name on page 2, we add that capability name
// to the api call along with all the other parts we already have.
function capabilityLink(filterVals, capabilityName) {
  const retVal =
    '/component_readiness/capability' +
    filterVals +
    `&capability=${capabilityName}`
  return retVal
}

// Represents a row when you clicked a cell from page 1
// We display capabilities on the left.
export default function CompCapRow(props) {
  // capabilityName is the name of the capability
  // results is an array of columns and contains the status value per columnName
  // columnNames is the calculated array of columns
  // filterVals: the parts of the url containing input values
  const { capabilityName, results, columnNames, filterVals } = props

  const [capabilityParam, setCapabilityParam] = useQueryParam(
    'capability',
    StringParam
  )

  const handleClick = (event) => {
    event.preventDefault()
    setCapabilityParam(capabilityName)
    window.open(
      '/sippy-ng' + capabilityLink(filterVals, capabilityName),
      '_blank'
    )
  }

  // Put the capabilityName on the left side with a link to a capability specific
  // capabilities report.
  const capabilityNameColumn = (
    <TableCell className={'cr-component-name'} key={capabilityName}>
      <Tooltip title={'Capabilities report for ' + capabilityName}>
        <Typography className="cr-cell-name">
          <Link
            to={capabilityLink(filterVals, capabilityName)}
            onClick={handleClick}
          >
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
}
