import './ComponentReadiness.css'
import { Link } from 'react-router-dom'
import { makeRFC3339Time, safeEncodeURIComponent } from '../helpers'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import PropTypes from 'prop-types'
import React from 'react'
import SeverityIcon from './SeverityIcon'
import TableCell from '@material-ui/core/TableCell'

// Construct an URL with all existing filters plus component and environment.
function componentReport(componentName, columnVal, filterVals) {
  const retUrl =
    '/componentreadiness/tests' +
    filterVals +
    '&component=' +
    safeEncodeURIComponent(componentName) +
    '&environment=' +
    safeEncodeURIComponent(columnVal)

  const apiCallStr = makeRFC3339Time(
    'http://localhost:8080/api' +
      makeRFC3339Time(retUrl).replace(
        'componentreadiness',
        'component_readiness'
      )
  )
  //console.log('apiCallStrR: ', apiCallStr)
  return retUrl
}
export default function CompReadyCell(props) {
  const { status, columnVal, componentName, filterVals } = props
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
        <Link to={componentReport(componentName, columnVal, filterVals)}>
          <SeverityIcon status={status} />
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
