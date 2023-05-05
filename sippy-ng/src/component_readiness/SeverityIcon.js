import './ComponentReadiness.css'
import { Tooltip } from '@material-ui/core'
import { useTheme } from '@material-ui/core/styles'
import green from './green-3.png'
import green_missing_data from './green_no_data.png'
import heart from './green-heart.png'
import PropTypes from 'prop-types'
import React from 'react'
import red from './red-3.png'
import red_3d from './red-3d.png'

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
        src={green_missing_data}
        alt="MissingBasisAndSample"
        width="15px"
        height="15px"
      />
    )
  } else if (status == 1) {
    statusStr = statusStr + 'MissingBasis indicates basis data missing'
    icon = (
      <img
        src={green_missing_data}
        alt="MissingBasisAndSample"
        width="15px"
        height="15px"
      />
    )
  } else if (status == 0) {
    statusStr = statusStr + 'NotSignificant indicates no significant difference'
    icon = <img src={green} alt="NotSignificant" />
  } else if (status == -1) {
    statusStr = statusStr + 'MissingSample indicates sample data missing'
    icon = (
      <img
        src={green_missing_data}
        alt="MissingBasisAndSample"
        width="15px"
        height="15px"
      />
    )
  } else if (status == -2) {
    statusStr = statusStr + 'SignificantRegression shows significant regression'
    icon = <img src={red} alt="SignificantRegression" />
  } else if (status <= -3) {
    statusStr =
      statusStr +
      'ExtremeRegression shows regression with >15% pass rate change'
    icon = <img src={red_3d} alt="ExtremRegressio >15%n" />
  }

  return <Tooltip title={statusStr}>{icon}</Tooltip>
}

SeverityIcon.propTypes = {
  status: PropTypes.number,
}
