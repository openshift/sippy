import './ComponentReadiness.css'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import EmojiEmotionsOutlinedIcon from '@material-ui/icons/EmojiEmotionsOutlined'
import FireplaceIcon from '@material-ui/icons/Fireplace'
import PropTypes from 'prop-types'
import React from 'react'
import RemoveIcon from '@material-ui/icons/Remove'

export default function SeverityIcon(props) {
  const theme = useTheme()
  const status = props.status

  let icon = ''

  let statusStr = status + ': '

  if (status >= 3) {
    statusStr =
      statusStr + 'SignificantImprovement indicates improved sample rate'
    icon = (
      <EmojiEmotionsOutlinedIcon
        data-icon="EmojiEmotionsOutlinedIcon"
        fontSize="large"
        style={{ color: theme.palette.success.main }}
      />
    )
  } else if (status == 2) {
    statusStr =
      statusStr +
      'MissingBasisAndSample indicates basis and sample data missing'
    icon = (
      <RemoveIcon
        data-icon="RemoveIcon"
        fontSize="large"
        style={{ color: theme.palette.grey }}
      />
    )
  } else if (status == 1) {
    statusStr = statusStr + 'MissingBasis indicates basis data missing'
    icon = (
      <RemoveIcon
        data-icon="RemoveIcon"
        fontSize="small"
        style={{ color: theme.palette.success.main }}
      />
    )
  } else if (status == 0) {
    statusStr = statusStr + 'NotSignificant indicates no significant difference'
    icon = (
      <EmojiEmotionsOutlinedIcon
        data-icon="EmojiEmotionsOutlinedIcon"
        fontSize="medium"
        style={{ color: theme.palette.success.main }}
      />
    )
  } else if (status == -1) {
    statusStr = statusStr + 'MissingSample indicates sample data missing'
    icon = (
      <RemoveIcon
        data-icon="RemoveIcon"
        fontSize="small"
        style={{ color: theme.palette.success.main }}
      />
    )
  } else if (status == -2) {
    statusStr = statusStr + 'SignificantRegression shows significant regression'
    icon = (
      <FireplaceIcon
        data-icon="FireplaceIcon"
        fontSize="small"
        style={{
          color: theme.palette.error.main,
        }}
      />
    )
  } else if (status <= -3) {
    statusStr =
      statusStr +
      'ExtremeRegression shows regression with >15% pass rate change'
    icon = (
      <FireplaceIcon
        data-icon="FireplaceIcon"
        fontSize="large"
        style={{ color: theme.palette.error.main }}
      />
    )
  }

  return <Tooltip title={statusStr}>{icon}</Tooltip>
}

SeverityIcon.propTypes = {
  status: PropTypes.number,
}
