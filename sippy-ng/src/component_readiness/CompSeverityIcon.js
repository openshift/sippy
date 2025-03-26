import './ComponentReadiness.css'
import { AccessibilityModeContext } from '../components/AccessibilityModeProvider'
import { Badge, Tooltip } from '@mui/material'
import { getStatusAndIcon } from './CompReadyUtils'
import { useTheme } from '@mui/material/styles'
import { withStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'

export default function CompSeverityIcon(props) {
  const theme = useTheme()
  const { accessibilityModeOn } = useContext(AccessibilityModeContext)
  const { explanations, status, grayFactor, count } = props

  const [statusStr, icon] = getStatusAndIcon(
    status,
    grayFactor,
    accessibilityModeOn
  )

  let toolTip = ''
  if (explanations !== undefined) {
    toolTip = explanations.join(' ')
  }

  const StyledBadge = withStyles((theme) => ({
    badge: {
      height: 12,
      maxHeight: 12,
      minHeight: 12,
      width: 12,
      maxWidth: 12,
      minWidth: 12,
    },
  }))(Badge)
  return (
    <div>
      {status < -1 && count > 1 ? (
        <StyledBadge badgeContent={count} color="error">
          <Tooltip title={toolTip}>{icon}</Tooltip>
        </StyledBadge>
      ) : (
        <Tooltip title={toolTip}>{icon}</Tooltip>
      )}
    </div>
  )
}

CompSeverityIcon.propTypes = {
  status: PropTypes.number,
  explanations: PropTypes.array,
  grayFactor: PropTypes.number,
  count: PropTypes.number,
}
