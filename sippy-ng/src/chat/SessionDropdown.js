import {
  Box,
  Button,
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  CallSplit as CallSplitIcon,
  Chat as ChatIcon,
  Close as CloseIcon,
  FiberManualRecord as DotIcon,
  ExpandMore as ExpandMoreIcon,
  List as ListIcon,
  NoteAdd as NoteAddIcon,
  Person as PersonIcon,
  Share as ShareIcon,
} from '@mui/icons-material'
import { CONNECTION_STATES } from './store/webSocketSlice'
import { getSessionIconName, getSessionTitle } from './sessionUtils'
import { makeStyles } from '@mui/styles'
import { MESSAGE_TYPES } from './chatUtils'
import { relativeTime } from '../helpers'
import {
  useConnectionState,
  useSessionActions,
  useSessionState,
} from './store/useChatStore'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

const useStyles = makeStyles((theme) => ({
  menuPaper: {
    maxHeight: 400,
    width: 350,
  },
  dropdownButton: {
    textTransform: 'none',
    maxWidth: '200px',
    color: theme.palette.text.primary,
  },
  dropdownButtonCompact: {
    minWidth: 'auto',
    maxWidth: 'none',
  },
  buttonLabel: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    textAlign: 'left',
    flex: 1,
  },
  sessionInfo: {
    flex: 1,
    minWidth: 0,
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
}))

export default function SessionManager({ onNewSession, mode = 'fullPage' }) {
  const classes = useStyles()
  const [anchorEl, setAnchorEl] = useState(null)
  const open = Boolean(anchorEl)

  // Get state and actions from custom hooks
  const { sessions, activeSessionId } = useSessionState()
  const { switchSession, startNewSession, deleteSession } = useSessionActions()
  const { connectionState, isTyping } = useConnectionState()

  const isConnected = connectionState === CONNECTION_STATES.CONNECTED
  const disabled = !isConnected || isTyping

  const handleClick = (event) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const handleNewSession = () => {
    // Start new session (find empty or create) - automatically ensures empty session
    startNewSession()

    // Call side-effect callback (for clearing shared URL, etc.)
    if (onNewSession) {
      onNewSession()
    }
  }

  const handleSelectSession = (sessionId) => {
    switchSession(sessionId)

    // Sync URL in fullPage mode
    if (mode === 'fullPage') {
      const targetSession = sessions.find((s) => s.id === sessionId)
      if (targetSession && targetSession.sharedId) {
        window.history.pushState(
          null,
          '',
          `/sippy-ng/chat/${targetSession.sharedId}`
        )
      } else {
        window.history.pushState(null, '', '/sippy-ng/chat')
      }
    }

    handleClose()
  }

  const handleDeleteSession = (event, sessionId) => {
    event.stopPropagation()
    deleteSession(sessionId)

    // Sync URL in fullPage mode
    if (mode === 'fullPage') {
      window.history.pushState(null, '', '/sippy-ng/chat')
    }
  }

  // Helper to get the timestamp used for display
  const getSessionTimestamp = (session) => {
    const messages = session.messages || []
    return messages.length > 0
      ? messages[messages.length - 1].timestamp
      : session.updatedAt
  }

  // Sort sessions by the same timestamp shown in the UI (most recent first)
  // This ensures the order matches what users see and doesn't jump around
  const sortedSessions = [...sessions].sort(
    (a, b) =>
      new Date(getSessionTimestamp(b)) - new Date(getSessionTimestamp(a))
  )

  // Get icon component based on session type
  const getIconComponent = (session) => {
    const iconName = getSessionIconName(session.type)
    let icon = null

    switch (iconName) {
      case 'Person':
        icon = <PersonIcon fontSize="small" />
        break
      case 'Share':
        icon = <ShareIcon fontSize="small" />
        break
      case 'CallSplit':
        icon = <CallSplitIcon fontSize="small" />
        break
      default:
        icon = <ChatIcon fontSize="small" />
    }

    // Add tooltip for shared sessions
    if (session.type === 'shared' && session.sharedBy) {
      return (
        <Tooltip title={`Shared by ${session.sharedBy}`} placement="left">
          {icon}
        </Tooltip>
      )
    }

    return icon
  }

  // Get the active session to display its title
  const activeSession = sessions.find((s) => s.id === activeSessionId)
  const currentTitle = activeSession
    ? getSessionTitle(activeSession)
    : 'Untitled Conversation'

  // Show compact version (icon only) in drawer mode
  const isCompact = mode === 'drawer'

  return (
    <>
      <Tooltip title="Chat sessions">
        <Button
          size="small"
          onClick={handleClick}
          disabled={disabled}
          variant="text"
          className={`${classes.dropdownButton} ${
            isCompact ? classes.dropdownButtonCompact : ''
          }`}
          endIcon={isCompact ? null : <ExpandMoreIcon />}
        >
          {isCompact ? (
            <ListIcon />
          ) : (
            <span className={classes.buttonLabel}>{currentTitle}</span>
          )}
        </Button>
      </Tooltip>

      <Tooltip title="New chat">
        <span>
          <IconButton
            size="small"
            onClick={handleNewSession}
            disabled={disabled}
          >
            <NoteAddIcon />
          </IconButton>
        </span>
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
        {sortedSessions.length === 0 ? (
          <Box
            sx={{ padding: 2, textAlign: 'center', color: 'text.secondary' }}
          >
            <Typography variant="body2">No chat sessions</Typography>
          </Box>
        ) : (
          sortedSessions.map((session) => {
            const isActive = session.id === activeSessionId

            // Count only user and assistant messages, not thinking steps or system messages
            const messages = session.messages || []
            const countableMessages = messages.filter(
              (msg) =>
                msg.type === MESSAGE_TYPES.USER ||
                msg.type === MESSAGE_TYPES.ASSISTANT
            )
            const messageCount = countableMessages.length

            // Get the timestamp to display (same as used for sorting)
            const lastMessageTime = getSessionTimestamp(session)

            return (
              <MenuItem
                key={session.id}
                onClick={() => handleSelectSession(session.id)}
                selected={isActive}
              >
                <ListItemIcon sx={{ minWidth: 36 }}>
                  {getIconComponent(session)}
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
                    <span>
                      {relativeTime(new Date(lastMessageTime), new Date())}
                    </span>
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
                  size="small"
                  onClick={(e) => handleDeleteSession(e, session.id)}
                  aria-label="Remove from list"
                  sx={{
                    padding: 0.5,
                    '&:hover': {
                      color: 'error.main',
                    },
                  }}
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

SessionManager.propTypes = {
  onNewSession: PropTypes.func,
  mode: PropTypes.oneOf(['fullPage', 'drawer']),
}
