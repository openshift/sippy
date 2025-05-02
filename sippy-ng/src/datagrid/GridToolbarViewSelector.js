import { Button, Menu, MenuItem } from '@mui/material'
import { ViewCarousel } from '@mui/icons-material'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function GridToolbarViewSelector(props) {
  const [anchor, setAnchor] = React.useState('')

  const handleClick = (event) => {
    setAnchor(event.currentTarget)
  }

  const handleClose = () => {
    setAnchor(null)
  }

  return (
    <Fragment>
      <Button
        aria-controls="view-menu"
        aria-haspopup="true"
        startIcon={<ViewCarousel />}
        color="primary"
        onClick={handleClick}
      >
        {props.view} View
      </Button>
      <Menu
        id="view-menu"
        anchorEl={anchor}
        keepMounted
        open={Boolean(anchor)}
        onClose={handleClose}
      >
        {Object.entries(props.views).map(([e, v]) => (
          <MenuItem
            key={e}
            style={{
              fontWeight: props.view === e ? 'bold' : 'normal',
            }}
            onClick={() => {
              props.setView(e)
              handleClose()
            }}
          >
            {e}
          </MenuItem>
        ))}
      </Menu>
    </Fragment>
  )
}

GridToolbarViewSelector.propTypes = {
  view: PropTypes.string,
  views: PropTypes.object,
  setView: PropTypes.func.isRequired,
}
