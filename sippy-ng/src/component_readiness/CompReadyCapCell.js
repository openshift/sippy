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

export default function CompReadyCapCell(props) {
  const { status, environment, testId, filterVals, component, capability } =
    props
  const theme = useTheme()
  const classes = useContext(ComponentReadinessStyleContext)
  const { expandEnvironment } = useContext(CompReadyVarsContext)

  // Construct a URL with all existing filters plus testId and environment.
  // This is the url used when you click inside a TableCell.
  function testReport(
    testId,
    environmentVal,
    filterVals,
    componentName,
    capabilityName
  ) {
    const safeComponentName = safeEncodeURIComponent(componentName)
    const safeTestId = safeEncodeURIComponent(testId)
    const retUrl =
      '/component_readiness/env_test' +
      filterVals +
      `&testId=${safeTestId}` +
      expandEnvironment(environmentVal) +
      `&component=${safeComponentName}` +
      `&capability=${capabilityName}`

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
          backgroundColor: 'white',
        }}
      >
        <Link
          to={testReport(
            testId,
            environment,
            filterVals,
            component,
            capability
          )}
        >
          <CompSeverityIcon status={status} />
        </Link>
      </TableCell>
    )
  }
}

CompReadyCapCell.propTypes = {
  status: PropTypes.number.isRequired,
  environment: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
}
