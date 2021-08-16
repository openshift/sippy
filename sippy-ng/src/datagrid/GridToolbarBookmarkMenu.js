import { Button, Menu, MenuItem, Tooltip } from '@material-ui/core'
import { Bookmark } from '@material-ui/icons'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function GridToolbarBookmarkMenu (props) {
  const [anchorEl, setAnchorEl] = React.useState(null)

  const handleClick = (event) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const selectFilter = (bookmark) => {
    props.setFilterModel(bookmark)
    handleClose()
  }

  const menuItems = () => {
    const filters = []
    for (const [, filter] of props.bookmarks.entries()) {
      filters.push(
        <MenuItem key={'filter-' + filter.name}
          onClick={() => selectFilter(filter.model)}>
            {filter.name}
        </MenuItem>)
    }
    return filters
  }

  return (
        <Fragment>
            <Tooltip title="Bookmarks are saved filters.">
                <Button aria-controls="reports-menu" aria-haspopup="true" onClick={handleClick} startIcon={<Bookmark/>}
                        color="primary">Bookmarks</Button>
            </Tooltip>
            <Menu
                id="reports-menu"
                anchorEl={anchorEl}
                keepMounted
                open={Boolean(anchorEl)}
                onClose={handleClose}
            >

                {menuItems()}
            </Menu>
        </Fragment>
  )
}

GridToolbarBookmarkMenu.defaultProps = {
  initialFilters: [],
  allowedFilters: []
}

GridToolbarBookmarkMenu.propTypes = {
  bookmarks: PropTypes.arrayOf(PropTypes.shape({
    name: PropTypes.string,
    model: PropTypes.array
  })),
  setFilterModel: PropTypes.func.isRequired,
  classes: PropTypes.object
}
