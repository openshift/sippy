import './ComponentReadiness.css'
import { getStatusAndIcon } from './CompReadyUtils'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import PropTypes from 'prop-types'
import React from 'react'

export default function CompSeverityIcon(props) {
  const theme = useTheme()
  const status = props.status

  const [statusStr, icon] = getStatusAndIcon(status)
  return <Tooltip title={statusStr}>{icon}</Tooltip>
}

CompSeverityIcon.propTypes = {
  status: PropTypes.number,
}
