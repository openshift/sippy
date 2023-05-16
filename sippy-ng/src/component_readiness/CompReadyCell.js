import './ComponentReadiness.css'
import { expandEnvironment, getAPIUrl, makeRFC3339Time } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@material-ui/core/TableCell'

// Construct an URL with all existing filters plus component and environment.
// This is the url used when you click inside a TableCell.
// Note that we are keeping the environment value so we can use it later for displays.
function componentReport(componentName, columnVal, filterVals) {
  const retUrl =
    '/component_readiness/env_capabilities' +
    filterVals +
    '&component=' +
    safeEncodeURIComponent(componentName) +
    expandEnvironment(columnVal)

  //const apiCallStr = makeRFC3339Time(getAPIUrl() + makeRFC3339Time(retUrl))
  //console.log('apiCallStrR: ', apiCallStr)
  return retUrl
}
export default function CompReadyCell(props) {
  const { status, columnVal, componentName, filterVals } = props
  const theme = useTheme()

  const [componentParam, setComponentParam] = useQueryParam(
    'component',
    StringParam
  )
  const [environmentParam, setEnvironmentParam] = useQueryParam(
    'environment',
    StringParam
  )

  const handleClick = (event) => {
    event.preventDefault()
    setComponentParam(componentName)
    setEnvironmentParam(columnVal)
    window.location.href = componentReport(componentName, columnVal, filterVals)
  }

  if (status === undefined) {
    return (
      <Tooltip title="No data">
        <TableCell
          className="cr-cell-result"
          style={{
            textAlign: 'center',
            backgroundColor: theme.palette.text.disabled,
          }}
        >
          <HelpOutlineIcon style={{ color: theme.palette.text.disabled }} />
        </TableCell>
      </Tooltip>
    )
  } else {
    return (
      <TableCell
        className="cr-cell-result"
        style={{
          textAlign: 'center',
          backgroundColor: 'white',
        }}
      >
        <Link
          to={componentReport(componentName, columnVal, filterVals)}
          onClick={handleClick}
        >
          <CompSeverityIcon status={status} />
        </Link>
      </TableCell>
    )
  }
}

CompReadyCell.propTypes = {
  status: PropTypes.number.isRequired,
  columnVal: PropTypes.string.isRequired,
  componentName: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
}
