import './ComponentReadiness.css'
import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { CompReadyVarsContext } from './CompReadyVars'
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
  const { status, environment, filterVals, regressedTest } = props
  const theme = useTheme()
  const classes = useContext(ComponentReadinessStyleContext)

  const { expandEnvironment } = useContext(CompReadyVarsContext)

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
    console.log('regressedTest', regressedTest)
    return (
      <TableCell
        className={classes.crCellResult}
        style={{
          textAlign: 'center',
        }}
      >
        <a
          href={generateTestDetailsReportLink(
            regressedTest,
            filterVals,
            expandEnvironment
          )}
        >
          <CompSeverityIcon status={status} />
        </a>
      </TableCell>
    )
  }
}

CompReadyTestCell.propTypes = {
  status: PropTypes.number.isRequired,
  environment: PropTypes.string.isRequired,
  filterVals: PropTypes.string.isRequired,
  regressedTest: PropTypes.object.isRequired,
}
