import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadinessStyles'
import { generateTestDetailsReportLink } from './CompReadyUtils'
import { Tooltip } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import CompSeverityIcon from './CompSeverityIcon'
import HelpOutlineIcon from '@mui/icons-material/HelpOutline'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import TableCell from '@mui/material/TableCell'

// CompReadyTestCall is for rendering the cells on the right of page4 or page4a
export default function CompReadyTestCell(props) {
  const { columnIndex, status, test } = props
  const theme = useTheme()
  const classes = useContext(ComponentReadinessStyleContext)

  if (status === undefined) {
    return (
      <Tooltip title="No data">
        <TableCell
          sx={classes.crCellResult}
          style={{
            textAlign: 'center',
            backgroundColor: theme.palette.text.disabled,
          }}
        >
          <HelpOutlineIcon style={{ color: theme.palette.text.disabled }} />
        </TableCell>
      </Tooltip>
    )
  }

  const link = test ? generateTestDetailsReportLink(test) : null

  return (
    <TableCell
      sx={classes.crCellResult}
      style={{
        textAlign: 'center',
      }}
    >
      {link ? (
        <a data-testid={`cr-test-details-link-${columnIndex}`} href={link}>
          <CompSeverityIcon status={status} />
        </a>
      ) : (
        <CompSeverityIcon status={status} />
      )}
    </TableCell>
  )
}

CompReadyTestCell.propTypes = {
  columnIndex: PropTypes.number.isRequired,
  status: PropTypes.number.isRequired,
  test: PropTypes.object,
}
