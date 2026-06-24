import './SippyLogo.css'
import { getSeasonalLogo, isWorldCup } from './seasonalLogo'
import { makeStyles } from '@mui/styles'
import Popover from '@mui/material/Popover'
import React from 'react'
import Typography from '@mui/material/Typography'

const useStyles = makeStyles((theme) => ({
  popover: {
    pointerEvents: 'none',
  },
  paper: {
    padding: theme.spacing(1),
  },
}))

export default function SippyLogo() {
  const classes = useStyles()
  const logo = getSeasonalLogo()
  const [anchorEl, setAnchorEl] = React.useState(null)

  const handlePopoverOpen = (event) => {
    setAnchorEl(event.currentTarget)
  }

  const handlePopoverClose = () => {
    setAnchorEl(null)
  }

  const open = Boolean(anchorEl)

  return (
    <div align="center">
      <Typography
        aria-owns={open ? 'mouse-over-popover' : undefined}
        aria-haspopup="true"
        onMouseEnter={handlePopoverOpen}
        onMouseLeave={handlePopoverClose}
      >
        <img
          className="Sippy-logo"
          src={logo}
          alt="CIPI (Continuous Integration Private Investigator) aka Sippy."
        />
      </Typography>
      <Popover
        id="mouse-over-popover"
        className={classes.popover}
        classes={{
          paper: classes.paper,
        }}
        open={open}
        anchorEl={anchorEl}
        anchorOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        onClose={handlePopoverClose}
        disableRestoreFocus
        disableScrollLock
      >
        <Typography>
          {isWorldCup() ? (
            <>
              GOOOAL! I&apos;m Sippy, your CI
              <br />
              Private Investigator, cheering
              <br />
              on the 2026 FIFA World Cup!
            </>
          ) : (
            <>
              Hi, I&apos;m Sippy! The Continuous Integration
              <br />
              Private Investigator (CIPI).
            </>
          )}
        </Typography>
      </Popover>
    </div>
  )
}
