import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { sortQueryParams } from './CompReadyUtils'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@mui/icons-material/HelpOutline'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import TableCell from '@mui/material/TableCell'

export default function CompReadyCell(props) {
  const {
    status,
    environment,
    componentName,
    filterVals,
    grayFactor,
    regressedCount,
    accessibilityMode,
  } = props
  const theme = useTheme()
  const classes = useContext(ComponentReadinessStyleContext)

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

    return sortQueryParams(retUrl)
  }

  if (status === undefined) {
    return (
      <Tooltip title="No data">
        <TableCell
          className={classes.crCellResult}
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
        className={classes.crCellResult}
        style={{
          textAlign: 'center',
        }}
      >
        <Link to={componentReport(componentName, environment, filterVals)}>
          <CompSeverityIcon
            status={status}
            grayFactor={grayFactor}
            count={regressedCount}
            accessibilityMode={accessibilityMode}
          />
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
  regressedCount: PropTypes.number,
  accessibilityMode: PropTypes.bool,
}
