import './ComponentReadiness.css'
import { CompReadyVarsContext } from '../CompReadyVars'
import { getAPIUrl, makeRFC3339Time } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import TableCell from '@material-ui/core/TableCell'

export default function CompReadyCapsCell(props) {
  const { status, environment, capabilityName, filterVals } = props
  const theme = useTheme()

  const [capabilityParam, setCapabilityParam] = useQueryParam(
    'capability',
    StringParam
  )
  const [environmentParam, setEnvironmentParam] = useQueryParam(
    'environment',
    StringParam
  )

  const { expandEnvironment } = useContext(CompReadyVarsContext)

  // Construct an URL with all existing filters plus capability and environment
  // (environment seems to already be there because of where this is called from)
  // This is the url used when you click inside a TableCell on the right.
  function capabilityReport(capabilityName, environmentVal, filterVals) {
    const retUrl =
      '/component_readiness/env_capability' +
      filterVals +
      '&capability=' +
      safeEncodeURIComponent(capabilityName) +
      expandEnvironment(environmentVal)
    return retUrl
  }

  const handleClick = (event) => {
    if (!event.metaKey) {
      event.preventDefault()
      setCapabilityParam(capabilityName)
      setEnvironmentParam(environment)
      window.location.href =
        '/sippy-ng' + capabilityReport(capabilityName, environment, filterVals)
    }
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
          to={capabilityReport(capabilityName, environment, filterVals)}
          onClick={handleClick}
        >
          <CompSeverityIcon status={status} />
        </Link>
      </TableCell>
    )
  }
}

CompReadyCapsCell.propTypes = {
  status: PropTypes.number.isRequired,
  environment: PropTypes.string.isRequired,
  capabilityName: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
}
