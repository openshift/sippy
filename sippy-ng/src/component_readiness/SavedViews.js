import { Button, Menu, MenuItem, Tooltip } from '@mui/material'
import { ViewCarousel } from '@mui/icons-material'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function SavedViews(props) {
  const [anchor, setAnchor] = React.useState('')
  const [buttonName, setButtonName] = React.useState(props.view)

  const setViewParam = props.setViewParam

  setViewParam(buttonName)
  const handleClick = (event) => {
    setAnchor(event.currentTarget)
    setViewParam(buttonName)
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
        View: {buttonName}
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
              props.applyView(e)
              setButtonName(e)
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

SavedViews.propTypes = {
  view: PropTypes.string,
  views: PropTypes.object,
  applyView: PropTypes.func.isRequired,
  viewParam: PropTypes.string.isRequired,
  setViewParam: PropTypes.func.isRequired,
}
