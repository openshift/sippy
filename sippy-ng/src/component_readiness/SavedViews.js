import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Menu,
  MenuItem,
  TextField,
  Tooltip,
} from '@mui/material'
import { CompReadyVarsContext } from './CompReadyVars'
import { Save, ViewCarousel } from '@mui/icons-material'
import DeleteIcon from '@mui/icons-material/Delete'
import IconButton from '@mui/material/IconButton'
import PropTypes from 'prop-types'
import React, { Fragment, useContext, useState } from 'react'

export default function SavedViews(props) {
  const [anchor, setAnchor] = React.useState('')
  const [buttonName, setButtonName] = React.useState(props.view)

  const setViewParam = props.setViewParam
  const views = props.views
  const maxSavedViewLength = 15
  const setViews = props.setViews

  setViewParam(buttonName)
  const handleClick = (event) => {
    setAnchor(event.currentTarget)
    setViewParam(buttonName)
  }

  const handleClose = () => {
    setAnchor(null)
  }

  const [saveDialogIsOpen, setSaveDialogIsOpen] = useState(false)
  const [newViewName, setNewViewName] = useState('')
  const [newViewDescription, setNewViewDescription] = useState('')

  const [deleteDialogIsOpen, setDeleteDialogIsOpen] = useState(false)
  const [viewToDelete, setViewToDelete] = useState(null)

  const handleOpenSaveDialog = () => {
    setSaveDialogIsOpen(true)
  }

  const handleCloseSaveDialog = () => {
    setSaveDialogIsOpen(false)
  }

  const varsContext = useContext(CompReadyVarsContext)

  const handleSaveView = () => {
    if (newViewName.length > 0) {
      const newViewConfig = {
        config: {
          class: 'user',
          help: newViewDescription,
          'Group By': varsContext.groupByCheckedItems,
          'Exclude Arches': varsContext.excludeArchesCheckedItems,
          'Exclude Networks': varsContext.excludeNetworksCheckedItems,
          'Exclude Clouds': varsContext.excludeCloudsCheckedItems,
          'Exclude Upgrades': varsContext.excludeUpgradesCheckedItems,
          'Exclude Variants': varsContext.excludeVariantsCheckedItems,
          Confidence: varsContext.confidence,
          Pity: varsContext.pity,
          'Min Fail': varsContext.minFail,
          'Ignore Missing': varsContext.ignoreMissing,
          'Ignore Disruption': varsContext.ignoreDisruption,
        },
      }

      // Append the new view to the existing views.
      const newViews = {
        ...views,
        [newViewName]: newViewConfig,
      }
      setViews(newViews)

      // Update on save.
      varsContext.saveViewsToLocal(newViews)
      handleCloseSaveDialog()
    }
  }

  const openDeleteDialog = (viewName) => {
    setViewToDelete(viewName)
    setDeleteDialogIsOpen(true)
  }

  const closeDeleteDialog = () => {
    setDeleteDialogIsOpen(false)
    setViewToDelete(null)
  }

  const userViewDelete = () => {
    const newViews = { ...views }
    delete newViews[viewToDelete]
    setViews(newViews)

    // Update on delete.
    varsContext.saveViewsToLocal(newViews)

    // User deleted their current view, so switch to Default.
    if (viewToDelete === buttonName) {
      setButtonName(Object.keys(newViews)[0])
      setViewParam(Object.keys(newViews)[0])
    }
    closeDeleteDialog()
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
        <Tooltip title={views[buttonName].config.help}>
          View: {buttonName}
        </Tooltip>
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
            <Tooltip title={v.config.help}>{e}</Tooltip>
            {/* only user created views can be deleted */}
            {v.config.class && v.config.class == 'user' ? (
              <IconButton
                edge="end"
                aria-label="delete"
                onClick={(event) => {
                  // Don't fire onclick.
                  event.stopPropagation()
                  openDeleteDialog(e)
                }}
              >
                <Tooltip title="Delete View">
                  <DeleteIcon />
                </Tooltip>
              </IconButton>
            ) : (
              ''
            )}
          </MenuItem>
        ))}
      </Menu>

      <Button
        color="primary"
        startIcon={<Save />}
        onClick={handleOpenSaveDialog}
      >
        Save Current View
      </Button>

      <Dialog open={saveDialogIsOpen} onClose={handleCloseSaveDialog}>
        <DialogTitle>Save Current View</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Name for new view (max {maxSavedViewLength} characters)
          </DialogContentText>
          <TextField
            autoFocus
            margin="dense"
            id="view-name"
            label="New View Name"
            type="text"
            fullWidth
            inputProps={{ maxLength: maxSavedViewLength }}
            value={newViewName}
            onChange={(event) => setNewViewName(event.target.value)}
          />
          <TextField
            margin="dense"
            id="view-description"
            label="Description"
            type="text"
            fullWidth
            multiline
            rows={4}
            value={newViewDescription}
            onChange={(event) => setNewViewDescription(event.target.value)}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseSaveDialog} color="primary">
            Cancel
          </Button>
          <Button onClick={handleSaveView} color="primary">
            Save
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog
        open={deleteDialogIsOpen}
        onClose={closeDeleteDialog}
        aria-labelledby="alert-dialog-title"
        aria-describedby="alert-dialog-description"
      >
        <DialogTitle id="alert-dialog-title">{`About to delete view: ${viewToDelete}`}</DialogTitle>
        <DialogContent>
          <DialogContentText id="alert-dialog-description">
            Are you sure?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={closeDeleteDialog} color="primary">
            Cancel
          </Button>
          <Button onClick={userViewDelete} color="primary" autoFocus>
            Yes
          </Button>
        </DialogActions>
      </Dialog>
    </Fragment>
  )
}

SavedViews.propTypes = {
  view: PropTypes.string,
  views: PropTypes.object,
  setViews: PropTypes.func.isRequired,
  applyView: PropTypes.func.isRequired,
  viewParam: PropTypes.string.isRequired,
  setViewParam: PropTypes.func.isRequired,
}
