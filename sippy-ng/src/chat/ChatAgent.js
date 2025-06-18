import {
  Alert,
  Fade,
  IconButton,
  Paper,
  Tooltip,
  Typography,
} from '@mui/material'
import { DEFAULT_CHAT_SETTINGS, MESSAGE_TYPES } from './chatUtils'
import { makeStyles } from '@mui/styles'
import {
  Psychology as PsychologyIcon,
  Settings as SettingsIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material'
import { useChatWebSocket } from './useChatWebSocket'
import ChatInput from './ChatInput'
import ChatMessage from './ChatMessage'
import ChatSettings from './ChatSettings'
import React, { useEffect, useRef, useState } from 'react'
import SippyLogo from '../components/SippyLogo'
import ThinkingStep from './ThinkingStep'

const useStyles = makeStyles((theme) => ({
  root: {
    height: 'calc(100vh - 64px - 48px)', // App bar (64px) + Main padding (24px top + 24px bottom)
    display: 'flex',
    flexDirection: 'column',
    backgroundColor: theme.palette.background.default,
    overflow: 'hidden',
    margin: -theme.spacing(3), // Counteract Main component's padding
    marginTop: 0, // Keep top margin as 0 since DrawerHeader handles spacing
  },
  header: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  headerTitle: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  headerActions: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
  messagesContainer: {
    flex: 1,
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
  },
  messagesList: {
    flex: 1,
    overflowY: 'auto',
    padding: theme.spacing(1, 2),
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
  },
}))

export default function ChatAgent() {
  const classes = useStyles()
  const [settings, setSettings] = useState(DEFAULT_CHAT_SETTINGS)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const messagesEndRef = useRef(null)
  const messagesListRef = useRef(null)

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
  } = useChatWebSocket(settings)

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    if (settings.autoScroll && messagesEndRef.current) {
      messagesEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages, currentThinking, settings.autoScroll])

  // Set page title
  useEffect(() => {
    document.title = 'Sippy > Chat Agent'
    return () => {
      document.title = 'Sippy'
    }
  }, [])

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
  }

  const filteredMessages = settings.showThinking
    ? messages
    : messages.filter((msg) => msg.type !== MESSAGE_TYPES.THINKING_STEP)

  const renderEmptyState = () => (
    <div className={classes.emptyState}>
      <SippyLogo />
      <Typography variant="h5" gutterBottom>
        Sippy Chat Agent
      </Typography>
      <Typography variant="body2" color="textSecondary" paragraph>
        I can help you analyze Prow jobs, investigate test failures, check
        release payloads, and correlate issues with known incidents.
      </Typography>
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

  return (
    <div className={classes.root}>
      {/* Header */}
      <div className={classes.header}>
        <div className={classes.headerTitle}>
          <SmartToyIcon color="primary" />
          <Typography variant="h6">Chat Agent</Typography>
          {isTyping && (
            <Tooltip title="Agent is thinking">
              <PsychologyIcon color="primary" />
            </Tooltip>
          )}
        </div>

        <div className={classes.headerActions}>
          <Tooltip title="Chat settings">
            <IconButton onClick={() => setSettingsOpen(true)}>
              <SettingsIcon />
            </IconButton>
          </Tooltip>
        </div>
      </div>

      {/* Error display */}
      {error && (
        <Alert
          severity="error"
          className={classes.errorAlert}
          onClose={() => {
            /* Could add error dismissal */
          }}
        >
          {error}
        </Alert>
      )}

      {/* Messages */}
      <div className={classes.messagesContainer}>{renderMessages()}</div>

      {/* Input */}
      <ChatInput
        onSendMessage={handleSendMessage}
        disabled={!isConnected}
        isConnected={isConnected}
        isTyping={isTyping}
        onRetry={handleReconnect}
      />

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
    </div>
  )
}
