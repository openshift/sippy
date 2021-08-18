import { Button, Menu, MenuItem, Tooltip } from '@material-ui/core'
import { Filter2, Filter7 } from '@material-ui/icons'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function GridToolbarPeriodSelector (props) {
  const periodFilter = `The period to report on. When selecting two days, the
    previous two days are compared with the previous 7 days. A default report typically
    compares the previous 7 days to the prior 7 day period.`

  const [anchor, setAnchor] = React.useState('')
  const [period, setPeriod] = React.useState(props.period)

  const handleClick = (event) => {
    setAnchor(event.currentTarget)
  }

  const handleClose = () => {
    setAnchor(null)
  }

  const selectPeriod = (p) => {
    setPeriod(p)
    props.selectPeriod(p)
    handleClose()
  }

  let periodIcon = <Filter7 />
  if (period === 'twoDay') {
    periodIcon = <Filter2 />
  }

  return (
        <Fragment>
            <Tooltip title={periodFilter}>
                <Button aria-controls="period-menu" aria-haspopup="true" startIcon={periodIcon} color="primary" onClick={handleClick}>
                    Period
                </Button>
            </Tooltip>
            <Menu
                id="period-menu"
                anchorEl={anchor}
                keepMounted
                open={Boolean(anchor)}
                onClose={handleClose}
            >
                <MenuItem style={{ fontWeight: (period === 'twoDay') ? 'normal' : 'bold' }} onClick={() => selectPeriod('default')}>Default</MenuItem>
                <MenuItem style={{ fontWeight: (period === 'twoDay') ? 'bold' : 'normal' }} onClick={() => selectPeriod('twoDay')}>Two Days</MenuItem>
            </Menu>
        </Fragment>
  )
}

GridToolbarPeriodSelector.propTypes = {
  selectPeriod: PropTypes.func.isRequired,
  period: PropTypes.string
}
