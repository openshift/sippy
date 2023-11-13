import './SippyLogo.css'
import { styled } from '@mui/material/styles';
import { makeStyles } from '@mui/material/styles'
import logo from '../sippy.svg'
import Popover from '@mui/material/Popover'
import React from 'react'
import Typography from '@mui/material/Typography'

const PREFIX = 'SippyLogo';

const classes = {
  popover: `${PREFIX}-popover`,
  paper: `${PREFIX}-paper`
};

const Root = styled('div')((
  {
    theme
  }
) => ({
  [`& .${classes.popover}`]: {
    pointerEvents: 'none',
  },

  [`& .${classes.paper}`]: {
    padding: theme.spacing(1),
  }
}));

export default function SippyLogo() {

  const [anchorEl, setAnchorEl] = React.useState(null)

  const handlePopoverOpen = (event) => {
    setAnchorEl(event.currentTarget)
  }

  const handlePopoverClose = () => {
    setAnchorEl(null)
  }

  const open = Boolean(anchorEl)

  return (
    <Root align="center">
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
        <br />
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
      >
        <Typography>
          Hi, I&apos;m Sippy! The Continuous Integration
          <br />
          Private Investigator (CIPI).
        </Typography>
      </Popover>
    </Root>
  );
}
