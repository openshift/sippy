import { Bookmark } from '@material-ui/icons'
import { Button, Icon, Menu, MenuItem, Tooltip } from '@material-ui/core'
import CheckIcon from '@material-ui/icons/Check'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function GridToolbarQueriesMenu(props) {
  const [anchorEl, setAnchorEl] = React.useState(null)
  const [selectedFilters, setSelectedFilters] = React.useState(
    props.initialFilters
  )

  const handleClick = (theEvent) => {
    setAnchorEl(theEvent.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const selectFilter = (theName, conflicts) => {
    let newFilters = []

    if (selectedFilters.includes(theName)) {
      newFilters = selectedFilters.filter((e) => e !== theName)
    } else if (theName === 'all') {
      newFilters = []
    } else {
      newFilters = selectedFilters
        .filter((e) => e !== 'all' && e !== conflicts)
        .concat(theName)
    }

    props.setFilters(newFilters)
    setSelectedFilters(newFilters)
    handleClose()
  }

  const menuItems = () => {
    const filters = []
    for (const [, filter] of props.allowedFilters.entries()) {
      filters.push(
        <MenuItem
          key={'filter-' + filter.filter}
          selected={selectedFilters.includes(filter.filter)}
          onClick={() => selectFilter(filter.filter, filter.conflictsWith)}
        >
          {selectedFilters.includes(filter.filter) ? (
            <CheckIcon fontSize="small" />
          ) : (
            <Icon />
          )}
          &nbsp;{filter.title}
        </MenuItem>
      )
    }
    return filters
  }

  return (
    <Fragment>
      <Tooltip title="Bookmarks are saved filters, you can select more than one.">
        <Button
          aria-controls="reports-menu"
          aria-haspopup="true"
          onClick={handleClick}
          startIcon={<Bookmark />}
          color="primary"
        >
          Bookmarks
        </Button>
      </Tooltip>
      <Tooltip title="Click to clear any selected bookmarks.">
        <Button color="secondary" onClick={() => selectFilter('all')}>
          {selectedFilters.join(', ')}
        </Button>
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

GridToolbarQueriesMenu.defaultProps = {
  initialFilters: [],
  allowedFilters: [],
}

GridToolbarQueriesMenu.propTypes = {
  initialFilters: PropTypes.array,
  allowedFilters: PropTypes.arrayOf(
    PropTypes.shape({
      title: PropTypes.string,
      conflictsWith: PropTypes.string,
      filter: PropTypes.string,
    })
  ),

  setFilters: PropTypes.func.isRequired,
  classes: PropTypes.object,
}
