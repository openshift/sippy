import './ComponentReadiness.css'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import PropTypes from 'prop-types'
import React from 'react'
import SeverityIcon from './SeverityIcon'
import TableCell from '@material-ui/core/TableCell'

export default function CompReadyCell(props) {
  const status = props.status
  const theme = useTheme()

  if (status === undefined) {
    return (
      <Tooltip title="No data">
        <TableCell
          className="cell-result"
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
        className="cell-result"
        style={{
          textAlign: 'center',
          backgroundColor: 'white',
        }}
      >
        <SeverityIcon status={status} />
      </TableCell>
    )
  }
}

CompReadyCell.propTypes = {
  status: PropTypes.number,
}
