import { Tooltip } from '@mui/material'
import ArrowDownwardRoundedIcon from '@mui/icons-material/ArrowDownwardRounded'
import ArrowUpwardRoundedIcon from '@mui/icons-material/ArrowUpwardRounded'
import PropTypes from 'prop-types'
import React from 'react'
import SyncAltRoundedIcon from '@mui/icons-material/SyncAltRounded'

/**
 * PassRateIcon returns an up, down, or side-to-side arrows
 * indicating whether something improved, regressed, or stayed
 * the same.
 */
export default function PassRateIcon({
  inverted = false,
  tooltip = false,
  ...props
}) {
  let icon = ''

  if (Math.abs(props.improvement) <= 2) {
    icon = (
      <SyncAltRoundedIcon
        data-icon="SyncAltRoundedIcon"
        style={{ color: 'grey' }}
      />
    )
  } else if (props.improvement >= 2) {
    icon = (
      <ArrowUpwardRoundedIcon
        data-icon="ArrowUpwardRoundedIcon"
        style={{
          stroke: inverted ? 'darkred' : 'green',
          strokeWidth: 3,
          color: inverted ? 'darkred' : 'green',
        }}
      />
    )
  } else {
    icon = (
      <ArrowDownwardRoundedIcon
        data-icon="ArrowDownwardRoundedIcon"
        style={{
          stroke: inverted ? 'green' : 'darkred',
          strokeWidth: 3,
          color: inverted ? 'green' : 'darkred',
        }}
      />
    )
  }

  if (tooltip) {
    return <Tooltip title={props.improvement.toFixed(2) + '%'}>{icon}</Tooltip>
  } else {
    return icon
  }
}

PassRateIcon.propTypes = {
  improvement: PropTypes.number,
  inverted: PropTypes.bool,
  tooltip: PropTypes.bool,
}
