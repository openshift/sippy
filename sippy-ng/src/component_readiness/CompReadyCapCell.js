import './ComponentReadiness.css'
import { CompReadyVarsContext } from './CompReadyVars'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { sortQueryParams } from './CompReadyUtils'
import { StringParam, useQueryParam } from 'use-query-params'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import TableCell from '@material-ui/core/TableCell'

export default function CompReadyCapCell(props) {
  const { colStatus, environment, testId, filterVals, component, capability } =
    props
  const theme = useTheme()

  const [componentParam, setComponentParam] = useQueryParam(
    'component',
    StringParam
  )
  const [capabilityParam, setCapabilityParam] = useQueryParam(
    'capability',
    StringParam
  )
  const [environmentParam, setEnvironmentParam] = useQueryParam(
    'environment',
    StringParam
  )
  const [testIdParam, setTestIdParam] = useQueryParam('testId', StringParam)

  const { expandEnvironment } = useContext(CompReadyVarsContext)

  // Construct a URL with all existing filters plus testId and environment.
  // This is the url used when you click inside a TableCell.
  function testReport(
    testIdent,
    environmentVal,
    filtVals,
    componentName,
    capabilityName
  ) {
    const safeComponentName = safeEncodeURIComponent(componentName)
    const safeTestId = safeEncodeURIComponent(testIdent)
    const retUrl =
      '/component_readiness/env_test' +
      filtVals +
      `&testId=${safeTestId}` +
      expandEnvironment(environmentVal) +
      `&component=${safeComponentName}` +
      `&capability=${capabilityName}`

    return sortQueryParams(retUrl)
  }

  if (colStatus === undefined) {
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
          to={testReport(
            testId,
            environment,
            filterVals,
            component,
            capability
          )}
        >
          <CompSeverityIcon status={colStatus} />
        </Link>
      </TableCell>
    )
  }
}

CompReadyCapCell.propTypes = {
  colStatus: PropTypes.number.isRequired,
  environment: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
}
