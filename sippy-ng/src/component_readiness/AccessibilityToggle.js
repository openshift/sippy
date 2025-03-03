import { Accessibility } from '@mui/icons-material'
import { AccessibilityModeContext } from '../App'
import { Tooltip } from '@mui/material'
import IconButton from '@mui/material/IconButton'
import React, { useContext } from 'react'

export default function AccessibilityToggle() {
  const accessibilityMode = useContext(AccessibilityModeContext)

  return (
    <Tooltip
      title={
        accessibilityMode.accessibilityModeOn
          ? 'Toggle accessibility mode off'
          : 'Toggle accessibility mode on'
      }
    >
      <IconButton
        sx={{ ml: 1 }}
        onClick={accessibilityMode.toggleAccessibilityMode}
        color="inherit"
      >
        {<Accessibility />}
      </IconButton>
    </Tooltip>
  )
}
