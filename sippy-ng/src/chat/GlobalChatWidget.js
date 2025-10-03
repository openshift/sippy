import {
  Alert,
  Chip,
  Drawer,
  Fade,
  IconButton,
  Paper,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  Close as CloseIcon,
  FullscreenExit as FullscreenExitIcon,
  Fullscreen as FullscreenIcon,
  Masks as MasksIcon,
  Psychology as PsychologyIcon,
  Settings as SettingsIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material'
import { DEFAULT_CHAT_SETTINGS, MESSAGE_TYPES } from './chatUtils'
import { makeStyles } from '@mui/styles'
import { useCookies } from 'react-cookie'
import { useGlobalChat } from './useGlobalChat'
import { usePersonas } from './usePersonas'
import ChatInput from './ChatInput'
import ChatMessage from './ChatMessage'
import ChatSettings from './ChatSettings'
import PropTypes from 'prop-types'
import React, { useEffect, useRef, useState } from 'react'
import SippyLogo from '../components/SippyLogo'
import ThinkingStep from './ThinkingStep'

const DRAWER_WIDTH = 500
const DRAWER_WIDTH_MAXIMIZED = '80%'

const useStyles = makeStyles((theme) => ({
  drawer: {
    flexShrink: 0,
  },
  drawerPaper: {
    display: 'flex',
    flexDirection: 'column',
    [theme.breakpoints.down('sm')]: {
      width: '100%',
    },
  },
  drawerPaperNormal: {
    width: DRAWER_WIDTH,
  },
  drawerPaperMaximized: {
    width: DRAWER_WIDTH_MAXIMIZED,
  },
  header: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    flexShrink: 0,
  },
  headerTitle: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    flex: 1,
    minWidth: 0,
  },
  headerActions: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    flexShrink: 0,
  },
  contextBadge: {
    marginLeft: theme.spacing(1),
  },
  messagesContainer: {
    flex: 1,
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
    minHeight: 0,
  },
  messagesList: {
    flex: 1,
    overflowY: 'auto',
    padding: theme.spacing(1, 2),
    minHeight: 0,
    '&::-webkit-scrollbar': {
      width: 8,
    },
    '&::-webkit-scrollbar-track': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? 'rgba(255, 255, 255, 0.1)'
          : 'rgba(0, 0, 0, 0.1)',
    },
    '&::-webkit-scrollbar-thumb': {
      backgroundColor:
        theme.palette.mode === 'dark'
          ? 'rgba(255, 255, 255, 0.3)'
          : 'rgba(0, 0, 0, 0.3)',
      borderRadius: 4,
    },
  },
  emptyState: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    height: '100%',
    textAlign: 'center',
    padding: theme.spacing(4),
  },
  emptyStateIcon: {
    fontSize: 64,
    color: theme.palette.text.secondary,
    marginBottom: theme.spacing(2),
  },
  currentThinking: {
    margin: theme.spacing(1, 2),
    padding: theme.spacing(1),
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.05)'
        : 'rgba(0, 0, 0, 0.02)',
    borderRadius: theme.shape.borderRadius,
  },
  errorAlert: {
    margin: theme.spacing(1, 2),
    flexShrink: 0,
  },
  inputContainer: {
    flexShrink: 0,
  },
}))

export default function GlobalChatWidget({ open, onClose, pageContext }) {
  const classes = useStyles()
  const [cookies, setCookie] = useCookies(['sippyChatSettings'])

  // Load settings from cookie or use defaults
  const [settings, setSettings] = useState(() => {
    if (cookies.sippyChatSettings) {
      return { ...DEFAULT_CHAT_SETTINGS, ...cookies.sippyChatSettings }
    }
    return DEFAULT_CHAT_SETTINGS
  })

  const [settingsOpen, setSettingsOpen] = useState(false)
  const [isMaximized, setIsMaximized] = useState(false)
  const messagesEndRef = useRef(null)
  const messagesListRef = useRef(null)

  // Use shared chat state from global context
  const {
    messages,
    connectionState,
    currentThinking,
    error,
    isTyping,
    sendMessage,
    clearMessages,
    connect,
    disconnect,
    isConnected,
  } = useGlobalChat()

  const { personas } = usePersonas()

  const getCurrentPersonaDisplay = () => {
    const personaName = settings.persona || 'default'
    return (
      personaName.charAt(0).toUpperCase() +
      personaName.slice(1).replace(/_/g, ' ')
    )
  }

  const getCurrentPersonaTooltip = () => {
    const persona = personas.find((p) => p.name === settings.persona)
    return persona ? persona.description : 'Default AI assistant'
  }

  const getContextDisplay = () => {
    if (!pageContext || !pageContext.page) {
      return null
    }
    const pageName = pageContext.page
      .split('-')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
    return pageName
  }

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    if (settings.autoScroll && messagesEndRef.current) {
      messagesEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages, currentThinking, settings.autoScroll])

  const handleSendMessage = (content) => {
    return sendMessage(content)
  }

  const handleClearMessages = () => {
    clearMessages()
    setSettingsOpen(false)
  }

  const handleReconnect = () => {
    disconnect()
    setTimeout(() => connect(), 1000)
  }

  const handleSettingsChange = (newSettings) => {
    setSettings(newSettings)
    // Save settings to cookie
    setCookie('sippyChatSettings', newSettings, {
      path: '/',
      sameSite: 'Strict',
      expires: new Date('3000-12-31'),
    })
  }

  const filteredMessages = settings.showThinking
    ? messages
    : messages.filter((msg) => msg.type !== MESSAGE_TYPES.THINKING_STEP)

  const renderEmptyState = () => (
    <div className={classes.emptyState}>
      <SippyLogo />
      <Typography variant="h6" gutterBottom>
        Sippy Chat Assistant
      </Typography>
      <Typography variant="body2" color="textSecondary" paragraph>
        I can help you analyze jobs, investigate failures, check payloads, and
        correlate issues.
      </Typography>
      {pageContext && pageContext.page && (
        <Typography variant="caption" color="textSecondary">
          I have context from the current page and can answer questions about
          what you&apos;re viewing.
        </Typography>
      )}
    </div>
  )

  const renderMessages = () => (
    <div className={classes.messagesList} ref={messagesListRef}>
      {filteredMessages.length === 0 ? (
        renderEmptyState()
      ) : (
        <>
          {filteredMessages.map((message) => (
            <ChatMessage
              key={message.id}
              message={message}
              showTimestamp={true}
              showTools={true}
            />
          ))}

          {/* Current thinking step (if not showing thinking in messages) */}
          {!settings.showThinking && currentThinking && (
            <Fade in={true}>
              <Paper className={classes.currentThinking} elevation={1}>
                <ThinkingStep
                  data={currentThinking}
                  isInProgress={true}
                  defaultExpanded={true}
                />
              </Paper>
            </Fade>
          )}

          {/* Scroll anchor */}
          <div ref={messagesEndRef} />
        </>
      )}
    </div>
  )

  const contextDisplay = getContextDisplay()

  return (
    <>
      <Drawer
        className={classes.drawer}
        anchor="right"
        open={open}
        onClose={onClose}
        classes={{
          paper: `${classes.drawerPaper} ${
            isMaximized
              ? classes.drawerPaperMaximized
              : classes.drawerPaperNormal
          }`,
        }}
      >
        {/* Header */}
        <div className={classes.header}>
          <div className={classes.headerTitle}>
            <SmartToyIcon color="primary" />
            <Typography variant="h6" noWrap>
              Chat
            </Typography>
            {isTyping && (
              <Tooltip title="Agent is thinking">
                <PsychologyIcon color="primary" fontSize="small" />
              </Tooltip>
            )}
          </div>

          <div className={classes.headerActions}>
            <Tooltip title={isMaximized ? 'Restore' : 'Maximize'}>
              <IconButton
                size="small"
                onClick={() => setIsMaximized(!isMaximized)}
              >
                {isMaximized ? (
                  <FullscreenExitIcon fontSize="small" />
                ) : (
                  <FullscreenIcon fontSize="small" />
                )}
              </IconButton>
            </Tooltip>
            <Tooltip title="Settings">
              <IconButton size="small" onClick={() => setSettingsOpen(true)}>
                <SettingsIcon fontSize="small" />
              </IconButton>
            </Tooltip>
            <Tooltip title="Close">
              <IconButton size="small" onClick={onClose}>
                <CloseIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </div>
        </div>

        {/* Error display */}
        {error && (
          <Alert severity="error" className={classes.errorAlert}>
            {error}
          </Alert>
        )}

        {/* Messages */}
        <div className={classes.messagesContainer}>{renderMessages()}</div>

        {/* Input */}
        <div className={classes.inputContainer}>
          <ChatInput
            onSendMessage={handleSendMessage}
            disabled={!isConnected}
            isConnected={isConnected}
            isTyping={isTyping}
            onRetry={handleReconnect}
            contextChip={
              contextDisplay ? (
                <Tooltip title={`Context: ${contextDisplay}`}>
                  <Chip
                    label={contextDisplay}
                    size="small"
                    color="primary"
                    variant="outlined"
                  />
                </Tooltip>
              ) : null
            }
            personaChip={
              personas.length > 0 && settings.persona !== 'default' ? (
                <Tooltip title={getCurrentPersonaTooltip()}>
                  <Chip
                    icon={<MasksIcon />}
                    label={getCurrentPersonaDisplay()}
                    size="small"
                    color="secondary"
                    variant="outlined"
                  />
                </Tooltip>
              ) : null
            }
          />
        </div>
      </Drawer>

      {/* Settings Drawer */}
      <ChatSettings
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        settings={settings}
        onSettingsChange={handleSettingsChange}
        onClearMessages={handleClearMessages}
        onReconnect={handleReconnect}
        connectionState={connectionState}
        messageCount={messages.length}
        isConnected={isConnected}
      />
    </>
  )
}

GlobalChatWidget.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  pageContext: PropTypes.shape({
    page: PropTypes.string,
    url: PropTypes.string,
    data: PropTypes.object,
  }),
}
