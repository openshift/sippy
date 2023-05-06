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

// Construct a URL with all existing filters plus testName and environment.
// This is the url used when you click inside a TableCell.
function testReport(testName, columnVal, filterVals) {
  const retUrl =
    '/component_readiness/env_test' +
    filterVals +
    '&test_name=' +
    safeEncodeURIComponent(testName) +
    expandEnvironment(columnVal)

  return retUrl
}
export default function CompReadyCapCell(props) {
  const { status, columnVal, testName, filterVals } = props
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
        <Link to={testReport(testName, columnVal, filterVals)}>
          <CompSeverityIcon status={status} />
        </Link>
      </TableCell>
    )
  }
}

CompReadyCapCell.propTypes = {
  status: PropTypes.number.isRequired,
  columnVal: PropTypes.string.isRequired,
  testName: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
}
