import './ComponentReadiness.css'
import { getStatusAndIcon } from './CompReadyUtils'
import { Tooltip } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import PropTypes from 'prop-types'
import React from 'react'

export default function CompSeverityIcon(props) {
  const theme = useTheme()
  const { status, grayFactor } = props

  const [statusStr, icon] = getStatusAndIcon(status, grayFactor)
  return <Tooltip title={statusStr}>{icon}</Tooltip>
}

CompSeverityIcon.propTypes = {
  status: PropTypes.number,
  grayFactor: PropTypes.number,
}
