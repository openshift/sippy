import { ExpandLess as ExpandLessIcon } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import { Paper, Typography } from '@mui/material'
import { useWebSocketActions } from './store/useChatStore'
import ChatInterface from './ChatInterface'
import PropTypes from 'prop-types'
import React from 'react'
import sippyLogo from '../sippy.svg'

const useStyles = makeStyles((theme) => ({
  collapsedTab: {
    position: 'fixed',
    right: theme.spacing(3),
    bottom: 0,
    zIndex: theme.zIndex.drawer - 1,
    backgroundColor: theme.palette.background.paper,
    borderTopLeftRadius: theme.shape.borderRadius * 2,
    borderTopRightRadius: theme.shape.borderRadius * 2,
    borderTop: `2px solid ${theme.palette.divider}`,
    borderLeft: `1px solid ${theme.palette.divider}`,
    borderRight: `1px solid ${theme.palette.divider}`,
    boxShadow: theme.shadows[8],
    cursor: 'pointer',
    transition: 'all 0.3s ease',
    '&:hover': {
      boxShadow: theme.shadows[10],
      bottom: 0,
    },
  },
  collapsedContent: {
    display: 'flex',
    flexDirection: 'row',
    alignItems: 'center',
    padding: theme.spacing(1, 2),
    gap: theme.spacing(1.5),
  },
  horizontalText: {
    fontSize: '0.875rem',
    fontWeight: 500,
    color: theme.palette.text.primary,
    whiteSpace: 'nowrap',
  },
  sippyLogo: {
    width: 32,
    height: 32,
  },
}))

export default function CollapsibleChatDrawer({ open, onOpen, onClose }) {
  const classes = useStyles()
  const { connectWebSocket } = useWebSocketActions()

  const handleOpen = (e) => {
    e.preventDefault()
    e.stopPropagation()
    // Connect websocket when drawer is opened
    connectWebSocket()
    onOpen()
  }

  return (
    <>
      {/* Collapsed tab - shown when drawer is closed */}
      {!open && (
        <Paper
          className={classes.collapsedTab}
          onClick={handleOpen}
          elevation={3}
        >
          <div className={classes.collapsedContent}>
            <img src={sippyLogo} alt="Sippy" className={classes.sippyLogo} />
            <Typography className={classes.horizontalText}>
              Chat with Sippy
            </Typography>
            <ExpandLessIcon fontSize="small" />
          </div>
        </Paper>
      )}

      {/* Full drawer - shown when open */}
      <ChatInterface mode="drawer" open={open} onClose={onClose} />
    </>
  )
}

CollapsibleChatDrawer.propTypes = {
  open: PropTypes.bool.isRequired,
  onOpen: PropTypes.func.isRequired,
  onClose: PropTypes.func.isRequired,
}
