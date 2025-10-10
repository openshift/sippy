import { ComponentReadinessStyleContext } from './ComponentReadiness'
import { Drawer } from '@mui/material'
import { useTheme } from '@mui/styles'
import AccessibilityToggle from '../components/AccessibilityToggle'
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import clsx from 'clsx'
import CompReadyMainInputs from './CompReadyMainInputs'
import IconButton from '@mui/material/IconButton'
import MenuIcon from '@mui/icons-material/Menu'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'

export default function Sidebar({ filterByCapabilities, isTestDetails }) {
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
        className={clsx(classes.menuButton, drawerOpen && classes.hide)}
        size="large"
      >
        <MenuIcon />
      </IconButton>
      <Drawer
        className={classes.drawer}
        variant="persistent"
        anchor="left"
        open={drawerOpen}
        classes={{
          paper: classes.drawerPaper,
        }}
      >
        <div className={classes.drawerHeader}>
          <AccessibilityToggle />
          <IconButton onClick={handleDrawerClose} size="large">
            {theme.direction === 'ltr' ? (
              <ChevronLeftIcon />
            ) : (
              <ChevronRightIcon />
            )}
          </IconButton>
        </div>
        <CompReadyMainInputs
          isTestDetails={isTestDetails}
          filterByCapabilities={filterByCapabilities}
        />
      </Drawer>
    </div>
  )
}

Sidebar.propTypes = {
  filterByCapabilities: PropTypes.bool, // only makes sense in CR when not already drilled down to a single capability
  isTestDetails: PropTypes.bool,
}
