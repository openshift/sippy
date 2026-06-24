import './SippyLogo.css'
import { makeStyles } from '@mui/styles'
import fifaLogo from '../sippy-fifa.svg'
import logo from '../sippy.svg'
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

export function isFifaWorldCup() {
  const now = new Date()
  const start = new Date(2026, 5, 11)
  const end = new Date(2026, 6, 20)
  return now >= start && now < end
}

export default function SippyLogo() {
  const classes = useStyles()
  const [anchorEl, setAnchorEl] = React.useState(null)
  const isFifa = isFifaWorldCup()

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
          src={isFifa ? fifaLogo : logo}
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
          {isFifa ? (
            <>
              GOOOAL! I&apos;m Sippy, celebrating
              <br />
              the 2026 FIFA World Cup! &#9917;
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
