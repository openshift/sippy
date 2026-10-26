import { Box } from '@mui/material'
import { ComponentReadinessStyleContext } from './ComponentReadinessStyles'
import { Drawer } from '@mui/material'
import { useTheme } from '@mui/material/styles'
import AccessibilityToggle from '../components/AccessibilityToggle'
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import CompReadyMainInputs from './CompReadyMainInputs'
import IconButton from '@mui/material/IconButton'
import MenuIcon from '@mui/icons-material/Menu'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'

export default function Sidebar({ controlsOpts }) {
  const classes = useContext(ComponentReadinessStyleContext)
  const theme = useTheme()
  const [drawerOpen, setDrawerOpen] = React.useState(true)
  const handleDrawerOpen = () => {
    setDrawerOpen(true)
  }

  const handleDrawerClose = () => {
    setDrawerOpen(false)
  }

  return (
    <div>
      <IconButton
        color="inherit"
        aria-label="open drawer"
        onClick={handleDrawerOpen}
        edge="start"
        sx={[classes.menuButton, drawerOpen && classes.hide]}
        size="large"
      >
        <MenuIcon />
      </IconButton>
      <Drawer
        sx={classes.drawer}
        variant="persistent"
        anchor="left"
        open={drawerOpen}
        slotProps={{ paper: { sx: classes.drawerPaper } }}
      >
        <Box sx={classes.drawerHeader}>
          <AccessibilityToggle />
          <IconButton onClick={handleDrawerClose} size="large">
            {theme.direction === 'ltr' ? (
              <ChevronLeftIcon />
            ) : (
              <ChevronRightIcon />
            )}
          </IconButton>
        </Box>
        <CompReadyMainInputs controlsOpts={controlsOpts} />
      </Drawer>
    </div>
  )
}

Sidebar.propTypes = {
  // object passing booleans for conditional display of controls; currently includes:
  //   filterByCapabilities: whether to show the option to filter tests by capabilities (only makes sense before drilling down to a capability)
  //   isTestDetails: whether we are in the Test Details page, where most options are already determined
  controlsOpts: PropTypes.object,
}
