import {
  AddCircleOutline as AddIcon,
  CallSplit as CallSplitIcon,
  Chat as ChatIcon,
  Close as CloseIcon,
  FiberManualRecord as DotIcon,
  Link as LinkIcon,
  List as ListIcon,
  Share as ShareIcon,
} from '@mui/icons-material'
import {
  Box,
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  formatSessionTime,
  getSessionIconName,
  getSessionTitle,
} from './useChatSessions'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

const useStyles = makeStyles((theme) => ({
  menuPaper: {
    maxHeight: 400,
    width: 350,
  },
  sessionItem: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingRight: theme.spacing(1),
  },
  sessionInfo: {
    flex: 1,
    minWidth: 0,
    marginRight: theme.spacing(1),
  },
  sessionTitle: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  sessionMeta: {
    display: 'flex',
    gap: theme.spacing(1),
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
  },
  activeIndicator: {
    width: 4,
    height: 4,
    borderRadius: '50%',
    backgroundColor: theme.palette.primary.main,
    marginRight: theme.spacing(1),
    flexShrink: 0,
  },
  newChatItem: {
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  divider: {
    margin: theme.spacing(0.5, 0),
  },
  deleteButton: {
    padding: theme.spacing(0.5),
    '&:hover': {
      color: theme.palette.error.main,
    },
  },
  emptyState: {
    padding: theme.spacing(2),
    textAlign: 'center',
    color: theme.palette.text.secondary,
  },
}))

export default function SessionDropdown({
  sessions,
  activeSessionId,
  onSelectSession,
  onNewSession,
  onDeleteSession,
  disabled = false,
}) {
  const classes = useStyles()
  const [anchorEl, setAnchorEl] = useState(null)
  const open = Boolean(anchorEl)

  const handleClick = (event) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const handleSelectSession = (sessionId) => {
    onSelectSession(sessionId)
    handleClose()
  }

  const handleNewSession = () => {
    onNewSession()
    handleClose()
  }

  const handleDeleteSession = (event, sessionId) => {
    event.stopPropagation()
    onDeleteSession(sessionId)
  }

  // Sort sessions by updated time (most recent first)
  const sortedSessions = [...sessions].sort(
    (a, b) => new Date(b.updatedAt) - new Date(a.updatedAt)
  )

  // Get icon component based on session type
  const getIconComponent = (sessionType) => {
    const iconName = getSessionIconName(sessionType)
    switch (iconName) {
      case 'Link':
        return <LinkIcon fontSize="small" />
      case 'Share':
        return <ShareIcon fontSize="small" />
      case 'CallSplit':
        return <CallSplitIcon fontSize="small" />
      default:
        return <ChatIcon fontSize="small" />
    }
  }

  return (
    <>
      <Tooltip title="Chat sessions">
        <IconButton size="small" onClick={handleClick} disabled={disabled}>
          <ListIcon />
        </IconButton>
      </Tooltip>

      <Menu
        anchorEl={anchorEl}
        open={open}
        onClose={handleClose}
        classes={{ paper: classes.menuPaper }}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        disableScrollLock
      >
        <MenuItem
          onClick={handleNewSession}
          className={classes.newChatItem}
          sx={{ fontWeight: 600 }}
        >
          <ListItemIcon>
            <AddIcon />
          </ListItemIcon>
          <ListItemText primary="New Chat" />
        </MenuItem>

        {sortedSessions.length === 0 ? (
          <Box className={classes.emptyState}>
            <Typography variant="body2">No chat sessions</Typography>
          </Box>
        ) : (
          sortedSessions.map((session) => {
            const isActive = session.id === activeSessionId
            const messageCount = session.messages?.length || 0

            return (
              <MenuItem
                key={session.id}
                onClick={() => handleSelectSession(session.id)}
                selected={isActive}
                className={classes.sessionItem}
              >
                <ListItemIcon sx={{ minWidth: 36 }}>
                  {getIconComponent(session.type)}
                </ListItemIcon>

                <Box className={classes.sessionInfo}>
                  <ListItemText
                    primary={getSessionTitle(session)}
                    primaryTypographyProps={{
                      className: classes.sessionTitle,
                      variant: 'body2',
                    }}
                  />
                  <Box className={classes.sessionMeta}>
                    <span>{formatSessionTime(session.updatedAt)}</span>
                    <span>•</span>
                    <span>
                      {messageCount} msg{messageCount !== 1 ? 's' : ''}
                    </span>
                  </Box>
                </Box>

                {isActive && (
                  <DotIcon
                    sx={{
                      fontSize: 12,
                      color: 'primary.main',
                      marginRight: 1,
                    }}
                  />
                )}

                <IconButton
                  className={classes.deleteButton}
                  size="small"
                  onClick={(e) => handleDeleteSession(e, session.id)}
                  aria-label="Remove from list"
                >
                  <CloseIcon fontSize="small" />
                </IconButton>
              </MenuItem>
            )
          })
        )}
      </Menu>
    </>
  )
}

SessionDropdown.propTypes = {
  sessions: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.string.isRequired,
      type: PropTypes.string.isRequired,
      messages: PropTypes.array,
      updatedAt: PropTypes.string,
    })
  ).isRequired,
  activeSessionId: PropTypes.string,
  onSelectSession: PropTypes.func.isRequired,
  onNewSession: PropTypes.func.isRequired,
  onDeleteSession: PropTypes.func.isRequired,
  disabled: PropTypes.bool,
}
