import {
  Alert,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Drawer,
  Fade,
  IconButton,
  Paper,
  Snackbar,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  Close as CloseIcon,
  ContentCopy as ContentCopyIcon,
  ExpandMore as ExpandMoreIcon,
  Help as HelpIcon,
  Masks as MasksIcon,
  Fullscreen as MaximizeIcon,
  FullscreenExit as MinimizeIcon,
  Settings as SettingsIcon,
  Share as ShareIcon,
  SmartToy as SmartToyIcon,
} from '@mui/icons-material'
import { createMessage, MESSAGE_TYPES } from './chatUtils'
import { makeStyles } from '@mui/styles'
import { SESSION_TYPES, useChatSessions } from './useChatSessions'
import { useChatInterface } from './useChatInterface'
import { useGlobalChat } from './useGlobalChat'
import ChatInput from './ChatInput'
import ChatMessage from './ChatMessage'
import ChatSettings from './ChatSettings'
import PropTypes from 'prop-types'
import React, { useEffect, useRef, useState } from 'react'
import SessionDropdown from './SessionDropdown'
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

  loadingContainer: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    height: '100%',
  },

  shareDialogTitle: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingRight: theme.spacing(1),
  },

  shareDialogContent: {
    paddingTop: theme.spacing(2),
  },

  shareTextField: {
    marginTop: theme.spacing(2),
    '& .MuiOutlinedInput-root': {
      fontFamily: 'monospace',
      fontSize: '0.9rem',
    },
  },

  copyButton: {
    marginRight: theme.spacing(1),
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
  pageContext = null,
  conversationId = null,
}) {
  const classes = useStyles()
  const [isMaximized, setIsMaximized] = useState(false)
  const [shareLoading, setShareLoading] = useState(false)
  const [shareDialogOpen, setShareDialogOpen] = useState(false)
  const [sharedUrl, setSharedUrl] = useState('') // Cleared when new messages added
  const [shareSnackbar, setShareSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success',
  })
  const [loadingShared, setLoadingShared] = useState(!!conversationId)
  const loadedConversationRef = useRef(null) // Track which conversation we've loaded
  const justForkedRef = useRef(false) // Track if we just forked to avoid reloading messages

  // Session management
  const {
    sessions,
    activeSessionId,
    activeSession,
    createSession,
    switchSession,
    deleteSession,
    updateSessionMessages,
    updateSessionMetadata,
    forkActiveSession,
    loadSharedConversation,
  } = useChatSessions()

  // Use shared chat interface logic
  const {
    settings,
    settingsOpen,
    setSettingsOpen,
    messages,
    filteredMessages,
    connectionState,
    currentThinking,
    error,
    isTyping,
    isConnected,
    personas,
    messagesEndRef,
    messagesListRef,
    lastMessageRef,
    handleSendMessage,
    handleClearMessages,
    handleReconnect,
    handleSettingsChange,
    getCurrentPersonaDisplay,
    getCurrentPersonaTooltip,
  } = useChatInterface()

  // Access the underlying WebSocket to manipulate messages directly
  const globalChat = useGlobalChat()

  // Set page title for full page mode
  useEffect(() => {
    if (mode === 'fullPage') {
      document.title = 'Sippy > Chat Assistant'
      return () => {
        document.title = 'Sippy'
      }
    }
  }, [mode])

  // Load messages from active session when it changes
  useEffect(() => {
    if (!activeSessionId || !activeSession) return

    // Skip loading if we just forked - messages are already correct
    if (justForkedRef.current) {
      justForkedRef.current = false
      return
    }

    const sessionMessages = activeSession.messages || []

    // Check if we need to load messages - compare message IDs or lengths
    const needsLoad =
      messages.length !== sessionMessages.length ||
      (messages.length > 0 &&
        sessionMessages.length > 0 &&
        messages[0]?.id !== sessionMessages[0]?.id)

    if (needsLoad) {
      handleClearMessages()
      if (sessionMessages.length > 0) {
        // Use setTimeout to ensure the clear happens before adding messages
        setTimeout(() => {
          globalChat.addMessages(sessionMessages)
        }, 0)
      }
    }
  }, [activeSessionId])

  // Save messages to active session whenever they change
  // Debounce the save to avoid excessive updates
  useEffect(() => {
    if (!activeSessionId) return

    const timeoutId = setTimeout(() => {
      if (messages.length >= 0) {
        updateSessionMessages(messages)
      }
    }, 100) // Small debounce to batch updates

    return () => clearTimeout(timeoutId)
  }, [messages, activeSessionId, updateSessionMessages])

  // Load shared conversation if conversationId is provided
  useEffect(() => {
    if (!conversationId) return

    // Don't fetch if we've already loaded this conversation
    if (loadedConversationRef.current === conversationId) {
      return
    }

    const abortController = new AbortController()

    const fetchConversation = async () => {
      try {
        setLoadingShared(true)
        const response = await fetch(
          `${process.env.REACT_APP_API_URL}/api/chat/conversations/${conversationId}`,
          { signal: abortController.signal }
        )

        if (!response.ok) {
          let errorMessage = 'Failed to load shared conversation'
          try {
            const errorData = await response.json()
            errorMessage = errorData.message || errorMessage
          } catch (e) {
            errorMessage = response.statusText || errorMessage
          }
          throw new Error(errorMessage)
        }

        const data = await response.json()

        // Load shared messages
        const loadedMessages = data.messages.map((msg, idx) => ({
          ...msg,
          id: msg.id || `loaded_${idx}`,
        }))

        // Add a system message to mark this shared conversation
        const systemMessage = createMessage(
          MESSAGE_TYPES.SYSTEM,
          `Shared by ${data.user} • ${new Date(
            data.created_at
          ).toLocaleString()}`,
          {
            id: 'system_' + conversationId,
            conversationId: conversationId,
          }
        )

        const allMessages = [...loadedMessages, systemMessage]

        // Load into session management
        loadSharedConversation(conversationId, allMessages, {
          createdAt: data.created_at,
          parentId: data.parent_id,
        })

        // Clear any existing messages and load the shared conversation
        handleClearMessages()
        globalChat.addMessages(allMessages)

        // Set the shared URL so clicking share again doesn't create a duplicate
        const url = `${window.location.origin}/sippy-ng/chat/${conversationId}`
        setSharedUrl(url)

        // Mark this conversation as loaded to prevent re-fetching
        loadedConversationRef.current = conversationId
      } catch (err) {
        // Don't show error for aborted requests
        if (err.name === 'AbortError') {
          return
        }
        console.error('Error loading shared conversation:', err)
        setShareSnackbar({
          open: true,
          message: err.message,
          severity: 'error',
        })
      } finally {
        setLoadingShared(false)
      }
    }

    fetchConversation()

    // Cleanup: abort fetch if component unmounts or conversationId changes
    return () => {
      abortController.abort()
    }
  }, [conversationId, loadSharedConversation])

  const handleSendMessageWrapper = (content) => {
    // Fork the session if it's a shared conversation (either shared with you or shared by you)
    if (
      activeSession &&
      (activeSession.type === SESSION_TYPES.SHARED ||
        activeSession.type === SESSION_TYPES.SHARED_BY_ME)
    ) {
      // Set flag to prevent message reload when switching to forked session
      justForkedRef.current = true

      // Fork creates a new session with current messages copied
      const forkedSession = forkActiveSession()

      // Update URL bar when forking a shared conversation (only in fullPage mode)
      if (mode === 'fullPage' && conversationId) {
        window.history.pushState(null, '', '/sippy-ng/chat')
      }

      // The forked session now has the messages, so we don't need to reload
      // Just send the new message
    }

    // Clear cached share URL since conversation has changed
    setSharedUrl('')
    return handleSendMessage(content)
  }

  const handleNewChat = () => {
    // Look for an existing empty "New Chat" session
    const emptySession = sessions.find(
      (s) =>
        s.type === SESSION_TYPES.OWN &&
        (!s.messages || s.messages.length === 0) &&
        !s.sharedId
    )

    if (emptySession && emptySession.id !== activeSessionId) {
      // Switch to existing empty session
      switchSession(emptySession.id)
    } else if (!emptySession) {
      // No empty session exists, create a new one
      createSession()
    }
    // If the current session is already empty, do nothing

    // Clear messages in the UI
    handleClearMessages()

    // Clear URL if viewing a shared conversation (only in fullPage mode)
    if (mode === 'fullPage' && conversationId) {
      window.history.pushState(null, '', '/sippy-ng/chat')
    }

    setSharedUrl('')
  }

  const handleSessionSwitch = (sessionId) => {
    switchSession(sessionId)

    // Update URL when switching sessions (only in fullPage mode)
    if (mode === 'fullPage') {
      const targetSession = sessions.find((s) => s.id === sessionId)
      if (targetSession && targetSession.sharedId) {
        // Switch to a shared or forked session - update URL to include the shared ID
        window.history.pushState(
          null,
          '',
          `/sippy-ng/chat/${targetSession.sharedId}`
        )
      } else {
        // Switch to a local session - clear the URL
        window.history.pushState(null, '', '/sippy-ng/chat')
      }
    }
  }

  const handleSessionDelete = (sessionId) => {
    const wasActiveSession = deleteSession(sessionId)

    // If we deleted the active session, clear messages and URL
    if (wasActiveSession) {
      handleClearMessages()

      if (mode === 'fullPage') {
        window.history.pushState(null, '', '/sippy-ng/chat')
      }
    }
  }

  const handleShareConversation = async () => {
    // Save all current messages (including any system messages marking previous shares)
    if (filteredMessages.length === 0) {
      setShareSnackbar({
        open: true,
        message: 'No messages to share',
        severity: 'warning',
      })
      return
    }

    // If we already have a shared URL (and it hasn't been cleared by new messages),
    // just show the existing dialog
    if (sharedUrl) {
      setShareDialogOpen(true)
      return
    }

    setShareLoading(true)

    try {
      // Format messages for API - preserve complete message structure
      const messagesToShare = filteredMessages.map((msg) => ({
        type: msg.type,
        content: msg.content,
        timestamp: msg.timestamp,
        ...(msg.data && { data: msg.data }),
        ...(msg.pageContext && { pageContext: msg.pageContext }),
        ...(msg.conversationId && { conversationId: msg.conversationId }),
      }))

      // Prepare metadata
      const metadata = {
        persona: settings.persona,
        pageContext: pageContext,
        sharedAt: new Date().toISOString(),
      }

      // If we're sharing a forked conversation, include the parent ID
      const payload = {
        messages: messagesToShare,
        metadata: metadata,
      }

      if (conversationId) {
        payload.parent_id = conversationId
      }

      const response = await fetch(
        process.env.REACT_APP_API_URL + '/api/chat/conversations',
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(payload),
        }
      )

      if (!response.ok) {
        let errorMessage = 'Failed to share conversation'
        try {
          const errorData = await response.json()
          errorMessage = errorData.message || errorMessage
        } catch (e) {
          // If response is not JSON, use status text
          errorMessage = response.statusText || errorMessage
        }
        throw new Error(errorMessage)
      }

      const data = await response.json()

      // Construct the shareable URL from the conversation ID
      const url = `${window.location.origin}/sippy-ng/chat/${data.id}`
      setSharedUrl(url)
      setShareDialogOpen(true)

      // Update session metadata to record that it's been shared
      // and mark it as "shared by me"
      if (activeSessionId) {
        updateSessionMetadata(activeSessionId, {
          sharedId: data.id,
          type: SESSION_TYPES.SHARED_BY_ME,
        })
      }

      // Update browser URL to the shared URL (only in fullPage mode)
      if (mode === 'fullPage') {
        window.history.pushState(null, '', `/sippy-ng/chat/${data.id}`)
      }

      // Also copy to clipboard
      try {
        await navigator.clipboard.writeText(url)
      } catch (err) {
        console.warn('Failed to copy to clipboard:', err)
        // Continue anyway, user can copy from dialog
      }
    } catch (err) {
      console.error('Error sharing conversation:', err)
      setShareSnackbar({
        open: true,
        message: err.message || 'Failed to share conversation',
        severity: 'error',
      })
    } finally {
      setShareLoading(false)
    }
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
    if (filteredMessages.length === 0 && !currentThinking) {
      return renderEmptyState()
    }

    return (
      <>
        {filteredMessages.map((msg, index) => {
          const isLastMessage = index === filteredMessages.length - 1
          if (msg.type === MESSAGE_TYPES.THINKING_STEP && msg.data) {
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

  const renderHeader = () => {
    const contextDisplay = getContextDisplay()

    return (
      <div
        className={
          mode === 'fullPage' ? classes.fullPageHeader : classes.drawerHeader
        }
      >
        <div className={classes.headerTitle}>
          <SmartToyIcon sx={{ flexShrink: 0, marginTop: '4px' }} />
          <div className={classes.headerTextContainer}>
            <Typography
              variant="h6"
              sx={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                minWidth: 0,
              }}
            >
              Chat Assistant
            </Typography>
            <Typography
              className={classes.aiNotice}
              sx={{
                fontSize: '0.65rem !important',
              }}
            >
              Always review AI generated content prior to use.
            </Typography>
          </div>
        </div>

        <div className={classes.headerActions}>
          <SessionDropdown
            sessions={sessions}
            activeSessionId={activeSessionId}
            onSelectSession={handleSessionSwitch}
            onNewSession={handleNewChat}
            onDeleteSession={handleSessionDelete}
            disabled={!isConnected || isTyping}
          />
          <Tooltip title="Share conversation">
            <span>
              <IconButton
                size="small"
                onClick={handleShareConversation}
                disabled={
                  shareLoading ||
                  filteredMessages.length === 0 ||
                  isTyping ||
                  currentThinking
                }
              >
                {shareLoading ? <CircularProgress size={20} /> : <ShareIcon />}
              </IconButton>
            </span>
          </Tooltip>

          <Tooltip title="Help">
            <IconButton
              size="small"
              component="a"
              href={
                'https://source.redhat.com/departments/products_and_global_engineering/openshift_development/openshift_wiki/sippy_chat_user_guide'
              }
              target="_blank"
              rel="noopener noreferrer"
            >
              <HelpIcon />
            </IconButton>
          </Tooltip>

          <Tooltip title="Settings">
            <IconButton size="small" onClick={() => setSettingsOpen(true)}>
              <SettingsIcon />
            </IconButton>
          </Tooltip>

          {mode === 'drawer' && (
            <>
              <Tooltip title={isMaximized ? 'Minimize' : 'Maximize'}>
                <IconButton
                  size="small"
                  onClick={() => setIsMaximized(!isMaximized)}
                >
                  {isMaximized ? <MinimizeIcon /> : <MaximizeIcon />}
                </IconButton>
              </Tooltip>

              <Tooltip title="Minimize">
                <IconButton size="small" onClick={onClose}>
                  <ExpandMoreIcon />
                </IconButton>
              </Tooltip>
            </>
          )}
        </div>
      </div>
    )
  }

  const renderContent = () => (
    <>
      {renderHeader()}

      <div ref={messagesListRef} className={classes.messagesContainer}>
        {renderMessages()}
      </div>

      <div className={classes.inputContainer}>
        <ChatInput
          onSendMessage={handleSendMessageWrapper}
          disabled={!isConnected}
          isConnected={isConnected}
          isTyping={isTyping}
          onRetry={handleReconnect}
          suggestedQuestions={pageContext?.suggestedQuestions || null}
          contextChip={
            pageContext && getContextDisplay() ? (
              <Tooltip title={`Context: ${getContextDisplay()}`}>
                <Chip
                  label={getContextDisplay()}
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

      <ChatSettings
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        settings={settings}
        onSettingsChange={handleSettingsChange}
        onClearMessages={handleNewChat}
        onReconnect={handleReconnect}
        connectionState={connectionState}
        messageCount={messages.length}
        isConnected={isConnected}
      />

      <Dialog
        open={shareDialogOpen}
        onClose={() => setShareDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle className={classes.shareDialogTitle}>
          <Typography variant="h6" component="span">
            Conversation Shared
          </Typography>
          <IconButton
            edge="end"
            onClick={() => setShareDialogOpen(false)}
            aria-label="close"
            size="small"
          >
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent className={classes.shareDialogContent}>
          <DialogContentText>
            Your conversation has been shared! Anyone with this link can view
            and continue the conversation.
          </DialogContentText>
          <DialogContentText sx={{ mt: 1, fontStyle: 'italic' }}>
            Note: Shared conversations will be available for up to 90 days.
          </DialogContentText>
          <TextField
            autoFocus
            margin="dense"
            label="Shareable Link"
            fullWidth
            variant="outlined"
            value={sharedUrl}
            InputProps={{
              readOnly: true,
            }}
            className={classes.shareTextField}
            onClick={(e) => e.target.select()}
          />
        </DialogContent>
        <DialogActions>
          <Button
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(sharedUrl)
                setShareSnackbar({
                  open: true,
                  message: 'Link copied to clipboard!',
                  severity: 'success',
                })
              } catch (err) {
                setShareSnackbar({
                  open: true,
                  message: 'Failed to copy to clipboard',
                  severity: 'error',
                })
              }
            }}
            startIcon={<ContentCopyIcon />}
            variant="contained"
            className={classes.copyButton}
          >
            Copy Link
          </Button>
          <Button onClick={() => setShareDialogOpen(false)} variant="outlined">
            Close
          </Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={shareSnackbar.open}
        autoHideDuration={6000}
        onClose={() => setShareSnackbar({ ...shareSnackbar, open: false })}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={() => setShareSnackbar({ ...shareSnackbar, open: false })}
          severity={shareSnackbar.severity}
          sx={{ width: '100%' }}
        >
          {shareSnackbar.message}
        </Alert>
      </Snackbar>
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

  // Full page mode
  return <Paper className={classes.fullPageRoot}>{renderContent()}</Paper>
}

ChatInterface.propTypes = {
  mode: PropTypes.oneOf(['fullPage', 'drawer']),
  open: PropTypes.bool,
  onClose: PropTypes.func,
  pageContext: PropTypes.shape({
    page: PropTypes.string,
    url: PropTypes.string,
    data: PropTypes.object,
    instructions: PropTypes.string,
    suggestedQuestions: PropTypes.arrayOf(PropTypes.string),
  }),
  conversationId: PropTypes.string,
}
