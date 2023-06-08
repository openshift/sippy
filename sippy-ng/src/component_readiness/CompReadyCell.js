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

export default function CompReadyCell(props) {
  const { status, environment, componentName, filterVals, grayFactor } = props
  const theme = useTheme()

  const [componentParam, setComponentParam] = useQueryParam(
    'component',
    StringParam
  )
  const [environmentParam, setEnvironmentParam] = useQueryParam(
    'environment',
    StringParam
  )

  const { expandEnvironment } = useContext(CompReadyVarsContext)

  // Construct an URL with all existing filters plus component and environment.
  // This is the url used when you click inside a TableCell.
  // Note that we are keeping the environment value so we can use it later for displays.
  function componentReport(componentName, environmentVal, filterVals) {
    const retUrl =
      '/component_readiness/env_capabilities' +
      filterVals +
      '&component=' +
      safeEncodeURIComponent(componentName) +
      expandEnvironment(environmentVal)

    return retUrl
  }

  const handleClick = (event) => {
    if (!event.metaKey) {
      event.preventDefault()
      setComponentParam(componentName)
      setEnvironmentParam(environment)
      window.location.href =
        '/sippy-ng' + componentReport(componentName, environment, filterVals)
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
          to={componentReport(componentName, environment, filterVals)}
          onClick={handleClick}
        >
          <CompSeverityIcon status={status} grayFactor={grayFactor} />
        </Link>
      </TableCell>
    )
  }
}

CompReadyCell.propTypes = {
  status: PropTypes.number.isRequired,
  environment: PropTypes.string.isRequired,
  componentName: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
  grayFactor: PropTypes.number.isRequired,
}
