import './ComponentReadiness.css'
import { expandEnvironment, getAPIUrl, makeRFC3339Time } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import { safeEncodeURIComponent } from '../helpers'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import PropTypes from 'prop-types'
import React from 'react'
import TableCell from '@material-ui/core/TableCell'

// Construct a URL with all existing filters plus testId and environment.
// This is the url used when you click inside a TableCell.
function testReport(
  testId,
  columnVal,
  filterVals,
  componentName,
  capabilityName
) {
  const safeComponentName = safeEncodeURIComponent(componentName)
  const retUrl =
    '/component_readiness/env_test' +
    filterVals +
    '&test_id=' +
    safeEncodeURIComponent(testId) +
    expandEnvironment(columnVal) +
    `&component=${safeComponentName}` +
    `&capability=${capabilityName}`

  return retUrl
}
export default function CompReadyCapCell(props) {
  const { status, columnVal, testId, filterVals, component, capability } = props
  const theme = useTheme()

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
          to={testReport(testId, columnVal, filterVals, component, capability)}
        >
          <CompSeverityIcon status={status} />
        </Link>
      </TableCell>
    )
  }
}

CompReadyCapCell.propTypes = {
  status: PropTypes.number.isRequired,
  columnVal: PropTypes.string.isRequired,
  testId: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
  component: PropTypes.string.isRequired,
  capability: PropTypes.string.isRequired,
}
