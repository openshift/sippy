import './ComponentReadiness.css'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import EmojiEmotionsOutlinedIcon from '@material-ui/icons/EmojiEmotionsOutlined'
import FireplaceIcon from '@material-ui/icons/Fireplace'
import green from './green-3.png'
import heart from './green-heart.png'
import PropTypes from 'prop-types'
import question from './red-question-mark.png'
import React from 'react'
import red from './red-3.png'
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
      <img
        src={heart}
        width="20px"
        height="20px"
        alt="SignificantImprovement"
      />
    )
  } else if (status == 2) {
    statusStr =
      statusStr +
      'MissingBasisAndSample indicates basis and sample data missing'
    icon = (
      <img
        src={question}
        alt="MissingBasisAndSample"
        width="20px"
        height="20px"
      />
    )
  } else if (status == 1) {
    statusStr = statusStr + 'MissingBasis indicates basis data missing'
    icon = (
      <img
        src={question}
        alt="MissingBasisAndSample"
        width="20px"
        height="20px"
      />
    )
  } else if (status == 0) {
    statusStr = statusStr + 'NotSignificant indicates no significant difference'
    icon = <img src={green} alt="NotSignificant" />
  } else if (status == -1) {
    statusStr = statusStr + 'MissingSample indicates sample data missing'
    icon = (
      <img
        src={question}
        alt="MissingBasisAndSample"
        width="20px"
        height="20px"
      />
    )
  } else if (status == -2) {
    statusStr = statusStr + 'SignificantRegression shows significant regression'
    icon = <img src={red} alt="SignificantRegression" />
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
