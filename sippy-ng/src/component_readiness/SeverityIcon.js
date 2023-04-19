import './ComponentReadiness.css'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import EmojiEmotionsOutlinedIcon from '@material-ui/icons/EmojiEmotionsOutlined'
import FireplaceIcon from '@material-ui/icons/Fireplace'
import PropTypes from 'prop-types'
import React from 'react'

export default function SeverityIcon(props) {
  const theme = useTheme()
  const status = props.status

  let icon = ''

  if (status > 8) {
    icon = (
      <EmojiEmotionsOutlinedIcon
        data-icon="EmojiEmotionsOutlinedIcon"
        fontSize="large"
        style={{ color: theme.palette.success.main }}
      />
    )
  } else if (status > 5) {
    icon = (
      <FireplaceIcon
        data-icon="FireplaceIcon"
        fontSize="small"
        style={{
          color: theme.palette.error.main,
        }}
      />
    )
  } else {
    icon = (
      <FireplaceIcon
        data-icon="FireplaceIcon"
        fontSize="large"
        style={{ color: theme.palette.error.main }}
      />
    )
  }

  return <Tooltip title={status}>{icon}</Tooltip>
}

SeverityIcon.propTypes = {
  status: PropTypes.number,
}
