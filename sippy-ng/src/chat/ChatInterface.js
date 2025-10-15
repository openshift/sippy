import {
  Alert,
  CircularProgress,
  Drawer,
  Fade,
  Paper,
  Typography,
} from '@mui/material'
import { CONNECTION_STATES } from './store/webSocketSlice'
import { makeStyles } from '@mui/styles'
import { MESSAGE_TYPES } from './chatUtils'
import { SESSION_TYPES } from './store/sessionSlice'
import {
  useConnectionState,
  usePageContextForChat,
  useSessionActions,
  useSessionState,
  useSettings,
  useShareActions,
  useShareState,
  useWebSocketActions,
} from './store/useChatStore'
import { useScrollManagement } from './useScrollManagement'
import { useSessionRating } from './useSessionRating'
import ChatHeader from './ChatHeader'
import ChatInput from './ChatInput'
import ChatMessage from './ChatMessage'
import ChatSettings from './ChatSettings'
import ChatTour from './ChatTour'
import PropTypes from 'prop-types'
import Rating from './Rating'
import React, { useEffect, useState } from 'react'
import ShareDialog from './ShareDialog'
import SippyLogo from '../components/SippyLogo'
import ThinkingStep from './ThinkingStep'

const DRAWER_HEIGHT = 600
const DRAWER_HEIGHT_MAXIMIZED = '90vh'

const useStyles = makeStyles((theme) => ({
  // Full page styles
  fullPageRoot: {
    height: 'calc(100vh - 64px - 48px)',
    display: 'flex',
    flexDirection: 'column',
    backgroundColor: theme.palette.background.default,
    overflow: 'hidden',
    margin: -theme.spacing(3),
    marginTop: 0,
  },
  fullPageHeader: {
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    backgroundColor: theme.palette.background.paper,
  },

  // Drawer styles
  drawer: {
    flexShrink: 0,
  },
  drawerPaper: {
    height: DRAWER_HEIGHT,
    width: 600,
    maxWidth: '100vw',
    right: theme.spacing(3),
    left: 'auto',
    borderTopLeftRadius: theme.shape.borderRadius * 2,
    borderTopRightRadius: theme.shape.borderRadius * 2,
    borderTop: `2px solid ${theme.palette.divider}`,
    borderLeft: `1px solid ${theme.palette.divider}`,
    borderRight: `1px solid ${theme.palette.divider}`,
    boxShadow: theme.shadows[8],
    transition: theme.transitions.create(['height', 'width'], {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.enteringScreen,
    }),
  },
  drawerPaperMaximized: {
    height: DRAWER_HEIGHT_MAXIMIZED,
    width: '80vw',
  },
  drawerHeader: {
    display: 'flex',
    alignItems: 'center',
    padding: theme.spacing(2),
    justifyContent: 'space-between',
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    flexWrap: 'nowrap',
    overflow: 'hidden',
  },

  // Shared styles
  headerActions: {
    display: 'flex',
    gap: theme.spacing(1),
    alignItems: 'center',
    flexShrink: 0,
  },
  headerTitle: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: theme.spacing(1),
    minWidth: 0,
    flexShrink: 1,
    overflow: 'hidden',
  },
  headerTextContainer: {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(0.25),
    minWidth: 0,
  },
  aiNotice: {
    fontSize: '0.65rem',
    color: theme.palette.text.secondary,
    fontStyle: 'italic',
    lineHeight: 1.2,
  },
  messagesContainer: {
    flex: 1,
    overflowY: 'auto',
    padding: theme.spacing(2),
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(2),
  },
  emptyState: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    height: '100%',
    padding: theme.spacing(4),
    textAlign: 'center',
  },
  messageWrapper: {
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
  sessionRatingContainer: {
    display: 'flex',
    justifyContent: 'center',
    padding: theme.spacing(1, 2),
    borderTop: `1px solid ${theme.palette.divider}`,
    backgroundColor:
      theme.palette.mode === 'dark'
        ? 'rgba(255, 255, 255, 0.02)'
        : 'rgba(0, 0, 0, 0.02)',
  },

  loadingContainer: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    height: '100%',
  },
}))

/**
 * Unified chat interface component that can render as either:
 * - A full-page chat view (mode='fullPage')
 * - A drawer widget (mode='drawer')
 */
export default function ChatInterface({
  mode = 'fullPage',
  open = true,
  onClose,
  conversationId = null,
}) {
  const classes = useStyles()
  const { pageContext } = usePageContextForChat()
  const [isMaximized, setIsMaximized] = useState(false)

  // Get state and actions from custom hooks
  const { sessions, activeSessionId, activeSession } = useSessionState()
  const {
    initializeSessions,
    createSession,
    switchSession,
    deleteSession,
    forkActiveSession,
  } = useSessionActions()

  const { shareLoading, loadingShared } = useShareState()
  const { clearSharedUrl, loadSharedConversationFromAPI } = useShareActions()

  const { connectionState, isTyping, error, currentThinking } =
    useConnectionState()
  const { settings, ensureClientId } = useSettings()

  // Get messages from active session
  const messages = activeSession?.messages || []

  // WebSocket actions
  const { sendMessage, connectWebSocket } = useWebSocketActions()

  // Session rating
  const { submitRating } = useSessionRating()

  // Scroll management
  const { messagesEndRef, messagesListRef, lastMessageRef } =
    useScrollManagement(activeSessionId, activeSession, messages, settings)

  const isConnected = connectionState === CONNECTION_STATES.CONNECTED

  // Initialize sessions and client ID on mount
  useEffect(() => {
    initializeSessions()
    ensureClientId()
  }, [initializeSessions, ensureClientId])

  // Load shared conversation if conversationId is provided
  useEffect(() => {
    if (conversationId) {
      loadSharedConversationFromAPI(conversationId)
    }
  }, [conversationId, loadSharedConversationFromAPI])

  // Set page title for full page mode
  useEffect(() => {
    if (mode === 'fullPage') {
      document.title = 'Sippy > Chat Assistant'
      return () => {
        document.title = 'Sippy'
      }
    }
  }, [mode])

  // Check if there are any assistant messages to show rating
  const hasAssistantMessages = messages.some(
    (msg) => msg.type === MESSAGE_TYPES.ASSISTANT
  )

  // Get the last non-system, non-thinking message
  const lastInteractionMessage = messages
    .filter(
      (msg) =>
        msg.type === MESSAGE_TYPES.USER || msg.type === MESSAGE_TYPES.ASSISTANT
    )
    .slice(-1)[0]

  // Only show rating if the last interaction was an assistant reply
  const lastMessageIsAssistant =
    lastInteractionMessage?.type === MESSAGE_TYPES.ASSISTANT

  // Determine if rating should be shown
  const canRateConversation =
    activeSession &&
    activeSession.type !== 'shared' &&
    hasAssistantMessages &&
    lastMessageIsAssistant &&
    !activeSession.rated

  const handleSessionRate = (messageId, rating) => {
    if (!activeSession || !activeSession.id) {
      console.error('No active session to rate')
      return
    }

    submitRating(activeSession.id, activeSession.type, messages, rating)
  }

  const handleNewChat = () => {
    // Side effects only (session creation handled in dropdown)
    if (mode === 'fullPage' && conversationId) {
      window.history.pushState(null, '', '/sippy-ng/chat')
    }

    clearSharedUrl()
  }

  const handleSendMessageWithFork = (content) => {
    if (
      activeSession &&
      (activeSession.type === SESSION_TYPES.SHARED ||
        activeSession.type === SESSION_TYPES.SHARED_BY_ME)
    ) {
      forkActiveSession()
      if (mode === 'fullPage' && conversationId) {
        window.history.pushState(null, '', '/sippy-ng/chat')
      }
    }

    clearSharedUrl()
    return sendMessage(content)
  }

  if (loadingShared) {
    return (
      <Paper className={classes.fullPageRoot}>
        <div className={classes.loadingContainer}>
          <CircularProgress />
        </div>
      </Paper>
    )
  }

  const renderEmptyState = () => (
    <div className={classes.emptyState}>
      <SippyLogo />
      <Typography variant={mode === 'fullPage' ? 'h5' : 'h6'} gutterBottom>
        Sippy Chat Assistant
      </Typography>
      <Typography variant="body2" color="textSecondary" paragraph>
        I can help you analyze jobs, investigate failures, check payloads, and
        more.
      </Typography>
    </div>
  )

  const renderMessages = () => {
    if (messages.length === 0 && !currentThinking) {
      return renderEmptyState()
    }

    return (
      <>
        {messages.map((msg, index) => {
          const isLastMessage = index === messages.length - 1
          if (msg.type === MESSAGE_TYPES.THINKING_STEP && msg.data) {
            // Only show thinking steps if the setting is enabled
            if (!settings.showThinking) {
              return null
            }
            return (
              <div
                key={index}
                className={classes.messageWrapper}
                ref={isLastMessage ? lastMessageRef : null}
              >
                <ThinkingStep data={msg.data} />
              </div>
            )
          }
          if (msg.type !== MESSAGE_TYPES.THINKING_STEP) {
            return (
              <div
                key={index}
                className={classes.messageWrapper}
                ref={isLastMessage ? lastMessageRef : null}
              >
                <ChatMessage message={msg} />
              </div>
            )
          }
          return null
        })}

        {currentThinking && settings.showThinking && (
          <Fade in timeout={300}>
            <div className={classes.currentThinking} ref={lastMessageRef}>
              <ThinkingStep data={currentThinking} isInProgress={true} />
            </div>
          </Fade>
        )}

        {error && (
          <Alert severity="error" className={classes.errorAlert}>
            {error}
          </Alert>
        )}

        <div ref={messagesEndRef} />
      </>
    )
  }

  const renderContent = () => (
    <>
      <ChatHeader
        mode={mode}
        onNewSession={handleNewChat}
        onMaximize={() => setIsMaximized(!isMaximized)}
        onClose={onClose}
        isMaximized={isMaximized}
      />

      <div ref={messagesListRef} className={classes.messagesContainer}>
        {renderMessages()}
      </div>

      {canRateConversation && (
        <div className={classes.sessionRatingContainer}>
          <Rating messageId="session" onRate={handleSessionRate} />
        </div>
      )}

      <div className={classes.inputContainer}>
        <ChatInput
          onSendMessage={handleSendMessageWithFork}
          onRetry={connectWebSocket}
          pageContext={pageContext}
          suggestedQuestions={pageContext?.suggestedQuestions || null}
        />
      </div>

      <ChatSettings
        onClearMessages={handleNewChat}
        onReconnect={connectWebSocket}
      />

      <ShareDialog />

      <ChatTour mode={mode} />
    </>
  )

  if (mode === 'drawer') {
    return (
      <Drawer
        className={classes.drawer}
        variant="persistent"
        anchor="bottom"
        open={open}
        classes={{
          paper: `${classes.drawerPaper} ${
            isMaximized ? classes.drawerPaperMaximized : ''
          }`,
        }}
      >
        {renderContent()}
      </Drawer>
    )
  }

  return <Paper className={classes.fullPageRoot}>{renderContent()}</Paper>
}

ChatInterface.propTypes = {
  mode: PropTypes.oneOf(['fullPage', 'drawer']),
  open: PropTypes.bool,
  onClose: PropTypes.func,
  conversationId: PropTypes.string,
}
