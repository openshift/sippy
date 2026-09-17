import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { sortQueryParams } from './CompReadyUtils'
import { Tooltip } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@mui/icons-material/HelpOutline'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import TableCell from '@mui/material/TableCell'

export default function CompReadyCapsCell(props) {
  const { status, environment, capabilityName, filterVals, regressedCount } =
    props
  const theme = useTheme()
  const classes = useContext(ComponentReadinessStyleContext)

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
        <Link to={capabilityReport(capabilityName, environment, filterVals)}>
          <CompSeverityIcon status={status} count={regressedCount} />
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
  regressedCount: PropTypes.number,
}
